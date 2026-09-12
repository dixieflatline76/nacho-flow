/// <reference lib="dom" />
/**
 * dashboard-webview.test.ts
 *
 * Tests for the dashboard.js webview runtime logic using a JSDOM environment.
 *
 * dashboard.js is a plain-JS IIFE that runs inside a VS Code webview (browser context).
 * It is excluded from the TypeScript compilation pipeline and therefore invisible to the
 * normal Jest coverage collector. This file provides explicit coverage by:
 *
 *   1. Bootstrapping a minimal DOM with the elements that dashboard.js writes to.
 *   2. Mocking acquireVsCodeApi() and localStorage.
 *   3. eval()-ing the script so that its module-level IIFE runs in the JSDOM context.
 *   4. Dispatching synthetic 'message' events (the same channel VS Code uses) to drive
 *      the updateStats / setTimeWindow paths.
 *
 * @jest-environment jsdom
 */

import * as fs from 'fs';
import * as path from 'path';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * Build a full stats payload for tests.
 * All window CK objects are *present* (as the v0.8.4 daemon emits them).
 */
function makeStats(overrides: Record<string, unknown> = {}): unknown {
  return {
    total_requests: 100,
    total_tokens: 500000,
    total_tokens_routed_locally: 400000,
    total_cost_spent_usd: 1.5,
    estimated_cost_saved_usd: 8.5,
    cost_reduction_pct: 85,
    started_at: '2026-08-01T00:00:00Z',
    cycle_killer: {
      total_interventions: 12,
      avoided_runaway_tokens: 24000,
      avoided_gpu_seconds: 720,
      stage1_local_heals: 10,
      stage2_cloud_escalations: 2,
      local_heal_success_rate_pct: 83,
    },
    windows: {
      past_1_hour: {
        requests: 2,
        tokens_total: 4000,
        tokens_local: 3200,
        cost_spent_usd: 0.02,
        cost_saved_usd: 0.18,
        cost_reduction_pct: 90,
        cycle_killer: {
          total_interventions: 1,
          avoided_runaway_tokens: 8000,
          avoided_gpu_seconds: 240,
          stage1_local_heals: 1,
          stage2_cloud_escalations: 0,
          local_heal_success_rate_pct: 100,
        },
        nts_tokens_saved: 1200,
        nts_bytes_saved: 4800,
        nts_compacted_turns: 2,
      },
      today: {
        requests: 5,
        tokens_total: 10000,
        tokens_local: 8000,
        cost_spent_usd: 0.05,
        cost_saved_usd: 0.45,
        cost_reduction_pct: 90,
        cycle_killer: {
          total_interventions: 0,
          avoided_runaway_tokens: 0,
          avoided_gpu_seconds: 0,
          stage1_local_heals: 0,
          stage2_cloud_escalations: 0,
          local_heal_success_rate_pct: 0,
        },
      },
      yesterday: {
        requests: 15,
        tokens_total: 75000,
        tokens_local: 60000,
        cost_spent_usd: 0.25,
        cost_saved_usd: 1.75,
        cost_reduction_pct: 88,
        cycle_killer: {
          total_interventions: 4,
          avoided_runaway_tokens: 8000,
          avoided_gpu_seconds: 240,
          stage1_local_heals: 3,
          stage2_cloud_escalations: 1,
          local_heal_success_rate_pct: 75,
        },
      },
      this_week: {
        requests: 42,
        tokens_total: 210000,
        tokens_local: 168000,
        cost_spent_usd: 0.63,
        cost_saved_usd: 3.57,
        cost_reduction_pct: 85,
        cycle_killer: {
          total_interventions: 12,
          avoided_runaway_tokens: 24000,
          avoided_gpu_seconds: 720,
          stage1_local_heals: 10,
          stage2_cloud_escalations: 2,
          local_heal_success_rate_pct: 83,
        },
      },
      this_month: {
        requests: 42,
        tokens_total: 210000,
        tokens_local: 168000,
        cost_spent_usd: 0.63,
        cost_saved_usd: 3.57,
        cost_reduction_pct: 85,
        cycle_killer: {
          total_interventions: 12,
          avoided_runaway_tokens: 24000,
          avoided_gpu_seconds: 720,
          stage1_local_heals: 10,
          stage2_cloud_escalations: 2,
          local_heal_success_rate_pct: 83,
        },
      },
      all_time: {
        requests: 100,
        tokens_total: 500000,
        tokens_local: 400000,
        cost_spent_usd: 1.5,
        cost_saved_usd: 8.5,
        cost_reduction_pct: 85,
        cycle_killer: {
          total_interventions: 12,
          avoided_runaway_tokens: 24000,
          avoided_gpu_seconds: 720,
          stage1_local_heals: 10,
          stage2_cloud_escalations: 2,
          local_heal_success_rate_pct: 83,
        },
      },
    },
    ...overrides,
  };
}

/**
 * Legacy daemon: windows present but no cycle_killer field on any window.
 * Root-level stats.cycle_killer holds all data.
 */
function makeLegacyStats(): unknown {
  return {
    total_requests: 50,
    total_tokens: 200000,
    total_tokens_routed_locally: 150000,
    total_cost_spent_usd: 0.8,
    estimated_cost_saved_usd: 4.2,
    cost_reduction_pct: 84,
    started_at: '2026-08-01T00:00:00Z',
    cycle_killer: {
      total_interventions: 7,
      avoided_runaway_tokens: 14000,
      avoided_gpu_seconds: 420,
      stage1_local_heals: 6,
      stage2_cloud_escalations: 1,
      local_heal_success_rate_pct: 86,
    },
    windows: {
      today:      { requests: 3,  tokens_total: 6000,   tokens_local: 4500 },
      yesterday:  { requests: 10, tokens_total: 40000,  tokens_local: 30000 },
      this_week:  { requests: 20, tokens_total: 80000,  tokens_local: 60000 },
      this_month: { requests: 40, tokens_total: 160000, tokens_local: 120000 },
      all_time:   { requests: 50, tokens_total: 200000, tokens_local: 150000 },
    },
  };
}

// ---------------------------------------------------------------------------
// Test harness helpers
// ---------------------------------------------------------------------------

function loadDashboardScript(): void {
  const scriptPath = path.resolve(
    __dirname,
    '../../../resources/webview/dashboard.js'
  );
  const src = fs.readFileSync(scriptPath, 'utf8');
  // eslint-disable-next-line no-eval
  eval(src);
}

function postMessage(command: string, data: unknown): void {
  window.dispatchEvent(
    new MessageEvent('message', { data: { command, data } })
  );
}

function buildDOM(): void {
  document.body.innerHTML = `
    <div id="stats-content"></div>
    <div id="stats-timeframe-info"></div>
    <div id="cycle-killer-content"></div>
    <div id="nts-content"></div>
    <div id="routes-content"></div>
    <div id="circuits-content"></div>
    <div id="deals-content"></div>
    <div id="config-content"></div>
    <div id="tuner-banner" style="display:none"></div>
    <span id="active-preset-badge"></span>
    <button id="btn-edit-config"><svg></svg> config.yaml</button>
    <button id="tab-past_1_hour"></button>
    <button id="tab-all_time"></button>
    <button id="tab-today"></button>
    <button id="tab-yesterday"></button>
    <button id="tab-this_week"></button>
    <button id="tab-this_month"></button>
    <button id="refresh-60s"></button>
    <button id="refresh-30s"></button>
    <button id="refresh-15s"></button>
    <button id="refresh-off"></button>
  `;
}

type DashboardWindow = Window & {
  acquireVsCodeApi: () => { getState: () => null; setState: () => void; postMessage: () => void };
  setTimeWindow: (w: string, notify?: boolean) => void;
};

// ---------------------------------------------------------------------------
// Jest setup
// ---------------------------------------------------------------------------

beforeEach(() => {
  buildDOM();

  (window as unknown as DashboardWindow).acquireVsCodeApi = jest.fn(() => ({
    getState: jest.fn().mockReturnValue(null),
    setState: jest.fn(),
    postMessage: jest.fn(),
  }));

  Object.defineProperty(window, 'localStorage', {
    value: {
      getItem: jest.fn().mockReturnValue(null),
      setItem: jest.fn(),
      removeItem: jest.fn(),
    },
    writable: true,
  });

  loadDashboardScript();
  document.dispatchEvent(new Event('DOMContentLoaded'));
});

function ckContent(): string {
  return document.getElementById('cycle-killer-content')?.innerHTML ?? '';
}

function setWindow(w: string): void {
  (window as unknown as DashboardWindow).setTimeWindow(w, false);
}

// ---------------------------------------------------------------------------
// Suite 1: correct window selection per timeframe (v0.8.4 daemon)
// ---------------------------------------------------------------------------

describe('windowCycleKiller — timeframe selection (v0.8.4 daemon, all windows present)', () => {
  beforeEach(() => {
    postMessage('updateStats', makeStats());
  });

  it('Today: renders 0 interventions when today.cycle_killer has zero', () => {
    setWindow('today');
    expect(ckContent()).toContain('0 <span class="ck-unit">Loops</span>');
    expect(ckContent()).not.toContain('>12 <span class="ck-unit">Loops</span>');
  });

  it('Today: timeframe label mentions Today', () => {
    setWindow('today');
    const info = document.getElementById('stats-timeframe-info')?.textContent ?? '';
    expect(info).toContain('Today');
  });

  it('Yesterday: renders 4 interventions from yesterday window', () => {
    setWindow('yesterday');
    expect(ckContent()).toContain('4 <span class="ck-unit">Loops</span>');
  });

  it('Yesterday: timeframe label mentions Yesterday', () => {
    setWindow('yesterday');
    const info = document.getElementById('stats-timeframe-info')?.textContent ?? '';
    expect(info).toContain('Yesterday');
  });

  it('This Week: renders 12 interventions from this_week window', () => {
    setWindow('this_week');
    expect(ckContent()).toContain('12 <span class="ck-unit">Loops</span>');
  });

  it('This Month: renders 12 interventions from this_month window', () => {
    setWindow('this_month');
    expect(ckContent()).toContain('12 <span class="ck-unit">Loops</span>');
  });

  it('All Time: renders 12 interventions from all_time window', () => {
    setWindow('all_time');
    expect(ckContent()).toContain('12 <span class="ck-unit">Loops</span>');
  });

  it('All Time → Today: re-renders to zero correctly', () => {
    setWindow('all_time');
    expect(ckContent()).toContain('12 <span class="ck-unit">Loops</span>');
    setWindow('today');
    expect(ckContent()).toContain('0 <span class="ck-unit">Loops</span>');
    expect(ckContent()).not.toContain('>12 <span class="ck-unit">Loops</span>');
  });

  it('Today → Yesterday: switches from zero to 4 correctly', () => {
    setWindow('today');
    expect(ckContent()).toContain('0 <span class="ck-unit">Loops</span>');
    setWindow('yesterday');
    expect(ckContent()).toContain('4 <span class="ck-unit">Loops</span>');
  });

  it('Today → All Time: restores 12', () => {
    setWindow('today');
    setWindow('all_time');
    expect(ckContent()).toContain('12 <span class="ck-unit">Loops</span>');
  });

  it('Past 1 Hour: renders 1 intervention and NTS tokens from past_1_hour window', () => {
    setWindow('past_1_hour');
    expect(ckContent()).toContain('1 <span class="ck-unit">Loops</span>');
    expect(document.getElementById('nts-content')?.innerHTML).toContain('+1.2k <span class="nts-unit">Tokens</span>');
  });

  it('Past 1 Hour: timeframe label mentions Past 1 Hour', () => {
    setWindow('past_1_hour');
    const info = document.getElementById('stats-timeframe-info')?.textContent ?? '';
    expect(info).toContain('Past 1 Hour');
  });

  it('renders supervisor sub-brackets with Qu\'est-ce que c\'est?, Kickstart, and Fairy Dust', () => {
    setWindow('all_time');
    expect(ckContent()).toContain("Cycle Killer (Qu'est-ce que c'est?): In-Flight Stream Breaker");
    expect(ckContent()).toContain('Kickstart: Stall Resuscitation Engine');
    expect(ckContent()).toContain('Fairy Dust: Programmable Quality Checkpoints');
  });
});

// ---------------------------------------------------------------------------
// Suite 2: legacy daemon fallback
// ---------------------------------------------------------------------------

describe('windowCycleKiller — legacy daemon fallback (window CK absent)', () => {
  beforeEach(() => {
    postMessage('updateStats', makeLegacyStats());
  });

  it('Today: falls back to root stats.cycle_killer (7) when window CK is absent', () => {
    setWindow('today');
    expect(ckContent()).toContain('7 <span class="ck-unit">Loops</span>');
  });

  it('Yesterday: falls back to root stats.cycle_killer when window CK is absent', () => {
    setWindow('yesterday');
    expect(ckContent()).toContain('7 <span class="ck-unit">Loops</span>');
  });

  it('This Week: falls back to root stats.cycle_killer when window CK is absent', () => {
    setWindow('this_week');
    expect(ckContent()).toContain('7 <span class="ck-unit">Loops</span>');
  });

  it('This Month: falls back to root stats.cycle_killer when window CK is absent', () => {
    setWindow('this_month');
    expect(ckContent()).toContain('7 <span class="ck-unit">Loops</span>');
  });

  it('All Time: falls back to root stats.cycle_killer when window CK is absent', () => {
    setWindow('all_time');
    expect(ckContent()).toContain('7 <span class="ck-unit">Loops</span>');
  });
});

// ---------------------------------------------------------------------------
// Suite 3: null / missing stats
// ---------------------------------------------------------------------------

describe('renderCycleKiller — null or incomplete stats', () => {
  it('ck-content stays empty when no updateStats has been called (renderStats returns early)', () => {
    // No postMessage('updateStats') — currentState.stats is null.
    // renderStats short-circuits before calling renderCycleKiller, so the
    // CK element retains its initial empty string from buildDOM().
    setWindow('today');
    expect(ckContent()).toBe('');
  });

  it('shows placeholder when stats has no windows and no root cycle_killer', () => {
    postMessage('updateStats', { total_requests: 0 });
    setWindow('today');
    expect(ckContent()).toContain('No defense telemetry recorded yet.');
  });
});


// ---------------------------------------------------------------------------
// Suite 4: tab active-state management
// ---------------------------------------------------------------------------

describe('setTimeWindow — tab button active class', () => {
  it('marks past_1_hour tab active and clears others', () => {
    setWindow('past_1_hour');
    expect(document.getElementById('tab-past_1_hour')?.classList.contains('active')).toBe(true);
    expect(document.getElementById('tab-today')?.classList.contains('active')).toBe(false);
    expect(document.getElementById('tab-all_time')?.classList.contains('active')).toBe(false);
  });

  it('marks yesterday tab active and clears others', () => {
    setWindow('yesterday');
    expect(document.getElementById('tab-yesterday')?.classList.contains('active')).toBe(true);
    expect(document.getElementById('tab-today')?.classList.contains('active')).toBe(false);
    expect(document.getElementById('tab-all_time')?.classList.contains('active')).toBe(false);
    expect(document.getElementById('tab-this_week')?.classList.contains('active')).toBe(false);
    expect(document.getElementById('tab-this_month')?.classList.contains('active')).toBe(false);
  });

  it('marks this_week tab active and clears others', () => {
    setWindow('this_week');
    expect(document.getElementById('tab-this_week')?.classList.contains('active')).toBe(true);
    expect(document.getElementById('tab-today')?.classList.contains('active')).toBe(false);
    expect(document.getElementById('tab-yesterday')?.classList.contains('active')).toBe(false);
    expect(document.getElementById('tab-all_time')?.classList.contains('active')).toBe(false);
    expect(document.getElementById('tab-this_month')?.classList.contains('active')).toBe(false);
  });

  it('moves active class from this_week to this_month', () => {
    setWindow('this_week');
    setWindow('this_month');
    expect(document.getElementById('tab-this_week')?.classList.contains('active')).toBe(false);
    expect(document.getElementById('tab-this_month')?.classList.contains('active')).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// Suite 5: regression guard — exact bug fixed in 5dd9865
// ---------------------------------------------------------------------------

describe('Regression: Today must not show all-time totals when today has 0 interventions', () => {
  it('today.cycle_killer.total_interventions=0 renders 0, never the all-time 12', () => {
    postMessage('updateStats', makeStats()); // today=0, all_time=12
    setWindow('today');
    expect(ckContent()).toContain('0 <span class="ck-unit">Loops</span>');
    expect(ckContent()).not.toMatch(/>\s*12\s*<span class="ck-unit">Loops<\/span>/);
  });
});

describe('updateActivePreset and profile / remote indicators', () => {
  it('updates badge and config edit button for local profile', () => {
    postMessage('updateActivePreset', { label: 'Profile 2', isRemote: false });
    expect(document.getElementById('active-preset-badge')?.textContent).toBe('📋 Profile 2');
    expect(document.getElementById('btn-edit-config')?.textContent).toContain('Profile 2 (YAML)');
  });

  it('updates badge and config edit button for remote server', () => {
    postMessage('updateActivePreset', { label: 'Remote Server', isRemote: true });
    expect(document.getElementById('active-preset-badge')?.textContent).toBe('🌐 Remote Server');
    expect(document.getElementById('btn-edit-config')?.textContent).toContain('Remote config.yaml');
  });
});

describe('setOffline command in webview', () => {
  it('clears stale remote data across all panels and displays offline placeholders', () => {
    // Populate with stats first
    postMessage('updateStats', makeStats());
    postMessage('updateRoutes', { routes: [{ id: '1', selected_tier: 'Tier 1' }] });
    expect(document.getElementById('cycle-killer-content')?.textContent).not.toContain('offline');

    // Post setOffline
    postMessage('setOffline', { reason: 'Local engine is offline (Click Start in sidebar)' });

    expect(document.getElementById('stats-content')?.textContent).toContain('Local engine is offline');
    expect(document.getElementById('cycle-killer-content')?.textContent).toContain('Engine is offline');
    expect(document.getElementById('nts-content')?.textContent).toContain('Engine is offline');
    expect(document.getElementById('routes-content')?.textContent).toContain('Engine is offline');
    expect(document.getElementById('circuits-content')?.textContent).toContain('Engine is offline');
    expect(document.getElementById('deals-content')?.textContent).toContain('Engine is offline');
    expect(document.getElementById('config-content')?.textContent).toContain('Engine is offline');
  });
});

describe('syncSnapshot SSOT render pipeline in webview', () => {
  it('renders complete live snapshot across all panels in local mode', () => {
    const liveSnapshot = {
      timestamp: 1000,
      engine: {
        mode: 'local',
        isOnline: true,
        activeProfile: 'profile1',
        profileLabel: 'Profile 1',
        isRemote: false
      },
      stats: makeStats(),
      routes: { routes: [{ id: 'r-1', selected_tier: 'Tier 1: Frontier Cloud', status_code: 200, latency_ms: 120, cost_saved_usd: 0.05 }] },
      circuits: { circuits: [{ name: 'openrouter', provider: 'openrouter', is_available: true, state: 'closed', failures: 0 }] },
      deals: { deals: [{ id: 'deal-1', model: 'qwen/qwen-2.5-coder-32b', provider: 'openrouter', savings_pct: 40 }] },
      config: {
        port: 8000,
        tiers: [{ name: 'Tier 1: Frontier Cloud', model: 'anthropic/claude-sonnet-5' }]
      },
      timeWindow: 'this_week',
      routesRefreshInterval: 30
    };

    postMessage('syncSnapshot', liveSnapshot);

    expect(document.getElementById('active-preset-badge')?.textContent).toBe('📋 Profile 1');
    expect(document.getElementById('btn-edit-config')?.textContent).toContain('Profile 1 (YAML)');
    expect(document.getElementById('tab-this_week')?.classList.contains('active')).toBe(true);
    expect(document.getElementById('refresh-30s')?.classList.contains('active')).toBe(true);
    expect(document.getElementById('stats-content')?.textContent).not.toContain('offline');
    expect(document.getElementById('routes-content')?.textContent).toContain('Tier 1: Frontier Cloud');
    expect(document.getElementById('circuits-content')?.textContent).toContain('openrouter');
    expect(document.getElementById('deals-content')?.textContent).toContain('qwen/qwen-2.5-coder-32b');
    expect(document.getElementById('config-content')?.textContent).toContain('Tier 1: Frontier Cloud');
  });

  it('renders online remote server snapshot with remote indicators', () => {
    const remoteSnapshot = {
      timestamp: 2000,
      engine: {
        mode: 'remote',
        isOnline: true,
        activeProfile: 'profile1',
        profileLabel: 'Remote Server',
        isRemote: true
      },
      stats: makeStats(),
      routes: { routes: [] },
      circuits: { circuits: [] },
      deals: { deals: [] },
      config: null
    };

    postMessage('syncSnapshot', remoteSnapshot);

    expect(document.getElementById('active-preset-badge')?.textContent).toBe('🌐 Remote Server');
    expect(document.getElementById('btn-edit-config')?.textContent).toContain('Remote config.yaml');
  });

  it('atomically transitions from live remote to offline local and purges all residual data', () => {
    // 1. Send live remote snapshot
    postMessage('syncSnapshot', {
      timestamp: 3000,
      engine: {
        mode: 'remote',
        isOnline: true,
        activeProfile: 'profile1',
        profileLabel: 'Remote Server',
        isRemote: true
      },
      stats: makeStats(),
      routes: { routes: [{ id: 'rem-1', selected_tier: 'Remote Tier' }] },
      circuits: { circuits: [{ name: 'remote-gw', state: 'closed' }] },
      deals: { deals: [{ id: 'deal-rem', model: 'cloud-model' }] },
      config: { port: 8000 }
    });

    expect(document.getElementById('active-preset-badge')?.textContent).toBe('🌐 Remote Server');
    expect(document.getElementById('stats-content')?.textContent).not.toContain('offline');

    // 2. Send offline local snapshot
    postMessage('syncSnapshot', {
      timestamp: 4000,
      engine: {
        mode: 'local',
        isOnline: false,
        activeProfile: 'profile1',
        profileLabel: 'Profile 1',
        isRemote: false,
        offlineReason: 'Local engine is offline (Click Start in sidebar)'
      },
      stats: null,
      routes: { routes: [] },
      circuits: { circuits: [] },
      deals: { deals: [] },
      config: null
    });

    // Verify all 6 panels instantly show offline status and badge reflects Profile 1
    expect(document.getElementById('active-preset-badge')?.textContent).toBe('📋 Profile 1');
    expect(document.getElementById('btn-edit-config')?.textContent).toContain('Profile 1 (YAML)');
    expect(document.getElementById('stats-content')?.textContent).toContain('Local engine is offline');
    expect(document.getElementById('cycle-killer-content')?.textContent).toContain('Engine is offline');
    expect(document.getElementById('routes-content')?.textContent).toContain('Engine is offline');
    expect(document.getElementById('circuits-content')?.textContent).toContain('Engine is offline');
    expect(document.getElementById('deals-content')?.textContent).toContain('Engine is offline');
    expect(document.getElementById('config-content')?.textContent).toContain('Engine is offline');
  });

  it('rejects stale out-of-order snapshots with older timestamps', () => {
    postMessage('syncSnapshot', {
      timestamp: 5000,
      engine: {
        mode: 'local',
        isOnline: true,
        activeProfile: 'profile3',
        profileLabel: 'Profile 3',
        isRemote: false
      }
    });
    expect(document.getElementById('active-preset-badge')?.textContent).toBe('📋 Profile 3');

    // Stale snapshot with timestamp 4500
    postMessage('syncSnapshot', {
      timestamp: 4500,
      engine: {
        mode: 'local',
        isOnline: true,
        activeProfile: 'profile1',
        profileLabel: 'Profile 1',
        isRemote: false
      }
    });

    // Must remain Profile 3
    expect(document.getElementById('active-preset-badge')?.textContent).toBe('📋 Profile 3');
  });
});



