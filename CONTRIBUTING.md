# 🤝 Contributing to Nacho Flow

Thank you for your interest in contributing to **Nacho Flow**! We welcome bug reports, feature suggestions, documentation improvements, and pull requests.

---

## 🛠️ Development Workflow

1. **Fork and Clone** the repository:
   ```bash
   git clone https://github.com/dixieflatline76/nacho-flow.git
   cd nacho-flow
   ```

2. **Create a Feature Branch**:
   ```bash
   git checkout -b feature/my-cool-feature
   ```

3. **Make Changes Following Test-Driven Development (TDD)**:
   - Write failing tests first before writing implementation code.
   - Keep packages decoupled and adhere to clean Go patterns.

4. **Verify Quality, Security & Race Tests**:
   Run the all-in-one quality gate:
   ```bash
   make check
   # Runs: gofmt, go vet, gosec AST analysis, and race-detected unit tests
   ```

5. **Commit and Push**:
   ```bash
   git commit -m "feat(telemetry): add support for custom provider pricing"
   git push origin feature/my-cool-feature
   ```

6. **Open a Pull Request**: Submit your PR with a clear summary of changes and test verification results.

---

## 📋 Coding Standards & Guidelines

- **Zero Lock Contention on Hot Paths**: Hot request paths must avoid global mutexes. Use `atomic.Pointer` for read-heavy lookup maps (RCU pattern) and buffered channels for asynchronous event loops.
- **Structured Logging (`slog`)**: Always use `log/slog` with structured key-value pairs (e.g. `slog.String("tier", tier)`). Never use unformatted `fmt.Println` or global `log.Printf` inside packages.
- **Standard Library First**: Avoid pulling in heavy external dependencies unless strictly necessary.
- **Cross-Platform Compatibility**: Code must compile and run cleanly across Linux, macOS, and Windows (`CGO_ENABLED=0`).

---

## 💬 Community & Support

- **Website**: [spicerack.dev](https://spicerack.dev)
- **Issues**: [GitHub Issues](https://github.com/dixieflatline76/nacho-flow/issues)
- **Discussions**: [GitHub Discussions](https://github.com/dixieflatline76/nacho-flow/discussions)
