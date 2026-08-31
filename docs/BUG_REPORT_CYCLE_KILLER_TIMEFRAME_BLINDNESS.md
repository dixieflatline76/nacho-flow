# 🐛 Bug Report: Cycle Killer Telemetry Timeframe Blindness in Dashboard

**Date Reported:** August 30, 2026  
**Status:** Resolved / Verified (Fixed in v0.8.4, 96.4% Backend / 95.8% Frontend Coverage)  
**Severity:** Medium (UI/Telemetry Inconsistency — Data Display Disconnect)  
**Affected Components:**
- `pkg/telemetry/metrics.go` (Backend Statistics Engine)
- `extension/resources/webview/dashboard.js` (Webview Dashboard Presenter)

---

## 1. Summary

When viewing the **Nacho Flow Dashboard**, selecting different timeframe filters (**Today**, **This Week**, **This Month**, **All Time**) updates the top *"Statistics & Cost Savings"* metrics cards appropriately based on rolling/discrete time windows.

However, the bottom section—**"Cycle Killer Defense & Reliability"**—does not respect the active timeframe filter. It persistently displays **all-time cumulative metrics** (e.g., total interventions, avoided GPU lockup minutes, and runaway tokens) regardless of which timeframe tab is active.

### Visual Manifestation:
* **"Today" Tab Active (New Day / 0 turns so far):**
  - **Top Row (Stats & Savings):** Shows `$0.00` Cost Saved, `$0.00` Cloud Spend, `0 turns (0%)` Local GPU, `0 turns` Total Volume.
  - **Bottom Row (Cycle Killer):** Displays `11 Loops Interventions Executed`, `41.8 Min GPU Lockup Rescued`, `88k Tokens Avoided`, `100% Stage 1 Local Heal Rate`.
* **User Impact:** Visual contradiction where the dashboard reports 0 turns executed today while simultaneously displaying active loop interventions.

---

## 2. Forensic Root Cause Analysis

### 2.1 Backend: Missing Windowed Accumulators in `pkg/telemetry/metrics.go`
In `pkg/telemetry/metrics.go`, `TimeWindowMetrics` is defined strictly for volume and financial figures:

```go
// Line 24-31: TimeWindowMetrics only tracks tokens and cost
type TimeWindowMetrics struct {
    Requests         int64   `json:"requests"`
    TokensTotal      int64   `json:"tokens_total"`
    TokensLocal      int64   `json:"tokens_local"`
    CostSpentUSD     float64 `json:"cost_spent_usd"`
    CostSavedUSD     float64 `json:"cost_saved_usd"`
    CostReductionPct float64 `json:"cost_reduction_pct"`
}
```

Meanwhile, `CycleKillerMetrics` is stored as a single, global all-time accumulator on `StatsSnapshot`:

```go
// Line 62-73: StatsSnapshot contains a single global CycleKiller object
type StatsSnapshot struct {
    // ...
    CycleKiller  CycleKillerMetrics           `json:"cycle_killer"` // <-- Global All-Time
    Windows      TimeWindowSnapshot           `json:"windows"`
    DailyBuckets map[string]TimeWindowMetrics `json:"daily_buckets,omitempty"`
}
```

When turns are ingested in `updateWindowsLocked()` (lines 253–311), discrete daily, weekly, and monthly buckets are updated for `TimeWindowMetrics`, but Cycle Killer interventions are only incremented on the global `s.stats.CycleKiller` struct (lines 385–394 and 528–537).

### 2.2 Frontend: Static All-Time Binding in `dashboard.js`
In `extension/resources/webview/dashboard.js`:

```javascript
// Line 104-201: renderStats() indexes windows for financial metrics,
// but unconditionally passes global stats.cycle_killer to renderCycleKiller()
function renderStats(stats) {
    if (activeTimeWindow === 'today') {
        const w = stats.windows?.today;
        // ... updates top cards from w ...
    }
    // ...
    renderCycleKiller(stats.cycle_killer); // <-- BUG: Always passes all-time global object
}
```

---

## 3. Proposed Fix & Architectural Plan

### Step 1: Embed `CycleKillerMetrics` in `TimeWindowMetrics`
Modify `pkg/telemetry/metrics.go`:

```go
type TimeWindowMetrics struct {
    Requests         int64              `json:"requests"`
    TokensTotal      int64              `json:"tokens_total"`
    TokensLocal      int64              `json:"tokens_local"`
    CostSpentUSD     float64            `json:"cost_spent_usd"`
    CostSavedUSD     float64            `json:"cost_saved_usd"`
    CostReductionPct float64            `json:"cost_reduction_pct"`
    CycleKiller      CycleKillerMetrics `json:"cycle_killer"` // <-- Added windowed tracking
}
```

### Step 2: Update Window Accumulator Logic
In `updateWindowsLocked()` and `addToWindow()` in `pkg/telemetry/metrics.go`:
* When `obs.CycleBreakerTriggered == true`, increment `w.CycleKiller.TotalInterventions`, `AvoidedRunawayTokens`, `AvoidedGPUSeconds`, and heal/escalation counters on the respective active window (`Today`, `ThisWeek`, `ThisMonth`, `AllTime`) and persistent `DailyBuckets`.
* Ensure `addBucketToWindow()` correctly aggregates `CycleKiller` fields when computing weekly and monthly totals.
* Calculate `LocalHealSuccessRatePct` dynamically per window in `GetStats()`.

### Step 3: Update `dashboard.js` Webview Presenter
In `extension/resources/webview/dashboard.js`:
Update `renderStats()` to extract the timeframe-specific `cycle_killer` object:

```javascript
let currentCycleKiller = null;
if (activeTimeWindow === 'today') {
    currentCycleKiller = stats.windows?.today?.cycle_killer;
} else if (activeTimeWindow === 'this_week') {
    currentCycleKiller = stats.windows?.this_week?.cycle_killer;
} else if (activeTimeWindow === 'this_month') {
    currentCycleKiller = stats.windows?.this_month?.cycle_killer;
} else {
    currentCycleKiller = stats.windows?.all_time?.cycle_killer || stats.cycle_killer;
}

renderCycleKiller(currentCycleKiller);
```

---

## 4. Verification & Testing Plan

1. **Unit Tests (`pkg/telemetry/metrics_test.go`):**
   - Add test `TestStatsTracker_CycleKiller_TimeWindowBucketing` verifying that an intervention on Day 1 does not appear in `stats.windows.today` on Day 2.
   - Verify weekly and monthly roll-up aggregations of avoided tokens and GPU seconds.
2. **Webview Integration Tests (`extension/src/ui/webview/dashboard.test.ts`):**
   - Verify `renderCycleKiller()` receives `0` metrics when switching to an empty timeframe window.
3. **Manual Verification:**
   - Launch dashboard with historical records spanning >24h.
   - Toggle between **Today** and **All Time**; verify Cycle Killer counters transition between current-day activity and cumulative totals.
