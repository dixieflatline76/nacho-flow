import * as child_process from 'child_process';
import * as fs from 'fs';
import * as http from 'http';
import * as path from 'path';
import * as vscode from 'vscode';
import { ProcessManager } from './process-manager';

jest.mock('child_process');
jest.mock('fs');
jest.mock('http');

describe('ProcessManager', () => {
	let processManager: ProcessManager;
	let mockOutputChannel: any;
	let mockExtensionUri: vscode.Uri;

	beforeEach(() => {
		jest.clearAllMocks();
		mockOutputChannel = {
			append: jest.fn(),
			appendLine: jest.fn()
		};
		mockExtensionUri = {
			fsPath: '/mock/extension/path'
		} as any;

		processManager = new ProcessManager(mockExtensionUri, mockOutputChannel);
	});

	describe('isLocalUrl', () => {
		it('should identify local URLs accurately', () => {
			expect(processManager.isLocalUrl('http://127.0.0.1:8000')).toBe(true);
			expect(processManager.isLocalUrl('http://localhost:8000')).toBe(true);
			expect(processManager.isLocalUrl('http://[::1]:8000')).toBe(true);
			expect(processManager.isLocalUrl('http://192.168.0.205:8000')).toBe(false);
			expect(processManager.isLocalUrl('https://api.openai.com')).toBe(false);
			expect(processManager.isLocalUrl('invalid-url-127.0.0.1')).toBe(true);
		});
	});

	describe('resolveBinary', () => {
		it('should resolve bundled binary when it exists', () => {
			(fs.existsSync as jest.Mock).mockImplementation((p: string) => {
				return p.includes('bin');
			});
			(fs.chmodSync as jest.Mock).mockImplementation(() => {});

			const result = processManager.resolveBinary();
			expect(result).not.toBeNull();
			expect(result?.command).toContain('bin');
			expect(result?.args).toEqual([]);
		});

		it('should resolve from system PATH when bundled is missing', () => {
			const originalPath = process.env.PATH;
			process.env.PATH = ['/custom/bin', '/another/bin'].join(path.delimiter);

			(fs.existsSync as jest.Mock).mockImplementation((p: string) => {
				return p.includes('custom') && !p.includes('/mock/extension/path');
			});

			const result = processManager.resolveBinary();
			expect(result).not.toBeNull();
			expect(result?.command).toContain('custom');

			process.env.PATH = originalPath;
		});

		it('should resolve from workspace Go source when in dev repo', () => {
			const originalPath = process.env.PATH;
			process.env.PATH = '';

			(vscode.workspace as any).workspaceFolders = [
				{ uri: { fsPath: '/mock/workspace' } }
			];

			(fs.existsSync as jest.Mock).mockImplementation((p: string) => {
				return p.includes('main.go');
			});

			const result = processManager.resolveBinary();
			expect(result).not.toBeNull();
			expect(result?.command).toBe('go');
			expect(result?.args).toEqual(['run', './cmd/nacho-flow']);

			process.env.PATH = originalPath;
		});

		it('should return null when no binary or source is found', () => {
			(fs.existsSync as jest.Mock).mockReturnValue(false);
			(vscode.workspace as any).workspaceFolders = [];

			const result = processManager.resolveBinary();
			expect(result).toBeNull();
		});
	});

	describe('checkHealth', () => {
		it('should return true when health endpoint responds with 200', async () => {
			(http.get as jest.Mock).mockImplementation((_url: string, _opts: any, callback: Function) => {
				const mockRes = { statusCode: 200 };
				callback(mockRes);
				return { on: jest.fn() };
			});

			const isUp = await processManager.checkHealth('http://127.0.0.1:8000');
			expect(isUp).toBe(true);
		});

		it('should return false on non-200 status code', async () => {
			(http.get as jest.Mock).mockImplementation((_url: string, _opts: any, callback: Function) => {
				const mockRes = { statusCode: 503 };
				callback(mockRes);
				return { on: jest.fn() };
			});

			const isUp = await processManager.checkHealth('http://127.0.0.1:8000');
			expect(isUp).toBe(false);
		});

		it('should return false on request error or timeout', async () => {
			(http.get as jest.Mock).mockImplementation((_url: string, _opts: any, _callback: Function) => {
				const handlers: Record<string, Function> = {};
				setTimeout(() => {
					if (handlers['error']) {
						handlers['error'](new Error('Connection refused'));
					}
					if (handlers['timeout']) {
						handlers['timeout']();
					}
				}, 10);
				return {
					on: (event: string, fn: Function) => {
						handlers[event] = fn;
					},
					destroy: jest.fn()
				};
			});

			const isUp = await processManager.checkHealth('http://127.0.0.1:8000');
			expect(isUp).toBe(false);
		});
	});

	describe('start, stop, and restart', () => {
		it('should reject starting remote engine URLs', async () => {
			const result = await processManager.start('http://192.168.0.205:8000');
			expect(result.success).toBe(false);
			expect(result.error).toContain('Cannot start remote engine');
		});

		it('should succeed immediately if engine is already healthy', async () => {
			jest.spyOn(processManager, 'checkHealth').mockResolvedValue(true);

			const result = await processManager.start('http://127.0.0.1:8000');
			expect(result.success).toBe(true);
			expect(mockOutputChannel.appendLine).toHaveBeenCalledWith(expect.stringContaining('already active'));
		});

		it('should fail if executable cannot be resolved', async () => {
			jest.spyOn(processManager, 'checkHealth').mockResolvedValue(false);
			jest.spyOn(processManager, 'resolveBinary').mockReturnValue(null);

			const result = await processManager.start('http://127.0.0.1:8000');
			expect(result.success).toBe(false);
			expect(result.error).toContain('Executable not found');
		});

		it('should spawn process, stream logs, and report success when online', async () => {
			jest.spyOn(processManager, 'resolveBinary').mockReturnValue({
				command: '/mock/bin/nacho-flow',
				args: []
			});

			const stdoutEmitter = { on: jest.fn((_event, fn) => fn(Buffer.from('Server started on :8000\n'))) };
			const stderrEmitter = { on: jest.fn((_event, fn) => fn(Buffer.from('Info log\n'))) };
			const mockChild: any = {
				stdout: stdoutEmitter,
				stderr: stderrEmitter,
				on: jest.fn(),
				kill: jest.fn(),
				killed: false,
				pid: 12345
			};

			(child_process.spawn as jest.Mock).mockReturnValue(mockChild);

			let callCount = 0;
			jest.spyOn(processManager, 'checkHealth').mockImplementation(async () => {
				callCount++;
				return callCount > 1; // Healthy on second check
			});

			const result = await processManager.start('http://127.0.0.1:8000', '/mock/config.yaml');
			expect(result.success).toBe(true);
			expect(processManager.isRunning()).toBe(true);
			expect(mockOutputChannel.append).toHaveBeenCalledWith('Server started on :8000\n');
		});

		it('should handle premature process exit with stderr message', async () => {
			jest.spyOn(processManager, 'resolveBinary').mockReturnValue({
				command: '/mock/bin/nacho-flow',
				args: []
			});

			let exitHandler: any;
			const stderrEmitter = { on: jest.fn((_event, fn) => fn(Buffer.from('bind: address already in use\n'))) };
			const mockChild: any = {
				stdout: { on: jest.fn() },
				stderr: stderrEmitter,
				on: jest.fn((event, fn) => {
					if (event === 'exit') exitHandler = fn;
				}),
				kill: jest.fn(),
				killed: false,
				pid: 12345
			};

			(child_process.spawn as jest.Mock).mockReturnValue(mockChild);
			jest.spyOn(processManager, 'checkHealth').mockResolvedValue(false);

			const startPromise = processManager.start('http://127.0.0.1:8000');
			setTimeout(() => {
				if (exitHandler) exitHandler(1, null);
			}, 100);

			const result = await startPromise;
			expect(result.success).toBe(false);
			expect(result.error).toContain('bind: address already in use');
		});

		it('should stop and restart active process correctly', async () => {
			const mockChild: any = {
				pid: 12345,
				kill: jest.fn(),
				killed: false
			};
			(processManager as any).childProcess = mockChild;
			(child_process.execSync as jest.Mock).mockImplementation(() => {});

			const stopped = await processManager.stop();
			expect(stopped).toBe(true);
			expect(processManager.isRunning()).toBe(false);

			// Test stop when no child process exists
			expect(await processManager.stop()).toBe(true);

			// Test restart
			jest.spyOn(processManager, 'start').mockResolvedValue({ success: true });
			const restartResult = await processManager.restart('http://127.0.0.1:8000');
			expect(restartResult.success).toBe(true);

			// Test stop error catch block
			(child_process.execSync as jest.Mock).mockImplementationOnce(() => { throw new Error('Exec error'); });
			(processManager as any).childProcess = {
				pid: 12345,
				kill: jest.fn().mockImplementation(() => { throw new Error('Kill failed'); })
			};
			const stopFailed = await processManager.stop();
			expect(stopFailed).toBe(false);
		});

		it('should cleanly dispose of child process and handle exceptions', () => {
			const mockChild: any = {
				pid: 12345,
				kill: jest.fn(),
				killed: false
			};
			(processManager as any).childProcess = mockChild;
			(child_process.execSync as jest.Mock).mockImplementation(() => {});

			processManager.dispose();
			expect(processManager.isRunning()).toBe(false);

			// Test dispose exception catch block
			(processManager as any).childProcess = {
				kill: jest.fn().mockImplementation(() => { throw new Error('Dispose fail'); })
			};
			processManager.dispose();
			expect(processManager.isRunning()).toBe(false);
		});

		it('should handle process error event and chmod error', async () => {
			// Chmod error branch
			const originalPlatform = process.platform;
			Object.defineProperty(process, 'platform', { value: 'linux' });
			(fs.existsSync as jest.Mock).mockReturnValue(true);
			(fs.chmodSync as jest.Mock).mockImplementation(() => { throw new Error('EPERM'); });
			const binaryRes = processManager.resolveBinary();
			expect(binaryRes).not.toBeNull();

			// Process error event branch
			let errorHandler: any;
			const mockChild: any = {
				stdout: { on: jest.fn() },
				stderr: { on: jest.fn() },
				on: jest.fn((event, fn) => {
					if (event === 'error') errorHandler = fn;
				}),
				kill: jest.fn(),
				killed: false
			};
			(child_process.spawn as jest.Mock).mockReturnValue(mockChild);
			jest.spyOn(processManager, 'checkHealth').mockResolvedValue(false);

			const startPromise = processManager.start('http://127.0.0.1:8000');
			setTimeout(() => {
				if (errorHandler) errorHandler(new Error('Spawn error'));
			}, 50);
			await startPromise;
			expect(mockOutputChannel.appendLine).toHaveBeenCalledWith(expect.stringContaining('Process error: Spawn error'));

			Object.defineProperty(process, 'platform', { value: originalPlatform });
		});
	});
});
