# Change Log

All notable changes to the Nacho Flow VS Code Extension will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.3.0] - 2026-09-23

## [1.2.2] - 2026-09-19

### Added
- **Config Schema Versioning & SemVer Check**: Proactively validates user configuration profiles against extension factory templates to detect outdated schemas.
- **Factory Template Comparison & Reset**: New commands `nacho-flow.compareProfileWithTemplate` and `nacho-flow.resetProfileToDefault` allow users to inspect diffs and safely restore factory defaults.
- **Outdated Profile Sidebar Notification**: Visual alert banner and action button in the VS Code sidebar when an active profile schema version is older than the current extension version.


## [1.2.1] - 2026-09-18

### Added
- **3-Lane Stream Normalizer & In-Flight Delimiter Leak Defense (`pkg/server`, `pkg/zeroalloc`)**: SSE streaming normalizer cleanly isolates Prose, Reasoning (`<think>`), and Tool Arguments into dedicated processing lanes, stripping control tokens in-place with zero heap allocations.
- **Battle-Tested Across 46 Real-World Agent Runs**: Rigorously validated over 46 multi-turn coding agent benchmark runs with Cline and ZooCode, maintaining 100% SSE stream integrity and zero connection drops across 147 live upstream streaming requests.
- **Dynamic Config Hot-Reload**: Added live `PUT` and `PATCH` endpoints to `/api/v1/config` enabling runtime reconfiguration of provider keys, routing tiers, and NTS pipelines without daemon restarts.
- **Native Cline Agent Profile (`data/agents/cline.json`)**: Added dedicated tool definitions, write signatures, and error pattern matchers for Cline.
- **Server Decomposition**: Decomposed the monolithic ~1,890-line `pkg/server/proxy.go` into 7 focused domain files (`dispatch.go`, `pipeline.go`, `cycle_recovery.go`, `telemetry.go`, `circuit_breaker.go`, `helpers.go`, and a lean `proxy.go`).

### Changed
- **NTS Opt-In Labs Transition**: Updated default configuration across all profiles to preserve 100% pristine context (`nts.enabled: false`) to maximize 90% provider prompt-cache discounts, while retaining the zero-alloc NTS engine as an experimental opt-in option.
- **Updated Benchmark & Coverage Metrics**: Verified 30,072+ req/s peak under full auth/normalization, 1,000 workers with 100% success, and 96.6% global test coverage.

### Fixed
- **Delimiter Bleed on Tool Calls**: Prevented trailing delimiter tokens from leaking into agent tool argument strings or editor file writes.
- **Cycle Killer Position 515 Crash**: Aborted turns emit standard OpenAI-compliant SSE frames without truncated JSON, preventing client-side V8 parser crashes.

## [1.2.0] - 2026-09-15

### Added
- **🗜️ Nacho Token Saver (NTS) Live Telemetry Panel**: Live flight instruments in the webview dashboard visualizing real-time context compaction: Total Tokens Saved (1.4M+), Payload Reduced (5.4 MB+), and Compacted Turns.
- **Zero-Allocation In-Flight Compaction Core (`pkg/nts`)**:
  - In-place ANSI escape code & terminal spinner purge (`|/-\`).
  - Carriage return (`\r`) progress overwrite collapse.
  - Superseded file-read compaction collapsing stale multi-turn reads into structural notices with configurable retention depth.
  - Harness shield stripping IDE boilerplate (`<notice>`, `<error_details>` search dumps).
  - Alphanumeric-guarded whitespace and log deduplication preserving ASCII boxes and art.
- **Agent Registry & Dynamic Multi-Agent Profiles (`pkg/agentregistry`, `data/agents/`)**: First-class capability detection for Zoo Code, Cline, Cursor, Aider, Anthropic, and standard OpenAI.
- **Status Bar HUD Revamp**: Interactive Markdown hover tooltip widget displaying server URL, active profile, dollar savings, NTS tokens saved, and loop breaker statistics.
- **Reasoning Stream Normalization (`<think>`)**: Intercepts SSE streams from DeepSeek-R1, QwQ, and Anthropic, cleanly normalizing thoughts into `<think>...</think>` tags for UI accordions.
- **HotSauce In-Chat Directives**: Steer routing tiers and guardrails on-the-fly directly from prompt text (`@nacho:local`, `@nacho:cloud`, `@nacho:kickstart-off`, `@nacho:reset`).
- **Rolling 1-Hour Window**: Added `Past 1 Hour` rolling window to telemetry aggregator alongside Today, This Week, and All Time.

### Changed
- **Dashboard UI Polish**: Modernized webview layout with right-aligned status chips, responsive flex header, minimalist server version badge, and single-row Kickstart & Fairy Dust supervisor layout.
- **Cleaner Number Formatting**: Removed ambiguous `+` prefix in front of savings amounts.
- **Landing Page Architecture Symmetry**: Synchronized Section A (Active Runtime Supervisors) and Section B (Nacho Token Saver) two-tier containers on `site/` and root landing pages.
- **Developer Control Plane**: Upgraded extension showcase into a balanced 4-card 2x2 grid.

### Fixed
- **Stream-End Context Healing**: Fixed reasoning context drops on streaming `[DONE]` events.
- **Dual-Lane Immunity Guard**: Ensured supervisor system prompts and tool error responses are never stripped by compaction.

## [1.1.0] - 2026-09-11

## [1.0.3] - 2026-09-08

## [1.0.2] - 2026-09-07

## [1.0.1] - 2026-09-07

### Added
- **Strict Engine Isolation**: Enforced complete separation between local embedded daemon and remote server modes. When local engine is stopped or set to remote, background telemetry polling, health pings, and SSE streaming connections are cleanly suppressed.
- **Persistent Auto-Resume**: Local engine state is persisted across VS Code restarts via `globalState`. If running prior to reload, the local daemon resumes automatically on launch (configurable via `nachoFlow.autoStartDaemon`).
- **Mode-Switch Intent Preservation**: Switching between Local and Remote modes cleanly stops local daemon processes to free ports and GPU resources while preserving the user's local running intent when switching back.
- **Engine Mode Setting**: Exposed `nachoFlow.engineMode` (`local` | `remote`) in VS Code configuration.
- **Safe Working Directory Resolution**: Resolved daemon process execution directory from preset configuration path, workspace folder, or user home rather than VS Code application path.
- **Transparent Error Parsing**: Enhanced API client error handling to parse detailed JSON backend error messages.

### Fixed
- **File Watcher Deduplication**: Atomic timestamp tracking prevents redundant daemon configuration reloads on editor save.
- **Null Safety in Auto-Resume**: Defensive short-circuit guards in `resumeLocalEngine` prevent potential runtime dereference if `processManager` is null.
- **SSE Error Handling**: Structured `try...catch` wrapper on SSE client initialization prevents resource leakage on transient connection failures.

## [1.0.0] - 2026-09-01

### Added
- **Companion Extension Architecture**: Native TypeScript companion extension for Nacho Flow agent supervisor and model dispatcher.
- **Sidebar Control Hub**: Activity bar view providing 1-click controls for daemon lifecycle, streaming logs, and agent pairing.
- **Preset Hot-Swapping**: Zero-downtime hot-swapping between Standard, Zoo Code, and Cline routing presets via in-memory RCU updates.
- **Real-Time Analytics Dashboard**: Flight instruments webview tracking spend, counterfactual savings, local vs. cloud turns, and live route history.
- **Heat Seeker Model Deals**: Continuous upstream discount oracle scanning 300+ models on OpenRouter with 1-click target tier adoption.
- **1-Click Auto-Tuner**: Historical traffic log analysis recommending optimal local context token limits.
- **Status Bar HUD**: Real-time status bar widget showing today's savings, local turn percentage, and rich hover card.
- **In-Chat Directives**: Full support for `@nacho:` prompt-based routing overrides and session guardrail toggles.

## [0.6.0] - 2026-08-23

### Added
- Initial release of Nacho Flow VS Code Extension
- Status bar item showing real-time cost savings and routing statistics
- Basic REST API client for communicating with Nacho Flow daemon
- SSE client for real-time event streaming
- Authentication token management using VS Code SecretStorage
- Core extension architecture and command registration