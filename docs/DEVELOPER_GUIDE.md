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
│   ├── nacho-flow/         # Production binary entrypoint
│   └── util/
│       ├── nacho_bench/    # In-memory load testing & stress benchmark CLI
│       ├── nacho_releaser/ # Automated multi-platform GitHub release tool
│       └── version_bump/   # Semantic version bumping utility
├── docs/                   # Architecture, Benchmarks, Tuning Guide, User & Dev Guides
├── logs/                   # Default directory for interactive log files
├── pkg/
│   ├── config/             # YAML configuration parser & validation
│   ├── contract/           # Core interface definitions & shared types
│   ├── provider/           # Capability interfaces (LLM, Auth, Header, Health, Registry)
│   ├── router/             # Context classifier, token estimator, image sanitizer
│   ├── server/             # HTTP reverse proxy & route handler
│   ├── store/              # Atomic disk persistence for telemetry (stats.json)
│   ├── strategy/           # Compiled expr-lang dynamic rule evaluator
│   └── telemetry/          # Pricing oracle, OpenRouter plugin, StatsTracker, slog
├── .github/workflows/      # CI/CD pipeline & Azure Trusted Signing
├── config.yaml             # Default configuration file
└── go.mod / go.sum         # Go module definitions
```

---

## 3. Building & Running Locally

```bash
# Build the binary
go build -o nacho-flow.exe ./cmd/nacho-flow

# Run interactively on custom port with debug logging
./nacho-flow.exe -config config.yaml -port 8080 -log-level debug
```

---

## 4. Quality Assurance, Security & Testing

All contributions must pass code quality formatting, static analysis, AST security scans (`gosec`), and the entire test suite with race detection enabled:

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

# Run advisory route tuner
make tune
```

---

## 5. Benchmarking & Load Testing

Nacho Flow includes both standard Go benchmarks and a dedicated high-throughput load test utility:

### Standard Go Benchmark
```bash
go test -bench=BenchmarkProxy_ChatCompletions_EndToEnd -benchmem ./pkg/server/...
```

### High-Concurrency Stress Benchmark
```bash
# Run multi-tier stress test up to 1,000 parallel workers:
go run ./cmd/util/nacho_bench

# Or run custom load:
go run ./cmd/util/nacho_bench -n 50000 -c 100
```

---

## 6. Extending Nacho Flow: Adding a Custom Provider Plugin

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

## 7. Release & CI/CD Pipeline

Nacho Flow uses a multi-step GitHub Actions pipeline (`.github/workflows/ci.yml`):
1. **Linux / macOS / Windows Matrix Builds**: Compiles statically (`CGO_ENABLED=0`).
2. **Azure Trusted Signing**: Cryptographically signs the Windows AMD64 binary with Authenticode certificates during the release workflow.
3. **Artifact Deployment**: Automated release creation via `cmd/util/nacho_releaser`.
