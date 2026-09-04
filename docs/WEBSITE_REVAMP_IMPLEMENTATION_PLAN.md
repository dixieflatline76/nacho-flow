# Master Implementation Plan: Website Hierarchy, Human-Level Framing & Fleet Governance

Restructure the Nacho Flow landing page (`site/index.html` and root `index.html`) and stylesheets (`site/index.css` and root `index.css`) using human-first developer messaging and grounded systems architecture:

1. **The Hero Section**: Direct, empathetic developer hook — stopping credit card burns, free local GPU runs, and only spending on Claude when things get hard.
2. **The Bridge: The Autonomous Agent Trap**: Why open-weights and cheap models fail outside of expensive proprietary silos without active guardrails.
3. **Section 1: The Three Fixes (Mechanism + Superpowers)**:
   - Centered native 16:9 widescreen video (`site/images/cycle-killer-demo.mp4`) positioned in a sleek terminal bezel directly below the section header and above the 3 boxes.
   - 🎸 **Cycle Killer `[In-Flight Stream Breaker]`**: Real-time N-gram loop termination ($W=6, T=3$) and runaway prose murder in $<3$s with zero session crash.
   - ⚡ **Kickstart `[Stall Resuscitation Engine]`**: Extensible schema detection (`HasWriteCapability`) auto-suspending in Plan Mode, single-turn frontier unsticking.
   - 🧚 **Fairy Dust `[Programmable Milestone Checkpoints]`**: Programmable intervention harness catching silent `// TODO` stubs, logic deadlocks, and spec drift.
4. **Section 2: The Cost Engine & Multi-Target Topology**:
   - Explicit multi-target topology: Orchestrating Ollama, vLLM, direct Claude subscriptions, OpenRouter, and Langdock behind `localhost:8000`.
   - 2x2 Grid: True Hybrid Routing, Heat Seeker Spot Deals, Prompt Cache-Aware Cost Engine, and Auto-Tuner CLI (`nacho-flow tune`).
5. **Section 3: Developer Control Plane**:
   - All-in-One VS Code Companion Extension (bundles native Go binary directly, zero Go installation or CLI setup required).
   - In-Band HotSauce Directives (`@nacho:local`, `@nacho:kickstart-off`, `@nacho:toggles`, `@nacho:reset`).
6. **Section 4: Scaling to Engineering Teams: Fleet Governance Without the Drag**:
   - Team-Wide Runaway Circuit Breakers (preventing $500 overnight burn on unmonitored test loops).
   - Private GPU Fleet Offloading (on-prem vLLM/Ollama clusters keeping proprietary code off public APIs).
   - Squad Spend Caps & Quotas (Core, Frontend, Data budgets without locking engineers out of favorite IDE agents).
7. **Architectural Trust Bar & Quickstart**:
   - Clean systems performance specs replacing confusing plumbing cards.
   - 10-second Quickstart across VS Code, Winget, Homebrew, and cURL.
8. **Roo Code Complete Purge**:
   - Purge all references to "Roo Code" across site, docs, and extension metadata. Active stack is strictly **Cline, Zoo Code, OpenCode, and Aider**.

---

## User Review Required

> [!IMPORTANT]
> **Locked-In Copy, Positioning & Structure**:
>
> 1. **The Hero Copy**:
>    > **Don't give up on local models and open-source coding agents just yet.**  
>    > Stop letting tools like Cline and Zoo Code burn your credit card on infinite loops. Nacho Flow runs the simple stuff on your own GPU for free, kicks your agent when it gets stuck reading files, and only spends money on Claude when things get hard.
>    - **CTAs**: `[ 🚀 Quick Install ]` `[ 🧩 VS Code Extension ]` `[ 📖 Documentation ]` `[ ⭐ Star on GitHub ]`
>    - **Nav Badge**: Bumped to **`v0.9.1`**
>    - **Purge**: Zero mentions of Roo Code. Active agent stack: Cline, Zoo Code, OpenCode, Aider.
>
> 2. **The Bridge: The Autonomous Agent Trap**:
>    > *"Outside of expensive proprietary silos, running autonomous agents on open-weight or non-frontier models is an exercise in rage-quitting:*  
>    > *• Models loop on the exact same failed edit 12 times in a row.*  
>    > *• Agents spin in 10-turn 'read-only planning loops' without writing a single line of code.*  
>    > *• Malformed tool calls cause 3-strike crashes that abort your entire session.*  
>    > *• The Token Snowball: context balloons 50k tokens per turn, paying $15 for broken code.*  
>    >  
>    > *Nacho Flow wraps local models in active runtime guardrails — giving open-weights models the discipline, reliability, and precision of frontier reasoning agents."*
>
> 3. **Section 1: The Three Fixes (Mechanism + Superpowers)**:
>    - **Layout**: Centered native 16:9 widescreen video player (`site/images/cycle-killer-demo.mp4`) in a terminal-style bezel directly below the section title, followed by 3 equal-width boxes.
>    - **🎸 Cycle Killer `[In-Flight Stream Breaker]`**:  
>      *Kills runaway philosophical rants that waste tokens (your money).*  
>      Monitors the live token stream in real time. Terminates repetitive N-gram loops ($W=6, T=3$) and runaway prose in $<3$s, cleanly self-healing the session at $0.00 with a steer prompt instead of throwing blunt HTTP 500 errors that crash your agent session.
>    - **⚡ Kickstart `[Stall Resuscitation Engine]`**:  
>      *Tells the AI to continue when the job is not yet done.*  
>      Monitors consecutive non-write turns. Auto-suspends during exploration via **extensible schema detection** (`HasWriteCapability`), and jolts agents out of passive read/plan procrastination—bursting to a frontier model for a single turn to unstick execution, then handing control straight back to your local GPU.
>    - **🧚 Fairy Dust `[Programmable Milestone Checkpoints]`**:  
>      *Intelligently sprinkles in help from the big-gun models you can't run all day.*  
>      A programmable intervention harness. You define the cadence (every $N$ writes), audit prompt, target model, and spend cap—deploying frontier reasoning models precisely when and where quality verification matters to catch silent `// TODO` stubs, logic deadlocks, and spec drift before errors compound.
>
> 4. **Section 2: The Cost Engine & Multi-Target Topology**:
>    - Card 1 Headline & Topology: *"Orchestrate multiple local nodes (Ollama, vLLM), direct Claude subscriptions, and upstream aggregators (OpenRouter, Langdock) behind a single OpenAI-compatible `localhost:8000` endpoint."*
>    - 2x2 Grid: True Hybrid Local GPU + Cloud Routing, Heat Seeker Spot Deals, Prompt Cache-Aware Cost Engine, Curated Intelligence & Auto-Tuner CLI.
>
> 5. **Section 3: Developer Control Plane**:
>    - All-in-One VS Code Extension: Bundles and supervises native Go binary directly with 0 manual Go toolchain or CLI setup.
>    - HotSauce Directives: In-chat `@nacho:` steering stripped cleanly before dispatch.
>
> 6. **Section 4: Scaling to Engineering Teams: Fleet Governance Without the Drag**:
>    - Dedicated enterprise / team adoption module right below Developer Control Plane:
>      - Team-Wide Runaway Circuit Breakers
>      - Private GPU Fleet Offloading
>      - Squad Spend Caps & Quotas
>
> 7. **Architectural Trust Bar**:
>    - `⚡ < 0.18ms Go Core Latency` | `🚀 31,424+ Req/Sec P99` | `🛠️ Universal Tool Normalization` | `🛡️ Zero-Crash Stream Healing` | `📏 Context Boundary Guards` | `📐 O(1) Physical Limit Guards`

---

## Complete Page ASCII Wireframe

```text
====================================================================================================
[NAVBAR]   🌮 Nacho Flow [v0.9.1]             Home   Documentation ▾   Roadmap   Support   [★ GitHub]
====================================================================================================

                                    [ HERO: THE WALLET HOOK ]

                     [ Open Source  •  Zero Dependencies  •  Written in Go ]
                                     [ Level 1-1 Hero Art ]

             "Don't give up on local models and open-source coding agents just yet."
    Stop letting tools like Cline and Zoo Code burn your credit card on infinite loops.
    Nacho Flow runs the simple stuff on your own GPU for free, kicks your agent when it gets
    stuck reading files, and only spends money on Claude when things get hard.

        [ 🚀 Quick Install ]   [ 🧩 VS Code Extension ]   [ 📖 Documentation ]   [ ⭐ Star on GitHub ]

----------------------------------------------------------------------------------------------------
                            [ THE BRIDGE: THE AUTONOMOUS AGENT TRAP ]
                     Why open & cheap models fail outside of $200/mo setups
----------------------------------------------------------------------------------------------------

 "Outside of expensive proprietary silos, running autonomous agents on open-weight or non-frontier
  models is an exercise in rage-quitting:
  • Models loop on the exact same failed edit 12 times in a row.
  • Agents spin in 10-turn 'read-only planning loops' without writing a single line of code.
  • Malformed tool calls cause 3-strike crashes that abort your entire session.
  • The Token Snowball: context balloons 50k tokens per turn, paying $15 for broken code.

  Nacho Flow wraps local models in active runtime guardrails — giving open-weights models the
  discipline, reliability, and precision of frontier reasoning agents."

----------------------------------------------------------------------------------------------------
                                [ REAL-WORLD SESSION ECONOMICS ]
                     $14.80 Direct Cloud vs $0.78 Hybrid Routing (94.7% Saved)
----------------------------------------------------------------------------------------------------

----------------------------------------------------------------------------------------------------
                                [ SECTION 1: THE THREE FIXES ]
                            Why Your Autonomous Agent Finally Works
----------------------------------------------------------------------------------------------------

          [ 📺 NATIVE 16:9 WIDESCREEN VIDEO: SEE CYCLE KILLER IN ACTION (1280 x 720) ]
        Width: 900px (matching Terminal) • Native 16:9 Aspect Ratio • Autoplay/Loop Muted
  +--------------------------------------------------------------------------------------------+
  | ● ● ●  cycle-killer-demo.mp4                                                               |
  |--------------------------------------------------------------------------------------------|
  |                 [ 1280 x 720 NATIVE WIDESCREEN VIDEO: CYCLE KILLER ]                       |
  |  Server Room ➔ N-Gram Loop x 3 ➔ Laser [SYSTEM OVERRIDE] ➔ Stream Healed 2,340 tokens saved|
  +--------------------------------------------------------------------------------------------+

  +-------------------------------+ +-------------------------------+ +-------------------------------+
  | 🎸 Cycle Killer               | | ⚡ Kickstart                  | | 🧚 Fairy Dust                 |
  | [In-Flight Stream Breaker]    | | [Stall Resuscitation Engine]  | | [Programmable Checkpoints]    |
  | Kills runaway rants that waste| | Tells the AI to continue when | | Sprinkles in help from the big|
  | tokens (your money).          | | the job is not yet done.      | | guns you can't run all day.   |
  |-------------------------------| |-------------------------------| |-------------------------------|
  | Monitors live token stream in | | Monitors consecutive non-write| | A programmable intervention   |
  | real time. Terminates loops   | | turns. Auto-suspends during   | | harness. You define cadence   |
  | (W=6, T=3) and runaway prose  | | exploration via extensible    | | (every N writes), prompt,     |
  | in <3s, cleanly self-healing  | | schema detection (HasWrite-   | | target model, and spend cap — |
  | the session at $0.00 without  | | Capability), and jolts agents | | deploying frontier reasoning  |
  | blunt HTTP 500 crashes.       | | into action, bursting to cloud| | to catch silent // TODO stubs|
  |                               | | for 1 turn when needed.       | | and deadlocks before cascade. |
  +-------------------------------+ +-------------------------------+ +-------------------------------+

----------------------------------------------------------------------------------------------------
                      [ SECTION 2: THE COST ENGINE & MULTI-TARGET TOPOLOGY ]
                       70%–92% Fleet Savings Backed by Verified Mathematics
----------------------------------------------------------------------------------------------------

  +--------------------------------------------+  +--------------------------------------------+
  | 🖥️☁️ True Hybrid Local GPU + Cloud Routing  |  | 🔥 Heat Seeker Spot Deal Scout             |
  | Multi-Target Topology ($0.00 + Cloud Burst)|  | Autonomous Market Price Drop Discovery     |
  |--------------------------------------------|  |--------------------------------------------|
  | Orchestrate multiple local nodes (Ollama,  |  | Scans 300+ models in the background for    |
  | vLLM), direct Claude subscriptions, and    |  | underpriced capacity and flash subsidies.  |
  | upstream aggregators (OpenRouter, Langdock)|  | Automatically maps live deals into your    |
  | behind a single OpenAI-compatible          |  | active tier roles while you sleep.         |
  | localhost:8000 endpoint.                   |  |                                            |
  +--------------------------------------------+  +--------------------------------------------+
  +--------------------------------------------+  +--------------------------------------------+
  | 💰 Prompt Cache-Aware Cost Engine          |  | 🎛️ Curated Intelligence & Auto-Tuner CLI  |
  | Dollar-Accurate Accounting That Matches    |  | OTA Model Semver Updates + Grid Search     |
  |--------------------------------------------|  |--------------------------------------------|
  | Ingests upstream provider prompt cache     |  | Embedded vetted model gallery with auto OTA|
  | discounts (~80% off prompt tokens). Real   |  | GitHub updates. Run `nacho-flow tune` to   |
  | mathematical cost tracking ensures your    |  | grid-search your session logs and optimize |
  | dashboard matches your actual invoice.     |  | routing rules without touching config files|
  +--------------------------------------------+  +--------------------------------------------+

----------------------------------------------------------------------------------------------------
                              [ SECTION 3: DEVELOPER CONTROL PLANE ]
                             Ergonomics & Total Control Inside Your IDE
----------------------------------------------------------------------------------------------------

  +--------------------------------------------------------------------------------------------+
  | 🧩 All-in-One VS Code Companion Extension (Carries the Engine Inside)                       |
  | No Go Toolchain or Manual CLI Setup Required                                               |
  |--------------------------------------------------------------------------------------------|
  | • The extension bundles and launches the native Go routing binary directly.                |
  | • Sidebar Control Hub: 1-click start/stop, live daemon logs, and instant preset hot-swap   |
  |   (Standard / Zoo Code / Cline) with 0-downtime synchronization.                           |
  | • Real-Time Dashboard: Zero-polling SSE live metrics, route inspector, and defense chips. |
  +--------------------------------------------------------------------------------------------+
  +--------------------------------------------------------------------------------------------+
  | 🌶️ HotSauce Directives & In-Prompt Stream Bypass (`@nacho:`)                               |
  | Complete Control Without Leaving Your Editor Chat                                          |
  |--------------------------------------------------------------------------------------------|
  | Override routing right in the chat turn with zero token latency:                           |
  | • `@nacho:local` / `@nacho:cloud` / `@nacho:tier="..."` — Force execution tier.            |
  | • `@nacho:kickstart-off` / `@nacho:kickstart-on` — Mid-flight guardrail toggles.           |
  | • `@nacho:toggles` / `@nacho:reset` / `@nacho:deals` — Inspect & manage session in chat.   |
  | • `@nacho:raw` / `@nacho:shield-off` — 100% bit-for-bit raw upstream stream for batch.     |
  +--------------------------------------------------------------------------------------------+

----------------------------------------------------------------------------------------------------
             [ SECTION 4: SCALING TO ENGINEERING TEAMS: FLEET GOVERNANCE WITHOUT THE DRAG ]
----------------------------------------------------------------------------------------------------

  +------------------------------------+ +------------------------------------+ +------------------------------------+
  | 🛡️ Team Runaway Circuit Breakers    | | 🔒 Private GPU Fleet Offloading    | | 📊 Squad Spend Caps & Quotas       |
  | Hard spend limits and automated    | | Route routine codebase indexing and| | Allocate developer AI budgets by   |
  | killswitches prevent developers    | | file inspections to internal       | | squad (Core, Frontend, Data)       |
  | from burning $500 overnight when an| | on-prem vLLM/Ollama clusters,      | | without locking engineers out of   |
  | agent gets caught in an            | | keeping proprietary source code    | | their favorite IDE agents.         |
  | unmonitored test-debug loop.       | | off public APIs.                   | |                                    |
  +------------------------------------+ +------------------------------------+ +------------------------------------+

----------------------------------------------------------------------------------------------------
                         [ ARCHITECTURAL SPECIFICATION: UNDER THE HOOD ]
               Replacing confusing plumbing cards with rock-solid Trust Specifications
----------------------------------------------------------------------------------------------------
  ⚡ < 0.18ms Go Core Latency  |  🚀 31,424+ Req/Sec P99  |  🛠️ Universal Tool Normalization
  🛡️ Zero-Crash Stream Healing |  📏 Context Boundary Guards |  📐 O(1) Physical Limit Guards
----------------------------------------------------------------------------------------------------

                               [ LIVE TERMINAL & DASHBOARD SHOWCASE ]
                          (Interactive Terminal Stream & Extension Screenshot)

----------------------------------------------------------------------------------------------------
                                [ 10-SECOND QUICKSTART / INSTALL ]
                     [ VS Code Extension ]   [ Winget ]   [ Homebrew ]   [ Curl ]
----------------------------------------------------------------------------------------------------
```

---

## Proposed Changes

### Assets & Video

#### [NEW] [cycle-killer-demo.mp4](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/site/images/cycle-killer-demo.mp4)
- Copy `C:\Users\karlk\Downloads\can_you_make_this_video_longer.mp4` to `site/images/cycle-killer-demo.mp4`.
- Exact specifications: $1280 \times 720$, 16:9 widescreen, 8.9 MB, H.264/AAC.

---

### Landing Page Markup (`site/index.html` and `index.html`)

#### [MODIFY] [site/index.html](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/site/index.html)
#### [MODIFY] [index.html](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/index.html)

1. **Navigation Bar**:
   - Update version badge: `<span class="logo-badge logo-badge-version" id="version-badge">v0.9.1</span>`.
   - Purge any lingering Roo Code references in dropdown documentation descriptions.

2. **Hero Section (The Wallet Hook)**:
   - Replace lines 118-123 with:
   ```html
   <h1>Don't give up on local models and open-source coding agents just yet.</h1>
   <p class="subtitle">Stop letting tools like Cline and Zoo Code burn your credit card on infinite loops.</p>
   <p class="explainer">
       Nacho Flow runs the simple stuff on your own GPU for free, kicks your agent when it gets stuck reading files, and only spends money on Claude when things get hard.
   </p>
   ```
   - Keep the 4 CTA buttons: `[ 🚀 Quick Install ]`, `[ 🧩 VS Code Extension ]`, `[ 📖 Documentation ]`, `[ ⭐ Star on GitHub ]`.

3. **The Bridge: The Autonomous Agent Trap**:
   - Replace lines 142-165 (`.story-section`) with:
   ```html
   <section class="story-section agent-trap-section">
       <div class="agent-trap-container glass">
           <div class="agent-trap-badge">THE REALITY GAP</div>
           <h2 class="section-title">The Autonomous Agent Trap</h2>
           <p class="section-subtitle">Why open-weight and cheap models fail outside of $200/mo proprietary silos.</p>
           
           <div class="agent-trap-body">
               <p class="agent-trap-intro">Outside of expensive proprietary silos, running autonomous agents on open-weight or non-frontier models is an exercise in rage-quitting:</p>
               <ul class="agent-trap-list">
                   <li><span class="trap-bullet">🔄</span> <strong>Infinite Repetition:</strong> Models loop on the exact same failed edit 12 times in a row without making progress.</li>
                   <li><span class="trap-bullet">⏳</span> <strong>Analysis Paralysis:</strong> Agents spin in 10-turn "read-only planning loops" without writing a single line of code.</li>
                   <li><span class="trap-bullet">💥</span> <strong>Schema Breakage:</strong> Malformed tool calls and hallucinated parameters trigger 3-strike crashes that abort your session.</li>
                   <li><span class="trap-bullet">💸</span> <strong>The Token Snowball:</strong> Context balloons by 50k tokens per turn, leaving you paying $15 for broken code.</li>
               </ul>
               <div class="agent-trap-punchline">
                   <strong>Nacho Flow wraps local models in active runtime guardrails</strong> — giving open-weights models the discipline, reliability, and precision of frontier reasoning agents.
               </div>
           </div>
       </div>
   </section>
   ```

4. **Preserve Real-World Session Economics**:
   - Keep the side-by-side comparison ($14.80 Direct Cloud vs $0.78 Hybrid Routing, 94.7% Saved) with updated text referencing **Zoo Code / Cline / OpenCode / Aider**.

5. **Section 1: The Three Fixes**:
   - Replace lines 328-456 (the uncurated 20-card grid) with 4 structured sections:
   ```html
   <section class="three-fixes-section" id="mechanisms">
       <h2 class="section-title">Why Your Autonomous Agent Finally Works</h2>
       <p class="section-subtitle">Three active runtime supervisors that eliminate the catastrophic failure modes of local models.</p>

       <!-- Centered 16:9 Widescreen Video Terminal Frame -->
       <div class="cycle-killer-video-container">
           <div class="terminal-window">
               <div class="terminal-header">
                   <div class="terminal-dots">
                       <span class="dot-red"></span>
                       <span class="dot-yellow"></span>
                       <span class="dot-green"></span>
                   </div>
                   <div class="terminal-title">🎸 cycle-killer-demo.mp4 — In-Flight Stream Breaker (< 3s Loop Termination)</div>
                   <div class="terminal-tag" style="color: #10b981; font-weight: 600; font-size: 0.75rem;">● 1280x720 NATIVE 16:9</div>
               </div>
               <div class="video-wrapper">
                   <video id="cycle-killer-video" autoplay loop muted playsinline preload="auto">
                       <source src="images/cycle-killer-demo.mp4" type="video/mp4">
                       Your browser does not support the video tag.
                   </video>
               </div>
           </div>
       </div>

       <!-- The Three Fixes Mechanism Cards -->
       <div class="three-fixes-grid">
           <div class="fix-card glass">
               <div class="fix-badge badge-killer">🎸 IN-FLIGHT STREAM BREAKER</div>
               <h3>Cycle Killer</h3>
               <p class="fix-superpower">Kills runaway philosophical rants that waste tokens (your money).</p>
               <p class="fix-mechanism">
                   Monitors the live token stream in real time. Terminates repetitive N-gram loops (<em>W=6, T=3</em>) and runaway prose in &lt;3s, cleanly self-healing the session at $0.00 with a steer prompt instead of throwing blunt HTTP 500 errors that crash your agent session.
               </p>
           </div>

           <div class="fix-card glass">
               <div class="fix-badge badge-kickstart">⚡ STALL RESUSCITATION ENGINE</div>
               <h3>Kickstart</h3>
               <p class="fix-superpower">Tells the AI to continue when the job is not yet done.</p>
               <p class="fix-mechanism">
                   Monitors consecutive non-write turns. Auto-suspends during exploration via <strong>extensible schema detection</strong> (<code>HasWriteCapability</code>), and jolts agents out of passive read/plan procrastination—bursting to a frontier model for a single turn to unstick execution, then handing control straight back to your local GPU.
               </p>
           </div>

           <div class="fix-card glass">
               <div class="fix-badge badge-fairydust">🧚 PROGRAMMABLE CHECKPOINTS</div>
               <h3>Fairy Dust</h3>
               <p class="fix-superpower">Intelligently sprinkles in help from the big-gun models you can't run all day.</p>
               <p class="fix-mechanism">
                   A programmable intervention harness. You define the cadence (every <em>N</em> writes), audit prompt, target model, and spend cap—deploying frontier reasoning models precisely when and where quality verification matters to catch silent <code>// TODO</code> stubs, logic deadlocks, and spec drift before errors compound.
               </p>
           </div>
       </div>
   </section>
   ```

6. **Section 2: The Cost Engine & Multi-Target Topology**:
   ```html
   <section class="cost-engine-section" id="cost-engine">
       <h2 class="section-title">The Cost Engine &amp; Multi-Target Topology</h2>
       <p class="section-subtitle">70%–92% fleet savings backed by verified mathematics and multi-provider dispatch.</p>

       <div class="cost-engine-grid">
           <div class="cost-card glass">
               <div class="cost-icon">🖥️☁️</div>
               <div class="cost-header-tag">MULTI-TARGET TOPOLOGY</div>
               <h3>True Hybrid Local GPU + Cloud Routing</h3>
               <p>Orchestrate multiple local nodes (Ollama, vLLM), direct Claude subscriptions, and upstream aggregators (OpenRouter, Langdock) behind a single OpenAI-compatible <code>localhost:8000</code> endpoint.</p>
           </div>

           <div class="cost-card glass">
               <div class="cost-icon">🔥</div>
               <div class="cost-header-tag">AUTONOMOUS PRICE DISCOVERY</div>
               <h3>Heat Seeker Spot Deal Scout</h3>
               <p>Scans 300+ models in the background for underpriced capacity and flash subsidies. Automatically maps live deals into your active tier roles while you sleep.</p>
           </div>

           <div class="cost-card glass">
               <div class="cost-icon">💰</div>
               <div class="cost-header-tag">DOLLAR-ACCURATE ACCOUNTING</div>
               <h3>Prompt Cache-Aware Cost Engine</h3>
               <p>Ingests upstream provider prompt cache discounts (~80% off prompt tokens). Real mathematical cost tracking ensures your dashboard matches your actual invoice.</p>
           </div>

           <div class="cost-card glass">
               <div class="cost-icon">🎛️</div>
               <div class="cost-header-tag">GRID-SEARCH HEURISTICS</div>
               <h3>Curated Intelligence &amp; Auto-Tuner CLI</h3>
               <p>Embedded vetted model gallery with auto OTA GitHub updates. Run <code>nacho-flow tune</code> to grid-search your session logs and optimize routing rules without touching config files.</p>
           </div>
       </div>
   </section>
   ```

7. **Section 3: Developer Control Plane**:
   - Update `.extension-grid` to feature:
     - Card 1: 🧩 **All-in-One VS Code Companion Extension (Carries the Engine Inside)**:
       Bundles and supervises native Go binary directly. Zero manual Go toolchain or CLI setup required.
     - Card 2: 🌶️ **HotSauce Directives & In-Prompt Stream Bypass (`@nacho:`)**:
       Turn-by-turn steering (`@nacho:local`, `@nacho:cloud`, `@nacho:kickstart-off`, `@nacho:toggles`, `@nacho:reset`) stripped cleanly before upstream dispatch.

8. **Section 4: Scaling to Engineering Teams: Fleet Governance Without the Drag**:
   ```html
   <section class="fleet-governance-section" id="fleet-governance">
       <h2 class="section-title">🏢 Scaling to Engineering Teams: Fleet Governance Without the Drag</h2>
       <p class="section-subtitle">Bottom-up developer ergonomics with top-down spend and security controls.</p>

       <div class="fleet-governance-grid">
           <div class="fleet-card glass">
               <div class="fleet-icon">🛡️</div>
               <h3>Team Runaway Circuit Breakers</h3>
               <p>Hard spend limits and automated killswitches prevent developers from burning $500 overnight when an agent gets caught in an unmonitored test-debug loop.</p>
           </div>

           <div class="fleet-card glass">
               <div class="fleet-icon">🔒</div>
               <h3>Private GPU Fleet Offloading</h3>
               <p>Route routine codebase indexing and file inspections to internal on-prem vLLM/Ollama clusters, keeping proprietary source code off public APIs.</p>
           </div>

           <div class="fleet-card glass">
               <div class="fleet-icon">📊</div>
               <h3>Squad Spend Caps &amp; Quotas</h3>
               <p>Allocate developer AI budgets by squad (Core, Frontend, Data) without locking engineers out of their favorite IDE agents.</p>
           </div>
       </div>
   </section>
   ```

9. **Architectural Trust Bar**:
   ```html
   <div class="arch-trust-bar-container">
       <div class="arch-trust-bar glass">
           <div class="trust-item"><span class="trust-icon">⚡</span> <span class="trust-val">&lt; 0.18ms</span> <span class="trust-lbl">Go Core Latency</span></div>
           <div class="trust-divider"></div>
           <div class="trust-item"><span class="trust-icon">🚀</span> <span class="trust-val">31,424+</span> <span class="trust-lbl">Req/Sec P99</span></div>
           <div class="trust-divider"></div>
           <div class="trust-item"><span class="trust-icon">🛠️</span> <span class="trust-val">Universal</span> <span class="trust-lbl">Tool Normalization</span></div>
           <div class="trust-divider"></div>
           <div class="trust-item"><span class="trust-icon">🛡️</span> <span class="trust-val">Zero-Crash</span> <span class="trust-lbl">Stream Healing</span></div>
           <div class="trust-divider"></div>
           <div class="trust-item"><span class="trust-icon">📏</span> <span class="trust-val">Context</span> <span class="trust-lbl">Boundary Guards</span></div>
           <div class="trust-divider"></div>
           <div class="trust-item"><span class="trust-icon">📐</span> <span class="trust-val">O(1)</span> <span class="trust-lbl">Physical Limit Guards</span></div>
       </div>
   </div>
   ```

10. **Quickstart & Integrations Purge**:
    - Purge `roocode` key from `ideConfigs` JavaScript object.
    - Update Quickstart IDE tab buttons to: `Zoo Code / Cline`, `OpenCode`, `Aider`, `Cursor`.
    - Zero occurrences of "Roo Code" in `site/index.html` and `index.html`.

---

### Stylesheet Additions (`site/index.css` and `index.css`)

#### [MODIFY] [site/index.css](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/site/index.css)
#### [MODIFY] [index.css](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/index.css)

Add the following CSS rules:

```css
/* ==========================================================================
   The Autonomous Agent Trap Section
   ========================================================================== */
.agent-trap-section {
    padding: 3rem 2rem;
    max-width: 1000px;
    margin: 0 auto;
}

.agent-trap-container {
    padding: 2.5rem 3rem;
    border-radius: 16px;
    border: 1px solid rgba(239, 68, 68, 0.25);
    background: radial-gradient(circle at 50% 0%, rgba(239, 68, 68, 0.08) 0%, rgba(15, 17, 24, 0.95) 75%);
    box-shadow: 0 12px 40px rgba(0, 0, 0, 0.4);
}

.agent-trap-badge {
    display: inline-block;
    font-size: 0.72rem;
    font-weight: 700;
    color: #ef4444;
    background: rgba(239, 68, 68, 0.12);
    border: 1px solid rgba(239, 68, 68, 0.3);
    padding: 0.2rem 0.6rem;
    border-radius: 12px;
    letter-spacing: 1px;
    margin-bottom: 1rem;
}

.agent-trap-intro {
    font-size: 1.1rem;
    color: var(--text-primary);
    margin-bottom: 1.5rem;
    line-height: 1.6;
}

.agent-trap-list {
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: 1rem;
    margin-bottom: 2rem;
}

.agent-trap-list li {
    display: flex;
    align-items: flex-start;
    gap: 0.75rem;
    font-size: 0.98rem;
    color: var(--text-secondary);
    line-height: 1.5;
}

.trap-bullet {
    font-size: 1.1rem;
    flex-shrink: 0;
    margin-top: 0.1rem;
}

.agent-trap-punchline {
    border-top: 1px solid rgba(255, 255, 255, 0.08);
    padding-top: 1.5rem;
    font-size: 1.08rem;
    color: var(--cyan);
    line-height: 1.6;
}

/* ==========================================================================
   Section 1: The Three Fixes & Native 16:9 Video Bezel
   ========================================================================== */
.three-fixes-section {
    padding: 4rem 2rem;
    max-width: 1200px;
    margin: 0 auto;
}

.cycle-killer-video-container {
    max-width: 900px;
    margin: 2.5rem auto 3rem auto;
    border-radius: 14px;
    overflow: hidden;
    box-shadow: 0 16px 48px rgba(0, 0, 0, 0.5), 0 0 30px rgba(239, 68, 68, 0.15);
    border: 1px solid rgba(239, 68, 68, 0.3);
}

.cycle-killer-video-container .terminal-window {
    margin: 0;
    border-radius: 0;
    border: none;
}

.cycle-killer-video-container .video-wrapper {
    position: relative;
    width: 100%;
    aspect-ratio: 16 / 9;
    background: #000;
}

.cycle-killer-video-container video {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
}

.three-fixes-grid {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 1.5rem;
}

.fix-card {
    padding: 2rem;
    border-radius: 14px;
    display: flex;
    flex-direction: column;
    transition: transform 0.25s ease, box-shadow 0.25s ease;
}

.fix-card:hover {
    transform: translateY(-4px);
}

.fix-badge {
    font-size: 0.68rem;
    font-weight: 700;
    letter-spacing: 0.8px;
    padding: 0.2rem 0.55rem;
    border-radius: 8px;
    width: fit-content;
    margin-bottom: 1rem;
}

.badge-killer {
    color: #f87171;
    background: rgba(239, 68, 68, 0.12);
    border: 1px solid rgba(239, 68, 68, 0.3);
}

.badge-kickstart {
    color: #fbbf24;
    background: rgba(245, 158, 11, 0.12);
    border: 1px solid rgba(245, 158, 11, 0.3);
}

.badge-fairydust {
    color: #c084fc;
    background: rgba(168, 85, 247, 0.12);
    border: 1px solid rgba(168, 85, 247, 0.3);
}

.fix-card h3 {
    font-size: 1.35rem;
    margin-bottom: 0.5rem;
    color: #fff;
}

.fix-superpower {
    font-size: 0.95rem;
    font-weight: 600;
    color: var(--accent-color);
    margin-bottom: 1rem;
    line-height: 1.4;
}

.fix-mechanism {
    font-size: 0.9rem;
    color: var(--text-secondary);
    line-height: 1.6;
}

/* ==========================================================================
   Section 2: The Cost Engine & Multi-Target Topology (2x2 Grid)
   ========================================================================== */
.cost-engine-section {
    padding: 4rem 2rem;
    max-width: 1200px;
    margin: 0 auto;
}

.cost-engine-grid {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 1.5rem;
    margin-top: 2.5rem;
}

.cost-card {
    padding: 2.2rem;
    border-radius: 14px;
    display: flex;
    flex-direction: column;
}

.cost-icon {
    font-size: 2rem;
    margin-bottom: 0.8rem;
}

.cost-header-tag {
    font-size: 0.68rem;
    font-weight: 700;
    letter-spacing: 0.8px;
    color: var(--cyan);
    margin-bottom: 0.5rem;
}

.cost-card h3 {
    font-size: 1.25rem;
    margin-bottom: 0.75rem;
    color: #fff;
}

.cost-card p {
    font-size: 0.92rem;
    color: var(--text-secondary);
    line-height: 1.6;
}

/* ==========================================================================
   Section 4: Fleet Governance Grid
   ========================================================================== */
.fleet-governance-section {
    padding: 4rem 2rem;
    max-width: 1200px;
    margin: 0 auto;
}

.fleet-governance-grid {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 1.5rem;
    margin-top: 2.5rem;
}

.fleet-card {
    padding: 2rem;
    border-radius: 14px;
    border-top: 2px solid var(--accent-color);
}

.fleet-icon {
    font-size: 1.8rem;
    margin-bottom: 0.8rem;
}

.fleet-card h3 {
    font-size: 1.15rem;
    margin-bottom: 0.6rem;
    color: #fff;
}

.fleet-card p {
    font-size: 0.9rem;
    color: var(--text-secondary);
    line-height: 1.55;
}

/* ==========================================================================
   Architectural Trust Bar
   ========================================================================== */
.arch-trust-bar-container {
    max-width: 1200px;
    margin: 2rem auto;
    padding: 0 2rem;
}

.arch-trust-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1.2rem 2rem;
    border-radius: 12px;
    border: 1px solid var(--border-color);
    background: rgba(15, 17, 24, 0.8);
    flex-wrap: wrap;
    gap: 1rem;
}

.trust-item {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.88rem;
}

.trust-val {
    font-family: 'JetBrains Mono', monospace;
    font-weight: 700;
    color: var(--cyan);
}

.trust-lbl {
    color: var(--text-secondary);
    font-size: 0.82rem;
}

.trust-divider {
    width: 1px;
    height: 24px;
    background: rgba(255, 255, 255, 0.1);
}

/* ==========================================================================
   Responsive Breakpoints
   ========================================================================== */
@media (max-width: 1024px) {
    .three-fixes-grid,
    .fleet-governance-grid {
        grid-template-columns: repeat(2, 1fr);
    }
    .arch-trust-bar {
        justify-content: center;
        gap: 1.5rem;
    }
    .trust-divider {
        display: none;
    }
}

@media (max-width: 768px) {
    .three-fixes-grid,
    .cost-engine-grid,
    .fleet-governance-grid {
        grid-template-columns: 1fr;
    }
    .agent-trap-container {
        padding: 1.75rem;
    }
    .arch-trust-bar {
        flex-direction: column;
        align-items: flex-start;
        gap: 0.8rem;
    }
}
```

---

### Clean Documentation Purge (`docs/` & `site/docs/`)

#### [MODIFY] [docs/USER_GUIDE.md](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/docs/USER_GUIDE.md)
#### [MODIFY] [site/docs/USER_GUIDE.md](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/site/docs/USER_GUIDE.md)
#### [MODIFY] [docs/TUNING_GUIDE.md](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/docs/TUNING_GUIDE.md)
#### [MODIFY] [site/docs/TUNING_GUIDE.md](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/site/docs/TUNING_GUIDE.md)
#### [MODIFY] [docs/AGENT_BENCHMARK_SUITE.md](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/docs/AGENT_BENCHMARK_SUITE.md)
#### [MODIFY] [site/docs/AGENT_BENCHMARK_SUITE.md](file:///c:/Users/karlk/development/Go/src/github.com/dixieflatline76/nacho-flow/site/docs/AGENT_BENCHMARK_SUITE.md)
- Replace any remaining mentions of "Roo Code" with the active stack: **Cline, Zoo Code, OpenCode, Aider**.

---

## Detailed Execution Steps

### Phase 1: Video & Asset Preparation
1. Copy `C:\Users\karlk\Downloads\can_you_make_this_video_longer.mp4` to `site/images/cycle-killer-demo.mp4`.
2. Verify asset dimensions ($1280 \times 720$, 16:9 widescreen) and responsive loading.

### Phase 2: HTML Overhaul (`site/index.html` & root `index.html`)
1. **Nav Badge**: Update to `v0.9.1`.
2. **Hero Section**:
   - Update `H1` and explainer paragraph to exact approved copy.
   - Retain the 4 CTA buttons: Quick Install, VS Code Extension, Documentation, Star on GitHub.
3. **The Autonomous Agent Trap**:
   - Add the narrative card highlighting the 4 real-world rage-quit failure modes.
4. **Section 1: The Three Fixes**:
   - Insert the centered 16:9 native video player with terminal bezel header (`cycle-killer-demo.mp4`).
   - Insert the 3-column grid for Cycle Killer, Kickstart, and Fairy Dust with the exact approved copy.
5. **Section 2: The Cost Engine & Multi-Target Topology**:
   - Structure into the 2x2 grid with multi-target topology (Ollama, vLLM, Claude, OpenRouter, Langdock).
6. **Section 3: Developer Control Plane**:
   - Feature the all-in-one VS Code Extension (bundled Go binary) and HotSauce in-chat directives.
7. **Section 4: Scaling to Engineering Teams: Fleet Governance Without the Drag**:
   - Insert the 3 team-governance cards (Runaway Circuit Breakers, Private GPU Offloading, Squad Quotas).
8. **Architectural Trust Bar**:
   - Render the horizontal trust metrics bar.
9. **Roo Code Complete Purge**:
   - Scan and replace any lingering mentions of "Roo Code" across HTML files, docs, and extension metadata with the active stack (**Cline, Zoo Code, OpenCode, Aider**).

### Phase 3: Stylesheet Additions (`site/index.css` & root `index.css`)
1. Add `.agent-trap-section` and container styles.
2. Add `.cycle-killer-video-container` 16:9 widescreen terminal bezel styles.
3. Add `.three-fixes-grid` responsive card styles.
4. Add `.cost-engine-grid` 2x2 layout styles.
5. Add `.fleet-governance-grid` 3-column layout styles.
6. Add `.arch-trust-bar` horizontal metrics strip styles.
7. Add tablet (1024px) and mobile (768px/375px) responsive media queries.

### Phase 4: Verification & Visual Polish
1. Run local dev server on port 8080.
2. Open `site/index.html` in browser subagent and record visual verification.
3. Inspect layout at 1440px (Desktop), 1024px (Tablet), and 375px (Mobile).
4. Verify interactive terminal animations, video playback, copy buttons, and dropdown menus continue functioning without console errors.

---

## Verification Plan

### Automated Checks
- `git status` & `git diff` review ensuring all changes match specifications cleanly.
- Grep check across repository: `git grep -i 'roo code'` to confirm zero lingering references in site and active docs.
- Asset verification: confirm `site/images/cycle-killer-demo.mp4` exists with valid size.

### Visual & Browser Subagent Verification
- Spin up static server on `localhost:8080`.
- Use `browser_subagent` to navigate to `http://localhost:8080/site/index.html`:
  - Verify video playback in the terminal bezel.
  - Verify layout across Desktop (1440px), Tablet (1024px), and Mobile (375px).
  - Verify dropdown navigation, copy buttons, and interactive terminal animations.
