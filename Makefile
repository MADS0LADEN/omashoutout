.PHONY: build test install uninstall clean

VERSION ?= $(shell git describe --tags --always --dirty)

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/shoutout ./cmd/shoutout

test:
	go test -race ./...
	go vet ./...

install: build
	./bin/shoutout install

uninstall:
	shoutout uninstall

clean:
	rm -rf bin dist
