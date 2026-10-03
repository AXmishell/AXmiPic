.PHONY: build run vet tidy test clean

BINARY := bin/axmipic

build:
	go build -o $(BINARY) ./cmd/axmipic

run:
	go run ./cmd/axmipic -config configs/config.example.yaml

vet:
	go vet ./...

tidy:
	go mod tidy

test:
	go test ./...

clean:
	go clean
	rm -rf bin data
