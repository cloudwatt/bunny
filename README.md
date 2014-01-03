bunny: remote Nagios host/service check execution worker
========================================================

**bunny** is [mod_bunny](http://github.com/cloudwatt/mod_bunny)'s worker companion: it executes host/service checks commands queued by Nagios's mod_bunny and publishes the check results back into the RabbitMQ broker for mod_bunny to reinject them into Nagios.

Requirements
------------

* Go language [distribution](http://golang.org/doc/install#download) (>= 1.1)
* Go [client](https://github.com/streadway/amqp) for AMQP 0.9.1

Installation
------------

Inside your repository local checkout, execute the following command:

```
make && sudo make install
```

The default installation path is `/usr/bin/bunny` and `/etc/bunny.conf` for configuration; set the environment `PREFIX` to whatever path prefix you prefer (e.g. `PREFIX=/usr/local`).

Configuration
-------------

The configuration file is using JSON format. Here are the supported settings and their default value:

* `"broker": "amqp://guest:guest@localhost:5672/"` Broker URL (URL syntax: amqp://username:password@host:port/)
* `"consumer_id": "bunny-worker"` AMQP consumer identifier
* `"consumer_exchange": "nagios"` Broker exchange to connect to for consuming checks messages
* `"consumer_exchange_type": "direct"` Broker consumer exchange type*
* `"consumer_queue": "nagios_checks"` Queue to bind to for consuming check messages
* `"consumer_binding_key": "nagios_checks"` Binding key to use to consume check messages
* `"publisher_exchange": "nagios"` Broker exchange to connect to for publishing checks result messages
* `"publisher_exchange_type": "direct"` Broker publisher exchange type
* `"publisher_routing_key": "nagios_results"` Routing key to apply when publishing check result messages
* `"max_exec_timeout": 30` Maximum check command execution time (in seconds)
* `"max_concurrency": <N CPU * 10>` Maximum check commands allowed to run concurrently
* `"retry_wait_time": 3` Time to wait (in seconds) before trying to reconnect to the broker
* `"report_stderr": false` Report check command output on `stderr` in addition to `stdout`
* `"append_worker_hostname": true` Append worker host name to Nagios check results
* `"debug_level": 0` Debugging output verbosity (0 = none, 1 = show AMQP events and worker activity, 2 = same as 1 + dump received/sent AMQP messages and executed commands execution details)

\* : To benefit from the _round-robin_ load-balancing RabbitMQ feature, the consumer exchange **MUST** be of type _direct_. Read [this](http://www.rabbitmq.com/tutorials/amqp-concepts.html#exchange-direct) to understand why.

Basic configuration example:

```
{
  "broker": "amqp://bunnyUser:bunnyPassword@some.amqp.broker.example.net:5672/"
}
```

Usage
-----

To start bunny worker, use the following command (`-c` option specifies path to configuration file):

```
bunny -c /etc/bunny.conf
```

Note: `bunny` always runs in foreground, use `&` to start it in background. You can spawn as many bunny instances as you have available CPU cores to level the checks processing among your worker hosts, resulting in a weighted round-robin load balancing.

Bugs
----

Probably. Let me know if you find any.

License / Copyright
-------------------

This software is released under the MIT License.

Copyright (c) 2013 Marc Falzon / Cloudwatt

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
