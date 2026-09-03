# RFC-002: Kickstart Auto-Suspension for Exploration/Plan Modes & `@nacho:no-kick` Directive

* **Status**: Proposed / Targeted for v0.9.2
* **Target Release**: v0.9.2 (or Next Minor Patch)
* **Author**: dixieflatline76 / Nacho Flow Team
* **Topic**: Eliminating False-Positive Kickstart Jolts During Bug Investigation, Architecture Planning, and Exploration Phases

---

## 1. Executive Summary & Problem Statement

Nacho Flow's **Kickstart** defense (`kickstart_write_only: true`, default threshold: 5 turns) monitors consecutive session turns to detect idle churn and semantic stalls (e.g., an agent trapped in a loop reading files or checking status without producing concrete code). When triggered, it injects an authoritative `[SYSTEM OVERRIDE]` prompt ordering the agent to stop meta-work and edit files or run commands immediately.

### The Friction Scenario
When a developer initiates an intentional exploration, debugging, or planning task:
> *"Investigate the authentication token flow across the codebase and draft a refactoring architecture."*

Or when the developer switches their coding harness into **Plan Mode** or **Architect Mode** (supported natively by Cline, Roo Code, etc.):
1. The agent legitimately spends 5+ turns reading source code files (`auth.go`, `token.go`, `db.go`, `router.go`), searching code via ripgrep, and gathering context.
2. Because these turns do not modify files (`HasWriteProgress == false`) and do not read test files or compiler error logs (`HasTestProgress == false`), the daemon counts every exploration turn as an "idle stall".
3. **On Turn 5, Kickstart fires**:
   ```text
   [SYSTEM OVERRIDE] You have not produced any file writes or terminal commands in multiple 
   consecutive turns. Stop meta-work. Execute a concrete action: write a file or run a command NOW.
   ```
4. The agent—which was explicitly instructed by the developer to *only explore and plan*—is suddenly hijacked and commanded by the router to write code immediately, aborting the investigation.

This RFC proposes a two-layer solution:
1. **Layer 1 (Automatic Zero-Config Inference)**: Automatically suspend `kickstart_write_only` when the client's payload schema contains **zero file-write/execution tools** (e.g. Cline Plan Mode).
2. **Layer 2 (Explicit Developer Control)**: Introduce the plain-English, zero-confusion HotSauce in-prompt directive: **`@nacho:no-kick`** (and alias `@nacho:disable-kick`).

---

## 2. Architecture & Design Specification

```text
                                  INCOMING INFERENCE REQUEST
                                              │
                                              ▼
                        ┌───────────────────────────────────────────┐
                        │       pkg/router/classifier.go:           │
                        │    RequestClassifier.Classify(body)       │
                        └─────────────────────┬─────────────────────┘
                                              │
                   ┌──────────────────────────┴──────────────────────────┐
                   ▼                                                     ▼
      [ Check Available Tools ]                             [ Scan In-Prompt Directives ]
  Inspect payload `tools` schema:                         Check user prompt for HotSauce tag:
  Does `tools` contain ANY write tools?                   Is `@nacho:no-kick` or
  (write_to_file, replace_in_file, etc.)                  `@nacho:disable-kick` present?
                   │                                                     │
        ┌──────────┴──────────┐                               ┌──────────┴──────────┐
        │                     │                               │                     │
      ZERO                  >= 1                           PRESENT               ABSENT
   Write Tools           Write Tools                          │                     │
        │                     │                               ▼                     │
        ▼                     │                   reqCtx.NoKickstart = true         │
reqCtx.HasWriteCapability     │                                                     │
        = false               ▼                                                     ▼
        │             reqCtx.HasWriteCapability                               Standard
        │                     = true                                         Evaluation
        └─────────────────────┬─────────────────────────────────────────────────────┘
                              │
                              ▼
                ┌───────────────────────────┐
                │   pkg/server/proxy.go:    │
                │  Kickstart Evaluation     │
                └─────────────┬─────────────┘
                              │
              ┌───────────────┴───────────────┐
              ▼                               ▼
    Should Kickstart Evaluate?       kickstartProgress =
    if reqCtx.NoKickstart ||         reqCtx.HasWriteProgress ||
       !reqCtx.HasWriteCapability    reqCtx.HasTestProgress
    ➔ BYPASS KICKSTART (0 false jolts)
```

---

## 3. Detailed Technical Requirements

### 3.1 Layer 1: Automatic Available Tool Inspection (Zero-Config)

#### Why This Works
When clients like Cline, Roo Code, or OpenHands enter **Plan Mode**, they explicitly strip modification tools from the OpenAI-compatible `tools` array. The model is only provided read-only tools:
- `read_file`
- `list_dir`
- `grep_search`
- `ask_followup_question`

If the model literally cannot write files because the harness did not grant it write tools, commanding it to write files via `[SYSTEM OVERRIDE]` is a guaranteed failure.

#### Implementation in `pkg/router/classifier.go`
1. During `Classify()`, inspect the top-level `tools` array.
2. Compare each tool name against `GetKickstartWriteTools()` (the pre-configured write tool registry):
   ```go
   hasWriteCapability := false
   if tools, ok := raw["tools"].([]interface{}); ok && len(tools) > 0 {
       writeLookup := c.GetKickstartWriteTools()
       for _, t := range tools {
           if tMap, ok := t.(map[string]interface{}); ok {
               fnName := extractToolName(tMap)
               if writeLookup[strings.ToLower(strings.TrimSpace(fnName))] {
                   hasWriteCapability = true
                   break
               }
           }
       }
   }
   reqCtx.HasWriteCapability = hasWriteCapability
   ```
3. In `pkg/server/proxy.go`:
   ```go
   // If the agent does not possess write tools in this turn (e.g. Plan Mode),
   // auto-suspend kickstart_write_only to prevent false stall alarms.
   if cfg.CycleKiller.KickstartWriteOnly && !reqCtx.HasWriteCapability && reqCtx.HasTools {
       // Do not increment KickstartCount; maintain current session state
       kickstartProgress = true
   }
   ```

---

## 3.2 Layer 2: Explicit HotSauce In-Prompt Directive (`@nacho:no-kick`)

#### The Problem with `@nacho:no-shield`
The legacy directive `@nacho:no-shield` was confusing and opaque to users. It turned off the Agentic Tool Shield (interactive fallback synthesis), but users assumed it disabled Kickstart or other routing protections.

#### The New Directive: `@nacho:no-kick` (and alias `@nacho:disable-kick`)
When a developer is in standard edit mode (where write tools are available) but wants to run an open-ended multi-turn investigation without the 5-turn kickstart countdown:
```text
@nacho:no-kick Analyze our memory leak in pkg/server and explain the root cause.
```

#### Directive Syntax Specification
* Canonical Directive: **`@nacho:no-kick`**
* Recognized Aliases: **`@nacho:disable-kick`**, **`@nacho:nokick`**, **`@nacho:no_kick`**
* Zero Performance Cost: Parsed in the existing zero-allocation directive scanner (`pkg/router/directive.go`).
* Clean Prompt Sanitization: The directive is automatically stripped before sending the request to the upstream LLM, leaving clean user text.

#### Implementation in `pkg/router/directive.go`
```go
case "no-kick", "nokick", "no_kick", "disable-kick", "disable_kick":
    info.Directive = "no-kick"
```
In `pkg/contract/interfaces.go`:
```go
type RequestContext struct {
    ...
    NoKickstart bool `json:"no_kickstart,omitempty"`
}
```
In `pkg/server/proxy.go`:
```go
if reqCtx.NoKickstart {
    // Developer explicitly disabled kickstart for this exploration turn
    kickstartProgress = true
}
```

---

## 4. Why Alternative 1.2 (System Prompt Parsing) is Rejected

Alternative 1.2 proposed scanning incoming system prompts for phrases like `"You are in PLAN MODE"`. This is **explicitly rejected** because:
1. **Brittle & Client-Coupled**: Different clients phrase plan modes differently (`PLAN MODE`, `ARCHITECT MODE`, `RESEARCH ONLY`, `READ ONLY`).
2. **Language/Localization Fragility**: Translated prompts or custom user instructions break string matching.
3. **Performance Penalty**: Scanning large 10k+ character system prompts with regex on every turn burns unnecessary CPU cycles.
4. **Tool Schema Inspection (Layer 1) is 100% Deterministic**: If the agent has no write tools, it cannot write. That is a physical, unforgeable constraint.

---

## 5. Ergonomics & User Guide Updates

The documentation and website will be updated to document this capability cleanly:

### Cheat Sheet: Exploration vs Execution
| Scenario | Behavior | Control |
| :--- | :--- | :--- |
| **Normal Coding Session** | Kickstart fires after 5 turns without code edits | Automatic |
| **Cline / Roo "Plan Mode"** | Kickstart **auto-suspends** (0 write tools in schema) | **100% Automatic (Zero Config)** |
| **Investigation with Edit Mode active** | Developer adds directive to prompt | **`@nacho:no-kick`** in chat |
| **Activity Bar Profile** | Switch to a dedicated research profile | Preset hot-swap |

---

## 6. Verification & Test Plan

1. **Unit Tests (`pkg/router/classifier_test.go`)**:
   - Verify `HasWriteCapability == false` when payload `tools` only contains `read_file` and `list_dir`.
   - Verify `HasWriteCapability == true` when payload `tools` includes `write_to_file` or `execute_command`.
   - Verify `@nacho:no-kick` sets `reqCtx.NoKickstart = true` and strips clean prompt.
   - Verify `@nacho:disable-kick` alias resolves identically.
2. **Integration Tests (`pkg/server/proxy_test.go`)**:
   - Simulate 10 consecutive read-only turns with `tools: [read_file]` and `kickstart_write_only: true`. Verify `reqCtx.SessionKickstarted` remains `false`.
   - Simulate 10 consecutive read-only turns with `tools: [read_file, write_to_file]`. Verify `reqCtx.SessionKickstarted` turns `true` on turn 5.
   - Re-run with `@nacho:no-kick` directive present. Verify Kickstart is suppressed on turn 5.
