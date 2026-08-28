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

export interface ParsedStartupError {
	type: 'PORT_IN_USE' | 'CONFIG_ERROR' | 'RULE_ERROR' | 'BINARY_NOT_FOUND' | 'REMOTE_ENGINE' | 'SPAWN_ERROR' | 'UNKNOWN';
	message: string;
	port?: number;
	rawError: string;
}

/**
 * Deterministically classifies daemon startup and runtime errors from stderr/stdout streams.
 */
export function parseStartupError(stderr: string, stdout: string = '', defaultPort = 8000): ParsedStartupError {
	const combined = `${stderr}\n${stdout}`.trim();

	// 1. Port In Use / Bind Collision
	const portMatch = combined.match(/\[FATAL:PORT_IN_USE:(\d+)\]/);
	if (
		portMatch ||
		combined.includes('PORT_IN_USE') ||
		combined.toLowerCase().includes('eaddrinuse') ||
		combined.includes('10048') ||
		combined.toLowerCase().includes('address already in use')
	) {
		const port = portMatch ? parseInt(portMatch[1], 10) : defaultPort;
		return {
			type: 'PORT_IN_USE',
			port,
			message: `Port ${port} is already in use by another application. Please free port ${port} or change the port in config.yaml.`,
			rawError: combined
		};
	}

	// 2. Config Syntax / Validation Error
	if (
		combined.includes('[FATAL:CONFIG_ERROR]') ||
		combined.includes('yaml: unmarshal errors') ||
		combined.includes('Config load error') ||
		combined.includes('no providers defined in configuration')
	) {
		const cleaned = stderr
			.split('\n')
			.map((line) => line.replace(/^\[FATAL:CONFIG_ERROR\]\s*/, '').trim())
			.filter((line) => line.length > 0 && !line.startsWith('time='))
			.join(' ');
		return {
			type: 'CONFIG_ERROR',
			message: `Configuration error in config.yaml: ${cleaned || 'Invalid YAML format or schema validation failed.'}`,
			rawError: combined
		};
	}

	// 3. Routing Rule Compilation Error
	if (
		combined.includes('[FATAL:RULE_ERROR]') ||
		combined.includes('Evaluator compile error') ||
		combined.includes('failed to compile expr')
	) {
		const cleaned = stderr
			.split('\n')
			.map((line) => line.replace(/^\[FATAL:RULE_ERROR\]\s*/, '').trim())
			.filter((line) => line.length > 0 && !line.startsWith('time='))
			.join(' ');
		return {
			type: 'RULE_ERROR',
			message: `Routing rule error in config.yaml: ${cleaned || 'Failed to compile tier expression.'}`,
			rawError: combined
		};
	}

	// 4. Fatal Server Error
	if (combined.includes('[FATAL:SERVER_ERROR]')) {
		const cleaned = stderr
			.split('\n')
			.map((line) => line.replace(/^\[FATAL:SERVER_ERROR\]\s*/, '').trim())
			.filter((line) => line.length > 0 && !line.startsWith('time='))
			.join(' ');
		return {
			type: 'SPAWN_ERROR',
			message: `Nacho Flow server error: ${cleaned}`,
			rawError: combined
		};
	}

	return {
		type: 'UNKNOWN',
		message: stderr.trim() || 'Nacho Flow process exited prematurely.',
		rawError: combined
	};
}

export class ProcessManager {
	private childProcess: child_process.ChildProcess | null = null;
	private outputChannel: vscode.OutputChannel;
	private extensionUri: vscode.Uri;
	private lastStderr: string[] = [];
	private lastStdout: string[] = [];
	private isStopping = false;

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
	 * Starts the local Nacho Flow engine background process with structured error parsing.
	 */
	public async start(
		daemonUrl: string,
		configPath?: string
	): Promise<{ success: boolean; error?: string; parsedError?: ParsedStartupError }> {
		if (!this.isLocalUrl(daemonUrl)) {
			const error =
				'Nacho Flow: Cannot start remote engine via local subprocess. Please ensure the remote daemon is running on its host machine.';
			return {
				success: false,
				error,
				parsedError: {
					type: 'REMOTE_ENGINE',
					message: error,
					rawError: error
				}
			};
		}

		let targetPort = 8000;
		try {
			const parsed = new URL(daemonUrl);
			if (parsed.port) {
				targetPort = parseInt(parsed.port, 10);
			}
		} catch {
			// fallback default 8000
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
			const error = 'Nacho Flow: Executable not found. Please install nacho-flow or place the binary in your PATH.';
			return {
				success: false,
				error,
				parsedError: {
					type: 'BINARY_NOT_FOUND',
					message: error,
					rawError: error
				}
			};
		}

		const args = [...launchConfig.args];
		if (configPath) {
			args.push('--config', configPath);
		}

		this.lastStderr = [];
		this.lastStdout = [];
		this.isStopping = false;
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
				this.lastStdout.push(text);
				if (this.lastStdout.length > 20) {
					this.lastStdout.shift();
				}
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
				if (this.isStopping) {
					this.outputChannel.appendLine('[ProcessManager] Engine stopped cleanly.');
					this.isStopping = false;
				} else {
					this.outputChannel.appendLine(`[ProcessManager] Engine exited with code ${code}, signal: ${signal}`);
				}
				this.childProcess = null;
			});

			// 3. Poll for readiness up to 5 seconds
			const maxAttempts = 25;
			for (let i = 0; i < maxAttempts; i++) {
				await new Promise((r) => setTimeout(r, 200));

				// Check if process crashed prematurely
				if (!this.childProcess && !alreadyHealthy) {
					const errorDetail = this.lastStderr.join('').trim();
					const stdoutDetail = this.lastStdout.join('').trim();
					const parsed = parseStartupError(errorDetail, stdoutDetail, targetPort);
					return {
						success: false,
						error: parsed.message,
						parsedError: parsed
					};
				}

				const isUp = await this.checkHealth(daemonUrl, 300);
				if (isUp) {
					this.outputChannel.appendLine('[ProcessManager] Engine started successfully and verified online.');
					return { success: true };
				}
			}

			const errorDetail = this.lastStderr.join('').trim();
			const stdoutDetail = this.lastStdout.join('').trim();
			const parsed = parseStartupError(errorDetail, stdoutDetail, targetPort);
			return {
				success: false,
				error: parsed.type !== 'UNKNOWN' ? parsed.message : 'Nacho Flow startup timed out waiting for health check.',
				parsedError: parsed
			};
		} catch (err: any) {
			this.childProcess = null;
			const msg = `Failed to spawn Nacho Flow: ${err.message}`;
			return {
				success: false,
				error: msg,
				parsedError: {
					type: 'SPAWN_ERROR',
					message: msg,
					rawError: err.stack || err.message
				}
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
			this.isStopping = true;
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
			this.isStopping = false;
			this.outputChannel.appendLine(`[ProcessManager] Error stopping process: ${err.message}`);
			this.childProcess = null;
			return false;
		}
	}

	/**
	 * Restarts the engine by stopping and starting.
	 */
	public async restart(
		daemonUrl: string,
		configPath?: string
	): Promise<{ success: boolean; error?: string; parsedError?: ParsedStartupError }> {
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
				this.isStopping = true;
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
