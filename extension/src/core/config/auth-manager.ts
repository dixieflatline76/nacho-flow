import * as vscode from 'vscode';

export class AuthManager {
	private static readonly LOCAL_AUTH_KEY = 'nacho-flow.auth-token';
	private static readonly REMOTE_AUTH_KEY = 'nacho-flow.remote-auth-token';
	private static readonly REMOTE_URL_KEY = 'nacho-flow.remote-url';
	private static readonly ENGINE_MODE_KEY = 'nacho-flow.engine-mode';
	private context: vscode.ExtensionContext;

	constructor(context: vscode.ExtensionContext) {
		this.context = context;
	}

	public getEngineMode(): 'local' | 'remote' {
		return this.context.globalState?.get<'local' | 'remote'>(AuthManager.ENGINE_MODE_KEY, 'local') || 'local';
	}

	public async setEngineMode(mode: 'local' | 'remote'): Promise<void> {
		await this.context.globalState?.update(AuthManager.ENGINE_MODE_KEY, mode);
	}

	public getRemoteUrl(): string {
		const savedRemote = this.context.globalState?.get<string>(AuthManager.REMOTE_URL_KEY);
		if (savedRemote && savedRemote.trim() !== '') {
			return savedRemote.trim();
		}
		const configUrl = vscode.workspace.getConfiguration('nachoFlow').get<string>('remoteDaemonUrl') ||
			vscode.workspace.getConfiguration('nachoFlow').get<string>('daemonUrl');
		if (configUrl && configUrl !== 'http://127.0.0.1:8000' && configUrl !== 'http://localhost:8000') {
			return configUrl.trim();
		}
		return 'http://192.168.0.205:8000';
	}

	public async setRemoteUrl(url: string): Promise<void> {
		const cleanUrl = url.trim();
		await this.context.globalState?.update(AuthManager.REMOTE_URL_KEY, cleanUrl);
		try {
			const config = vscode.workspace.getConfiguration('nachoFlow');
			if (config && typeof config.update === 'function') {
				await config.update('remoteDaemonUrl', cleanUrl, vscode.ConfigurationTarget.Global);
			}
		} catch (_) {
			// Ignore update errors
		}
	}

	public async getRemoteToken(): Promise<string | undefined> {
		const secret = await this.context.secrets.get(AuthManager.REMOTE_AUTH_KEY);
		if (secret && secret.trim() !== '') {
			return secret.trim();
		}
		return undefined;
	}

	public async setRemoteToken(token: string): Promise<void> {
		if (token.trim() === '') {
			await this.context.secrets.delete(AuthManager.REMOTE_AUTH_KEY);
		} else {
			await this.context.secrets.store(AuthManager.REMOTE_AUTH_KEY, token.trim());
		}
	}

	public async getAuthToken(): Promise<string | undefined> {
		const mode = this.getEngineMode();
		if (mode === 'remote') {
			return await this.getRemoteToken();
		}
		const secret = await this.context.secrets.get(AuthManager.LOCAL_AUTH_KEY);
		if (secret) return secret;
		const configToken = vscode.workspace.getConfiguration('nachoFlow').get<string>('authToken');
		return configToken && configToken.trim() !== '' ? configToken.trim() : undefined;
	}

	public async setAuthToken(token: string): Promise<void> {
		const mode = this.getEngineMode();
		if (mode === 'remote') {
			await this.setRemoteToken(token);
		} else {
			await this.context.secrets.store(AuthManager.LOCAL_AUTH_KEY, token);
		}
	}

	public async deleteAuthToken(): Promise<void> {
		const mode = this.getEngineMode();
		if (mode === 'remote') {
			await this.context.secrets.delete(AuthManager.REMOTE_AUTH_KEY);
		} else {
			await this.context.secrets.delete(AuthManager.LOCAL_AUTH_KEY);
		}
	}

	public async getBaseUrl(): Promise<string> {
		const mode = this.getEngineMode();
		if (mode === 'local') {
			return 'http://127.0.0.1:8000';
		}
		return this.getRemoteUrl();
	}
}