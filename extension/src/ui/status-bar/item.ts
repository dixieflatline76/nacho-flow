import * as vscode from 'vscode';

export class StatusBarManager {
	private item: vscode.StatusBarItem;
	private stats: any = null;
	private timeWindow: string = 'all_time';
	private baseUrl: string = 'http://127.0.0.1:8000';
	private activePreset: string = 'standard';

	private static readonly PRESET_LABELS: Record<string, string> = {
		standard: '🌮 Standard',
		zoo: '🤖 Zoo Code',
		cline: '🛠️ Cline',
	};

	constructor() {
		this.item = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 100);
		this.item.command = 'nacho-flow.showDashboard';
		this.updateStatusBar();
	}

	public setTimeWindow(timeWindow: string): void {
		this.timeWindow = timeWindow || 'all_time';
		this.updateStatusBar();
	}

	public getTimeWindow(): string {
		return this.timeWindow;
	}

	public setBaseUrl(baseUrl: string): void {
		if (baseUrl && baseUrl.trim() !== '') {
			this.baseUrl = baseUrl.trim();
			this.updateStatusBar();
		}
	}

	public setActivePreset(preset: string): void {
		if (preset && preset.trim() !== '') {
			this.activePreset = preset.trim();
			this.updateStatusBar();
		}
	}

	public updateStats(stats: any): void {
		this.stats = stats;
		this.updateStatusBar();
	}

	private updateStatusBar(): void {
		if (this.stats === null) {
			this.item.text = '$(circle-slash) Nacho: Offline';
			const md = new vscode.MarkdownString(undefined, true);
			md.isTrusted = true;
			md.supportThemeIcons = true;
			md.appendMarkdown(`**🌮 Nacho Flow: Offline**\n\n`);
			md.appendMarkdown(`Cannot connect to \`${this.getBaseUrl()}\`\n\n`);
			md.appendMarkdown(`---\n\n[🔄 Open Dashboard / Retry](command:nacho-flow.showDashboard)`);
			this.item.tooltip = md;
		} else {
			const metrics = this.extractMetricsForTimeWindow();
			const suffix = this.timeWindow === 'today' ? ' Today' : 
				(this.timeWindow === 'yesterday' ? ' Yesterday' : 
				(this.timeWindow === 'this_week' ? ' This Week' : 
				(this.timeWindow === 'this_month' ? ' This Month' : '')));

			this.item.text = `🌮 $${metrics.savedUSD.toFixed(2)} Saved${suffix} (${metrics.localPct}% Local)`;
			this.item.tooltip = this.buildTooltip(metrics);
		}
		this.item.show();
	}

	private extractMetricsForTimeWindow() {
		let totalReqs = 0;
		let localReqs = 0;
		let totalTokens = 0;
		let localTokens = 0;
		let spentUSD = 0;
		let savedUSD = 0;
		let reductionPct = 0;
		let timeframeTitle = 'All Time (Cumulative)';

		if (!this.stats) {
			return { totalReqs, localReqs, totalTokens, localTokens, spentUSD, savedUSD, reductionPct, localPct: 0, timeframeTitle };
		}

		if (this.timeWindow === 'today') {
			const w = this.stats.windows?.today;
			totalReqs = w?.requests || 0;
			totalTokens = w?.tokens_total || 0;
			localTokens = w?.tokens_local || 0;
			spentUSD = w?.cost_spent_usd || 0;
			savedUSD = w?.cost_saved_usd || 0;
			reductionPct = w?.cost_reduction_pct || ((savedUSD + spentUSD) > 0 ? Math.round((savedUSD / (savedUSD + spentUSD)) * 100) : 0);
			localReqs = totalTokens > 0 ? Math.round((localTokens / totalTokens) * totalReqs) : 0;
			timeframeTitle = 'Today (Active 24h)';
		} else if (this.timeWindow === 'yesterday') {
			const w = this.stats.windows?.yesterday;
			totalReqs = w?.requests || 0;
			totalTokens = w?.tokens_total || 0;
			localTokens = w?.tokens_local || 0;
			spentUSD = w?.cost_spent_usd || 0;
			savedUSD = w?.cost_saved_usd || 0;
			reductionPct = w?.cost_reduction_pct || ((savedUSD + spentUSD) > 0 ? Math.round((savedUSD / (savedUSD + spentUSD)) * 100) : 0);
			localReqs = totalTokens > 0 ? Math.round((localTokens / totalTokens) * totalReqs) : 0;
			timeframeTitle = 'Yesterday (Prior 24h)';
		} else if (this.timeWindow === 'this_week') {
			const w = this.stats.windows?.this_week;
			totalReqs = w?.requests || 0;
			totalTokens = w?.tokens_total || 0;
			localTokens = w?.tokens_local || 0;
			spentUSD = w?.cost_spent_usd || 0;
			savedUSD = w?.cost_saved_usd || 0;
			reductionPct = w?.cost_reduction_pct || ((savedUSD + spentUSD) > 0 ? Math.round((savedUSD / (savedUSD + spentUSD)) * 100) : 0);
			localReqs = totalTokens > 0 ? Math.round((localTokens / totalTokens) * totalReqs) : 0;
			timeframeTitle = 'This Week (Current ISO Week)';
		} else if (this.timeWindow === 'this_month') {
			const w = this.stats.windows?.this_month;
			totalReqs = w?.requests || 0;
			totalTokens = w?.tokens_total || 0;
			localTokens = w?.tokens_local || 0;
			spentUSD = w?.cost_spent_usd || 0;
			savedUSD = w?.cost_saved_usd || 0;
			reductionPct = w?.cost_reduction_pct || ((savedUSD + spentUSD) > 0 ? Math.round((savedUSD / (savedUSD + spentUSD)) * 100) : 0);
			localReqs = totalTokens > 0 ? Math.round((localTokens / totalTokens) * totalReqs) : 0;
			timeframeTitle = 'This Month (Current Month)';
		} else {
			// All Time
			const w = this.stats.windows?.all_time;
			totalReqs = w?.requests || this.stats.total_requests || 0;
			totalTokens = w?.tokens_total || this.stats.total_tokens || 0;
			localTokens = w?.tokens_local || this.stats.total_tokens_routed_locally || 0;
			spentUSD = w?.cost_spent_usd || this.stats.total_cost_spent_usd || 0;
			savedUSD = w?.cost_saved_usd || this.stats.estimated_cost_saved_usd || 0;
			reductionPct = w?.cost_reduction_pct || this.stats.cost_reduction_pct || ((savedUSD + spentUSD) > 0 ? Math.round((savedUSD / (savedUSD + spentUSD)) * 100) : 0);
			localReqs = this.stats.tier_breakdown?.tier1_local_free || (totalTokens > 0 ? Math.round((localTokens / totalTokens) * totalReqs) : 0);
			timeframeTitle = 'All Time (Cumulative)';
		}

		const localPct = totalTokens > 0 ? Math.round((localTokens / totalTokens) * 100) : (totalReqs > 0 ? Math.round((localReqs / totalReqs) * 100) : 0);

		return { totalReqs, localReqs, totalTokens, localTokens, spentUSD, savedUSD, reductionPct, localPct, timeframeTitle };
	}

	private calculateLocalPercentage(): number {
		return this.extractMetricsForTimeWindow().localPct;
	}

	private buildTooltip(metrics?: ReturnType<typeof this.extractMetricsForTimeWindow>): vscode.MarkdownString {
		const md = new vscode.MarkdownString(undefined, true);
		md.isTrusted = true;
		md.supportThemeIcons = true;

		if (!this.stats) {
			md.appendMarkdown('**🌮 Nacho Flow** • Agent Supervisor & Model Dispatcher');
			return md;
		}

		const m = metrics || this.extractMetricsForTimeWindow();
		const presetLabel = StatusBarManager.PRESET_LABELS[this.activePreset] || this.activePreset;
		
		md.appendMarkdown(`**🌮 Nacho Flow** • Agent Supervisor & Model Dispatcher\n\n`);
		md.appendMarkdown(`🕒 **Timeframe**: ${m.timeframeTitle}\n\n`);
		md.appendMarkdown(`---\n\n`);
		md.appendMarkdown(`💵 **Est. Cost Saved**: \`+$${m.savedUSD.toFixed(2)}\` *(${Math.round(m.reductionPct)}% saved)*\n\n`);
		md.appendMarkdown(`📉 **Cloud API Spend**: \`$${m.spentUSD.toFixed(2)}\`\n\n`);
		md.appendMarkdown(`⚡ **Local GPU ($0.00)**: \`${m.localPct}%\` *(${m.localReqs}/${m.totalReqs} turns)*\n\n`);
		md.appendMarkdown(`🪙 **Total Prompt Turns**: \`${m.totalReqs}\` *(${m.totalTokens.toLocaleString()} tokens)*\n\n`);
		md.appendMarkdown(`🛣️ **Model Dispatcher**: \`${this.getBaseUrl()}\`\n\n`);
		md.appendMarkdown(`📋 **Active Preset**: \`${presetLabel}\`\n\n`);
		md.appendMarkdown(`---\n\n`);
		md.appendMarkdown(`Switch: [Today](command:nacho-flow.setTimeWindowToday) &nbsp;|&nbsp; [Yesterday](command:nacho-flow.setTimeWindowYesterday) &nbsp;|&nbsp; [This Week](command:nacho-flow.setTimeWindowWeek) &nbsp;|&nbsp; [This Month](command:nacho-flow.setTimeWindowMonth) &nbsp;|&nbsp; [All Time](command:nacho-flow.setTimeWindowAllTime)\n\n`);
		md.appendMarkdown(`---\n\n`);
		md.appendMarkdown(`[📊 Dashboard](command:nacho-flow.showDashboard) &nbsp;|&nbsp; [⚡ Auto-Tune](command:nacho-flow.runOptimizer) &nbsp;|&nbsp; [🔥 Heat Seeker](command:nacho-flow.refreshDeals) &nbsp;|&nbsp; [⚙️ Settings](command:nacho-flow.openSettings)`);

		return md;
	}

	public dispose(): void {
		this.item.dispose();
	}

	private getBaseUrl(): string {
		return this.baseUrl;
	}
}
