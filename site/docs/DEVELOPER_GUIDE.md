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
│   ├── nacho-flow/         # Production binary entrypoint (CLI, deals reporter, service manager, tuner)
│   └── util/
│       ├── gen_catalog/    # Curated model catalog generator & OTA sync updater
│       ├── nacho_bench/    # In-memory load testing & bare-metal doc synchronization harness (-sync)
│       ├── nacho_cover/    # Statement-level test coverage matrix analyzer & doc synchronizer
│       ├── nacho_releaser/ # Automated multi-platform GitHub release tool
│       └── version_bump/   # Semantic version bumping utility
├── data/
│   └── models.json         # Canonical remote model catalog for GitHub OTA serving
├── docs/                   # Architecture, Benchmarks, Tuning Guide, User & Dev Guides
├── extension/              # VS Code Companion Extension (TypeScript thin client, Webview, SSE IPC)
├── logs/                   # Default directory for interactive log files
├── pkg/
│   ├── config/             # YAML configuration parser, RCU reloader & validation
│   ├── contract/           # Core interface definitions, ProviderType enum & bitmask DTOs
│   ├── provider/           # Capability interfaces (LLM, Auth, Header, Health, CircuitBreaker, Registry)
│   ├── router/             # Context classifier, adaptive estimator, session tracker, image sanitizer, tool normalizer, feature flags
│   │   └── shield/         # Agentic fallback shield, sliding tail-buffer & question heuristic engine
│   ├── safeio/             # Bounded path and safe atomic filesystem operations
│   ├── server/             # HTTP reverse proxy, delayed header validator, stream normalizer (reasoning -> think), deals API
│   ├── store/              # Atomic disk persistence for telemetry (stats.json)
│   ├── strategy/           # Compiled expr-lang dynamic rule evaluator with MaxContext guards
│   ├── telemetry/          # Pricing oracle, factory registry, OpenRouter plugin, StatsTracker, 3-tier classifier
│   │   └── curation/       # Embedded baseline + OTA GitHub semver catalog manager
│   └── tuner/              # Cost-penalty rule synthesizer & advisory engine
├── scripts/                # Universal Linux/macOS shell installer & test harness
├── site/                   # Static landing page, interactive documentation hub & benchmarks
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
| **`make check`** | `fmt vet sec test-race test-extension` | **Primary local gate**: Run before every commit or PR. |
| **`make build`** | `go build ... -o bin/nacho-flow` | Compiles optimized local binary with embedded version. |
| **`make fmt`** | `gofmt -s -w .` | Simplifies and formats all Go source files. |
| **`make vet`** | `go vet ./...` | Analyzes code for potential correctness and bug patterns. |
| **`make lint`** | `golangci-lint run ./...` | Runs configured linter suite (`.golangci.yml`). |
| **`make sec`** | `gosec -exclude=G706 ./...` | Runs AST security analysis for CWE vulnerabilities. |
| **`make test`** | `go test -v ./...` | Runs standard unit tests across all packages. |
| **`make test-race`** | `go test -v -race -count=1 ./...` | Runs test suite with Go's race detector active. |
| **`make test-cover`**| `go test -coverprofile=...` | Generates interactive HTML test coverage report. |
| **`make bench`** | `go run ./cmd/util/nacho_bench` | Runs pre-warmed high-concurrency benchmark suite. |
| **`make bench-sync`**| `go run ./cmd/util/nacho_bench -sync` | Runs bare-metal benchmark harness and updates doc tables. |
| **`make cover-sync`**| `go run ./cmd/util/nacho_cover` | Evaluates Go + Jest coverage and syncs documentation matrices. |
| **`make test-sync`** | `go test -race ./... && nacho_cover` | Executes full race test suite and updates coverage tables. |
| **`make tune`** | `go run ./cmd/nacho-flow tune` | Analyzes `traffic.jsonl` and prints advisory tuning report. |
| **`make tune-apply`**| `go run ./cmd/nacho-flow tune --apply` | Synthesizes rules and atomically updates `config.yaml`. |
| **`make package-extension`** | `npx vsce package ...` | Builds and packages local VS Code extension `.vsix`. |
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
- **Coverage Gate**: Every individual package must maintain $\ge 95\%$ statement coverage (repository target $\ge 96\%$).
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

### 8.2 Adding a Pricing Provider via Factory Registry (`pkg/telemetry/registry.go`)

Nacho Flow uses an open, decoupled factory registry for dynamic model pricing lookups and "Heat Seeker" spot deal discovery:

1. **Implement `PricingProvider` (`pkg/telemetry/interfaces.go`)**:
   ```go
   package custompricing

   import (
       "context"
       "github.com/dixieflatline76/nacho-flow/pkg/contract"
       "github.com/dixieflatline76/nacho-flow/pkg/telemetry"
   )

   type CustomPricingProvider struct {
       apiKey string
   }

   func (c *CustomPricingProvider) ID() string { return "custom_aggregator" }

   func (c *CustomPricingProvider) FetchPricing(ctx context.Context) (map[string]contract.ModelPricing, error) {
       // Fetch and parse model pricing from custom API or internal catalog
       return map[string]contract.ModelPricing{ ... }, nil
   }
   ```

2. **Self-Register in `init()`**:
   ```go
   func init() {
       telemetry.RegisterPricingFactory("custom_aggregator", func(apiKey string) telemetry.PricingProvider {
           return &CustomPricingProvider{apiKey: apiKey}
       })
   }
   ```

3. **Zero Core Modifications**:
   Because `cmd/nacho-flow/main.go` and `pkg/server/api.go` iterate over `telemetry.GetRegisteredPricingFactories()`, adding your new pricing provider file automatically enables:
   - Live startup pricing catalog synchronization.
   - On-demand refresh via `POST /v1/pricing/refresh`.
   - Transparent integration with the lock-free atomic pricing map (`atomic.Pointer[map[string]ModelMetadata]`).

---

## 9. Release & CI/CD Pipeline

Nacho Flow uses a 2-stage release lifecycle in GitHub Actions (`.github/workflows/ci.yml`):

1. **Stage 1: Prerelease Build & Artifact Upload (`publish-release`)**:
   - Triggered when a Release is **published as a Pre-release** (or Draft published as Pre-release).
   - Cross-compiles Linux (`amd64`, `arm64`), macOS (`amd64`, `arm64`), and Windows (`amd64`).
   - Azure Trusted Signing signs the Windows AMD64 executable.
   - Uploads all compiled binaries and `checksums.txt` to the release assets.

2. **Stage 2: Verification & Promotion to Latest (`distribute-release`)**:
   - Test the pre-release binaries.
   - Once verified, edit the release on GitHub and **uncheck "Set as a pre-release"** (promoting to Latest Release).
   - The `distribute-release` job triggers, updating the Homebrew Tap formula and pushing WinGet package manifests to the `winget-pkgs` fork.

---

## 10. Curated Catalog Generator & Quality Standards

### 10.1 Generating Canonical Catalog (`cmd/util/gen_catalog`)

Nacho Flow includes a dedicated Go utility to fetch live models from upstream registries, classify their SWE-bench and tool reliability capabilities, and generate both the canonical repository file (`data/models.json`) and the embedded binary catalog (`pkg/telemetry/curation/models.json`):

```bash
# Generate catalog for upcoming release
go run ./cmd/util/gen_catalog -version v1.1.0

# Custom paths or flags
go run ./cmd/util/gen_catalog -version v1.1.0 -out data/models.json -embed-out pkg/telemetry/curation/models.json
```

### 10.2 Quality & Coverage Mandate: Strict TDD
* **Project Coverage Target**: **100.0% Statement Coverage**.
* **Hard Minimum**: **$\ge 95.0\%$ Statement Coverage** enforced across every package with application logic.
* **Concurrency Guarantee**: **0 data races** under `go test -race ./...`.
* **Zero Allocations on Proxy Hot-Path**: Zero heap allocations in stream parsing fast bailout paths (`BenchmarkNormalize_PureProse_FastBailout`).

---

## 11. VS Code Extension Development Workflow

The TypeScript companion extension source code is located in the `extension/` directory.

### 11.1 Project Structure
```text
extension/
├── package.json               # Extension manifest, commands, views & settings
├── esbuild.config.js          # Fast JS/CSS bundler configuration
├── jest.config.js             # Test runner configuration
├── src/
│   ├── core/                  # ProcessManager, AuthManager, API Client, SSE Client, Controller
│   ├── ui/                    # Sidebar Provider, Status Bar Widget, Webview Dashboard Panels
│   ├── utils/                 # Money and token formatters
│   └── extension.ts           # Extension entrypoint (activate / deactivate)
└── resources/                 # Icons (SVGs), Sidebar HTML/CSS/JS, Webview HTML/CSS/JS
```

### 11.2 Development Commands
```bash
# 1. Install dependencies
cd extension && npm install

# 2. Compile TypeScript & bundle assets
npm run compile
node esbuild.config.js

# 3. Run Jest Unit & Integration Test Suite
npm test

# 4. Package VSIX bundle
npx vsce package --no-dependencies --out "../dist/nacho-flow-0.6.0.vsix"

# 5. Local Install into VS Code
code --install-extension "../dist/nacho-flow-0.6.0.vsix" --force
```

### 11.3 Debugging in VS Code
- Open the root workspace in VS Code.
- Press `F5` or select **Run Extension** in the Debug menu to launch an isolated **Extension Development Host** window.

