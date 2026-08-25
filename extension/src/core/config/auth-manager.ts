import * as vscode from 'vscode';

export class AuthManager {
	private static readonly SECRET_STORAGE_KEY = 'nacho-flow.auth-token';
	private context: vscode.ExtensionContext;

	constructor(context: vscode.ExtensionContext) {
		this.context = context;
	}

	public async getAuthToken(): Promise<string | undefined> {
		const secret = await this.context.secrets.get(AuthManager.SECRET_STORAGE_KEY);
		if (secret) return secret;
		const configToken = vscode.workspace.getConfiguration('nachoFlow').get<string>('authToken');
		return configToken && configToken.trim() !== '' ? configToken.trim() : undefined;
	}

	public async setAuthToken(token: string): Promise<void> {
		await this.context.secrets.store(AuthManager.SECRET_STORAGE_KEY, token);
	}

	public async deleteAuthToken(): Promise<void> {
		await this.context.secrets.delete(AuthManager.SECRET_STORAGE_KEY);
	}

	public async getBaseUrl(): Promise<string> {
		return vscode.workspace.getConfiguration('nachoFlow').get('daemonUrl', 'http://127.0.0.1:8000');
	}
}