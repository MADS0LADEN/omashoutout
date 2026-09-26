.PHONY: build kde test install uninstall package package-install clean

VERSION ?= $(shell git describe --tags --always --dirty)

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o build/shoutout ./cmd/shoutout

kde:
	cmake -S kde -B build/kde -G Ninja
	cmake --build build/kde

test:
	go test -race ./...
	go vet ./...

install: build kde
	./build/shoutout install

uninstall:
	shoutout uninstall

MAKEPKG_FLAGS ?= -s

package:
	mkdir -p build/arch/sources
	BUILDDIR="$(CURDIR)/build/arch" SRCDEST="$(CURDIR)/build/arch/sources" PKGDEST="$(CURDIR)/build" SRCPKGDEST="$(CURDIR)/build" makepkg $(MAKEPKG_FLAGS)

package-install: MAKEPKG_FLAGS = -si
package-install: package

clean:
	rm -rf build
