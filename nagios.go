// Copyright (c) 2013 Marc Falzon / Cloudwatt
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package main

import (
	"bytes"
	"fmt"
	"github.com/streadway/amqp"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const (
	nagiosFalse = 0
	nagiosTrue  = 1

	nagiosHostStatusOK          = 0
	nagiosHostStatusDown        = 1
	nagiosHostStatusUnreachable = 2

	nagiosServiceStatusOK       = 0
	nagiosServiceStatusWarning  = 1
	nagiosServiceStatusCritical = 2
	nagiosServiceStatusUnknown  = 3
)

type nagiosCheck struct {
	CheckOptions       int           `json:"check_options"`
	CommandLine        string        `json:"command_line"`
	HostName           string        `json:"host_name"`
	Latency            float64       `json:"latency"`
	Type               string        `json:"type"`
	ServiceDescription string        `json:"service_description"`
	StartTime          float64       `json:"start_time"`
	Timeout            int           `json:"timeout"`
	Message            amqp.Delivery `json:"-"`
}

type nagiosCheckResult struct {
	HostName           string  `json:"host_name"`
	ServiceDescription string  `json:"service_description"`
	CheckOptions       int     `json:"check_options"`
	ScheduledCheck     int     `json:"scheduled_check"`
	RescheduleCheck    int     `json:"reschedule_check"`
	Latency            float64 `json:"latency"`
	StartTime          float64 `json:"start_time"`
	FinishTime         float64 `json:"finish_time"`
	EarlyTimeout       int     `json:"early_timeout"`
	ExitedOk           int     `json:"exited_ok"`
	ReturnCode         int     `json:"return_code"`
	Output             string  `json:"output"`
	CorrelationID      string  `json:"-"`
	ReplyTo            string  `json:"-"`
}

func (check *nagiosCheck) execute() {
	var cmdStdout, cmdStderr bytes.Buffer

	checkResult := &nagiosCheckResult{
		CorrelationID:      check.Message.CorrelationId,
		HostName:           check.HostName,
		ServiceDescription: check.ServiceDescription,
		CheckOptions:       check.CheckOptions,
		StartTime:          check.StartTime,
		Latency:            check.Latency,
		EarlyTimeout:       nagiosFalse,
		ScheduledCheck:     nagiosTrue,
		RescheduleCheck:    nagiosTrue,
		ExitedOk:           nagiosTrue,
	}

	if check.Timeout > 0 {
		// Override Nagios check timeout if longer than configuration-defined value
		if check.Timeout > config.MaxExecTimeout {
			check.Timeout = config.MaxExecTimeout
		}
	} else {
		// Set configuration-defined timeout value if Nagios didn't set any
		check.Timeout = config.MaxExecTimeout
	}

	if config.LogLevel > 1 {
		logger.Printf("%s: worker: executing command \"%s\"",
			checkResult.CorrelationID,
			check.CommandLine)
	}

	cmd := exec.Command("/bin/sh", "-c", check.CommandLine)
	cmd.Stdout = &cmdStdout
	cmd.Stderr = &cmdStderr

	if err := cmd.Start(); err != nil {
		logger.Fatalf("%s: worker: error: unable to execute command line \"%s\": %s",
			checkResult.CorrelationID,
			check.CommandLine,
			err)
	}

	doneExec := make(chan error, 1)
	go func() {
		doneExec <- cmd.Wait()
	}()

	select {
	case <-time.After(time.Duration(check.Timeout) * time.Second):
		// Check execution time reached timeout, kill it with fire!
		cmd.Process.Kill()

		checkResult.EarlyTimeout = nagiosTrue

		if check.Type == "host" {
			checkResult.Output = "(bunny: host check timed out)"
			checkResult.ReturnCode = nagiosHostStatusUnreachable
		} else {
			checkResult.Output = "(bunny: service check timed out)"
			checkResult.ReturnCode = nagiosServiceStatusUnknown
		}

		if config.LogLevel > 2 {
			logger.Printf("%s: worker: command \"%s\" execution timed out",
				checkResult.CorrelationID,
				check.CommandLine)
		}

	case <-doneExec:
		// If command didn't output anything on stdout, print stderr instead
		if cmdStdout.Len() == 0 {
			if cmdStderr.Len() > 0 {
				checkResult.Output = cmdStderr.String()[0 : cmdStderr.Len()-1]
			} else {
				// If there's nothing on stderr either
				checkResult.Output = "(bunny: no check output)"
			}
		} else {
			checkResult.Output = cmdStdout.String()[0 : cmdStdout.Len()-1]

			// If command also output something on stderr and we're asked to report it
			if cmdStderr.Len() > 0 && config.ReportStderr {
				checkResult.Output += fmt.Sprintf("\nstderr: %s", cmdStderr.String()[0:cmdStderr.Len()-1])
			}
		}

		checkResult.ReturnCode = cmd.ProcessState.Sys().(syscall.WaitStatus).ExitStatus()

		// Check if return code is within acceptable bounds
		if check.Type == "host" {
			if checkResult.ReturnCode < 0 || checkResult.ReturnCode > nagiosHostStatusUnreachable {
				checkResult.ReturnCode = nagiosHostStatusUnreachable
			}
		} else {
			if checkResult.ReturnCode < 0 || checkResult.ReturnCode > nagiosServiceStatusUnknown {
				checkResult.ReturnCode = nagiosServiceStatusUnknown
			}
		}

		if config.LogLevel > 2 {
			logger.Printf("%s: worker: executed command \"%s\": [ReturnCode=%d stdOut=\"%s\" stdErr=\"%s\"]",
				checkResult.CorrelationID,
				check.CommandLine,
				checkResult.ReturnCode,
				strings.TrimSuffix(cmdStdout.String(), "\n"),
				strings.TrimSuffix(cmdStderr.String(), "\n"))
		}
	}

	timeEnd := time.Now()
	checkResult.FinishTime = float64(timeEnd.Unix()) + (float64(timeEnd.Nanosecond()) * 0.000000001)

	// Append worker hostname as signature
	if config.AppendWorkerHostname {
		checkResult.Output += fmt.Sprintf("\nbunny worker: %s", hostname)
	}

	// Acknowledge AMQP check message
	check.Message.Ack(false)

	// Send the check result to the AMQP publisher
	chkResChan <- checkResult

	// If the check message specifies a 'reply-to' header, else use the default
	if check.Message.ReplyTo != "" {
		checkResult.ReplyTo = check.Message.ReplyTo
	} else {
		checkResult.ReplyTo = config.PublisherRoutingKey
	}

	// Decrement running worker goroutines counter
	wg.Done()
}
