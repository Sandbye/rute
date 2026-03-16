.PHONY: build test lint clean

BINARY := rute
CMD     := ./cmd/rute

build:
	go build -o $(BINARY) $(CMD)

test:
	go test ./...

lint:
	go vet ./...

clean:
	rm -f $(BINARY)

run: build
	./$(BINARY)
