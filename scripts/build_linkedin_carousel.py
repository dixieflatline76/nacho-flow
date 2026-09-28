#!/usr/bin/env python3
"""
build_linkedin_carousel.py
Builds an executive-grade 16:9 landscape visual PDF carousel designed specifically
for LinkedIn document posts and mobile viewing.
"""

import os
import subprocess
import shutil

def build_carousel():
    repo_root = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
    
    html = '''<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <title>Why Autonomous Coding Agents Fail in Production - Executive Carousel</title>
    <style>
        @import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700;800;900&family=JetBrains+Mono:wght@400;600;700&display=swap');

        @page {
            size: 16in 9in;
            margin: 0;
        }

        *, *::before, *::after {
            box-sizing: border-box;
        }

        body {
            margin: 0;
            padding: 0;
            background: #090d16;
            color: #f1f5f9;
            font-family: 'Inter', -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            -webkit-print-color-adjust: exact;
            print-color-adjust: exact;
        }

        .slide {
            width: 16in;
            height: 9in;
            position: relative;
            padding: 0.8in 1in 0.7in 1in;
            page-break-after: always;
            overflow: hidden;
            background: radial-gradient(circle at 10% 20%, rgba(30, 41, 59, 0.4) 0%, #090d16 90%);
            display: flex;
            flex-direction: column;
            justify-content: space-between;
        }

        /* Top Brand Bar */
        .slide-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
        }

        .brand-badge {
            display: inline-flex;
            align-items: center;
            gap: 8px;
            background: rgba(245, 158, 11, 0.12);
            border: 1px solid rgba(245, 158, 11, 0.3);
            color: #fbbf24;
            padding: 6px 14px;
            border-radius: 9999px;
            font-size: 14pt;
            font-weight: 700;
            letter-spacing: 0.05em;
            text-transform: uppercase;
        }

        .slide-num {
            font-size: 15pt;
            font-weight: 600;
            color: #64748b;
            font-family: 'JetBrains Mono', monospace;
        }

        /* Main Content */
        .slide-body {
            flex: 1;
            display: flex;
            flex-direction: column;
            justify-content: center;
            margin: 0.4in 0;
        }

        /* Footer */
        .slide-footer {
            display: flex;
            justify-content: space-between;
            align-items: center;
            border-top: 1px solid rgba(255, 255, 255, 0.08);
            padding-top: 16px;
            font-size: 13pt;
            color: #64748b;
        }

        .slide-footer strong {
            color: #94a3b8;
        }

        /* Slide 1 Cover */
        .cover-title {
            font-size: 52pt;
            font-weight: 900;
            line-height: 1.1;
            letter-spacing: -0.03em;
            margin: 0 0 20px 0;
            background: linear-gradient(135deg, #ffffff 40%, #94a3b8 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }

        .cover-subtitle {
            font-size: 24pt;
            line-height: 1.4;
            color: #38bdf8;
            font-weight: 600;
            margin: 0 0 35px 0;
            max-width: 13in;
        }

        .stat-pills {
            display: grid;
            grid-template-columns: repeat(4, 1fr);
            gap: 20px;
            margin-top: 20px;
        }

        .pill-card {
            background: rgba(15, 23, 42, 0.7);
            border: 1px solid rgba(56, 189, 248, 0.25);
            border-radius: 12px;
            padding: 20px 24px;
        }

        .pill-card .val {
            font-size: 34pt;
            font-weight: 800;
            color: #f59e0b;
            font-family: 'JetBrains Mono', monospace;
            margin-bottom: 4px;
        }

        .pill-card .lbl {
            font-size: 13pt;
            font-weight: 600;
            color: #94a3b8;
            text-transform: uppercase;
            letter-spacing: 0.04em;
        }

        /* Standard Titles */
        .title-lg {
            font-size: 38pt;
            font-weight: 800;
            letter-spacing: -0.02em;
            margin: 0 0 12px 0;
            color: #ffffff;
        }

        .sub-lg {
            font-size: 18pt;
            color: #94a3b8;
            margin: 0 0 30px 0;
            line-height: 1.4;
        }

        /* Grid Layouts */
        .grid-2 {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 30px;
        }

        .grid-4 {
            display: grid;
            grid-template-columns: repeat(4, 1fr);
            gap: 24px;
        }

        .grid-3 {
            display: grid;
            grid-template-columns: repeat(3, 1fr);
            gap: 24px;
        }

        /* Cards */
        .card-dark {
            background: rgba(15, 23, 42, 0.8);
            border: 1px solid rgba(255, 255, 255, 0.1);
            border-radius: 14px;
            padding: 28px;
            position: relative;
        }

        .card-danger {
            border-left: 6px solid #ef4444;
        }

        .card-success {
            border-left: 6px solid #10b981;
        }

        .card-amber {
            border-left: 6px solid #f59e0b;
        }

        .card-cyan {
            border-left: 6px solid #38bdf8;
        }

        .card-title {
            font-size: 20pt;
            font-weight: 700;
            color: #ffffff;
            margin: 0 0 12px 0;
            display: flex;
            align-items: center;
            gap: 12px;
        }

        .card-desc {
            font-size: 15pt;
            color: #cbd5e1;
            line-height: 1.5;
            margin: 0;
        }

        /* Quote Callout */
        .quote-box {
            background: rgba(30, 41, 59, 0.4);
            border-left: 8px solid #f59e0b;
            padding: 24px 32px;
            border-radius: 0 16px 16px 0;
            font-size: 24pt;
            font-weight: 600;
            line-height: 1.4;
            color: #f8fafc;
            margin-bottom: 30px;
        }

        /* Big Comparison Cards */
        .comp-card {
            background: #0f172a;
            border-radius: 16px;
            padding: 32px;
            border: 2px solid rgba(255, 255, 255, 0.1);
            display: flex;
            flex-direction: column;
            justify-content: space-between;
        }

        .comp-card.winner {
            border-color: #10b981;
            background: linear-gradient(180deg, rgba(16, 185, 129, 0.08) 0%, rgba(15, 23, 42, 0.95) 100%);
        }

        .comp-badge {
            align-self: flex-start;
            padding: 6px 14px;
            border-radius: 6px;
            font-size: 12pt;
            font-weight: 800;
            text-transform: uppercase;
            letter-spacing: 0.06em;
            margin-bottom: 16px;
        }

        .badge-control { background: #3b0764; color: #d8b4fe; }
        .badge-nacho { background: #064e3b; color: #6ee7b7; }

        .comp-cost {
            font-size: 54pt;
            font-weight: 900;
            font-family: 'JetBrains Mono', monospace;
            margin: 10px 0 16px 0;
        }

        .cost-red { color: #f87171; }
        .cost-green { color: #34d399; }

        .comp-rows {
            display: flex;
            flex-direction: column;
            gap: 12px;
            margin-top: 14px;
        }

        .comp-row {
            display: flex;
            justify-content: space-between;
            font-size: 15pt;
            border-bottom: 1px solid rgba(255, 255, 255, 0.08);
            padding-bottom: 8px;
        }

        .comp-row span { color: #94a3b8; }
        .comp-row strong { color: #ffffff; }

        /* Code Cards */
        .code-box {
            background: #050811;
            border: 1px solid #1e293b;
            border-radius: 12px;
            padding: 20px 24px;
            font-family: 'JetBrains Mono', monospace;
            font-size: 13.5pt;
            line-height: 1.5;
            color: #e2e8f0;
            white-space: pre-wrap;
        }

        .code-comment { color: #64748b; }
        .code-keyword { color: #f43f5e; font-weight: 600; }
        .code-func { color: #38bdf8; font-weight: 600; }
        .code-string { color: #34d399; }

        /* Metric Block */
        .metric-big {
            font-size: 64pt;
            font-weight: 900;
            font-family: 'JetBrains Mono', monospace;
            color: #38bdf8;
            margin: 0 0 6px 0;
        }
    </style>
</head>
<body>

    <!-- SLIDE 1: COVER -->
    <section class="slide">
        <div class="slide-header">
            <div class="brand-badge">🌮 Systems Research Report · September 2026</div>
            <div class="slide-num">01 / 08</div>
        </div>
        <div class="slide-body">
            <h1 class="cover-title">Why Autonomous Coding Agents<br/>Fail in Production</h1>
            <h2 class="cover-subtitle">Mitigating Context Snowballs, Code Churn, and Protocol Fragility with an Active Layer 4 Execution Runtime</h2>
            
            <div class="stat-pills">
                <div class="pill-card">
                    <div class="val">$0.44</div>
                    <div class="lbl">64-Turn Autonomous Run</div>
                </div>
                <div class="pill-card">
                    <div class="val">11×</div>
                    <div class="lbl">Frontier Cost Reduction</div>
                </div>
                <div class="pill-card">
                    <div class="val">1.92M</div>
                    <div class="lbl">Monte Carlo Rounds/sec</div>
                </div>
                <div class="pill-card">
                    <div class="val">0</div>
                    <div class="lbl">Data Races · 100% Gates</div>
                </div>
            </div>
        </div>
        <div class="slide-footer">
            <div>Author: <strong>@dixieflatline76</strong> · Nacho Flow</div>
            <div>Stack: <strong>Nacho Flow v1.4.1 · Zoo Code v3.84 · Go 1.26</strong></div>
        </div>
    </section>

    <!-- SLIDE 2: THE INTELLIGENCE MYTH -->
    <section class="slide">
        <div class="slide-header">
            <div class="brand-badge">🌮 The Paradigm Shift</div>
            <div class="slide-num">02 / 08</div>
        </div>
        <div class="slide-body">
            <h2 class="title-lg">The Intelligence Myth in Autonomous Coding</h2>
            <div class="quote-box">
                "Autonomous agents don't fail from a lack of raw model logic.<br/>
                They fail because the wire protocol collapses underneath them."
            </div>

            <div class="grid-2">
                <div class="card-dark card-cyan">
                    <div class="card-title">🖥️ Interactive Copilot (Cursor / Windsurf)</div>
                    <p class="card-desc">
                        • <strong>Operator:</strong> Human actively staring at the screen.<br/>
                        • <strong>Constraint:</strong> Streaming Latency (70–100 tokens/sec perceived).<br/>
                        • <strong>Context:</strong> Shallow (1–5 turns per edit).<br/>
                        • <strong>Failure Mode:</strong> Developer manually intervenes on compile errors.
                    </p>
                </div>
                <div class="card-dark card-amber">
                    <div class="card-title">🤖 Background Autonomous Delegation (Zoo Code / OpenCode)</div>
                    <p class="card-desc">
                        • <strong>Operator:</strong> Agent executes unattended in background while you work.<br/>
                        • <strong>Constraint:</strong> <strong>Unit Economics & Completion Reliability</strong>.<br/>
                        • <strong>Context:</strong> Deep (40–80 turns accumulating 120k+ tokens).<br/>
                        • <strong>The Golden Rule:</strong> The moment you walk away, streaming latency stops mattering. Unit economics dominate by an order of magnitude.
                    </p>
                </div>
            </div>
        </div>
        <div class="slide-footer">
            <div>Nacho Flow Systems Whitepaper</div>
            <div>Section 1 & 2: Interactive vs Autonomous Split</div>
        </div>
    </section>

    <!-- SLIDE 3: 4 FATAL WIRE FAILURE MODES -->
    <section class="slide">
        <div class="slide-header">
            <div class="brand-badge">⚠️ Production Breakdown</div>
            <div class="slide-num">03 / 08</div>
        </div>
        <div class="slide-body">
            <h2 class="title-lg">Why Raw Budget Models Collapse on Turn 15+</h2>
            <p class="sub-lg">Frontier models undergo tens of millions in RL fine-tuning for rigid schema adherence. Budget models do not.</p>

            <div class="grid-2" style="gap: 20px;">
                <div class="card-dark card-danger">
                    <div class="card-title">1. The "Context Snowball" & 90% Tax</div>
                    <p class="card-desc">
                        Harnesses re-send full transcripts every turn ($O(N^2)$ cumulative token growth). By Turn 40, <strong>85%–95% of prompt tokens are redundant boilerplate</strong>, causing needle-in-a-haystack attention loss and multi-dollar prompt costs.
                    </p>
                </div>

                <div class="card-dark card-danger">
                    <div class="card-title">2. &lt;think&gt; Token Contamination</div>
                    <p class="card-desc">
                        Reasoning models emit Chain-of-Thought blocks that fragment across TCP packets. Raw proxies leak thought tokens directly into file-writing tools, <strong>writing internal monologue into source code</strong> and breaking compilers.
                    </p>
                </div>

                <div class="card-dark card-danger">
                    <div class="card-title">3. V8 JSON Crashes (position 515)</div>
                    <p class="card-desc">
                        When open-weight models truncate output tokens mid-stream, client extension runtimes throw unhandled <code>JSON.parse</code> exceptions (<code>SyntaxError at position 515</code>), <strong>permanently freezing the IDE webview</strong>.
                    </p>
                </div>

                <div class="card-dark card-danger">
                    <div class="card-title">4. 3-Strike Deadlocks & Test Fraud</div>
                    <p class="card-desc">
                        Smaller models answer in prose ("Should I proceed?") instead of invoking tools, tripping harness zero-turn strike limits. When pushed, agents cheat: <strong>softening <code>t.Errorf</code> to <code>t.Logf</code></strong> to fake a green test run.
                    </p>
                </div>
            </div>
        </div>
        <div class="slide-footer">
            <div>Nacho Flow Systems Whitepaper</div>
            <div>Section 2: Verified Real-World Developer Failure Modes</div>
        </div>
    </section>

    <!-- SLIDE 4: THE L1-L4 STACK -->
    <section class="slide">
        <div class="slide-header">
            <div class="brand-badge">🏛️ Architecture</div>
            <div class="slide-num">04 / 08</div>
        </div>
        <div class="slide-body">
            <h2 class="title-lg">The Solution: The Layer 4 Execution Runtime</h2>
            <p class="sub-lg">Nacho Flow is the deterministic, wire-speed mediation fabric operating between agent harnesses and inference endpoints.</p>

            <div class="grid-4">
                <div class="card-dark" style="border-top: 4px solid #38bdf8;">
                    <div class="card-title" style="font-size: 16pt;">💻 Layer 3<br/>Agent Harness</div>
                    <p class="card-desc" style="font-size: 13pt;">
                        <strong>Zoo Code v3.84 · OpenCode · Cursor Agent</strong><br/><br/>
                        Manages conversation transcript, user specs, and tool dispatch loops.
                    </p>
                </div>

                <div class="card-dark" style="border-top: 4px solid #f59e0b; background: rgba(245, 158, 11, 0.08); grid-column: span 2;">
                    <div class="card-title" style="font-size: 18pt; color: #fbbf24;">🌮 Layer 4: Nacho Flow Runtime</div>
                    <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 14px; margin-top: 10px;">
                        <div style="font-size: 13pt; color: #e2e8f0;">
                            • <strong>3-Lane SSE Normalizer:</strong> Sanitizes tool args in 38ns with 0 allocs.<br/>
                            • <strong>In-Flight Stream Healing:</strong> Closes truncated JSON; stops V8 515 crashes.
                        </div>
                        <div style="font-size: 13pt; color: #e2e8f0;">
                            • <strong>Nacho Token Saver (NTS):</strong> Deduplicates stale reads, cutting tokens 25–40%.<br/>
                            • <strong>Min-Conflicts Router:</strong> 4.4ns tail-buffer kills repetition loops.
                        </div>
                    </div>
                </div>

                <div class="card-dark" style="border-top: 4px solid #10b981;">
                    <div class="card-title" style="font-size: 16pt;">⚡ Layers 1 & 2<br/>Inference Endpoints</div>
                    <p class="card-desc" style="font-size: 13pt;">
                        • <strong>Local Workstation GPU:</strong> $0.00 / turn<br/>
                        • <strong>Dense Cloud (GLM-5.3):</strong> $0.65 / 1M<br/>
                        • <strong>Frontier (Sonnet 5):</strong> Escalation
                    </p>
                </div>
            </div>
        </div>
        <div class="slide-footer">
            <div>Nacho Flow Systems Whitepaper</div>
            <div>Section 4: System Architecture & Mediation Fabric</div>
        </div>
    </section>

    <!-- SLIDE 5: EMPIRICAL BENCHMARK -->
    <section class="slide">
        <div class="slide-header">
            <div class="brand-badge">📊 Empirical Benchmark</div>
            <div class="slide-num">05 / 08</div>
        </div>
        <div class="slide-body">
            <h2 class="title-lg">Head-to-Head: Building the Go Blackjack Engine</h2>
            <p class="sub-lg">Task: Full Blackjack CLI, S17 basic strategy trainer, and 10,000-round Monte Carlo simulator in pure Go 1.26 standard library.</p>

            <div class="grid-2">
                <div class="comp-card">
                    <div>
                        <div class="comp-badge badge-control">👑 Instrumented Frontier Control (Claude Sonnet 5 Direct)</div>
                        <div class="comp-cost cost-red">$4.87 <span style="font-size: 20pt; color: #94a3b8;">USD</span></div>
                    </div>
                    <div class="comp-rows">
                        <div class="comp-row"><span>Total Turns</span><strong>84 turns</strong></div>
                        <div class="comp-row"><span>Elapsed Time</span><strong>31.5 minutes</strong></div>
                        <div class="comp-row"><span>Failure Incidents</span><strong>9 max-token output stalls</strong></div>
                        <div class="comp-row"><span>Prompt Cache Hit Rate</span><strong>97.4% (Direct API)</strong></div>
                        <div class="comp-row"><span>Monthly Team Spend (10 tasks/day)</span><strong style="color: #f87171;">$1,461 / Month</strong></div>
                    </div>
                </div>

                <div class="comp-card winner">
                    <div>
                        <div class="comp-badge badge-nacho">🌮 Nacho Flow + Zoo Code v3.84 + GLM-5.3-Flash</div>
                        <div class="comp-cost cost-green">$0.44 <span style="font-size: 20pt; color: #94a3b8;">USD (11× Less)</span></div>
                    </div>
                    <div class="comp-rows">
                        <div class="comp-row"><span>Total Turns</span><strong>64 turns (Unattended)</strong></div>
                        <div class="comp-row"><span>Elapsed Time</span><strong>~52 minutes (Background)</strong></div>
                        <div class="comp-row"><span>Monte Carlo Velocity</span><strong style="color: #34d399;">1,923,076 rounds/sec</strong></div>
                        <div class="comp-row"><span>Test Coverage & Races</span><strong>88.6% · 0 Data Races</strong></div>
                        <div class="comp-row"><span>Monthly Team Spend (10 tasks/day)</span><strong style="color: #34d399;">$132 / Month</strong></div>
                    </div>
                </div>
            </div>
        </div>
        <div class="slide-footer">
            <div>Nacho Flow Systems Whitepaper</div>
            <div>Section 5 & 6: Benchmark Scorecard & Head-to-Head Comparison</div>
        </div>
    </section>

    <!-- SLIDE 6: SELF CORRECTION -->
    <section class="slide">
        <div class="slide-header">
            <div class="brand-badge">🔬 Autonomous Intelligence</div>
            <div class="slide-num">06 / 08</div>
        </div>
        <div class="slide-body">
            <h2 class="title-lg">Autonomous Self-Correction in Action</h2>
            <p class="sub-lg">During turns 42 to 58, the agent detected assertion failures and corrected subtle mathematical bugs human engineers routinely miss:</p>

            <div class="grid-2">
                <div>
                    <div style="font-size: 16pt; font-weight: 700; color: #fbbf24; margin-bottom: 8px;">
                        1. Split-Ace Natural Bonus Trap (Casino Rules)
                    </div>
                    <p style="font-size: 13.5pt; color: #cbd5e1; margin-bottom: 12px;">
                        In casino rules, a 21 on a split Ace pays 1:1, never 3:2. The agent detected this assertion failure and refactored <code>Hand.IsBlackjack()</code>:
                    </p>
                    <div class="code-box">
<span class="code-keyword">func</span> (h *Hand) <span class="code-func">IsBlackjack</span>() <span class="code-keyword">bool</span> {
    <span class="code-keyword">return</span> len(h.Cards) == <span class="code-string">2</span> && 
           h.Total() == <span class="code-string">21</span> && 
           <span class="code-func">!h.IsSplit</span> <span class="code-comment">// Fixed by agent</span>
}
                    </div>
                </div>

                <div>
                    <div style="font-size: 16pt; font-weight: 700; color: #fbbf24; margin-bottom: 8px;">
                        2. Net Round Delta Settlement (Multi-Hand)
                    </div>
                    <p style="font-size: 13.5pt; color: #cbd5e1; margin-bottom: 12px;">
                        When doubling or splitting, multiple hands exist. Classifying individual hands caused win % to exceed 100%. Refactored to Net Round Delta:
                    </p>
                    <div class="code-box">
<span class="code-keyword">if</span> roundNet > <span class="code-string">0</span> {
    wins++
} <span class="code-keyword">else if</span> roundNet &lt; <span class="code-string">0</span> {
    losses++
} <span class="code-keyword">else</span> {
    pushes++
}
                    </div>
                </div>
            </div>
        </div>
        <div class="slide-footer">
            <div>Nacho Flow Systems Whitepaper</div>
            <div>Section 5.3: Autonomous Test-Driven Verification</div>
        </div>
    </section>

    <!-- SLIDE 7: BARE METAL PERFORMANCE -->
    <section class="slide">
        <div class="slide-header">
            <div class="brand-badge">⚡ Bare-Metal Performance</div>
            <div class="slide-num">07 / 08</div>
        </div>
        <div class="slide-body">
            <h2 class="title-lg">Zero Overhead: Wire-Speed Gateway Telemetry</h2>
            <p class="sub-lg">An L4 runtime can never become the bottleneck. Benchmarked on physical hardware (AMD Ryzen 7 5700X3D, 16 threads):</p>

            <div class="grid-4" style="margin-bottom: 24px;">
                <div class="card-dark">
                    <div class="metric-big">27.5k</div>
                    <div style="font-size: 14pt; color: #94a3b8; font-weight: 700; text-transform: uppercase;">Throughput (Req/sec)</div>
                </div>
                <div class="card-dark">
                    <div class="metric-big">&lt; 250<span style="font-size: 32pt;">µs</span></div>
                    <div style="font-size: 14pt; color: #94a3b8; font-weight: 700; text-transform: uppercase;">Proxy Latency Overhead</div>
                </div>
                <div class="card-dark">
                    <div class="metric-big">0</div>
                    <div style="font-size: 14pt; color: #94a3b8; font-weight: 700; text-transform: uppercase;">Allocs/Op on Fast-Path</div>
                </div>
                <div class="card-dark">
                    <div class="metric-big">100%</div>
                    <div style="font-size: 14pt; color: #94a3b8; font-weight: 700; text-transform: uppercase;">350,000 Req Success Rate</div>
                </div>
            </div>

            <div class="code-box" style="font-size: 12pt; line-height: 1.6;">
<span class="code-comment">// High-concurrency zero-allocation microbenchmarks (pure Go 1.26):</span>
BenchmarkCycleBreaker_ProcessToolDelta_FileWrite-16   380,792,568    3.10 ns/op   0 B/op   <span class="code-string">0 allocs/op</span>
BenchmarkRuleEngine_Evaluate-16                      270,317,331    4.44 ns/op   0 B/op   <span class="code-string">0 allocs/op</span>
BenchmarkContainsFoldASCII_ZeroAlloc-16               31,206,358   38.76 ns/op   0 B/op   <span class="code-string">0 allocs/op</span>
            </div>
        </div>
        <div class="slide-footer">
            <div>Nacho Flow Systems Whitepaper</div>
            <div>Section 7: Gateway Performance & Stress Test Verification</div>
        </div>
    </section>

    <!-- SLIDE 8: SUMMARY & OPEN SOURCE -->
    <section class="slide">
        <div class="slide-header">
            <div class="brand-badge">🚀 The Verdict</div>
            <div class="slide-num">08 / 08</div>
        </div>
        <div class="slide-body" style="text-align: center; align-items: center;">
            <h2 class="title-lg" style="font-size: 46pt; margin-bottom: 20px;">Production Autonomous Coding<br/>Doesn't Need a $15 Frontier Tax.</h2>
            <div class="quote-box" style="border: none; background: rgba(245, 158, 11, 0.1); color: #fbbf24; font-size: 26pt; max-width: 12in;">
                "It requires an execution runtime designed for autonomy."
            </div>

            <p style="font-size: 18pt; color: #cbd5e1; max-width: 10in; line-height: 1.6; margin: 20px 0 35px 0;">
                Nacho Flow immunizes agents against open-weight protocol defects, compacts context snowballs in-flight, and delivers enterprise reliability at 90% lower cloud cost.
            </p>

            <div style="display: flex; gap: 24px; justify-content: center;">
                <div class="pill-card" style="padding: 16px 28px;">
                    <div style="font-size: 18pt; font-weight: 800; color: #ffffff;">⭐ 100% Free & Open Source</div>
                    <div style="font-size: 13pt; color: #38bdf8; margin-top: 4px;">github.com/dixieflatline76/nacho-flow</div>
                </div>
                <div class="pill-card" style="padding: 16px 28px;">
                    <div style="font-size: 18pt; font-weight: 800; color: #ffffff;">🧩 Official VS Code Extension</div>
                    <div style="font-size: 13pt; color: #38bdf8; margin-top: 4px;">code --install-extension dixieflatline76.nacho-flow</div>
                </div>
            </div>
        </div>
        <div class="slide-footer">
            <div>Read the full systems whitepaper & reproduction telemetry on GitHub</div>
            <div>Published by <strong>@dixieflatline76</strong> · September 2026</div>
        </div>
    </section>

</body>
</html>'''

    html_path = os.path.join(repo_root, "scripts", "linkedin_carousel.html")
    with open(html_path, "w", encoding="utf-8") as f:
        f.write(html)
    print("Generated carousel HTML:", html_path)

    pdf_dest = os.path.join(repo_root, "docs", "NACHO_FLOW_EXECUTIVE_WHITEPAPER_CAROUSEL.pdf")
    site_pdf = os.path.join(repo_root, "site", "docs", "NACHO_FLOW_EXECUTIVE_WHITEPAPER_CAROUSEL.pdf")
    artifacts_dir = r"C:\Users\karlk\.gemini\antigravity-ide\brain\b1402da8-4586-4f87-871b-3566338b03c1"
    artifact_pdf = os.path.join(artifacts_dir, "NACHO_FLOW_EXECUTIVE_WHITEPAPER_CAROUSEL.pdf")

    edge_bin = r"C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe"
    cmd = [
        edge_bin,
        "--headless",
        "--disable-gpu",
        "--run-all-compositor-stages-before-draw",
        "--virtual-time-budget=3000",
        "--no-pdf-header-footer",
        f"--print-to-pdf={pdf_dest}",
        html_path
    ]

    print("Rendering 16:9 LinkedIn carousel PDF via Edge engine...")
    subprocess.run(cmd, check=True)

    if os.path.exists(pdf_dest):
        size_kb = os.path.getsize(pdf_dest) / 1024
        print(f"Successfully generated carousel PDF: {pdf_dest} ({size_kb:.1f} KB)")
        shutil.copy2(pdf_dest, site_pdf)
        if os.path.exists(artifacts_dir):
            shutil.copy2(pdf_dest, artifact_pdf)
    else:
        raise RuntimeError("Carousel PDF generation failed.")

if __name__ == "__main__":
    build_carousel()
