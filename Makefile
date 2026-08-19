VERSION := $(shell sh -c "cat version.txt" 2> /dev/null || cmd /c "type version.txt")
BINARY_NAME=nacho-flow
LDFLAGS_COMMON := -s -w -X main.version=$(VERSION)

.PHONY: all build test bump-patch bump-minor bump-major build-win-amd64 build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 build-all clean

all: test build

build:
	@echo "Building local binary..."
	go build -ldflags="$(LDFLAGS_COMMON)" -o bin/$(BINARY_NAME) ./cmd/nacho-flow

test:
	@echo "Running unit tests..."
	go test -v ./...

bump-patch:
	go run cmd/util/version_bump/main.go -type=patch

bump-minor:
	go run cmd/util/version_bump/main.go -type=minor

bump-major:
	go run cmd/util/version_bump/main.go -type=major

build-win-amd64:
	@mkdir -p bin
	set CGO_ENABLED=0&& set GOOS=windows&& set GOARCH=amd64&& go build -ldflags="$(LDFLAGS_COMMON)" -o bin/$(BINARY_NAME)-$(VERSION)-windows-amd64.exe ./cmd/nacho-flow

build-linux-amd64:
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS_COMMON)" -o dist/$(BINARY_NAME)-$(VERSION)-linux-amd64 ./cmd/nacho-flow

build-linux-arm64:
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS_COMMON)" -o dist/$(BINARY_NAME)-$(VERSION)-linux-arm64 ./cmd/nacho-flow

build-darwin-amd64:
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS_COMMON)" -o dist/$(BINARY_NAME)-$(VERSION)-darwin-amd64 ./cmd/nacho-flow

build-darwin-arm64:
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS_COMMON)" -o dist/$(BINARY_NAME)-$(VERSION)-darwin-arm64 ./cmd/nacho-flow

build-all: build-win-amd64 build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64

clean:
	rm -rf bin dist checksums.txt
