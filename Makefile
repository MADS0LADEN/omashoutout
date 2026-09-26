.PHONY: build kde test install uninstall clean

VERSION ?= $(shell git describe --tags --always --dirty)

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/shoutout ./cmd/shoutout

kde:
	cmake -S kde -B build/kde -G Ninja
	cmake --build build/kde

test:
	go test -race ./...
	go vet ./...

install: build kde
	./bin/shoutout install

uninstall:
	shoutout uninstall

clean:
	rm -rf bin dist
