(function () {
	const vscode = acquireVsCodeApi();

	// State management
	let currentState = vscode.getState() || { routes: [], circuits: [], stats: null, deals: null, optimization: null };

	// Handle messages from the extension
	window.addEventListener('message', event => {
		const message = event.data;
		switch (message.command) {
			case 'updateStats':
				updateStats(message.data);
				break;
			case 'updateDeals':
				updateDeals(message.data);
				break;
			case 'updateRoutes':
				updateRoutes(message.data);
				break;
			case 'updateCircuits':
				updateCircuits(message.data);
				break;
			case 'updateConfig':
				updateConfig(message.data);
				break;
			case 'updateOptimization':
				updateOptimization(message.data);
				break;
			case 'setTimeWindow':
				if (message.data && message.data.timeWindow) {
					window.setTimeWindow(message.data.timeWindow, false);
				}
				break;
			case 'setRoutesRefreshInterval':
				if (message.data && typeof message.data.interval !== 'undefined') {
					window.setRoutesRefreshInterval(message.data.interval, false);
				}
				break;
		}
	});

	let activeTimeWindow = (currentState && currentState.timeWindow) || (function() {
		try { return localStorage.getItem('nacho_flow_time_window'); } catch(_) { return null; }
	})() || 'all_time';

	let activeRefreshInterval = (currentState && typeof currentState.routesRefreshInterval !== 'undefined') 
		? currentState.routesRefreshInterval 
		: (function() {
			try { 
				const val = localStorage.getItem('nacho_flow_routes_refresh_interval');
				return val !== null ? parseInt(val, 10) : 60;
			} catch(_) { return 60; }
		})() ?? 60;

	window.setTimeWindow = function(windowKey, notifyExtension = true) {
		activeTimeWindow = windowKey;
		currentState.timeWindow = windowKey;
		vscode.setState(currentState);
		try { localStorage.setItem('nacho_flow_time_window', windowKey); } catch (_) {}

		['all_time', 'today', 'yesterday', 'this_week', 'this_month'].forEach(k => {
			const btn = document.getElementById(`tab-${k}`);
			if (btn) {
				if (k === windowKey) btn.classList.add('active');
				else btn.classList.remove('active');
			}
		});

		if (currentState.stats) {
			renderStats(currentState.stats);
		}

		if (notifyExtension) {
			vscode.postMessage({ command: 'setTimeWindow', timeWindow: windowKey });
		}
	};

	window.setRoutesRefreshInterval = function(intervalSec, notifyExtension = true) {
		activeRefreshInterval = Number(intervalSec);
		currentState.routesRefreshInterval = activeRefreshInterval;
		vscode.setState(currentState);
		try { localStorage.setItem('nacho_flow_routes_refresh_interval', activeRefreshInterval.toString()); } catch (_) {}

		const btnMap = { 60: 'refresh-60s', 30: 'refresh-30s', 15: 'refresh-15s', 0: 'refresh-off' };
		Object.entries(btnMap).forEach(([sec, btnId]) => {
			const btn = document.getElementById(btnId);
			if (btn) {
				if (Number(sec) === activeRefreshInterval) btn.classList.add('active');
				else btn.classList.remove('active');
			}
		});

		if (notifyExtension) {
			vscode.postMessage({ command: 'setRoutesRefreshInterval', interval: activeRefreshInterval });
		}
	};

	function updateStats(stats) {
		currentState.stats = stats;
		vscode.setState(currentState);
		renderStats(stats);
	}

	function renderStats(stats) {
		const statsContent = document.getElementById('stats-content');
		const timeframeInfo = document.getElementById('stats-timeframe-info');
		if (!stats) {
			if (statsContent) statsContent.innerHTML = '<div class="loading">Connecting to Nacho Flow daemon...</div>';
			return;
		}

		let totalReqs = 0;
		let localReqs = 0;
		let totalTokens = 0;
		let localTokens = 0;
		let spentUSD = 0;
		let savedUSD = 0;
		let reductionPct = 0;
		let timeframeLabel = '';

		const startedDate = stats.started_at ? new Date(stats.started_at).toLocaleString() : 'Engine startup';

		if (activeTimeWindow === 'today') {
			const w = stats.windows?.today;
			totalReqs = w?.requests || 0;
			totalTokens = w?.tokens_total || 0;
			localTokens = w?.tokens_local || 0;
			spentUSD = w?.cost_spent_usd || 0;
			savedUSD = w?.cost_saved_usd || 0;
			reductionPct = w?.cost_reduction_pct || ((savedUSD + spentUSD) > 0 ? Math.round((savedUSD / (savedUSD + spentUSD)) * 100) : 0);
			localReqs = totalTokens > 0 ? Math.round((localTokens / totalTokens) * totalReqs) : 0;
			timeframeLabel = `📅 Today's Telemetry (Active 24h rolling UTC window)`;
		} else if (activeTimeWindow === 'yesterday') {
			const w = stats.windows?.yesterday;
			totalReqs = w?.requests || 0;
			totalTokens = w?.tokens_total || 0;
			localTokens = w?.tokens_local || 0;
			spentUSD = w?.cost_spent_usd || 0;
			savedUSD = w?.cost_saved_usd || 0;
			reductionPct = w?.cost_reduction_pct || ((savedUSD + spentUSD) > 0 ? Math.round((savedUSD / (savedUSD + spentUSD)) * 100) : 0);
			localReqs = totalTokens > 0 ? Math.round((localTokens / totalTokens) * totalReqs) : 0;
			timeframeLabel = `📅 Yesterday's Telemetry (Prior 24h UTC window)`;
		} else if (activeTimeWindow === 'this_week') {
			const w = stats.windows?.this_week;
			totalReqs = w?.requests || 0;
			totalTokens = w?.tokens_total || 0;
			localTokens = w?.tokens_local || 0;
			spentUSD = w?.cost_spent_usd || 0;
			savedUSD = w?.cost_saved_usd || 0;
			reductionPct = w?.cost_reduction_pct || ((savedUSD + spentUSD) > 0 ? Math.round((savedUSD / (savedUSD + spentUSD)) * 100) : 0);
			localReqs = totalTokens > 0 ? Math.round((localTokens / totalTokens) * totalReqs) : 0;
			timeframeLabel = `📆 This Week's Telemetry (Current ISO week)`;
		} else if (activeTimeWindow === 'this_month') {
			const w = stats.windows?.this_month;
			totalReqs = w?.requests || 0;
			totalTokens = w?.tokens_total || 0;
			localTokens = w?.tokens_local || 0;
			spentUSD = w?.cost_spent_usd || 0;
			savedUSD = w?.cost_saved_usd || 0;
			reductionPct = w?.cost_reduction_pct || ((savedUSD + spentUSD) > 0 ? Math.round((savedUSD / (savedUSD + spentUSD)) * 100) : 0);
			localReqs = totalTokens > 0 ? Math.round((localTokens / totalTokens) * totalReqs) : 0;
			timeframeLabel = `🗓️ This Month's Telemetry (Current calendar month)`;
		} else {
			// All Time
			const w = stats.windows?.all_time;
			totalReqs = w?.requests || stats.total_requests || 0;
			totalTokens = w?.tokens_total || stats.total_tokens || stats.all_time_tokens || 0;
			localTokens = w?.tokens_local || stats.total_tokens_routed_locally || 0;
			spentUSD = w?.cost_spent_usd || stats.total_cost_spent_usd || 0;
			savedUSD = w?.cost_saved_usd || stats.estimated_cost_saved_usd || 0;
			reductionPct = w?.cost_reduction_pct || stats.cost_reduction_pct || ((savedUSD + spentUSD) > 0 ? Math.round((savedUSD / (savedUSD + spentUSD)) * 100) : 0);
			localReqs = stats.tier_breakdown?.tier1_local_free || (totalTokens > 0 ? Math.round((localTokens / totalTokens) * totalReqs) : 0);
			timeframeLabel = `⚡ All-Time Cumulative Telemetry since engine started (${startedDate})`;
		}

		if (timeframeInfo) {
			timeframeInfo.textContent = timeframeLabel;
		}

		const localPct = totalTokens > 0 ? Math.round((localTokens / totalTokens) * 100) : (totalReqs > 0 ? Math.round((localReqs / totalReqs) * 100) : 0);
		const localTokenStr = localTokens >= 1000000 ? (localTokens / 1000000).toFixed(1) + 'M' : (localTokens >= 1000 ? (localTokens / 1000).toFixed(0) + 'k' : localTokens.toString());

		if (statsContent) {
			statsContent.innerHTML = `
				<div class="stat-grid">
					<div class="stat-item highlight">
						<div class="stat-value">$${savedUSD.toFixed(2)}</div>
						<div class="stat-label">💵 Est. Cost Saved</div>
						<div class="stat-sub">${Math.round(reductionPct)}% saved vs. direct cloud (claude-sonnet-5 baseline)</div>
					</div>
					<div class="stat-item spent-chip">
						<div class="stat-value">$${spentUSD.toFixed(2)}</div>
						<div class="stat-label">📉 Cloud API Spend</div>
						<div class="stat-sub">Billed for escalated reasoning turns</div>
					</div>
					<div class="stat-item local-chip">
						<div class="stat-value">${localReqs} <span class="stat-unit">turns (${localPct}%)</span></div>
						<div class="stat-label">⚡ Local GPU ($0.00)</div>
						<div class="stat-sub">${localTokens > 0 ? localTokenStr + ' tokens routed locally for free' : 'Zero cloud cost'}</div>
					</div>
					<div class="stat-item volume-chip">
						<div class="stat-value">${totalReqs} <span class="stat-unit">turns</span></div>
						<div class="stat-label">🪙 Prompt Turns & Volume</div>
						<div class="stat-sub">${totalTokens.toLocaleString()} tokens processed across all tiers</div>
					</div>
				</div>
			`;
		}

		// ─── FOOTGUN WARNING (for LLM agents and contributors) ──────────────────────
		// The /v1/stats payload has TWO locations for CycleKiller AND FairyDust data:
		//
		//   stats.cycle_killer              ← ROOT-LEVEL all-time global accumulator.
		//   stats.fairy_dust               ← ROOT-LEVEL all-time global accumulator.
		//                                     NEVER use these for windowed display.
		//   stats.windows.<window>.cycle_killer  ← Per-window object (correct source).
		//   stats.windows.<window>.fairy_dust    ← Per-window object (correct source).
		//
		// renderDefensePanel() must ALWAYS be called via windowCycleKiller() and
		// windowFairyDust() below, never directly with the root-level fields.
		// Using the root-level fields is a recurring bug: it makes Today/Yesterday
		// show all-time cumulative totals regardless of the active tab.
		// ─────────────────────────────────────────────────────────────────────────────

		// windowCycleKiller: returns the per-window CK object when it exists (including
		// when it is legitimately zero — a quiet window is not the same as "no data").
		// Falls back to the root accumulator ONLY for legacy daemons (pre-v0.8.4) that
		// never populated per-window CK fields.
		function windowCycleKiller(windowCK) {
			if (windowCK != null) {
				return windowCK;
			}
			// Legacy daemon: window object was never populated — fall back to root accumulator.
			return stats.cycle_killer ?? null;
		}

		// windowFairyDust: same per-window selection logic for FairyDust metrics.
		// Falls back to root stats.fairy_dust ONLY for legacy daemons.
		function windowFairyDust(windowFD) {
			if (windowFD != null) {
				return windowFD;
			}
			return stats.fairy_dust ?? null;
		}

		let currentCycleKiller = null;
		let currentFairyDust = null;
		if (activeTimeWindow === 'today') {
			currentCycleKiller = windowCycleKiller(stats.windows?.today?.cycle_killer);
			currentFairyDust = windowFairyDust(stats.windows?.today?.fairy_dust);
		} else if (activeTimeWindow === 'yesterday') {
			currentCycleKiller = windowCycleKiller(stats.windows?.yesterday?.cycle_killer);
			currentFairyDust = windowFairyDust(stats.windows?.yesterday?.fairy_dust);
		} else if (activeTimeWindow === 'this_week') {
			currentCycleKiller = windowCycleKiller(stats.windows?.this_week?.cycle_killer);
			currentFairyDust = windowFairyDust(stats.windows?.this_week?.fairy_dust);
		} else if (activeTimeWindow === 'this_month') {
			currentCycleKiller = windowCycleKiller(stats.windows?.this_month?.cycle_killer);
			currentFairyDust = windowFairyDust(stats.windows?.this_month?.fairy_dust);
		} else {
			currentCycleKiller = windowCycleKiller(stats.windows?.all_time?.cycle_killer);
			currentFairyDust = windowFairyDust(stats.windows?.all_time?.fairy_dust);
		}

		renderDefensePanel(currentCycleKiller, currentFairyDust);
	}

	// ─── renderDefensePanel ──────────────────────────────────────────────────────
	// ALWAYS call this function through renderStats() → windowCycleKiller() and
	// windowFairyDust(), which select the correct per-window objects based on
	// activeTimeWindow. Direct calls with root-level accumulators are a bug.
	// ─────────────────────────────────────────────────────────────────────────────
	function renderDefensePanel(ck, fd) {
		const ckContent = document.getElementById('cycle-killer-content');
		if (!ckContent) return;
		if (!ck) {
			ckContent.innerHTML = '<div class="loading">No defense telemetry recorded yet.</div>';
			return;
		}

		const totalInterventions = ck.total_interventions || 0;
		const avoidedTokens = ck.avoided_runaway_tokens || 0;
		const avoidedGPUSeconds = ck.avoided_gpu_seconds || 0;
		const stage1Heals = ck.stage1_local_heals || 0;
		const stage2Escalations = ck.stage2_cloud_escalations || 0;
		const kickstarts = ck.session_kickstarts || 0;
		const healRate = ck.local_heal_success_rate_pct !== undefined
			? ck.local_heal_success_rate_pct
			: (totalInterventions > 0 ? (stage1Heals / totalInterventions) * 100 : 0);
		const fairyTriggers = fd ? (fd.total_triggers || 0) : 0;

		const gpuMinutes = (avoidedGPUSeconds / 60).toFixed(1);
		const avoidedTokensStr = avoidedTokens >= 1000000
			? (avoidedTokens / 1000000).toFixed(1) + 'M'
			: (avoidedTokens >= 1000 ? (avoidedTokens / 1000).toFixed(0) + 'k' : avoidedTokens.toString());

		// Calculate verified clean streams from recent route telemetry
		let verifiedCleanCount = 0;
		let peakObservedNgram = 1;
		const routesList = (currentState.routes && currentState.routes.routes) ? currentState.routes.routes : (Array.isArray(currentState.routes) ? currentState.routes : []);
		routesList.forEach(r => {
			if (!r.cycle_breaker_triggered && (r.cycle_prose_tokens > 0 || r.cycle_max_ngram_freq > 0)) {
				verifiedCleanCount++;
				if (r.cycle_max_ngram_freq && r.cycle_max_ngram_freq > peakObservedNgram) {
					peakObservedNgram = r.cycle_max_ngram_freq;
				}
			}
		});

		const statusPillText = totalInterventions > 0
			? `${totalInterventions} Loops Intercepted &bull; Stage 1 Heals: ${stage1Heals} &bull; Stage 2 Escalations: ${stage2Escalations}`
			: (verifiedCleanCount > 0
				? `Active Defense: ${verifiedCleanCount} Streams Verified Clean &bull; Peak N-Gram: ${peakObservedNgram}x &bull; 0 False Positives`
				: 'Watchdog Active &bull; 0 Degeneracies Detected');

		const loopsLabel = totalInterventions > 0
			? `${totalInterventions} <span class="ck-unit">Loops</span>`
			: (verifiedCleanCount > 0 ? `0 <span class="ck-unit">(${verifiedCleanCount} Clean)</span>` : '0 <span class="ck-unit">Loops</span>');

		ckContent.innerHTML = `
			<div class="cycle-killer-grid">
				<div class="ck-item ${totalInterventions > 0 ? 'highlight' : 'heal-chip'}">
					<div class="ck-value">${loopsLabel}</div>
					<div class="ck-label">🛡️ Interventions Executed</div>
					<div class="ck-sub">${totalInterventions > 0 ? 'Runaway monologues & deliberation loops intercepted' : 'Zero false positives on unique code streams'}</div>
				</div>
				<div class="ck-item gpu-chip">
					<div class="ck-value">${gpuMinutes} <span class="ck-unit">Min</span></div>
					<div class="ck-label">⏱️ GPU Lockup Rescued</div>
					<div class="ck-sub">Compute saved from infinite token generation</div>
				</div>
				<div class="ck-item token-chip">
					<div class="ck-value">${avoidedTokensStr} <span class="ck-unit">Tokens</span></div>
					<div class="ck-label">🪙 Avoided Runaway Tokens</div>
					<div class="ck-sub">$0.00 compute waste prevented before context burn</div>
				</div>
				<div class="ck-item heal-chip">
					<div class="ck-value">${Math.round(healRate)}% <span class="ck-unit">(${stage1Heals}/${totalInterventions})</span></div>
					<div class="ck-label">⚡ Stage 1 Local Heal Rate</div>
					<div class="ck-sub">Steered with [SYSTEM OVERRIDE] @ $0.00</div>
				</div>
				<div class="ck-item kickstart-chip">
					<div class="ck-value">${kickstarts} <span class="ck-unit">Sessions</span></div>
					<div class="ck-label">🚀 Kickstart Escalations</div>
					<div class="ck-sub">Tool-less sessions rescued via frontier reasoning injection</div>
				</div>
				<div class="ck-item fairy-chip">
					<div class="ck-value">${fairyTriggers} <span class="ck-unit">Checkpoints</span></div>
					<div class="ck-label">✨ Fairy Dust Injections</div>
					<div class="ck-sub">Proactive thinking model checkpoints fired on write/turn 1</div>
				</div>
			</div>
			<div class="ck-footer-row">
				<div class="ck-status-pill ${totalInterventions > 0 ? 'active' : 'idle'}">
					<span class="status-dot"></span>
					${statusPillText}
				</div>
			</div>
		`;
	}

	function updateDeals(dealsData) {
		currentState.deals = dealsData;
		vscode.setState(currentState);

		const dealsContent = document.getElementById('deals-content');
		if (!dealsData) {
			dealsContent.innerHTML = '<div class="loading">Loading live deals from OpenRouter...</div>';
			return;
		}

		const deals = dealsData.deals || [];
		if (deals.length === 0) {
			dealsContent.innerHTML = '<div class="loading">No active deals found matching alert thresholds. Benchmark: $' + (dealsData.benchmark_cost_per_m || 3.00).toFixed(2) + '/1M</div>';
			return;
		}

		const benchmark = dealsData.benchmark_cost_per_m || 3.00;
		const dealCards = deals.map(deal => {
			const modelName = deal.model_id || deal.name || deal.model || 'Unknown Model';
			const isFree = deal.is_free || (deal.prompt_cost_per_m === 0 && deal.completion_cost_per_m === 0);
			let discountText = '0% OFF';
			if (isFree) {
				discountText = '100% FREE';
			} else if (deal.discount_pct && deal.discount_pct > 0) {
				discountText = `${Math.min(99, Math.round(deal.discount_pct))}% OFF`;
			}
			const promptCost = isFree ? '$0.00' : (deal.prompt_cost_per_m !== undefined && deal.prompt_cost_per_m >= 0 ? `$${deal.prompt_cost_per_m.toFixed(2)}` : '$0.00');
			const compCost = isFree ? '$0.00' : (deal.completion_cost_per_m !== undefined && deal.completion_cost_per_m >= 0 ? `$${deal.completion_cost_per_m.toFixed(2)}` : '$0.00');
			const toolsBadge = deal.supports_tools ? '<span class="badge badge-tools">🔧 Tools</span>' : '';
			const scoreBadge = deal.coding_index ? `<span class="badge badge-score">🧠 Index ${deal.coding_index.toFixed(1)}</span>` : '';
			const provider = deal.provider || 'openrouter';
			const recTiers = deal.recommended_tiers || deal.recommended_tier ? (Array.isArray(deal.recommended_tiers) ? deal.recommended_tiers : [deal.recommended_tier]) : [];
			const escapedModel = modelName.replace(/'/g, "\\'");
			const escapedProvider = provider.replace(/'/g, "\\'");
			const recTiersJson = JSON.stringify(recTiers).replace(/'/g, "\\'");

			return `
				<div class="deal-card">
					<div class="deal-card-header">
						<span class="deal-model">${modelName}</span>
						<span class="badge badge-deal">${discountText}</span>
					</div>
					<div class="deal-pricing">
						<div>Input: <strong>${promptCost}/1M</strong></div>
						<div>Output: <strong>${compCost}/1M</strong></div>
					</div>
					<div class="deal-badges">
						${toolsBadge}
						${scoreBadge}
						<span class="badge badge-provider">${provider}</span>
					</div>
					<div class="deal-actions">
						<button class="btn btn-deal-copy" onclick="copyModelId('${escapedModel}')" title="Copy model ID to clipboard">📋 Copy</button>
						<button class="btn btn-deal-adopt" onclick="adoptDeal('${escapedModel}', '${escapedProvider}', ${recTiersJson})" title="Adopt into Nacho Flow routing tier">⚡ Adopt</button>
					</div>
				</div>
			`;
		}).join('');

		dealsContent.innerHTML = `
			<div class="deals-summary-bar">
				<span>Frontier Benchmark: <strong>$${benchmark.toFixed(2)}/1M</strong></span>
				<span><strong>${deals.length}</strong> discount models discovered (replaces expensive cloud tiers)</span>
			</div>
			<div class="deals-grid">
				${dealCards}
			</div>
		`;
	}

	function updateOptimization(optData) {
		currentState.optimization = optData;
		vscode.setState(currentState);

		const banner = document.getElementById('tuner-banner');
		if (!optData) {
			banner.style.display = 'none';
			return;
		}

		const savingsVal = optData.projected_savings_usd !== undefined ? optData.projected_savings_usd : (optData.projected_savings || 0);
		const savingsFormatted = typeof savingsVal === 'number' ? `+$${savingsVal.toFixed(2)}` : savingsVal;
		const rule = optData.synthesized_rule || optData.rule || 'Tokens < 64000 && Retries == 0';
		const tierName = optData.target_tier_name || 'Tier 1 (Local GPU)';
		const sampleSize = optData.total_sample_turns || optData.sample_size || 0;

		banner.style.display = 'block';
		banner.innerHTML = `
			<div class="tuner-result">
				<div class="tuner-header">
					<div class="tuner-title-group">
						<h3>⚡ Auto-Tuner Policy Recommendation</h3>
						<span class="badge badge-deal">Optimal Policy Ready</span>
					</div>
					${savingsVal > 0 ? `<div class="tuner-savings-badge">Projected Savings: <strong>${savingsFormatted}</strong></div>` : ''}
				</div>
				<div class="tuner-details-grid">
					<div class="tuner-detail-item">
						<span class="tuner-detail-label">🎯 Target Tier:</span>
						<strong class="tuner-detail-val">${tierName}</strong>
					</div>
					<div class="tuner-detail-item">
						<span class="tuner-detail-label">📊 Sample Size:</span>
						<span class="tuner-detail-val">${sampleSize} real turns analyzed</span>
					</div>
					<div class="tuner-detail-item full-width">
						<span class="tuner-detail-label">✨ Synthesized AST Rule:</span>
						<code class="tuner-rule-code">${rule}</code>
					</div>
				</div>
				<div class="tuner-actions">
					<button class="btn btn-primary btn-glow" onclick="applyOptimization()">Apply Optimized Policy to config.yaml</button>
					<button class="btn btn-secondary" onclick="dismissTuner()">Dismiss</button>
				</div>
			</div>
		`;
	}

	function updateRoutes(routesData) {
		currentState.routes = routesData;
		vscode.setState(currentState);

		const routesContent = document.getElementById('routes-content');
		if (!routesData || !routesData.routes) {
			routesContent.innerHTML = '<div class="loading">Loading route history...</div>';
			return;
		}

		const routes = routesData.routes.slice(0, 10);
		if (routes.length === 0) {
			routesContent.innerHTML = '<div class="loading">No route history recorded yet. Route requests from Zoo Code, Cline, or Cursor to begin logging!</div>';
			return;
		}

		const tableRows = routes.map(route => {
			const isLocal = route.is_local ? 'badge-local' : 'badge-cloud';
			const badgeText = route.is_local ? 'Local' : 'Cloud';
			const fallbackBadge = route.is_fallback ? '<span class="badge badge-fallback">Fallback</span>' : '';

			let cycleBadge = '';
			if (route.cycle_breaker_triggered) {
				const reason = route.cycle_breaker_reason || 'runaway loop';
				const nFreq = route.cycle_max_ngram_freq ? ` (${route.cycle_max_ngram_freq}x)` : '';
				cycleBadge = `<span class="badge badge-cycle" title="⚠️ Cycle Killer Intercepted: ${reason}${nFreq}">⚠️ Loop${nFreq}</span>`;
			} else if ((route.cycle_prose_tokens && route.cycle_prose_tokens > 0) || (route.cycle_max_ngram_freq && route.cycle_max_ngram_freq > 0)) {
				const proseTk = (route.cycle_prose_tokens || 0).toLocaleString();
				const nFreq = route.cycle_max_ngram_freq || 1;
				cycleBadge = `<span class="badge badge-cycle-clean" title="🛡️ Cycle Defense Verified: ${proseTk} prose tokens, max N-gram: ${nFreq}x (Clean Pass)">🛡️ Clean ${nFreq}x</span>`;
			} else {
				cycleBadge = `<span class="badge-cycle-none">--</span>`;
			}

			const fairyBadge = route.fairy_dusted
				? `<span class="badge badge-fairy" title="✨ Fairy Dust: Proactive thinking model checkpoint fired${route.fairy_dust_entry ? ' (' + route.fairy_dust_entry + ')' : ''}">✨ Fairy</span>`
				: '';
			const kickstartBadge = route.session_kickstarted
				? `<span class="badge badge-kickstart" title="🚀 Kickstart: Tool-less session escalated to frontier reasoning model">🚀 Kick</span>`
				: '';

			return `
				<tr>
					<td>${new Date(route.timestamp).toLocaleTimeString()}</td>
					<td><strong>${route.selected_tier}</strong></td>
					<td><span class="badge ${isLocal}">${badgeText}</span> ${fallbackBadge}</td>
					<td>${(route.tokens || 0).toLocaleString()}</td>
					<td>${(route.latency_ms || 0).toFixed(0)}ms</td>
					<td>${cycleBadge}</td>
					<td>${fairyBadge}${kickstartBadge}</td>
					<td class="saved-val">$${(route.cost_saved_usd || 0).toFixed(4)}</td>
				</tr>
			`;
		}).join('');

		routesContent.innerHTML = `
			<table class="table">
				<thead>
					<tr>
						<th>Time</th>
						<th>Tier</th>
						<th>Type</th>
						<th>Tokens</th>
						<th>Latency</th>
						<th>Cycle Shield</th>
						<th>Proactive</th>
						<th>Saved</th>
					</tr>
				</thead>
				<tbody>
					${tableRows}
				</tbody>
			</table>
		`;

		// Re-render the full stats panel (which correctly picks the per-window CK
		// and FD objects via windowCycleKiller/windowFairyDust). Do NOT call
		// renderDefensePanel() directly here — that was the recurring bug that made
		// Cycle Killer always show all-time totals whenever route data refreshed.
		if (currentState.stats) {
			renderStats(currentState.stats);
		}
	}

	function updateCircuits(circuitsData) {
		const circuitsContent = document.getElementById('circuits-content');
		if (!circuitsData || !circuitsData.circuits) {
			circuitsContent.innerHTML = '<div class="loading">Loading circuit status...</div>';
			return;
		}

		const circuits = circuitsData.circuits;
		if (circuits.length === 0) {
			circuitsContent.innerHTML = '<div class="loading">No circuits configured.</div>';
			return;
		}

		const circuitCards = circuits.map(circuit => {
			const isAvailable = circuit.is_available ? 'badge-closed' : 'badge-open';
			const statusText = circuit.is_available ? 'Healthy (Closed)' : 'Tripped (Open)';
			
			return `
				<div class="circuit-card">
					<div class="circuit-header">
						<h3>${circuit.provider}</h3>
						<span class="badge ${isAvailable}">${statusText}</span>
					</div>
					<p>Failures: <strong>${circuit.failures || 0} / ${circuit.failure_threshold || 5}</strong></p>
					<button class="btn btn-secondary" onclick="resetCircuit('${circuit.provider}')">🔄 Reset Circuit</button>
				</div>
			`;
		}).join('');

		circuitsContent.innerHTML = `<div class="circuits-grid">${circuitCards}</div>`;
	}

	function updateConfig(config) {
		const configContent = document.getElementById('config-content');
		if (!config) {
			configContent.innerHTML = '<div class="loading">Loading configuration...</div>';
			return;
		}

		const conditionalTiers = config.tiers || [];
		const allTiers = [...conditionalTiers];
		if (config.default_tier) {
			allTiers.push({ ...config.default_tier, isDefault: true });
		}

		const tierList = allTiers.map((t, idx) => {
			const displayName = t.name.startsWith(`Tier ${idx + 1}`) ? t.name : `Tier ${idx + 1}: ${t.name}`;
			const badge = t.isDefault ? '<span class="badge badge-fallback" style="margin-left: 6px;">Default Rescue</span>' : '';
			return `
			<div class="tier-item">
				<div style="display: flex; align-items: center; justify-content: space-between;">
					<strong>${displayName}</strong>
					${badge}
				</div>
				<div class="tier-meta">Model: <code>${t.model}</code> | Provider: <code>${t.provider}</code></div>
				<div class="tier-condition">When: <code>${t.when || 'true'}</code></div>
			</div>
			`;
		}).join('');

		configContent.innerHTML = `
			<div class="config-summary">
				<div class="config-meta-row">
					<span>Listening Port: <strong>${config.port || 8000}</strong></span>
					<span>Active Tiers: <strong>${allTiers.length}</strong></span>
				</div>
				<div class="tiers-list">${tierList}</div>
				<button class="btn btn-primary" onclick="editConfig()" style="margin-top: 12px;">⚙️ Edit config.yaml</button>
			</div>
		`;
	}

	// Exposed global functions
	window.copyModelId = function(modelId) {
		vscode.postMessage({ command: 'copyToClipboard', data: { text: modelId } });
	};

	window.adoptDeal = function(modelId, provider, recTiers) {
		const recommendedTiers = Array.isArray(recTiers) ? recTiers : (recTiers ? [recTiers] : []);
		vscode.postMessage({
			command: 'adoptDeal',
			data: {
				modelId: modelId,
				provider: provider,
				recommendedTiers: recommendedTiers
			}
		});
	};

	window.runOptimizer = function() {
		const banner = document.getElementById('tuner-banner');
		if (banner) {
			banner.style.display = 'block';
			banner.innerHTML = '<div class="loading">⚡ Running autonomous optimizer on telemetry observations...</div>';
		}
		vscode.postMessage({ command: 'runOptimizer' });
	};

	window.refreshDeals = function() {
		vscode.postMessage({ command: 'refreshDeals' });
	};

	window.refreshAll = function() {
		vscode.postMessage({ command: 'refreshAll' });
	};

	window.openSettings = function() {
		vscode.postMessage({ command: 'openSettings' });
	};

	window.editConfig = function() {
		vscode.postMessage({ command: 'editConfig' });
	};

	window.resetCircuit = function(provider) {
		vscode.postMessage({ command: 'resetCircuit', provider: provider });
	};

	window.applyOptimization = function() {
		vscode.postMessage({
			command: 'applyOptimization',
			data: currentState.optimization || undefined
		});
	};

	window.dismissTuner = function() {
		currentState.optimization = null;
		vscode.setState(currentState);
		const banner = document.getElementById('tuner-banner');
		if (banner) banner.style.display = 'none';
	};

	// Initialize
	document.addEventListener('DOMContentLoaded', () => {
		window.setTimeWindow(activeTimeWindow);
		window.setRoutesRefreshInterval(activeRefreshInterval);
		vscode.postMessage({ command: 'initialize' });
	});
})();