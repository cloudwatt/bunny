all: bunny

bunny:
	go get github.com/streadway/amqp
	go build -o bunny

install:
	install -D bunny $(PREFIX)/usr/bin/bunny
	install -D -m 600 bunny.conf $(PREFIX)/etc/bunny.conf

clean:
	rm -f bunny
