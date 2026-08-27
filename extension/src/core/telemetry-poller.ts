import * as vscode from 'vscode';

export type RefreshIntervalSeconds = 60 | 30 | 15 | 0;

export interface TelemetryPollerOptions {
	intervalSeconds?: RefreshIntervalSeconds;
	onTick: () => Promise<void>;
}

/**
 * TelemetryPoller manages the periodic polling lifecycle for high-velocity
 * telemetry (Recent Routes and Statistics) with Single-Flight concurrency protection
 * and Webview visibility gating.
 */
export class TelemetryPoller implements vscode.Disposable {
	private intervalSeconds: RefreshIntervalSeconds;
	private onTick: () => Promise<void>;
	private timer: NodeJS.Timeout | null = null;
	private isExecuting: boolean = false;
	private isPaused: boolean = false;
	private isDisposed: boolean = false;

	constructor(options: TelemetryPollerOptions) {
		this.intervalSeconds = options.intervalSeconds ?? 60;
		this.onTick = options.onTick;

		if (this.intervalSeconds > 0) {
			this.armTimer();
		}
	}

	public getIntervalSeconds(): RefreshIntervalSeconds {
		return this.intervalSeconds;
	}

	public isRunning(): boolean {
		return this.timer !== null && !this.isPaused && !this.isDisposed;
	}

	/**
	 * Dynamically update the polling cadence (60s, 30s, 15s, or 0 for Off).
	 */
	public setIntervalSeconds(seconds: RefreshIntervalSeconds): void {
		if (this.isDisposed) return;
		this.intervalSeconds = seconds;
		this.clearTimer();

		if (this.intervalSeconds > 0 && !this.isPaused) {
			this.armTimer();
		}
	}

	/**
	 * Pause polling (e.g. when the dashboard webview is hidden or backgrounded).
	 */
	public pause(): void {
		if (this.isDisposed || this.isPaused) return;
		this.isPaused = true;
		this.clearTimer();
	}

	/**
	 * Resume polling and immediately trigger a fresh tick if unpaused.
	 */
	public resume(triggerImmediateTick: boolean = true): void {
		if (this.isDisposed || !this.isPaused) return;
		this.isPaused = false;

		if (this.intervalSeconds > 0) {
			this.armTimer();
			if (triggerImmediateTick) {
				void this.executeTick();
			}
		}
	}

	/**
	 * Executes a single tick with Single-Flight concurrency lock.
	 */
	public async executeTick(): Promise<void> {
		if (this.isDisposed || this.isExecuting) return;

		this.isExecuting = true;
		try {
			await this.onTick();
		} catch (error) {
			console.error('TelemetryPoller: error during tick execution', error);
		} finally {
			this.isExecuting = false;
		}
	}

	private armTimer(): void {
		this.clearTimer();
		if (this.intervalSeconds <= 0 || this.isDisposed || this.isPaused) return;

		this.timer = setInterval(() => {
			void this.executeTick();
		}, this.intervalSeconds * 1000);
	}

	private clearTimer(): void {
		if (this.timer !== null) {
			clearInterval(this.timer);
			this.timer = null;
		}
	}

	public dispose(): void {
		this.isDisposed = true;
		this.clearTimer();
	}
}
