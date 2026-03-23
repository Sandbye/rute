.PHONY: build test lint clean

BINARY := rute
CMD     := ./cmd/rute
VERSION ?= $(shell git describe --tags --always --dirty)

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) $(CMD)

test:
	go test ./...

lint:
	go vet ./...

clean:
	rm -f $(BINARY)

run: build
	./$(BINARY)
