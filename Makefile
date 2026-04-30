.PHONY: test build clean

test:
	go test ./...

build:
	go build -o bin/agentctl ./cmd/agentctl

clean:
	go clean
	rm -rf bin dist
