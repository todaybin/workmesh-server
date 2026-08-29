.PHONY: build test run

build:
	go build ./cmd/workmesh-server

test:
	go test ./...

run:
	go run ./cmd/workmesh-server
