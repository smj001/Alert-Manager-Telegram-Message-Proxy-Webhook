.PHONY: build run test clean

build:
	go build -trimpath -o bin/server ./cmd/server

run:
	go run ./cmd/server

test:
	go test -v ./...

clean:
	rm -rf bin/
	go clean
