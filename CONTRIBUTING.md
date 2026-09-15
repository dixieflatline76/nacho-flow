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

## 🤖 Contributing Coding Agent Profiles (`data/agents/*.json`)

Nacho Flow ships with built-in canonical profiles for leading autonomous coding agents (**Zoo Code**, **Cline**, **Cursor**, **Windsurf**, **Claude Code**, **Aider**, **Continue**, **OpenCode**, and **Goose**).

We actively welcome community contributions to keep this catalog comprehensive and resilient:

1. **Submitting Error Signatures**:
   If your agent encounters an unhandled tool validation error, Zod schema failure, or search/replace mismatch that caused it to loop without auto-escalating, grab the error substring from your agent console or `logs/traffic.jsonl` and add it to `error_signatures` in the agent's JSON spec.
2. **Adding Missing Write & Command Tools**:
   If your agent adds a new tool name for modifying code or executing commands, add it to `write_tools` or `command_tools` so Kickstart idle resuscitation and Fairy Dusting track state modifications accurately.
3. **Contributing New Agent Profiles**:
   Create a new specification `data/agents/<agent-id>.json` following the schema in `data/agents/zoo.json` and submit a Pull Request!
4. **Sharing Logs & Edge Cases**:
   Feel free to share sanitized turn traces or raw tool outputs from `logs/traffic.jsonl` in [GitHub Discussions](https://github.com/dixieflatline76/nacho-flow/discussions) to help us refine delimiter stripping and question heuristics.

---

## 💬 Community & Support

- **Website**: [spicebox.dev/nacho-flow](https://spicebox.dev/nacho-flow/)
- **General Inquiries & Security**: [info@spicebox.dev](mailto:info@spicebox.dev)
- **Technical Support**: [support@spicebox.dev](mailto:support@spicebox.dev)
- **Issues**: [GitHub Issues](https://github.com/dixieflatline76/nacho-flow/issues)
- **Discussions**: [GitHub Discussions](https://github.com/dixieflatline76/nacho-flow/discussions)
