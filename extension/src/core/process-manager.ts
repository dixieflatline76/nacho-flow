import * as child_process from 'child_process';
import * as fs from 'fs';
import * as http from 'http';
import * as path from 'path';
import * as vscode from 'vscode';

export interface ProcessLaunchConfig {
	command: string;
	args: string[];
	cwd?: string;
}

export class ProcessManager {
	private childProcess: child_process.ChildProcess | null = null;
	private outputChannel: vscode.OutputChannel;
	private extensionUri: vscode.Uri;
	private lastStderr: string[] = [];

	constructor(extensionUri: vscode.Uri, outputChannel: vscode.OutputChannel) {
		this.extensionUri = extensionUri;
		this.outputChannel = outputChannel;
	}

	/**
	 * Checks if a given daemon URL refers to the local machine.
	 */
	public isLocalUrl(urlStr: string): boolean {
		try {
			const parsed = new URL(urlStr);
			const hostname = parsed.hostname.toLowerCase();
			return hostname === '127.0.0.1' || hostname === 'localhost' || hostname === '::1' || hostname === '[::1]';
		} catch {
			return urlStr.includes('127.0.0.1') || urlStr.includes('localhost');
		}
	}

	/**
	 * Resolves the executable using a 3-tier cascade:
	 * Tier 1: Bundled native binary in extension/bin/
	 * Tier 2: System PATH (nacho-flow executable)
	 * Tier 3: Development fallback (go run ./cmd/nacho-flow)
	 */
	public resolveBinary(): ProcessLaunchConfig | null {
		const isWindows = process.platform === 'win32';
		const binaryName = isWindows ? 'nacho-flow.exe' : 'nacho-flow';

		// Tier 1: Bundled binary inside extension directory
		const extensionPath = this.extensionUri?.fsPath;
		if (extensionPath) {
			const bundledPath = path.join(extensionPath, 'bin', binaryName);
			if (fs.existsSync(bundledPath)) {
				if (!isWindows) {
					try {
						fs.chmodSync(bundledPath, 0o755);
					} catch {
						// Ignore chmod errors if permissions cannot be set
					}
				}
				return { command: bundledPath, args: [] };
			}
		}

		// Tier 2: System PATH check (look in standard paths or command lookup)
		const envPath = process.env.PATH || '';
		const pathDirs = envPath.split(path.delimiter);
		for (const dir of pathDirs) {
			const candidate = path.join(dir, binaryName);
			if (fs.existsSync(candidate)) {
				return { command: candidate, args: [] };
			}
		}

		// Tier 3: Workspace development repository fallback
		const workspaceFolders = vscode.workspace.workspaceFolders;
		if (workspaceFolders && workspaceFolders.length > 0) {
			for (const folder of workspaceFolders) {
				const mainGoPath = path.join(folder.uri.fsPath, 'cmd', 'nacho-flow', 'main.go');
				if (fs.existsSync(mainGoPath)) {
					return {
						command: 'go',
						args: ['run', './cmd/nacho-flow'],
						cwd: folder.uri.fsPath
					};
				}
			}
		}

		return null;
	}

	/**
	 * Pings the daemon health endpoint to verify if the server is actively responding.
	 */
	public async checkHealth(daemonUrl: string, timeoutMs: number = 800): Promise<boolean> {
		return new Promise<boolean>((resolve) => {
			try {
				const healthUrl = new URL('/health', daemonUrl).toString();
				const req = http.get(healthUrl, { timeout: timeoutMs }, (res) => {
					if (res.statusCode && res.statusCode >= 200 && res.statusCode < 300) {
						resolve(true);
					} else {
						resolve(false);
					}
				});
				req.on('error', () => resolve(false));
				req.on('timeout', () => {
					req.destroy();
					resolve(false);
				});
			} catch {
				resolve(false);
			}
		});
	}

	/**
	 * Starts the local Nacho Flow engine background process.
	 */
	public async start(daemonUrl: string, configPath?: string): Promise<{ success: boolean; error?: string }> {
		if (!this.isLocalUrl(daemonUrl)) {
			return {
				success: false,
				error: 'Nacho Flow: Cannot start remote engine via local subprocess. Please ensure the remote daemon is running on its host machine.'
			};
		}

		// 1. Check if engine is already running
		const alreadyHealthy = await this.checkHealth(daemonUrl, 500);
		if (alreadyHealthy) {
			this.outputChannel.appendLine('[ProcessManager] Engine is already active and healthy.');
			return { success: true };
		}

		// 2. Resolve executable
		const launchConfig = this.resolveBinary();
		if (!launchConfig) {
			return {
				success: false,
				error: 'Nacho Flow: Executable not found. Please install nacho-flow or place the binary in your PATH.'
			};
		}

		const args = [...launchConfig.args];
		if (configPath) {
			args.push('--config', configPath);
		}

		this.lastStderr = [];
		this.outputChannel.appendLine(`[ProcessManager] Launching: ${launchConfig.command} ${args.join(' ')}`);

		try {
			const child = child_process.spawn(launchConfig.command, args, {
				detached: false,
				stdio: ['ignore', 'pipe', 'pipe'],
				cwd: launchConfig.cwd || process.cwd()
			});

			this.childProcess = child;

			child.stdout?.on('data', (data: Buffer) => {
				const text = data.toString();
				this.outputChannel.append(text);
			});

			child.stderr?.on('data', (data: Buffer) => {
				const text = data.toString();
				this.lastStderr.push(text);
				if (this.lastStderr.length > 20) {
					this.lastStderr.shift();
				}
				this.outputChannel.append(text);
			});

			child.on('error', (err: Error) => {
				this.outputChannel.appendLine(`[ProcessManager] Process error: ${err.message}`);
				this.childProcess = null;
			});

			child.on('exit', (code: number | null, signal: string | null) => {
				this.outputChannel.appendLine(`[ProcessManager] Engine exited with code ${code}, signal: ${signal}`);
				this.childProcess = null;
			});

			// 3. Poll for readiness up to 5 seconds
			const maxAttempts = 25;
			for (let i = 0; i < maxAttempts; i++) {
				await new Promise((r) => setTimeout(r, 200));

				// Check if process crashed immediately
				if (!this.childProcess && !alreadyHealthy) {
					const errorDetail = this.lastStderr.join('').trim();
					return {
						success: false,
						error: errorDetail ? `Nacho Flow failed to start: ${errorDetail}` : 'Nacho Flow process exited prematurely.'
					};
				}

				const isUp = await this.checkHealth(daemonUrl, 300);
				if (isUp) {
					this.outputChannel.appendLine('[ProcessManager] Engine started successfully and verified online.');
					return { success: true };
				}
			}

			return {
				success: false,
				error: 'Nacho Flow startup timed out waiting for health check.'
			};
		} catch (err: any) {
			this.childProcess = null;
			return {
				success: false,
				error: `Failed to spawn Nacho Flow: ${err.message}`
			};
		}
	}

	/**
	 * Stops any active child process spawned by this extension.
	 */
	public async stop(): Promise<boolean> {
		if (!this.childProcess) {
			return true;
		}

		try {
			this.outputChannel.appendLine('[ProcessManager] Stopping local engine process...');
			if (process.platform === 'win32') {
				try {
					if (this.childProcess.pid) {
						child_process.execSync(`taskkill /pid ${this.childProcess.pid} /f /t`);
					}
				} catch {
					this.childProcess.kill();
				}
			} else {
				this.childProcess.kill('SIGTERM');
			}
			this.childProcess = null;
			return true;
		} catch (err: any) {
			this.outputChannel.appendLine(`[ProcessManager] Error stopping process: ${err.message}`);
			this.childProcess = null;
			return false;
		}
	}

	/**
	 * Restarts the engine by stopping and starting.
	 */
	public async restart(daemonUrl: string, configPath?: string): Promise<{ success: boolean; error?: string }> {
		await this.stop();
		await new Promise((r) => setTimeout(r, 500));
		return await this.start(daemonUrl, configPath);
	}

	/**
	 * Returns true if this extension is actively managing a running child process.
	 */
	public isRunning(): boolean {
		return this.childProcess !== null && !this.childProcess.killed;
	}

	/**
	 * Clean up resources on extension deactivation.
	 */
	public dispose(): void {
		if (this.childProcess) {
			try {
				if (process.platform === 'win32' && this.childProcess.pid) {
					child_process.execSync(`taskkill /pid ${this.childProcess.pid} /f /t`);
				} else {
					this.childProcess.kill('SIGTERM');
				}
			} catch {
				// Ignore errors on dispose
			}
			this.childProcess = null;
		}
	}
}
