VERSION := $(shell sh -c "cat version.txt" 2> /dev/null || cmd /c "type version.txt")
BINARY_NAME=nacho-flow
LDFLAGS_COMMON := -s -w -X main.version=$(VERSION)

.PHONY: all build test test-race test-cover fmt vet lint check ci bench tune tune-apply bump-patch bump-minor bump-major build-win-amd64 build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 build-all clean

all: check build

build:
	@echo "Building local binary..."
	go build -ldflags="$(LDFLAGS_COMMON)" -o bin/$(BINARY_NAME) ./cmd/nacho-flow

fmt:
	@echo "Formatting Go source files..."
	gofmt -s -w .

vet:
	@echo "Running go vet..."
	go vet ./...

lint:
	@echo "Running golangci-lint..."
	golangci-lint run ./...

sec:
	@echo "Running gosec security vulnerability analysis..."
	gosec -exclude=G706 ./...

test:
	@echo "Running unit tests..."
	go test -v ./...

test-race:
	@echo "Running tests with race detector..."
	go test -v -race -count=1 ./...

test-cover:
	@echo "Running tests with coverage profiling..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report written to coverage.html"

bench:
	@echo "Running high-concurrency benchmark suite..."
	go run ./cmd/util/nacho_bench

tune:
	@echo "Running advisory route tuner..."
	go run ./cmd/nacho-flow tune

tune-apply:
	@echo "Applying tuned rules to config.yaml..."
	go run ./cmd/nacho-flow tune --apply

check: fmt vet sec test-race
	@echo "✅ All code quality checks, security scans, and race tests passed!"

ci: check build-all
	@echo "🚀 CI Pipeline verification passed!"

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
	rm -rf bin dist coverage.out coverage.html checksums.txt
