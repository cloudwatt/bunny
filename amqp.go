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
	"fmt"
	"github.com/streadway/amqp"
	"time"
)

var amqpConsumer struct {
	connected  bool
	connection *amqp.Connection
	channel    *amqp.Channel
	queue      amqp.Queue
	messages   <-chan amqp.Delivery
	cError     chan *amqp.Error
}

var amqpPublisher struct {
	connected  bool
	connection *amqp.Connection
	channel    *amqp.Channel
	cError     chan *amqp.Error
}

func initAMQPConsumer() error {
	amqpConsumer.connected = false

	// Connection checking watcher
	go func() {
		amqpConsumer.cError = make(chan *amqp.Error)

		for e := range amqpConsumer.cError {
			logger.Printf("consumer: error: disconnected from broker: %s", e.Reason)
		}

		amqpConsumer.connected = false
	}()

	// Connect to broker
	if amqpConsumer.connection, err = amqp.Dial(config.Broker); err != nil {
		return fmt.Errorf("consumer: error: unable to connect to broker: %s", err)
	}

	amqpConsumer.connection.NotifyClose(amqpConsumer.cError)

	if amqpConsumer.channel, err = amqpConsumer.connection.Channel(); err != nil {
		return fmt.Errorf("consumer: error: unable to open channel on broker: %s", err)
	}

	if config.LogLevel > 0 {
		logger.Printf("consumer: connected to broker")
	}

	// Only receive <config.MaxConcurrency> messages per channel until ack'd
	if err = amqpConsumer.channel.Qos(config.MaxConcurrency, 0, false); err != nil {
		return fmt.Errorf("consumer: error: unable to set QoS on channel: %s", err)
	}

	if err = amqpConsumer.channel.ExchangeDeclare(
		config.ConsumerExchange,           // exchange name
		config.ConsumerExchangeType,       // exchange type
		config.ConsumerExchangeDurable,    // `durable` flag
		config.ConsumerExchangeAutodelete, // `auto delete` flag
		false, // `internal` flag
		false, // `nowait` flag
		nil,   // arguments
	); err != nil {
		return fmt.Errorf("consumer: error: unable to declare exchange \"%s\": %s", config.ConsumerExchange, err)
	}

	if config.LogLevel > 1 {
		logger.Printf("consumer: declared exchange %s (%s)", config.ConsumerExchange, config.ConsumerExchangeType)
	}

	if amqpConsumer.queue, err = amqpConsumer.channel.QueueDeclare(
		config.ConsumerQueue,           // queue name
		config.ConsumerQueueDurable,    // `durable` flag
		config.ConsumerQueueAutodelete, // `auto delete` flag
		config.ConsumerQueueExclusive,  // `exclusive` flag
		false, // `nowait` flag
		nil,   // arguments
	); err != nil {
		return fmt.Errorf("consumer: error: unable to declare queue \"%s\": %s", config.ConsumerQueue, err)
	}

	if err = amqpConsumer.channel.QueueBind(
		config.ConsumerQueue,      // queue name
		config.ConsumerBindingKey, // binding (routing) key
		config.ConsumerExchange,   // exchange to bind
		false, // `nowait` flag
		nil,   // arguments
	); err != nil {
		return fmt.Errorf("consumer: error: unable to bind queue %s to exchange %s: %s",
			config.ConsumerQueue,
			config.ConsumerExchange,
			err)
	}

	if config.LogLevel > 1 {
		logger.Printf("consumer: bound queue %q matching key %q to exchange %q",
			amqpConsumer.queue.Name,
			config.ConsumerBindingKey,
			config.ConsumerExchange)
	}

	if amqpConsumer.messages, err = amqpConsumer.channel.Consume(
		config.ConsumerQueue, // queue name
		config.ConsumerID,    // consumer identifier
		false,                // `noack` flag
		config.ConsumerQueueExclusive, // `exclusive` flag
		false, // `nolocal` flag
		false, // `nowait` flag
		nil,   // arguments
	); err != nil {
		return fmt.Errorf("consumer: error: unable to consume messages from queue \"%s\": %s",
			config.ConsumerQueue, err)
	}

	if config.LogLevel > 1 {
		logger.Printf("consumer: ready to consume from queue %s", config.ConsumerQueue)
	}

	amqpConsumer.connected = true

	return nil
}

func initAMQPPublisher() error {
	amqpPublisher.connected = false

	// Connection checking watcher
	go func() {
		amqpPublisher.cError = make(chan *amqp.Error)

		for e := range amqpPublisher.cError {
			logger.Printf("publisher: error: disconnected from broker: %s", e.Reason)
		}

		amqpPublisher.connected = false
	}()

	// Connect to broker
	if amqpPublisher.connection, err = amqp.Dial(config.Broker); err != nil {
		return fmt.Errorf("publisher: error: unable to connect to broker: %s", err)
	}

	amqpPublisher.connection.NotifyClose(amqpPublisher.cError)

	if amqpPublisher.channel, err = amqpPublisher.connection.Channel(); err != nil {
		return fmt.Errorf("publisher: error: unable to open channel on broker: %s", err)
	}

	if config.LogLevel > 0 {
		logger.Printf("publisher: connected to broker")
	}

	if err = amqpPublisher.channel.ExchangeDeclare(
		config.PublisherExchange,           // exchange name
		config.PublisherExchangeType,       // exchange type
		config.PublisherExchangeDurable,    // `durable` flag
		config.PublisherExchangeAutodelete, // `auto delete` flag
		false, // `internal` flag
		false, // `nowait` flag
		nil,   // arguments
	); err != nil {
		return fmt.Errorf("publisher: error: unable to declare exchange \"%s\": %s", config.PublisherExchange, err)
	}

	if config.LogLevel > 1 {
		logger.Printf("publisher: declared exchange %s (%s)", config.PublisherExchange, config.PublisherExchangeType)
	}

	amqpPublisher.connected = true

	return nil
}

func stopAMQPConsumer() {
	if amqpConsumer.connected {
		amqpConsumer.connected = false

		if config.LogLevel > 0 {
			logger.Printf("consumer: disconnecting from broker")
		}

		amqpConsumer.channel.Cancel(config.ConsumerID, false)
		amqpConsumer.channel.Close()
		amqpConsumer.connection.Close()
	}
}

func stopAMQPPublisher() {
	if amqpPublisher.connected {
		amqpPublisher.connected = false

		if config.LogLevel > 0 {
			logger.Printf("publisher: disconnecting from broker")
		}

		amqpPublisher.channel.Close()
		amqpPublisher.connection.Close()
	}
}

func runAMQPConsumer(checkChan chan<- *nagiosCheck) {
	// Run until termination signal
	for run {
		for !amqpConsumer.connected {
			if err := initAMQPConsumer(); err != nil {
				logger.Printf("%s", err)

				if config.LogLevel > 0 {
					logger.Printf("consumer: waiting for %ds before retry connecting", config.RetryWaitTime)
				}

				time.Sleep(time.Second * time.Duration(config.RetryWaitTime))
			}
		}

		if config.LogLevel > 0 {
			logger.Printf("consumer: entering loop")
		}

		for message := range amqpConsumer.messages {
			if config.LogLevel > 2 {
				logger.Printf(
					"%s: consumer: received message: ["+
						"ContentType=\"%s\" "+
						"Exchange=\"%s\" "+
						"RoutingKey=\"%s\" "+
						"ReplyTo=\"%s\" "+
						"Body=\"%s\""+
						"]",
					message.CorrelationId,
					message.ContentType,
					message.Exchange,
					message.RoutingKey,
					message.ReplyTo,
					message.Body)
			}

			// Discard non JSON-formatted messages
			if message.ContentType != "application/json" {
				if message.ContentType == "" {
					logger.Printf("%s: consumer: error: message has no content type",
						message.CorrelationId)
				} else {
					logger.Printf("%s: consumer: error: unsupported message content type \"%s\"",
						message.CorrelationId,
						message.ContentType)
				}

				message.Ack(true)

				continue
			}

			check := new(nagiosCheck)
			if err = json.Unmarshal(message.Body, check); err != nil {
				logger.Printf("%s: consumer: error: unable to unmarshal check: %s",
					message.CorrelationId,
					err)
				continue
			}

			check.Message = message

			checkChan <- check
		}

		if config.LogLevel > 0 {
			logger.Printf("consumer: loop stopped, disconnecting from broker")
		}

		stopAMQPConsumer()
	}
}

func runAMQPPublisher(checkResultChan <-chan *nagiosCheckResult) {
	var (
		checkResult *nagiosCheckResult
		crJSON      []byte
		message     amqp.Publishing
	)

	for checkResult = range checkResultChan {
		// Check that we are connected to the broker before going forward
		for !amqpPublisher.connected {
			if err := initAMQPPublisher(); err != nil {
				logger.Printf("%s", err)

				if config.LogLevel > 0 {
					logger.Printf("publisher: waiting for %ds before retry connecting",
						config.RetryWaitTime)
				}

				time.Sleep(time.Second * time.Duration(config.RetryWaitTime))
			}
		}

		if crJSON, err = json.Marshal(*checkResult); err != nil {
			logger.Printf("%s: publisher: error: unable to marshal check result: %s",
				checkResult.CorrelationID,
				err)
			continue
		}

		message = amqp.Publishing{
			ContentType:   "application/json",
			CorrelationId: checkResult.CorrelationID,
			DeliveryMode:  amqp.Transient,
			Timestamp:     time.Now(),
			Body:          crJSON,
		}

		if err = amqpPublisher.channel.Publish(
			config.PublisherExchange, // exchange name
			checkResult.ReplyTo,      // routing key
			false,                    // `mandatory` flag
			false,                    // `immediate` flag
			message); err != nil {
			logger.Printf("%s: publisher: error: unable to publish message: %s",
				checkResult.CorrelationID,
				err)
			stopAMQPPublisher()
		}

		if config.LogLevel > 2 {
			logger.Printf(
				"%s: publisher: sent message: ["+
					"ContentType=\"%s\" "+
					"Exchange=\"%s\" "+
					"RoutingKey=\"%s\" "+
					"Body=\"%s\""+
					"]",
				checkResult.CorrelationID,
				message.ContentType,
				config.PublisherExchange,
				checkResult.ReplyTo,
				message.Body)
		}
	}

	if config.LogLevel > 0 {
		logger.Printf("publisher: loop stopped")
	}
}
