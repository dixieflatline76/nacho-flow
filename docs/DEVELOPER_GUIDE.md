# 🌮 Nacho Flow: Developer Guide

This guide is intended for engineers contributing to, extending, or maintaining **Nacho Flow**.

---

## 1. Development Prerequisites

- **Go**: Version `1.22+` (supports `slog`, generic channels, and `atomic.Pointer`).
- **Make** (optional): For running build and release targets.
- **Git**: For version control.

---

## 2. Project Layout

```text
nacho-flow/
├── cmd/
│   ├── nacho-flow/         # Production binary entrypoint (CLI, service manager, tuner, version)
│   └── util/
│       ├── nacho_bench/    # In-memory load testing & stress benchmark CLI
│       ├── nacho_releaser/ # Automated multi-platform GitHub release tool
│       └── version_bump/   # Semantic version bumping utility
├── docs/                   # Architecture, Benchmarks, Tuning Guide, User & Dev Guides
├── logs/                   # Default directory for interactive log files
├── pkg/
│   ├── config/             # YAML configuration parser & validation
│   ├── contract/           # Core interface definitions & shared types
│   ├── provider/           # Capability interfaces (LLM, Auth, Header, Health, CircuitBreaker, Registry)
│   ├── router/             # Context classifier, adaptive estimator, session tracker, image sanitizer, tool normalizer
│   ├── server/             # HTTP reverse proxy, delayed header validator, stream normalizer (reasoning -> think)
│   ├── store/              # Atomic disk persistence for telemetry (stats.json)
│   ├── strategy/           # Compiled expr-lang dynamic rule evaluator with MaxContext guards
│   ├── telemetry/          # Pricing oracle, OpenRouter plugin, StatsTracker, slog
│   └── tuner/              # Cost-penalty rule synthesizer & advisory engine
├── scripts/                # Universal Linux/macOS shell installer & test harness
├── .github/workflows/      # CI/CD pipeline, Docker GHCR publisher & Azure Trusted Signing
├── Dockerfile              # Distroless multi-arch container image
├── .dockerignore           # Container build exclusions
├── config.yaml             # Default configuration file
└── go.mod / go.sum         # Go module definitions
```

---

## 3. Makefile Command Reference & Key Targets

Nacho Flow provides a comprehensive `Makefile` implementing Spice-grade quality, testing, security, tuning, and cross-platform compilation workflows:

| Target | Command Executed | Description & When to Use |
| :--- | :--- | :--- |
| **`make check`** | `fmt vet sec test-race` | **Primary local gate**: Run before every commit or PR. |
| **`make build`** | `go build ... -o bin/nacho-flow` | Compiles optimized local binary with embedded version. |
| **`make fmt`** | `gofmt -s -w .` | Simplifies and formats all Go source files. |
| **`make vet`** | `go vet ./...` | Analyzes code for potential correctness and bug patterns. |
| **`make lint`** | `golangci-lint run ./...` | Runs configured linter suite (`.golangci.yml`). |
| **`make sec`** | `gosec -exclude=G706 ./...` | Runs AST security analysis for CWE vulnerabilities. |
| **`make test`** | `go test -v ./...` | Runs standard unit tests across all packages. |
| **`make test-race`** | `go test -v -race -count=1 ./...` | Runs test suite with Go's race detector active. |
| **`make test-cover`**| `go test -coverprofile=...` | Generates interactive HTML test coverage report. |
| **`make bench`** | `go run ./cmd/util/nacho_bench` | Runs pre-warmed high-concurrency benchmark suite. |
| **`make tune`** | `go run ./cmd/nacho-flow tune` | Analyzes `traffic.jsonl` and prints advisory tuning report. |
| **`make tune-apply`**| `go run ./cmd/nacho-flow tune --apply` | Synthesizes rules and atomically updates `config.yaml`. |
| **`make bump-patch`**| `version_bump -type=patch` | Bumps patch version (e.g. `0.2.0` $\rightarrow$ `0.2.1`) & tags git. |
| **`make bump-minor`**| `version_bump -type=minor` | Bumps minor version (e.g. `0.2.0` $\rightarrow$ `0.3.0`) & tags git. |
| **`make bump-major`**| `version_bump -type=major` | Bumps major version (e.g. `0.2.0` $\rightarrow$ `1.0.0`) & tags git. |
| **`make build-all`** | Multiple `GOOS`/`GOARCH` builds | Compiles binaries for Windows, Linux, and macOS (amd64/arm64). |
| **`make ci`** | `check build-all` | Full CI verification gate. |
| **`make clean`** | `rm -rf bin dist coverage.*` | Cleans temporary build artifacts and test reports. |

---

## 4. Building & Running Locally

```bash
# Build the binary using make
make build

# Run interactively on custom port with debug logging
./bin/nacho-flow -config config.yaml -port 8080 -log-level debug
```

---

## 5. Quality Assurance, Security & Testing

Nacho Flow follows strict **Test-Driven Development (TDD)** and quality standards:
- **TDD Workflow**: Write failing tests first (`Red`), implement minimal clean code (`Green`), and refactor.
- **Coverage Gate**: Every individual package must maintain $\ge 90\%$ statement coverage (repository target $\ge 96\%$).
- **Zero Anti-Patterns**: Lock-free atomic synchronization on hot paths (`sync/atomic.Pointer`), zero detached background goroutine leaks (lazy TTL eviction), and zero external dependencies.

```bash
# Run all-in-one quality gate (fmt, vet, gosec, race tests)
make check

# Run AST security vulnerability analysis (matching Spice standards)
make sec

# Run golangci-lint suite
make lint

# Run all unit and integration tests with race detector
make test-race
# or: go test -count=1 -v -race ./...

# Generate HTML code coverage report
make test-cover

# Check per-package test coverage
go test -cover ./pkg/...

# Run advisory route tuner
make tune
```

---

## 6. Benchmarking & Load Testing

Nacho Flow includes both standard Go micro-benchmarks (`testing.B`) and a dedicated high-throughput A/B load test CLI:

### 6.1 End-to-End Proxy Pipeline Benchmark
```bash
# Benchmark core proxy HTTP pipeline with zero-alloc fast path:
go test -bench=BenchmarkProxy_ChatCompletions -benchmem -run=^$ ./pkg/server/...
```

### 6.2 Inner Tool Normalizer Micro-Benchmarks
```bash
# Measure nanosecond parsing and allocation efficiency across all 7 model formats:
go test -bench=BenchmarkNormalize -benchmem -run=^$ ./pkg/router/...
```

### 6.3 High-Concurrency A/B Stress Benchmark
```bash
# Run calibrated 250,000-request A/B stress test comparing raw pass-through vs full auth & normalization:
make bench
# or: go run ./cmd/util/nacho_bench
```

---

## 7. Extending the Multi-Model Tool Normalizer (`pkg/router/tool_normalizer.go`)

When adding support for a new open-source LLM format family:

1. **Add Token Signature to the Fast Pre-Filter**:
   In `hasCandidateToolTokens`, add the unique opening marker bytes (e.g. `[CUSTOM_CALL]`). If none match, non-tool prompts bail out in 23.5ns with zero allocations.
2. **Implement Lexical Extraction**:
   Use `extractBalancedJSON` to balance opening and closing `{}` or `[]` brackets without regex truncation.
3. **Add Comprehensive Test Case**:
   Add a test case in `pkg/router/tool_normalizer_test.go` verifying:
   - Tool name and arguments parsed correctly.
   - Arguments formatted as stringified JSON.
   - Cleaned text stripped of markers while preserving reasoning `<think>` tags.
   - Add a corresponding `BenchmarkNormalize_<Format>` micro-benchmark.

---

## 8. Extending Nacho Flow: Adding a Custom Provider Plugin

To add a specialized provider plugin:

1. **Implement Capability Interfaces (`pkg/provider/interfaces.go`)**:
   ```go
   package myprovider

   import "context"

   type MyCloudProvider struct {
       id     string
       apiKey string
   }

   func (p *MyCloudProvider) ID() string      { return "my_cloud" }
   func (p *MyCloudProvider) Name() string    { return "My Cloud Enterprise" }
   func (p *MyCloudProvider) BaseURL() string { return "https://api.mycloud.com/v1" }
   func (p *MyCloudProvider) IsLocal() bool   { return false }
   func (p *MyCloudProvider) GetAPIKey() string { return p.apiKey }
   func (p *MyCloudProvider) GetHeaders() map[string]string {
       return map[string]string{"X-Organization": "corp"}
   }
   ```

2. **Register in the Registry**:
   ```go
   registry.Register(&MyCloudProvider{id: "my_cloud", apiKey: key})
   ```

---

## 9. Release & CI/CD Pipeline

Nacho Flow uses a multi-step GitHub Actions pipeline (`.github/workflows/ci.yml`):
1. **Linux / macOS / Windows Matrix Builds**: Compiles statically (`CGO_ENABLED=0`).
2. **Azure Trusted Signing**: Cryptographically signs the Windows AMD64 binary with Authenticode certificates during the release workflow.
3. **Artifact Deployment**: Automated release creation via `cmd/util/nacho_releaser`.
