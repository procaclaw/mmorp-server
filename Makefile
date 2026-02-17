.PHONY: tidy test build run

tidy:
	go mod tidy

test:
	go test ./...

build:
	go build ./...

run:
	go run ./cmd/server
