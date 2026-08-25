import * as vscode from 'vscode';
import { AuthManager } from './auth-manager';

// Mock VS Code API
jest.mock('vscode', () => ({
  workspace: {
    getConfiguration: jest.fn().mockReturnValue({
      get: jest.fn()
    })
  },
  SecretStorage: jest.fn().mockImplementation(() => ({
    get: jest.fn(),
    store: jest.fn(),
    delete: jest.fn()
  }))
}), { virtual: true });

describe('AuthManager', () => {
  let authManager: AuthManager;
  let mockContext: vscode.ExtensionContext;
  let mockSecrets: any;

  beforeEach(() => {
    // Reset mocks
    jest.clearAllMocks();

    // Create mock secrets storage
    mockSecrets = {
      get: jest.fn(),
      store: jest.fn(),
      delete: jest.fn()
    };

    // Create mock context
    mockContext = {
      secrets: mockSecrets
    } as any;

    // Create AuthManager instance
    authManager = new AuthManager(mockContext);
  });

  describe('getAuthToken', () => {
    it('should retrieve auth token from secret storage', async () => {
      const mockToken = 'test-token';
      mockSecrets.get.mockResolvedValue(mockToken);

      const result = await authManager.getAuthToken();

      expect(result).toBe(mockToken);
      expect(mockSecrets.get).toHaveBeenCalledWith('nacho-flow.auth-token');
    });

    it('should return undefined if no auth token is stored', async () => {
      mockSecrets.get.mockResolvedValue(undefined);

      const result = await authManager.getAuthToken();

      expect(result).toBeUndefined();
      expect(mockSecrets.get).toHaveBeenCalledWith('nacho-flow.auth-token');
    });
  });

  describe('setAuthToken', () => {
    it('should store auth token in secret storage', async () => {
      const mockToken = 'test-token';

      await authManager.setAuthToken(mockToken);

      expect(mockSecrets.store).toHaveBeenCalledWith('nacho-flow.auth-token', mockToken);
    });
  });

  describe('deleteAuthToken', () => {
    it('should delete auth token from secret storage', async () => {
      await authManager.deleteAuthToken();

      expect(mockSecrets.delete).toHaveBeenCalledWith('nacho-flow.auth-token');
    });
  });

  describe('getBaseUrl', () => {
    it('should return configured daemon URL', async () => {
      const mockUrl = 'http://localhost:8000';
      (vscode.workspace.getConfiguration as jest.Mock).mockReturnValue({
        get: jest.fn().mockReturnValue(mockUrl)
      });

      const result = await authManager.getBaseUrl();

      expect(result).toBe(mockUrl);
      expect(vscode.workspace.getConfiguration).toHaveBeenCalledWith('nachoFlow');
    });

    it('should return default URL when not configured', async () => {
      (vscode.workspace.getConfiguration as jest.Mock).mockReturnValue({
        get: jest.fn((_key: string, defaultValue?: string) => defaultValue)
      });

      const result = await authManager.getBaseUrl();

      expect(result).toBe('http://127.0.0.1:8000');
    });
  });
});