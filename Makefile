.PHONY: all build test build-all clean

BINARY_NAME=nacho-flow
VERSION=0.1.0

all: test build

build:
	@echo "Building local binary..."
	@go build -o bin/$(BINARY_NAME) ./cmd/nacho-flow

test:
	@echo "Running tests..."
	@go test -v ./...

build-all:
	@echo "Cross-compiling binaries for Linux, Windows, and macOS..."
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dist/$(BINARY_NAME)-linux-amd64 ./cmd/nacho-flow
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o dist/$(BINARY_NAME)-linux-arm64 ./cmd/nacho-flow
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o dist/$(BINARY_NAME)-windows-amd64.exe ./cmd/nacho-flow
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o dist/$(BINARY_NAME)-darwin-amd64 ./cmd/nacho-flow
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o dist/$(BINARY_NAME)-darwin-arm64 ./cmd/nacho-flow

clean:
	@rm -rf bin dist
