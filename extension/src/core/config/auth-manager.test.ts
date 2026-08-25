import * as vscode from 'vscode';
import { AuthManager } from './auth-manager';

// Mock VS Code API
jest.mock('vscode', () => ({
  workspace: {
    getConfiguration: jest.fn().mockReturnValue({
      get: jest.fn(),
      update: jest.fn().mockResolvedValue(undefined)
    })
  },
  ConfigurationTarget: {
    Global: 1
  }
}), { virtual: true });

describe('AuthManager', () => {
  let authManager: AuthManager;
  let mockContext: vscode.ExtensionContext;
  let mockSecrets: any;
  let mockGlobalState: any;

  beforeEach(() => {
    jest.clearAllMocks();

    mockSecrets = {
      get: jest.fn(),
      store: jest.fn(),
      delete: jest.fn()
    };

    const stateStorage = new Map<string, any>();
    mockGlobalState = {
      get: jest.fn((key: string, defaultValue?: any) => stateStorage.has(key) ? stateStorage.get(key) : defaultValue),
      update: jest.fn((key: string, value: any) => { stateStorage.set(key, value); return Promise.resolve(); })
    };

    mockContext = {
      secrets: mockSecrets,
      globalState: mockGlobalState
    } as any;

    authManager = new AuthManager(mockContext);
  });

  describe('engineMode', () => {
    it('should default to local mode', () => {
      expect(authManager.getEngineMode()).toBe('local');
    });

    it('should set and get engine mode', async () => {
      await authManager.setEngineMode('remote');
      expect(authManager.getEngineMode()).toBe('remote');
      expect(mockGlobalState.update).toHaveBeenCalledWith('nacho-flow.engine-mode', 'remote');
    });
  });

  describe('remoteUrl', () => {
    it('should return default fallback remote URL when not configured', () => {
      (vscode.workspace.getConfiguration as jest.Mock).mockReturnValue({
        get: jest.fn().mockReturnValue(undefined)
      });
      expect(authManager.getRemoteUrl()).toBe('http://192.168.0.205:8000');
    });

    it('should save and retrieve custom remote URL', async () => {
      await authManager.setRemoteUrl('http://10.0.0.50:8000');
      expect(authManager.getRemoteUrl()).toBe('http://10.0.0.50:8000');
      expect(mockGlobalState.update).toHaveBeenCalledWith('nacho-flow.remote-url', 'http://10.0.0.50:8000');
    });
  });

  describe('remoteToken', () => {
    it('should save, retrieve, and delete remote token', async () => {
      mockSecrets.get.mockResolvedValue('secret-remote-token');
      expect(await authManager.getRemoteToken()).toBe('secret-remote-token');

      await authManager.setRemoteToken('new-token');
      expect(mockSecrets.store).toHaveBeenCalledWith('nacho-flow.remote-auth-token', 'new-token');

      await authManager.setRemoteToken('');
      expect(mockSecrets.delete).toHaveBeenCalledWith('nacho-flow.remote-auth-token');
    });
  });

  describe('getBaseUrl', () => {
    it('should return 127.0.0.1:8000 in local mode', async () => {
      await authManager.setEngineMode('local');
      expect(await authManager.getBaseUrl()).toBe('http://127.0.0.1:8000');
    });

    it('should return saved remote URL in remote mode', async () => {
      await authManager.setEngineMode('remote');
      await authManager.setRemoteUrl('http://myserver.tailscale:8000');
      expect(await authManager.getBaseUrl()).toBe('http://myserver.tailscale:8000');
    });
  });

  describe('getAuthToken & setAuthToken & deleteAuthToken', () => {
    it('should store and retrieve local token in local mode', async () => {
      await authManager.setEngineMode('local');
      mockSecrets.get.mockResolvedValue('local-token');

      expect(await authManager.getAuthToken()).toBe('local-token');
      expect(mockSecrets.get).toHaveBeenCalledWith('nacho-flow.auth-token');

      await authManager.setAuthToken('new-local-token');
      expect(mockSecrets.store).toHaveBeenCalledWith('nacho-flow.auth-token', 'new-local-token');

      await authManager.deleteAuthToken();
      expect(mockSecrets.delete).toHaveBeenCalledWith('nacho-flow.auth-token');
    });

    it('should store and retrieve remote token in remote mode', async () => {
      await authManager.setEngineMode('remote');
      mockSecrets.get.mockResolvedValue('remote-token');

      expect(await authManager.getAuthToken()).toBe('remote-token');
      expect(mockSecrets.get).toHaveBeenCalledWith('nacho-flow.remote-auth-token');

      await authManager.setAuthToken('new-remote-token');
      expect(mockSecrets.store).toHaveBeenCalledWith('nacho-flow.remote-auth-token', 'new-remote-token');

      await authManager.deleteAuthToken();
      expect(mockSecrets.delete).toHaveBeenCalledWith('nacho-flow.remote-auth-token');
    });
  });
});