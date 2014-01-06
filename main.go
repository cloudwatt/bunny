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
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
)

type bunnyConfig struct {
	Broker                      string `json:"broker"`
	ConsumerID                  string `json:"consumer_id"`
	ConsumerExchange            string `json:"consumer_exchange"`
	ConsumerExchangeType        string `json:"consumer_exchange_type"`
	ConsumerExchangeDurable     bool   `json:"consumer_exchange_durable"`
	ConsumerExchangeAutodelete  bool   `json:"consumer_exchange_autodelete"`
	ConsumerQueue               string `json:"consumer_queue"`
	ConsumerQueueDurable        bool   `json:"consumer_queue_durable"`
	ConsumerQueueAutodelete     bool   `json:"consumer_queue_autodelete"`
	ConsumerQueueExclusive      bool   `json:"consumer_queue_exclusive"`
	ConsumerBindingKey          string `json:"consumer_binding_key"`
	PublisherExchange           string `json:"publisher_exchange"`
	PublisherExchangeType       string `json:"publisher_exchange_type"`
	PublisherExchangeDurable    bool   `json:"publisher_exchange_durable"`
	PublisherExchangeAutodelete bool   `json:"publisher_exchange_autodelete"`
	PublisherRoutingKey         string `json:"publisher_routing_key"`
	RetryWaitTime               int    `json:"retry_wait_time"`
	MaxExecTimeout              int    `json:"max_exec_timeout"`
	MaxConcurrency              int    `json:"max_concurrency"`
	ReportStderr                bool   `json:"report_stderr"`
	AppendWorkerHostname        bool   `json:"append_worker_hostname"`
	DebugLevel                  int    `json:"debug_level"`
}

const (
	bunnyVersion = "0.2.0"
)

var (
	config      bunnyConfig
	configFile  *string
	showVersion *bool
	run         bool
	wg          sync.WaitGroup
	err         error
	hostname    string
	logger      *log.Logger
	chkChan     chan *nagiosCheck
	chkResChan  chan *nagiosCheckResult
)

func init() {
	logger = log.New(os.Stdout, "bunny: ", log.LstdFlags)

	if hostname, err = os.Hostname(); err != nil {
		logger.Fatalf("unable to get system hostname: %s", err)
	}

	// Default configuration values
	config = bunnyConfig{
		Broker:                      "amqp://guest:guest@localhost:5672/",
		ConsumerID:                  "bunny-worker",
		ConsumerExchange:            "nagios",
		ConsumerExchangeType:        "direct",
		ConsumerExchangeDurable:     true,
		ConsumerExchangeAutodelete:  false,
		ConsumerQueue:               "nagios_checks",
		ConsumerQueueDurable:        true,
		ConsumerQueueAutodelete:     false,
		ConsumerQueueExclusive:      false,
		ConsumerBindingKey:          "nagios_checks",
		PublisherExchange:           "nagios",
		PublisherExchangeType:       "direct",
		PublisherExchangeDurable:    true,
		PublisherExchangeAutodelete: false,
		PublisherRoutingKey:         "nagios_results",
		MaxExecTimeout:              30,
		MaxConcurrency:              runtime.NumCPU() * 10,
		RetryWaitTime:               3,
		ReportStderr:                false,
		AppendWorkerHostname:        true,
		DebugLevel:                  0,
	}

	configFile = flag.String("c", "/etc/bunny.conf", "Configuration file path")
	showVersion = flag.Bool("v", false, "Show software version")

	flag.Parse()

	run = true
}

func main() {
	if *showVersion {
		fmt.Printf("bunny version %s\nGo compiler: %s (%s)\n",
			bunnyVersion,
			runtime.Version(),
			runtime.Compiler)
		return
	}

	// Handle termination signals for clean exit
	sig := make(chan os.Signal, 1)
	exit := make(chan bool, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)

	go func() {
		<-sig // Block until we receive a notification on the chan from signal handler

		if config.DebugLevel > 0 {
			logger.Println("received termination signal")
		}

		run = false
		exit <- true
	}()

	logger.Println("starting")

	// Parse configuration file
	if err := parseConfig(configFile); err != nil {
		logger.Fatalf("cannot parse configuration file: %s", err)
	}

	chkChan = make(chan *nagiosCheck)
	chkResChan = make(chan *nagiosCheckResult)

	go func() {
		var nc *nagiosCheck

		// Execute each check request in a goroutine
		for nc = range chkChan {
			// Increment running worker goroutines counter
			wg.Add(1)
			go nc.execute()
		}
	}()

	// AMQP publisher sends the results from the checks run by the worker
	// to the queue to be consumed by Nagios's mod_bunny
	go runAMQPPublisher(chkResChan)

	// AMQP consumer fetches checks scheduled by Nagios from the queue,
	// sends them on the chkChan channel for the worker to execute them
	go runAMQPConsumer(chkChan)

	<-exit // Block until we receive a termination signal

	// Stop consuming from AMQP broker
	stopAMQPConsumer()

	// Wait for gorountines to finish
	wg.Wait()

	close(chkChan)
	close(chkResChan)

	stopAMQPPublisher()

	logger.Println("terminating")
}

func parseConfig(file *string) error {
	var confData []byte
	var err error

	if confData, err = ioutil.ReadFile(*file); err != nil {
		return err
	}

	if err = json.Unmarshal(confData, &config); err != nil {
		return err
	}

	return nil
}
