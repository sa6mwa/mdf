PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
BIN ?= bin/mdf

.PHONY: build install clean

build: $(BIN)

$(BIN):
	CGO_ENABLED=0 go build -o $(BIN) -trimpath -ldflags '-s -w' ./cmd/mdf/

install: $(BIN)
	install -m 0755 $(BIN) $(BINDIR)/mdf

clean:
	go clean ./...
	rm -rf bin
