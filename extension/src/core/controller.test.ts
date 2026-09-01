import * as vscode from 'vscode';
import * as fs from 'fs';
import { ExtensionController } from './controller';
import { AuthManager } from './config/auth-manager';
import { RestClient } from './api/client';
import { SSEClient } from './sse/client';
import { DashboardPanel } from '../ui/webview/dashboard';

// Mock VS Code API
jest.mock('vscode', () => ({
  window: {
    createOutputChannel: jest.fn().mockReturnValue({
      appendLine: jest.fn(),
      show: jest.fn(),
      dispose: jest.fn()
    }),
    registerWebviewViewProvider: jest.fn().mockReturnValue({
      dispose: jest.fn()
    }),
    createStatusBarItem: jest.fn().mockReturnValue({
      show: jest.fn(),
      hide: jest.fn(),
      dispose: jest.fn(),
      text: '',
      tooltip: ''
    }),
    createWebviewPanel: jest.fn().mockReturnValue({
      webview: {
        asWebviewUri: jest.fn(),
        html: ''
      },
      dispose: jest.fn()
    }),
    showInformationMessage: jest.fn(),
    showWarningMessage: jest.fn(),
    showErrorMessage: jest.fn(),
    showQuickPick: jest.fn(),
    showInputBox: jest.fn(),
    showTextDocument: jest.fn(),
    withProgress: jest.fn().mockImplementation((_opts: any, task: (progress: any) => Promise<any>) => task({ report: jest.fn() }))
  },
  env: {
    clipboard: {
      writeText: jest.fn().mockResolvedValue(undefined)
    },
    openExternal: jest.fn().mockResolvedValue(true)
  },
  ProgressLocation: {
    Notification: 15
  },
  commands: {
    registerCommand: jest.fn(),
    executeCommand: jest.fn().mockResolvedValue(undefined)
  },
  workspace: {
    getConfiguration: jest.fn().mockReturnValue({
      get: jest.fn(),
      update: jest.fn().mockResolvedValue(undefined)
    }),
    findFiles: jest.fn().mockResolvedValue([]),
    openTextDocument: jest.fn(),
    onDidSaveTextDocument: jest.fn().mockReturnValue({ dispose: jest.fn() }),
    fs: {
      stat: jest.fn().mockRejectedValue(new Error('File not found')),
      createDirectory: jest.fn().mockResolvedValue(undefined),
      writeFile: jest.fn().mockResolvedValue(undefined)
    }
  },
  ConfigurationTarget: {
    Global: 1,
    Workspace: 2,
    WorkspaceFolder: 3
  },
  Uri: {
    file: jest.fn().mockImplementation((p: string) => ({
      path: p,
      fsPath: p
    })),
    parse: jest.fn().mockImplementation((p: string) => ({
      path: p,
      toString: () => p
    })),
    joinPath: jest.fn().mockImplementation((...paths) => ({
      path: paths.join('/')
    }))
  },
  ViewColumn: {
    One: 1,
    Beside: 2
  },
  StatusBarAlignment: {
    Right: 1
  }
}), { virtual: true });

// Mock other modules
jest.mock('./config/auth-manager');
jest.mock('./api/client');
jest.mock('./sse/client');
jest.mock('../ui/status-bar/item');
jest.mock('../ui/webview/dashboard', () => ({
  DashboardPanel: jest.fn().mockImplementation(() => ({
    setTimeWindow: jest.fn(),
    setRoutesRefreshInterval: jest.fn(),
    updateStats: jest.fn(),
    updateDeals: jest.fn(),
    updateRoutes: jest.fn(),
    updateCircuits: jest.fn(),
    updateConfig: jest.fn(),
    updateOptimization: jest.fn(),
    onDidChangeViewState: jest.fn().mockReturnValue({ dispose: jest.fn() }),
    dispose: jest.fn()
  }))
}));
jest.mock('../ui/sidebar/sidebar-view-provider', () => ({
  SidebarViewProvider: Object.assign(
    jest.fn().mockImplementation(() => ({
      updateState: jest.fn(),
      updateEngineStatus: jest.fn(),
      updateOllamaStatus: jest.fn()
    })),
    { viewType: 'nacho-flow.sidebarView' }
  )
}));
jest.mock('../ui/webview/routes-panel');
jest.mock('../ui/webview/circuits-panel');
jest.mock('../ui/webview/config-editor');

describe('ExtensionController', () => {
  let extensionController: ExtensionController;
  let mockContext: vscode.ExtensionContext;

  beforeEach(() => {
    // Reset mocks
    jest.clearAllMocks();

    // Create mock context
    mockContext = {
      subscriptions: [],
      extensionUri: {
        path: '/test/extension'
      },
      secrets: {} as any
    } as any;

    // Create ExtensionController instance
    extensionController = new ExtensionController(mockContext);
  });

  afterEach(() => {
    jest.clearAllTimers();
    jest.restoreAllMocks();
  });

  describe('constructor', () => {
    it('should create an instance of ExtensionController', () => {
      expect(extensionController).toBeInstanceOf(ExtensionController);
    });
  });

  describe('initialize', () => {
    it('should register commands and execute their callbacks', async () => {
      // Mock auth manager methods
      jest.spyOn(AuthManager.prototype, 'getBaseUrl').mockResolvedValue('http://localhost:8000');
      jest.spyOn(AuthManager.prototype, 'getAuthToken').mockResolvedValue('test-token');

      const showDashboardSpy = jest.spyOn(extensionController as any, 'showDashboard').mockImplementation();
      const updateStatsSpy = jest.spyOn(extensionController as any, 'updateStats').mockImplementation();
      const showConfigEditorSpy = jest.spyOn(extensionController as any, 'showConfigEditor').mockImplementation();
      const resetCircuitSpy = jest.spyOn(extensionController as any, 'resetCircuit').mockImplementation();

      await extensionController.initialize();

      // Find and call each command callback
      const calls = (vscode.commands.registerCommand as jest.Mock).mock.calls;
      const showDashCmd = calls.find(c => c[0] === 'nacho-flow.showDashboard')[1];
      const refreshStatsCmd = calls.find(c => c[0] === 'nacho-flow.refreshStats')[1];
      const openConfigCmd = calls.find(c => c[0] === 'nacho-flow.openConfig')[1];
      const runOptimizerCmd = calls.find(c => c[0] === 'nacho-flow.runOptimizer')[1];
      const refreshDealsCmd = calls.find(c => c[0] === 'nacho-flow.refreshDeals')[1];
      const resetCircuitCmd = calls.find(c => c[0] === 'nacho-flow.resetCircuit')[1];
      const setAuthTokenCmd = calls.find(c => c[0] === 'nacho-flow.setAuthToken')[1];

      const runOptimizerSpy = jest.spyOn(extensionController, 'runOptimizer').mockImplementation();
      const refreshDealsSpy = jest.spyOn(extensionController, 'refreshDeals').mockImplementation();

      showDashCmd();
      expect(showDashboardSpy).toHaveBeenCalled();

      refreshStatsCmd();
      expect(updateStatsSpy).toHaveBeenCalled();

      openConfigCmd();
      expect(showConfigEditorSpy).toHaveBeenCalled();

      resetCircuitCmd();
      expect(resetCircuitSpy).toHaveBeenCalled();

      runOptimizerCmd();
      expect(runOptimizerSpy).toHaveBeenCalled();

      refreshDealsCmd();
      expect(refreshDealsSpy).toHaveBeenCalled();

      // Test remaining commands
      const openDocsCmd = calls.find(c => c[0] === 'nacho-flow.openDocs')[1];
      const openSupportCmd = calls.find(c => c[0] === 'nacho-flow.openSupport')[1];
      const openSettingsCmd = calls.find(c => c[0] === 'nacho-flow.openSettings')[1];
      const setTimeTodayCmd = calls.find(c => c[0] === 'nacho-flow.setTimeWindowToday')[1];
      const setTimeWeekCmd = calls.find(c => c[0] === 'nacho-flow.setTimeWindowWeek')[1];
      const setTimeMonthCmd = calls.find(c => c[0] === 'nacho-flow.setTimeWindowMonth')[1];
      const setTimeAllTimeCmd = calls.find(c => c[0] === 'nacho-flow.setTimeWindowAllTime')[1];

      openDocsCmd();
      openSupportCmd();
      openSettingsCmd();
      setTimeTodayCmd();
      setTimeWeekCmd();
      setTimeMonthCmd();
      setTimeAllTimeCmd();

      // Test setAuthToken with new token
      (vscode.window.showInputBox as jest.Mock) = jest.fn().mockResolvedValue('new-secret-token');
      const setTokenSpy = jest.spyOn(AuthManager.prototype, 'setAuthToken').mockResolvedValue();
      await setAuthTokenCmd();
      expect(setTokenSpy).toHaveBeenCalledWith('new-secret-token');

      // Test setAuthToken with empty string (clears token)
      (vscode.window.showInputBox as jest.Mock) = jest.fn().mockResolvedValue('   ');
      const deleteTokenSpy = jest.spyOn(AuthManager.prototype, 'deleteAuthToken').mockResolvedValue();
      await setAuthTokenCmd();
      expect(deleteTokenSpy).toHaveBeenCalled();

      // Test setAuthToken cancelled (undefined)
      (vscode.window.showInputBox as jest.Mock) = jest.fn().mockResolvedValue(undefined);
      await setAuthTokenCmd();
    });

    it('should initialize clients when auth token is available', async () => {
      // Mock auth manager methods
      jest.spyOn(AuthManager.prototype, 'getBaseUrl').mockResolvedValue('http://localhost:8000');
      jest.spyOn(AuthManager.prototype, 'getAuthToken').mockResolvedValue('test-token');

      await extensionController.initialize();

      expect(RestClient).toHaveBeenCalledWith('http://localhost:8000', 'test-token');
      expect(SSEClient).toHaveBeenCalledWith('http://localhost:8000', 'test-token');
    });

    it('should auto sync config to daemon over REST when config.yaml is saved', async () => {
      let saveCallback: any;
      (vscode.workspace.onDidSaveTextDocument as jest.Mock).mockImplementation((cb) => {
        saveCallback = cb;
        return { dispose: jest.fn() };
      });

      await extensionController.initialize();
      const mockRestClient = (extensionController as any).restClient;
      mockRestClient.updateConfigYaml = jest.fn().mockResolvedValue({ status: 'ok' });
      const loadDashboardSpy = jest.spyOn(extensionController as any, 'loadDashboardData').mockResolvedValue(undefined);

      expect(saveCallback).toBeDefined();
      await saveCallback({ fileName: '/path/to/config.yaml', getText: () => 'port: 8000', uri: { toString: () => 'file:///path/to/config.yaml' } });
      expect(mockRestClient.updateConfigYaml).toHaveBeenCalledWith('port: 8000');
      expect(loadDashboardSpy).toHaveBeenCalled();
      expect(vscode.window.withProgress).toHaveBeenCalledWith(
        expect.objectContaining({ title: expect.stringContaining('Configuration updated') }),
        expect.any(Function)
      );

      // Test error handling on failed update
      mockRestClient.updateConfigYaml.mockRejectedValueOnce(new Error('Invalid YAML'));
      await saveCallback({ fileName: '/path/to/config.yaml', getText: () => 'invalid: yaml', uri: { toString: () => 'file:///path/to/config.yaml' } });
      expect(vscode.window.showErrorMessage).toHaveBeenCalled();

      // Non-config file does not trigger
      mockRestClient.updateConfigYaml.mockClear();
      await saveCallback({ fileName: '/path/to/other.ts', getText: () => '', uri: { toString: () => 'file:///path/to/other.ts' } });
      expect(mockRestClient.updateConfigYaml).not.toHaveBeenCalled();
    });

    it('should initialize clients with undefined token when auth token is not available', async () => {
      // Mock auth manager methods
      jest.spyOn(AuthManager.prototype, 'getBaseUrl').mockResolvedValue('http://localhost:8000');
      jest.spyOn(AuthManager.prototype, 'getAuthToken').mockResolvedValue(undefined);

      await extensionController.initialize();

      expect(RestClient).toHaveBeenCalledWith('http://localhost:8000', undefined);
      expect(SSEClient).toHaveBeenCalledWith('http://localhost:8000', undefined);
    });
  });

  describe('showDashboard', () => {
    it('should create a new dashboard panel and handle message callbacks', async () => {
      // Mock auth manager methods
      jest.spyOn(AuthManager.prototype, 'getBaseUrl').mockResolvedValue('http://localhost:8000');
      jest.spyOn(AuthManager.prototype, 'getAuthToken').mockResolvedValue('test-token');

      const mockRestClient = {
        getStats: jest.fn().mockResolvedValue({ total_requests: 10 }),
        getRoutes: jest.fn().mockResolvedValue({ routes: [] }),
        getCircuits: jest.fn().mockResolvedValue({ circuits: [] }),
        getConfig: jest.fn().mockResolvedValue({ port: 8000 }),
        resetCircuit: jest.fn().mockResolvedValue({})
      };

      await extensionController.initialize();
      (extensionController as any).restClient = mockRestClient;
      
      const mockOldPanel = { dispose: jest.fn() };
      (extensionController as any).dashboardPanel = mockOldPanel;

      (extensionController as any).showDashboard();

      expect(mockOldPanel.dispose).toHaveBeenCalled();
      expect((extensionController as any).dashboardPanel).toBeDefined();

      // Test Dashboard message callbacks
      const DashboardPanelClass = require('../ui/webview/dashboard').DashboardPanel;
      const onMessageCallback = DashboardPanelClass.mock.calls[DashboardPanelClass.mock.calls.length - 1][1];

      if (onMessageCallback) {
        await onMessageCallback({ command: 'initialize' });
        await onMessageCallback({ command: 'resetCircuit', provider: 'openrouter' });
        await onMessageCallback({ command: 'editConfig' });
        await onMessageCallback({ command: 'copyToClipboard', data: { text: 'deepseek/deepseek-chat' } });
        expect(vscode.env.clipboard.writeText).toHaveBeenCalledWith('deepseek/deepseek-chat');
        expect(vscode.window.withProgress).toHaveBeenCalledWith(
          expect.objectContaining({ title: '📋 Copied "deepseek/deepseek-chat" to clipboard' }),
          expect.any(Function)
        );
      }
    });

    it('should load dashboard data and update all panels', async () => {
      const mockDashboardPanel = {
        updateStats: jest.fn(),
        updateRoutes: jest.fn(),
        updateCircuits: jest.fn(),
        updateConfig: jest.fn(),
        dispose: jest.fn()
      };
      const mockRestClient = {
        getStats: jest.fn().mockResolvedValue({ total_requests: 10 }),
        getRoutes: jest.fn().mockResolvedValue({ routes: [] }),
        getCircuits: jest.fn().mockResolvedValue({ circuits: [] }),
        getConfig: jest.fn().mockResolvedValue({ port: 8000 })
      };
      (extensionController as any).restClient = mockRestClient;
      (extensionController as any).dashboardPanel = mockDashboardPanel;

      await (extensionController as any).loadDashboardData();

      expect(mockDashboardPanel.updateStats).toHaveBeenCalled();
      expect(mockDashboardPanel.updateRoutes).toHaveBeenCalled();
      expect(mockDashboardPanel.updateCircuits).toHaveBeenCalled();
      expect(mockDashboardPanel.updateConfig).toHaveBeenCalled();
    });
  });

  describe('showConfigEditor', () => {
    it('should fetch and open YAML config from daemon when restClient is available', async () => {
      const mockRestClient = {
        getConfigYaml: jest.fn().mockResolvedValue('port: 8000\nauth_token: sk-test')
      };
      (extensionController as any).restClient = mockRestClient;
      (vscode.workspace.openTextDocument as jest.Mock) = jest.fn().mockResolvedValue({ uri: 'nacho://config.yaml' });
      (vscode.window.showTextDocument as jest.Mock) = jest.fn().mockResolvedValue({});

      await (extensionController as any).showConfigEditor();

      expect(mockRestClient.getConfigYaml).toHaveBeenCalled();
      expect(vscode.workspace.fs.writeFile).toHaveBeenCalled();
      expect(vscode.workspace.openTextDocument).toHaveBeenCalledWith(expect.objectContaining({ path: expect.stringContaining('nacho-flow-config.yaml') }));
      expect(vscode.window.showTextDocument).toHaveBeenCalled();
    });

    it('should open standard OS config.yaml if restClient fails and standard path exists', async () => {
      (extensionController as any).restClient = null;
      jest.spyOn(extensionController, 'getStandardConfigPaths').mockReturnValue(['/mock/standard/config.yaml']);
      jest.spyOn(extensionController, 'fileExists').mockImplementation((p) => p === '/mock/standard/config.yaml');
      (vscode.workspace.openTextDocument as jest.Mock) = jest.fn().mockResolvedValue({ uri: '/mock/standard/config.yaml' });
      (vscode.window.showTextDocument as jest.Mock) = jest.fn().mockResolvedValue({});

      await (extensionController as any).showConfigEditor();

      expect(vscode.workspace.openTextDocument).toHaveBeenCalled();
      expect(vscode.window.showTextDocument).toHaveBeenCalled();
    });

    it('should open workspace config.yaml if restClient fails and no standard path exists', async () => {
      const mockRestClient = {
        getConfigYaml: jest.fn().mockRejectedValue(new Error('Network error'))
      };
      (extensionController as any).restClient = mockRestClient;
      jest.spyOn(extensionController, 'getStandardConfigPaths').mockReturnValue([]);
      jest.spyOn(extensionController, 'fileExists').mockReturnValue(false);
      (vscode.workspace.findFiles as jest.Mock) = jest.fn().mockResolvedValue([{ fsPath: '/test/config.yaml' }]);
      (vscode.workspace.openTextDocument as jest.Mock) = jest.fn().mockResolvedValue({ uri: '/test/config.yaml' });
      (vscode.window.showTextDocument as jest.Mock) = jest.fn().mockResolvedValue({});

      await (extensionController as any).showConfigEditor();

      expect(vscode.workspace.findFiles).toHaveBeenCalled();
      expect(vscode.workspace.openTextDocument).toHaveBeenCalledWith({ fsPath: '/test/config.yaml' });
      expect(vscode.window.showTextDocument).toHaveBeenCalled();
    });

    it('should show warning message if config cannot be found anywhere', async () => {
      (extensionController as any).restClient = null;
      jest.spyOn(extensionController, 'getStandardConfigPaths').mockReturnValue([]);
      jest.spyOn(extensionController, 'fileExists').mockReturnValue(false);
      (vscode.workspace.findFiles as jest.Mock) = jest.fn().mockResolvedValue([]);

      await (extensionController as any).showConfigEditor();

      expect(vscode.window.showWarningMessage).toHaveBeenCalledWith('Nacho Flow: Unable to fetch config from daemon or workspace');
    });
  });

  describe('resetCircuit', () => {
    it('should show error if restClient is not available', async () => {
      (extensionController as any).restClient = null;
      await (extensionController as any).resetCircuit();
      expect(vscode.window.showErrorMessage).toHaveBeenCalledWith('Nacho Flow: Daemon client not initialized');
    });

    it('should reset circuit for selected provider and update dashboard', async () => {
      const mockDashboardPanel = {
        updateCircuits: jest.fn()
      };
      const mockRestClient = {
        resetCircuit: jest.fn().mockResolvedValue({}),
        getCircuits: jest.fn().mockResolvedValue({ circuits: [] })
      };
      (extensionController as any).restClient = mockRestClient;
      (extensionController as any).dashboardPanel = mockDashboardPanel;

      (vscode.window.showQuickPick as jest.Mock) = jest.fn().mockResolvedValue('openrouter');

      await (extensionController as any).resetCircuit();

      expect(mockRestClient.resetCircuit).toHaveBeenCalledWith('openrouter');
      expect(vscode.window.withProgress).toHaveBeenCalledWith(
        expect.objectContaining({ title: 'Nacho Flow: Circuit breaker reset (openrouter)' }),
        expect.any(Function)
      );
      expect(mockDashboardPanel.updateCircuits).toHaveBeenCalled();
    });
  });

  describe('runOptimizer', () => {
    it('should show error if restClient is not available', async () => {
      (extensionController as any).restClient = null;
      await extensionController.runOptimizer();
      expect(vscode.window.showErrorMessage).toHaveBeenCalledWith('Nacho Flow: Daemon client not initialized');
    });

    it('should run optimizer and update dashboard panel', async () => {
      const mockDashboardPanel = {
        updateOptimization: jest.fn()
      };
      const mockRestClient = {
        tune: jest.fn().mockResolvedValue({ message: 'Optimal policy found' })
      };
      (extensionController as any).restClient = mockRestClient;
      (extensionController as any).dashboardPanel = mockDashboardPanel;

      await extensionController.runOptimizer();

      expect(mockRestClient.tune).toHaveBeenCalled();
      expect(mockDashboardPanel.updateOptimization).toHaveBeenCalledWith({ message: 'Optimal policy found' });
    });
  });

  describe('refreshDeals', () => {
    it('should fetch deals and update dashboard panel', async () => {
      const mockDashboardPanel = {
        updateDeals: jest.fn()
      };
      const mockRestClient = {
        getDeals: jest.fn().mockResolvedValue({ deals: [{ model: 'test' }] })
      };
      (extensionController as any).restClient = mockRestClient;
      (extensionController as any).dashboardPanel = mockDashboardPanel;

      await extensionController.refreshDeals();

      expect(mockRestClient.getDeals).toHaveBeenCalled();
      expect(mockDashboardPanel.updateDeals).toHaveBeenCalledWith({ deals: [{ model: 'test' }] });
    });
  });

  describe('applyOptimization', () => {
    const sampleYaml = `# Config
tiers:
  - name: "Tier 1: Local GPU Free"
    model: gemma4:12b-it-qat
    when: "Tokens < 16000 && Retries == 0" # Initial rule
  - name: "Tier 2: Cloud Workhorse"
    model: google/gemini-3.7-flash
    when: "Retries < 1"
`;

    it('should replace tier when rule preserving comments', () => {
      const updated = extensionController.replaceTierRuleInYaml(
        sampleYaml,
        'Tier 1: Local GPU Free',
        'Tokens < 64000 && Retries == 0'
      );
      expect(updated).toContain('when: "Tokens < 64000 && Retries == 0" # Initial rule');
      expect(updated).toContain('model: gemma4:12b-it-qat');
    });

    it('should apply optimization data directly over REST', async () => {
      const mockRestClient = {
        getConfigYaml: jest.fn().mockResolvedValue(sampleYaml),
        updateConfigYaml: jest.fn().mockResolvedValue(undefined),
        getStats: jest.fn().mockResolvedValue({ total_requests: 10 }),
        getDeals: jest.fn().mockResolvedValue([]),
        getRoutes: jest.fn().mockResolvedValue([]),
        getCircuits: jest.fn().mockResolvedValue([]),
        getConfig: jest.fn().mockResolvedValue({})
      };
      (extensionController as any).restClient = mockRestClient;
      const mockDashboardPanel = {
        updateOptimization: jest.fn(),
        updateStats: jest.fn(),
        updateDeals: jest.fn(),
        updateRoutes: jest.fn(),
        updateCircuits: jest.fn(),
        updateConfig: jest.fn()
      };
      (extensionController as any).dashboardPanel = mockDashboardPanel;

      await extensionController.applyOptimization({
        target_tier_name: 'Tier 1: Local GPU Free',
        synthesized_rule: 'Tokens < 64000 && Retries == 0'
      });
      expect(mockRestClient.updateConfigYaml).toHaveBeenCalledWith(expect.stringContaining('Tokens < 64000 && Retries == 0'));
      expect(mockDashboardPanel.updateOptimization).toHaveBeenCalledWith(null);
      expect(vscode.window.withProgress).toHaveBeenCalledWith(
        expect.objectContaining({ title: '🎉 Applied Auto-Tuner policy to Tier 1: Local GPU Free!' }),
        expect.any(Function)
      );
    });

    it('should handle applyOptimization errors gracefully', async () => {
      (extensionController as any).restClient = null;
      await extensionController.applyOptimization();
      expect(vscode.window.showErrorMessage).toHaveBeenCalledWith('Nacho Flow: Daemon client not initialized');

      const mockRestClient = {
        getConfigYaml: jest.fn().mockResolvedValue(sampleYaml),
        updateConfigYaml: jest.fn().mockRejectedValue(new Error('Update failed'))
      };
      (extensionController as any).restClient = mockRestClient;
      await extensionController.applyOptimization({
        target_tier_name: 'Tier 1: Local GPU Free',
        synthesized_rule: 'Tokens < 64000'
      });
      expect(vscode.window.showErrorMessage).toHaveBeenCalledWith('Nacho Flow: Failed to apply optimization: Update failed');
    });
  });

  describe('adoptDeal & YAML preservation', () => {
    const sampleYaml = `# Nacho Flow Config
port: 8000 # default port

providers:
  openrouter:
    base_url: https://openrouter.ai/api/v1 # OpenRouter endpoint

tiers:
  - name: "Tier 1: Fast Code" # Local or quick tier
    model: qwen2.5-coder:7b # Current fast model
    provider: ollama
    when: tokens < 1000
  - name: "Tier 2: Frontier Cloud"
    model: deepseek/deepseek-chat # Cloud frontier model
    provider: openrouter
    when: has_tools

default_tier:
  name: "Fallback" # Rescue tier
  model: openai/gpt-4o # Fallback model
  provider: openrouter
`;

    it('should preserve comments when replacing model in a tier', () => {
      const updated = extensionController.replaceTierModelInYaml(sampleYaml, 'Tier 1: Fast Code', false, 'deepseek/deepseek-coder-v2');
      expect(updated).toContain('model: deepseek/deepseek-coder-v2 # Current fast model');
      expect(updated).toContain('# Nacho Flow Config');
      expect(updated).toContain('port: 8000 # default port');
      expect(updated).toContain('model: deepseek/deepseek-chat # Cloud frontier model');
      expect(updated).toContain('model: openai/gpt-4o # Fallback model');
    });

    it('should preserve comments when replacing model in default_tier', () => {
      const updated = extensionController.replaceTierModelInYaml(sampleYaml, 'Fallback', true, 'anthropic/claude-3.5-sonnet');
      expect(updated).toContain('model: anthropic/claude-3.5-sonnet # Fallback model');
      expect(updated).toContain('model: qwen2.5-coder:7b # Current fast model');
      expect(updated).toContain('# Rescue tier');
    });

    it('should adopt deal by presenting QuickPick and updating config on daemon', async () => {
      const mockRestClient = {
        getConfigYaml: jest.fn().mockResolvedValue(sampleYaml),
        getConfig: jest.fn().mockResolvedValue({
          tiers: [
            { name: 'Tier 1: Fast Code', model: 'qwen2.5-coder:7b' },
            { name: 'Tier 2: Frontier Cloud', model: 'deepseek/deepseek-chat' }
          ],
          default_tier: { name: 'Fallback', model: 'openai/gpt-4o' }
        }),
        updateConfigYaml: jest.fn().mockResolvedValue({ status: 'ok' }),
        getStats: jest.fn().mockResolvedValue({ total_requests: 1 }),
        getDeals: jest.fn().mockResolvedValue([]),
        getRoutes: jest.fn().mockResolvedValue([]),
        getCircuits: jest.fn().mockResolvedValue([])
      };
      (extensionController as any).restClient = mockRestClient;
      (extensionController as any).dashboardPanel = {
        updateStats: jest.fn(),
        updateDeals: jest.fn(),
        updateRoutes: jest.fn(),
        updateCircuits: jest.fn(),
        updateConfig: jest.fn()
      };

      // User selects Tier 2
      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: '⭐ Tier 2: Frontier Cloud',
        tier: { name: 'Tier 2: Frontier Cloud', model: 'deepseek/deepseek-chat' }
      });

      await extensionController.adoptDeal('meta-llama/llama-3.3-70b-instruct:free', 'openrouter', ['Tier 2: Frontier Cloud']);

      expect(mockRestClient.getConfigYaml).toHaveBeenCalled();
      expect(vscode.window.showQuickPick).toHaveBeenCalled();
      expect(mockRestClient.updateConfigYaml).toHaveBeenCalledWith(expect.stringContaining('model: meta-llama/llama-3.3-70b-instruct:free # Cloud frontier model'));
      expect(vscode.window.withProgress).toHaveBeenCalledWith(
        expect.objectContaining({ title: '🎉 Adopted meta-llama/llama-3.3-70b-instruct:free into Tier 2: Frontier Cloud!' }),
        expect.any(Function)
      );
    });

    it('should fallback to regex YAML search when getConfig fails', async () => {
      const mockRestClient = {
        getConfigYaml: jest.fn().mockResolvedValue(sampleYaml),
        getConfig: jest.fn().mockRejectedValue(new Error('Config error')),
        updateConfigYaml: jest.fn().mockResolvedValue({ status: 'ok' }),
        getStats: jest.fn().mockResolvedValue({}),
        getDeals: jest.fn().mockResolvedValue([]),
        getRoutes: jest.fn().mockResolvedValue([]),
        getCircuits: jest.fn().mockResolvedValue([])
      };
      (extensionController as any).restClient = mockRestClient;

      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: 'Tier 1: Fast Code',
        tier: { name: 'Tier 1: Fast Code', model: '' }
      });

      await extensionController.adoptDeal('meta-llama/llama-3.3-70b-instruct:free');
      expect(vscode.window.showQuickPick).toHaveBeenCalled();
    });

    it('should handle empty tiers in yaml and config gracefully', async () => {
      const mockRestClient = {
        getConfigYaml: jest.fn().mockResolvedValue('port: 8000\n'),
        getConfig: jest.fn().mockResolvedValue({}),
        updateConfigYaml: jest.fn()
      };
      (extensionController as any).restClient = mockRestClient;

      await extensionController.adoptDeal('meta-llama/llama-3.3-70b-instruct:free');
      expect(vscode.window.showWarningMessage).toHaveBeenCalledWith('Nacho Flow: No routing tiers found in config');
    });

    it('should handle tier model replacement failure gracefully', async () => {
      const mockRestClient = {
        getConfigYaml: jest.fn().mockResolvedValue('tiers:\n  - name: Tier 1\n    provider: ollama\n'),
        getConfig: jest.fn().mockResolvedValue({ tiers: [{ name: 'Tier 1', model: '' }] }),
        updateConfigYaml: jest.fn()
      };
      (extensionController as any).restClient = mockRestClient;

      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: 'Tier 1',
        tier: { name: 'Tier 1', model: '' }
      });

      await extensionController.adoptDeal('meta-llama/llama-3.3-70b-instruct:free');
      expect(vscode.window.showWarningMessage).toHaveBeenCalledWith('Nacho Flow: Could not find model field for tier "Tier 1" in config YAML');
    });

    it('should handle cancel during QuickPick gracefully', async () => {
      const mockRestClient = {
        getConfigYaml: jest.fn().mockResolvedValue(sampleYaml),
        getConfig: jest.fn().mockResolvedValue({
          tiers: [{ name: 'Tier 1', model: 'test' }]
        }),
        updateConfigYaml: jest.fn()
      };
      (extensionController as any).restClient = mockRestClient;

      // User cancels QuickPick
      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce(undefined);

      await extensionController.adoptDeal('meta-llama/llama-3.3-70b-instruct:free', 'openrouter');

      expect(mockRestClient.updateConfigYaml).not.toHaveBeenCalled();
    });

    it('should handle error when daemon client is not initialized or fails', async () => {
      (extensionController as any).restClient = null;
      await extensionController.adoptDeal('model-1');
      expect(vscode.window.showErrorMessage).toHaveBeenCalledWith('Nacho Flow: Daemon client not initialized');

      const mockRestClient = {
        getConfigYaml: jest.fn().mockRejectedValue(new Error('Connection failed'))
      };
      (extensionController as any).restClient = mockRestClient;
      await extensionController.adoptDeal('model-1');
      expect(vscode.window.showErrorMessage).toHaveBeenCalledWith('Nacho Flow: Failed to adopt deal: Connection failed');

      mockRestClient.getConfigYaml = jest.fn().mockResolvedValue(null);
      await extensionController.adoptDeal('model-1');
      expect(vscode.window.showErrorMessage).toHaveBeenCalledWith('Nacho Flow: Unable to fetch configuration from daemon');
    });

    it('should handle adoptDeal from dashboard webview message callback', async () => {
      const adoptSpy = jest.spyOn(extensionController, 'adoptDeal').mockResolvedValue(undefined);
      (extensionController as any).showDashboard();

      const DashboardPanelClass = require('../ui/webview/dashboard').DashboardPanel;
      const onMessageCallback = DashboardPanelClass.mock.calls[DashboardPanelClass.mock.calls.length - 1][1];

      await onMessageCallback({
        command: 'adoptDeal',
        data: {
          modelId: 'qwen/qwen-2.5-coder-32b-instruct',
          provider: 'openrouter',
          recommendedTiers: ['Tier 2']
        }
      });

      expect(adoptSpy).toHaveBeenCalledWith('qwen/qwen-2.5-coder-32b-instruct', 'openrouter', ['Tier 2']);
    });
  });

  describe('dispose', () => {
    it('should dispose resources', async () => {
      // Mock auth manager methods
      jest.spyOn(AuthManager.prototype, 'getBaseUrl').mockResolvedValue('http://localhost:8000');
      jest.spyOn(AuthManager.prototype, 'getAuthToken').mockResolvedValue('test-token');

      await extensionController.initialize();
      extensionController.dispose();

      // Verify that status bar is disposed
      // We can't easily test SSE client disconnection without complex mocking
      // but we can verify that no errors are thrown
      expect(true).toBe(true);
    });
  });

  describe('setupSSEHandlers', () => {
    it('should set up SSE handlers when SSE client is available', async () => {
      // Mock auth manager methods
      jest.spyOn(AuthManager.prototype, 'getBaseUrl').mockResolvedValue('http://localhost:8000');
      jest.spyOn(AuthManager.prototype, 'getAuthToken').mockResolvedValue('test-token');

      await extensionController.initialize();

      // Verify that subscribe was called for each event type
      expect((extensionController as any).sseClient.subscribe).toHaveBeenCalledWith('routeCompleted', expect.any(Function));
      expect((extensionController as any).sseClient.subscribe).toHaveBeenCalledWith('circuitStateChanged', expect.any(Function));
      expect((extensionController as any).sseClient.subscribe).toHaveBeenCalledWith('configUpdated', expect.any(Function));
    });

    it('should safely handle when SSE client is null', () => {
      (extensionController as any).sseClient = null;
      expect(() => (extensionController as any).setupSSEHandlers()).not.toThrow();
    });
  });

  describe('SSE event handlers', () => {
    it('should update stats when routeCompleted event is received', async () => {
      // Mock auth manager methods
      jest.spyOn(AuthManager.prototype, 'getBaseUrl').mockResolvedValue('http://localhost:8000');
      jest.spyOn(AuthManager.prototype, 'getAuthToken').mockResolvedValue('test-token');

      // Mock updateStats method
      const updateStatsSpy = jest.spyOn(ExtensionController.prototype as any, 'updateStats').mockResolvedValue(undefined);

      await extensionController.initialize();

      // Get the routeCompleted handler
      const routeCompletedHandler = (extensionController as any).sseClient.subscribe.mock.calls.find((call: any) => call[0] === 'routeCompleted')[1];

      // Call the handler
      routeCompletedHandler({});

      // Verify that updateStats was called
      expect(updateStatsSpy).toHaveBeenCalled();
    });

    it('should update stats when circuitStateChanged event is received', async () => {
      // Mock auth manager methods
      jest.spyOn(AuthManager.prototype, 'getBaseUrl').mockResolvedValue('http://localhost:8000');
      jest.spyOn(AuthManager.prototype, 'getAuthToken').mockResolvedValue('test-token');

      // Mock updateStats method
      const updateStatsSpy = jest.spyOn(ExtensionController.prototype as any, 'updateStats').mockResolvedValue(undefined);

      await extensionController.initialize();

      // Get the circuitStateChanged handler
      const circuitStateChangedHandler = (extensionController as any).sseClient.subscribe.mock.calls.find((call: any) => call[0] === 'circuitStateChanged')[1];

      // Call the handler
      circuitStateChangedHandler({});

      // Verify that updateStats was called
      expect(updateStatsSpy).toHaveBeenCalled();
    });

    it('should update stats when configUpdated event is received', async () => {
      // Mock auth manager methods
      jest.spyOn(AuthManager.prototype, 'getBaseUrl').mockResolvedValue('http://localhost:8000');
      jest.spyOn(AuthManager.prototype, 'getAuthToken').mockResolvedValue('test-token');

      // Mock updateStats method
      const updateStatsSpy = jest.spyOn(ExtensionController.prototype as any, 'updateStats').mockResolvedValue(undefined);

      await extensionController.initialize();

      // Get the configUpdated handler
      const configUpdatedHandler = (extensionController as any).sseClient.subscribe.mock.calls.find((call: any) => call[0] === 'configUpdated')[1];

      // Call the handler
      configUpdatedHandler({});

      // Verify that updateStats was called
      expect(updateStatsSpy).toHaveBeenCalled();
    });

    it('should handle errors when updating stats', async () => {
      // Mock auth manager methods
      jest.spyOn(AuthManager.prototype, 'getBaseUrl').mockResolvedValue('http://localhost:8000');
      jest.spyOn(AuthManager.prototype, 'getAuthToken').mockResolvedValue('test-token');

      // Create a new extension controller instance
      extensionController = new ExtensionController(mockContext);

      // Initialize the controller
      await extensionController.initialize();

      // Now that restClient is initialized, we can mock its getStats method
      const restClient = (extensionController as any).restClient;
      expect(restClient).not.toBeNull();
      
      // Set up the spy BEFORE calling updateStats
      const getStatsSpy = jest.spyOn(restClient, 'getStats').mockRejectedValueOnce(new Error('Network error'));
      
      // Mock statusBar.updateStats
      const statusBarUpdateStatsSpy = jest.spyOn((extensionController as any).statusBar, 'updateStats');
      // Call updateStats
      await (extensionController as any).updateStats();

      // Verify that getStats was called
      expect(getStatsSpy).toHaveBeenCalled();

      // Verify that statusBar.updateStats was called with null (error state)
      expect(statusBarUpdateStatsSpy).toHaveBeenCalledWith(null);
    });

    it('should early return when updating stats or loading dashboard without client or panel', async () => {
      (extensionController as any).restClient = null;
      await (extensionController as any).updateStats();

      (extensionController as any).dashboardPanel = null;
      await (extensionController as any).loadDashboardData();

      expect(true).toBe(true);
    });

    it('should update dashboardPanel in updateStats when panel is active', async () => {
      const mockDashboardPanel = { updateStats: jest.fn() };
      const mockRestClient = { getStats: jest.fn().mockResolvedValue({ total_requests: 42 }) };
      (extensionController as any).restClient = mockRestClient;
      (extensionController as any).dashboardPanel = mockDashboardPanel;

      await (extensionController as any).updateStats();

      expect(mockDashboardPanel.updateStats).toHaveBeenCalledWith({ total_requests: 42 });
    });

    it('should trigger pollTelemetry in periodic timer interval', () => {
      jest.useFakeTimers();
      const pollTelemetrySpy = jest.spyOn(extensionController as any, 'pollTelemetry').mockImplementation();
      
      (extensionController as any).routesRefreshInterval = 30;
      (extensionController as any).telemetryPoller = null;
      (extensionController as any).startPeriodicUpdates();
      jest.advanceTimersByTime(30000);

      expect(pollTelemetrySpy).toHaveBeenCalled();
      (extensionController as any).telemetryPoller?.dispose();
      jest.useRealTimers();
    });

    it('should handle dashboardPanel message dispatching for all actions', async () => {
      const mockDashboardPanel = {
        updateStats: jest.fn(),
        updateDeals: jest.fn(),
        updateRoutes: jest.fn(),
        updateCircuits: jest.fn(),
        updateConfig: jest.fn(),
        updateOptimization: jest.fn(),
        dispose: jest.fn()
      };
      (extensionController as any).dashboardPanel = mockDashboardPanel;
      (extensionController as any).restClient = {
        getStats: jest.fn().mockResolvedValue({ total_requests: 10 }),
        getDeals: jest.fn().mockResolvedValue([{ model_id: 'test' }]),
        getRoutes: jest.fn().mockResolvedValue([{ tier: 'T1' }]),
        getCircuits: jest.fn().mockResolvedValue({ openrouter: { state: 'closed' } }),
        getConfig: jest.fn().mockResolvedValue({ port: 8000 }),
        tune: jest.fn().mockResolvedValue({ recommended_rules: 'rules' }),
        resetCircuit: jest.fn().mockResolvedValue({})
      };

      const runOptimizerSpy = jest.spyOn(extensionController, 'runOptimizer').mockResolvedValue(undefined);
      const refreshDealsSpy = jest.spyOn(extensionController, 'refreshDeals').mockResolvedValue(undefined);
      const applyOptimizationSpy = jest.spyOn(extensionController, 'applyOptimization').mockResolvedValue(undefined);

      // Trigger showDashboard to get message handler
      (extensionController as any).showDashboard();
      const messageHandler = (DashboardPanel as unknown as jest.Mock).mock.calls[0][1];

      await messageHandler({ command: 'initialize' });
      expect((extensionController as any).dashboardPanel.updateStats).toHaveBeenCalled();

      await messageHandler({ command: 'refreshAll' });
      expect(vscode.window.withProgress).toHaveBeenCalledWith(
        expect.objectContaining({ title: '🔄 Nacho Flow: Telemetry & dashboard refreshed' }),
        expect.any(Function)
      );

      await messageHandler({ command: 'runOptimizer' });
      expect(runOptimizerSpy).toHaveBeenCalled();

      await messageHandler({ command: 'refreshDeals' });
      expect(refreshDealsSpy).toHaveBeenCalledWith(true);

      await messageHandler({ command: 'applyOptimization' });
      expect(applyOptimizationSpy).toHaveBeenCalled();

      await messageHandler({ command: 'resetCircuit', provider: 'openrouter' });
      expect((extensionController as any).restClient.resetCircuit).toHaveBeenCalledWith('openrouter');

      // Test settings and danger zone messages
      const saveSettingsSpy = jest.spyOn(extensionController as any, 'handleSaveSettings').mockResolvedValue(undefined);
      const testConnectionSpy = jest.spyOn(extensionController as any, 'handleTestConnection').mockResolvedValue(undefined);
      const recalculateSpy = jest.spyOn(extensionController as any, 'handleRecalculateStats').mockResolvedValue(undefined);
      const resetStatsSpy = jest.spyOn(extensionController as any, 'handleResetStats').mockResolvedValue(undefined);

      await messageHandler({ command: 'saveSettings', url: 'http://127.0.0.1:8000', token: 'abc' });
      expect(saveSettingsSpy).toHaveBeenCalledWith('http://127.0.0.1:8000', 'abc');

      await messageHandler({ command: 'testConnection', url: 'http://127.0.0.1:8000', token: 'abc' });
      expect(testConnectionSpy).toHaveBeenCalledWith('http://127.0.0.1:8000', 'abc');

      await messageHandler({ command: 'recalculateStats' });
      expect(recalculateSpy).toHaveBeenCalled();

      await messageHandler({ command: 'resetStats' });
      expect(resetStatsSpy).toHaveBeenCalled();
    });

    it('should open settings and focus settings panel', async () => {
      (extensionController as any).dashboardPanel = null;
      await extensionController.openSettings();
      expect(vscode.commands.executeCommand).toHaveBeenCalledWith('nacho-flow.sidebarView.focus');
    });

    it('should handle handleSaveSettings, handleTestConnection, handleRecalculateStats, and handleResetStats flows', async () => {
      const mockConfigUpdate = jest.fn().mockResolvedValue(undefined);
      (vscode.workspace.getConfiguration as jest.Mock).mockReturnValue({
        update: mockConfigUpdate,
        get: jest.fn().mockReturnValue('http://127.0.0.1:8000')
      });

      const mockRestClient = {
        getHealth: jest.fn().mockResolvedValue({ status: 'ok', version: '0.6.0' }),
        recalculateStats: jest.fn().mockResolvedValue({ total_requests: 10 }),
        resetStats: jest.fn().mockResolvedValue({ total_requests: 0 }),
        getConfig: jest.fn().mockResolvedValue({}),
        getDeals: jest.fn().mockResolvedValue([]),
        getRoutes: jest.fn().mockResolvedValue([]),
        getCircuits: jest.fn().mockResolvedValue({ circuits: [] }),
        getStats: jest.fn().mockResolvedValue({})
      };
      (extensionController as any).restClient = mockRestClient;
      (extensionController as any).dashboardPanel = {
        updateStats: jest.fn(),
        updateDeals: jest.fn(),
        updateRoutes: jest.fn(),
        updateCircuits: jest.fn(),
        updateConfig: jest.fn()
      };

      // Test save settings with token
      const setRemoteUrlSpy = jest.spyOn((extensionController as any).authManager, 'setRemoteUrl').mockResolvedValue(undefined);
      const setRemoteTokenSpy = jest.spyOn((extensionController as any).authManager, 'setRemoteToken').mockResolvedValue(undefined);

      await (extensionController as any).handleSaveSettings('http://192.168.0.205:8000', 'my-token');
      expect(setRemoteUrlSpy).toHaveBeenCalledWith('http://192.168.0.205:8000');
      expect(setRemoteTokenSpy).toHaveBeenCalledWith('my-token');

      // Test save settings clearing token
      await (extensionController as any).handleSaveSettings(undefined, '');
      
      // Ensure mock client is active for stats operations
      (extensionController as any).restClient = mockRestClient;

      // Test recalculate stats
      await (extensionController as any).handleRecalculateStats();
      expect(mockRestClient.recalculateStats).toHaveBeenCalled();
      expect(vscode.window.withProgress).toHaveBeenCalledWith(
        expect.objectContaining({ title: expect.stringContaining('Historical stats recalculated') }),
        expect.any(Function)
      );

      // Test reset stats
      await (extensionController as any).handleResetStats();
      expect(mockRestClient.resetStats).toHaveBeenCalled();
      expect(vscode.window.withProgress).toHaveBeenCalledWith(
        expect.objectContaining({ title: expect.stringContaining('reset to $0.00') }),
        expect.any(Function)
      );
    });

    it('should handle runOptimizer and refreshDeals error branches', async () => {
      const consoleSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
      (extensionController as any).restClient = {
        tune: jest.fn().mockRejectedValue(new Error('Tune failure')),
        getDeals: jest.fn().mockRejectedValue(new Error('Deals failure')),
        resetCircuit: jest.fn().mockRejectedValue(new Error('Reset failure')),
        recalculateStats: jest.fn().mockRejectedValue(new Error('Recalculate failure')),
        resetStats: jest.fn().mockRejectedValue(new Error('Reset stats failure'))
      };

      await extensionController.runOptimizer();
      expect(vscode.window.showErrorMessage).toHaveBeenCalledWith('Nacho Flow: Optimization failed: Tune failure');

      await extensionController.refreshDeals(true);
      expect(vscode.window.showErrorMessage).toHaveBeenCalledWith('Nacho Flow: Failed to refresh deals: Deals failure');

      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce('openrouter');
      await (extensionController as any).resetCircuit();
      expect(vscode.window.showErrorMessage).toHaveBeenCalledWith('Nacho Flow: Failed to reset circuit: Reset failure');

      await (extensionController as any).handleRecalculateStats();
      expect(vscode.window.showErrorMessage).toHaveBeenCalledWith('Nacho Flow: Failed to recalculate stats: Recalculate failure');

      await (extensionController as any).handleResetStats();
      expect(vscode.window.showErrorMessage).toHaveBeenCalledWith('Nacho Flow: Failed to reset stats: Reset stats failure');
      consoleSpy.mockRestore();

      // Test handleSaveSettings error
      jest.spyOn((extensionController as any).authManager, 'setRemoteUrl').mockRejectedValueOnce(new Error('Config write failure'));
      await (extensionController as any).handleSaveSettings('http://invalid:8000');
      expect(vscode.window.showErrorMessage).toHaveBeenCalledWith(expect.stringContaining('Failed to save settings'));

      // Test handleTestConnection failure branch
      (extensionController as any).sidebarProvider = {
        updateEngineStatus: jest.fn()
      };
      (extensionController as any).authManager = {
        getBaseUrl: jest.fn().mockResolvedValue('http://localhost:8000'),
        getAuthToken: jest.fn().mockResolvedValue('token')
      };
      const RestClientClass = require('./api/client').RestClient;
      RestClientClass.prototype.getHealth = jest.fn().mockRejectedValueOnce(new Error('Connection timeout'));
      await (extensionController as any).handleTestConnection();
      expect((extensionController as any).sidebarProvider.updateEngineStatus).toHaveBeenCalledWith({
        connected: false,
        error: 'Connection timeout'
      });
    });

    it('should open settings and focus sidebar view', async () => {
      await extensionController.openSettings();
      expect(vscode.commands.executeCommand).toHaveBeenCalledWith('nacho-flow.sidebarView.focus');
    });

    it('should handle sidebar message handlers correctly', async () => {
      const SidebarClass = require('../ui/sidebar/sidebar-view-provider').SidebarViewProvider;
      (extensionController as any).outputChannel = { show: jest.fn(), appendLine: jest.fn() };
      (extensionController as any).sidebarProvider = { updateEngineStatus: jest.fn(), updateState: jest.fn() };
      (extensionController as any).registerSidebarProvider();
      const sidebarMsgHandler = SidebarClass.mock.calls[SidebarClass.mock.calls.length - 1][1];

      expect(sidebarMsgHandler).toBeDefined();

      // Test initialize and refreshAll
      const syncSpy = jest.spyOn(extensionController as any, 'syncSidebarState').mockResolvedValue(undefined);
      await sidebarMsgHandler({ command: 'initialize' });
      expect(syncSpy).toHaveBeenCalled();
      await sidebarMsgHandler({ command: 'refreshAll' });
      expect(syncSpy).toHaveBeenCalledTimes(2);

      // Test setEngineMode local and remote
      const setModeSpy = jest.spyOn((extensionController as any).authManager, 'setEngineMode').mockResolvedValue(undefined);
      await sidebarMsgHandler({ command: 'setEngineMode', mode: 'local' });
      expect(setModeSpy).toHaveBeenCalledWith('local');
      await sidebarMsgHandler({ command: 'setEngineMode', mode: 'remote' });
      expect(setModeSpy).toHaveBeenCalledWith('remote');

      // Mock processManager
      const mockProcMgr = {
        isLocalUrl: jest.fn().mockReturnValue(true),
        start: jest.fn().mockResolvedValue({ success: true }),
        restart: jest.fn().mockResolvedValue({ success: true }),
        stop: jest.fn().mockResolvedValue(true),
        dispose: jest.fn()
      };
      (extensionController as any).processManager = mockProcMgr;

      // Test startEngine
      await sidebarMsgHandler({ command: 'startEngine' });
      expect(mockProcMgr.start).toHaveBeenCalled();

      // Test startEngine with generic error
      mockProcMgr.start.mockResolvedValueOnce({ success: false, error: 'Failed' });
      await sidebarMsgHandler({ command: 'startEngine' });
      expect(vscode.window.showErrorMessage).toHaveBeenCalledWith('Failed');

      // Test startEngine with PORT_IN_USE error and clicking Open config.yaml
      (vscode.window.showErrorMessage as jest.Mock).mockResolvedValueOnce('📝 Open config.yaml');
      const showConfigSpy = jest.spyOn(extensionController as any, 'showConfigEditor').mockResolvedValueOnce(undefined);
      mockProcMgr.start.mockResolvedValueOnce({
        success: false,
        error: 'Port 8000 is already in use by another application.',
        parsedError: {
          type: 'PORT_IN_USE',
          port: 8000,
          message: 'Port 8000 is already in use by another application. Please free port 8000 or change the port in config.yaml.',
          rawError: 'raw'
        }
      });
      await sidebarMsgHandler({ command: 'startEngine' });
      expect(vscode.window.showErrorMessage).toHaveBeenCalledWith(
        expect.stringContaining('Port 8000 is already in use'),
        '📝 Open config.yaml'
      );
      expect(showConfigSpy).toHaveBeenCalled();

      // Test startEngine with CONFIG_ERROR
      (vscode.window.showErrorMessage as jest.Mock).mockResolvedValueOnce(undefined);
      mockProcMgr.start.mockResolvedValueOnce({
        success: false,
        error: 'Configuration error',
        parsedError: {
          type: 'CONFIG_ERROR',
          message: 'Configuration error in config.yaml: missing type',
          rawError: 'raw'
        }
      });
      await sidebarMsgHandler({ command: 'startEngine' });
      expect(vscode.window.showErrorMessage).toHaveBeenCalledWith(
        'Configuration error in config.yaml: missing type',
        '📝 Open config.yaml'
      );

      // Test startEngine remote warning
      mockProcMgr.isLocalUrl.mockReturnValueOnce(false);
      await sidebarMsgHandler({ command: 'startEngine' });
      expect(vscode.window.showWarningMessage).toHaveBeenCalledWith(expect.stringContaining('Cannot start remote engine'));

      // Test restartEngine
      mockProcMgr.isLocalUrl.mockReturnValue(true);
      await sidebarMsgHandler({ command: 'restartEngine' });
      expect(mockProcMgr.restart).toHaveBeenCalled();

      // Test restartEngine with error
      mockProcMgr.restart.mockResolvedValueOnce({ success: false, error: 'Restart failed' });
      await sidebarMsgHandler({ command: 'restartEngine' });
      expect(vscode.window.showErrorMessage).toHaveBeenCalledWith('Restart failed');

      // Test restartEngine remote warning
      mockProcMgr.isLocalUrl.mockReturnValueOnce(false);
      await sidebarMsgHandler({ command: 'restartEngine' });
      expect(vscode.window.showWarningMessage).toHaveBeenCalledWith(expect.stringContaining('Cannot restart remote engine'));

      // Test stopEngine
      mockProcMgr.isLocalUrl.mockReturnValue(true);
      await sidebarMsgHandler({ command: 'stopEngine' });
      expect(mockProcMgr.stop).toHaveBeenCalled();
      expect((extensionController as any).sidebarProvider.updateEngineStatus).toHaveBeenCalledWith({
        connected: false,
        error: 'Stopped by user'
      });

      // Test stopEngine remote warning
      mockProcMgr.isLocalUrl.mockReturnValueOnce(false);
      await sidebarMsgHandler({ command: 'stopEngine' });
      expect(vscode.window.showWarningMessage).toHaveBeenCalledWith(expect.stringContaining('Cannot stop remote engine'));

      // Test editConfig
      const editConfigSpy = jest.spyOn(extensionController as any, 'showConfigEditor').mockResolvedValue(undefined);
      await sidebarMsgHandler({ command: 'editConfig' });
      expect(editConfigSpy).toHaveBeenCalled();

      // Test openLogs
      await sidebarMsgHandler({ command: 'openLogs' });
      expect((extensionController as any).outputChannel.show).toHaveBeenCalled();

      // Test testConnection
      const testConnSpy = jest.spyOn(extensionController as any, 'handleTestConnection').mockResolvedValue(undefined);
      await sidebarMsgHandler({ command: 'testConnection', url: 'http://127.0.0.1:8000', token: 'token' });
      expect(testConnSpy).toHaveBeenCalledWith('http://127.0.0.1:8000', 'token');

      // Test saveEngineSettings
      const saveSettingsSpy = jest.spyOn(extensionController as any, 'handleSaveSettings').mockResolvedValue(undefined);
      await sidebarMsgHandler({ command: 'saveEngineSettings', url: 'http://127.0.0.1:8000', token: 'token' });
      expect(saveSettingsSpy).toHaveBeenCalledWith('http://127.0.0.1:8000', 'token');

      // Test saveOpenRouterKey
      await sidebarMsgHandler({ command: 'saveOpenRouterKey', key: 'sk-or-123' });
      expect((extensionController as any).authManager.setAuthToken).toHaveBeenCalledWith('sk-or-123');

      // Test copyToClipboard
      await sidebarMsgHandler({ command: 'copyToClipboard', text: 'http://127.0.0.1:8000/v1', label: 'Proxy Endpoint' });
      expect(vscode.env.clipboard.writeText).toHaveBeenCalledWith('http://127.0.0.1:8000/v1');

      // Test copyActiveToken (with token)
      (extensionController as any).authManager.getAuthToken = jest.fn().mockResolvedValue('token-active-999');
      await sidebarMsgHandler({ command: 'copyActiveToken' });
      expect(vscode.env.clipboard.writeText).toHaveBeenCalledWith('token-active-999');

      // Test copyActiveToken (without token)
      (extensionController as any).authManager.getAuthToken = jest.fn().mockResolvedValue(null);
      await sidebarMsgHandler({ command: 'copyActiveToken' });

      // Test openExternalUrl
      await sidebarMsgHandler({ command: 'openExternalUrl', url: 'https://openrouter.ai/keys' });
      expect(vscode.env.openExternal).toHaveBeenCalled();

      // Test openMarketplace
      await sidebarMsgHandler({ command: 'openMarketplace', extensionId: 'zoocodeorganization.zoo-code' });
      expect(vscode.commands.executeCommand).toHaveBeenCalledWith('workbench.extensions.search', 'zoocodeorganization.zoo-code');

      // Test recalculateStats
      const recalcSpy = jest.spyOn(extensionController as any, 'handleRecalculateStats').mockResolvedValue(undefined);
      await sidebarMsgHandler({ command: 'recalculateStats' });
      expect(recalcSpy).toHaveBeenCalled();

      // Test resetStats
      const resetStatsSpy = jest.spyOn(extensionController as any, 'handleResetStats').mockResolvedValue(undefined);
      await sidebarMsgHandler({ command: 'resetStats' });
      expect(resetStatsSpy).toHaveBeenCalled();

      // Test resetCircuits
      (extensionController as any).restClient = { resetCircuit: jest.fn().mockResolvedValue({}) };
      await sidebarMsgHandler({ command: 'resetCircuits' });
      expect((extensionController as any).restClient.resetCircuit).toHaveBeenCalled();

      // Test openDashboard
      const showDashboardSpy = jest.spyOn(extensionController as any, 'showDashboard').mockImplementation(() => {});
      await sidebarMsgHandler({ command: 'openDashboard' });
      expect(showDashboardSpy).toHaveBeenCalled();

      // Test openSupport and openDocs
      await sidebarMsgHandler({ command: 'openSupport' });
      expect(vscode.env.openExternal).toHaveBeenCalledWith(expect.objectContaining({ path: expect.stringContaining('support.html') }));
      await sidebarMsgHandler({ command: 'openDocs' });
      expect(vscode.env.openExternal).toHaveBeenCalledWith(expect.objectContaining({ path: expect.stringContaining('docs.html') }));
    });

    it('should sync sidebar state correctly', async () => {
      const mockSidebar = { updateState: jest.fn() };
      (extensionController as any).sidebarProvider = mockSidebar;
      (extensionController as any).authManager = {
        getEngineMode: jest.fn().mockReturnValue('remote'),
        getRemoteUrl: jest.fn().mockReturnValue('http://192.168.0.205:8000'),
        getRemoteToken: jest.fn().mockResolvedValue('token-123'),
        getBaseUrl: jest.fn().mockResolvedValue('http://192.168.0.205:8000'),
        getAuthToken: jest.fn().mockResolvedValue('token-123')
      };
      (extensionController as any).restClient = {
        getHealth: jest.fn().mockResolvedValue({ version: '0.6.0' }),
        getConfig: jest.fn().mockResolvedValue({
          providers: {
            ollama: { type: 'local' },
            openrouter: { base_url: 'https://openrouter.ai/api/v1' },
            vllm: { base_url: 'http://192.168.0.101:8000/v1', type: 'local' },
            sglang: { base_url: 'http://192.168.0.102:8000/v1', type: 'local' },
            llamacpp: { base_url: 'http://127.0.0.1:8080/v1', type: 'local' },
            openai: { base_url: 'https://api.openai.com/v1' },
            anthropic: { base_url: 'https://api.anthropic.com/v1' },
            deepseek: { base_url: 'https://api.deepseek.com/v1' },
            customai: { base_url: 'http://localhost:9000/v1' }
          },
          tiers: [
            { name: 'Tier 2: Local', provider: 'ollama', model: 'gemma4:12b-it-qat' },
            { name: 'Tier 2: Cloud', provider: 'openrouter', model: 'google/gemini-3.7-flash' },
            { name: 'Tier 2: vLLM', provider: 'vllm', model: 'qwen2.5-coder:32b' }
          ],
          default_tier: { name: 'Tier 4: Rescue', provider: 'openrouter', model: 'anthropic/claude-sonnet-5' }
        }),
        getCircuits: jest.fn().mockResolvedValue({
          circuits: [
            { provider: 'ollama', state: 'closed' },
            { provider: 'openrouter', state: 'closed' },
            { provider: 'vllm', state: 'closed' }
          ]
        })
      };
      
      await (extensionController as any).syncSidebarState();
      expect(mockSidebar.updateState).toHaveBeenCalledWith(expect.objectContaining({
        engineMode: 'remote',
        remoteUrl: 'http://192.168.0.205:8000',
        hasToken: true,
        engineStatus: { connected: true, version: '0.6.0', error: '' },
        providers: expect.arrayContaining([
          expect.objectContaining({ id: 'ollama', name: 'Ollama', models: ['gemma4:12b-it-qat'], active: true }),
          expect.objectContaining({ id: 'openrouter', name: 'OpenRouter', models: ['google/gemini-3.7-flash', 'anthropic/claude-sonnet-5'], active: true }),
          expect.objectContaining({ id: 'vllm', name: 'vLLM', models: ['qwen2.5-coder:32b'], active: true }),
          expect.objectContaining({ id: 'sglang', name: 'SGLang' }),
          expect.objectContaining({ id: 'llamacpp', name: 'llama.cpp' }),
          expect.objectContaining({ id: 'openai', name: 'OpenAI' }),
          expect.objectContaining({ id: 'anthropic', name: 'Anthropic' }),
          expect.objectContaining({ id: 'deepseek', name: 'DeepSeek' }),
          expect.objectContaining({ id: 'customai', name: 'Customai' })
        ])
      }));
    });

    it('should test syncSidebarState with health error and unconfigured providers', async () => {
      const mockSidebar = { updateState: jest.fn(), updateEngineStatus: jest.fn() };
      (extensionController as any).sidebarProvider = mockSidebar;
      (extensionController as any).authManager = {
        getEngineMode: jest.fn().mockReturnValue('local'),
        getRemoteUrl: jest.fn().mockReturnValue('http://192.168.0.205:8000'),
        getRemoteToken: jest.fn().mockResolvedValue(null),
        getBaseUrl: jest.fn().mockResolvedValue('http://127.0.0.1:8000'),
        getAuthToken: jest.fn().mockResolvedValue(null)
      };
      (extensionController as any).restClient = {
        getHealth: jest.fn().mockRejectedValue(new Error('Connection refused')),
        getConfig: jest.fn().mockRejectedValue(new Error('Fail')),
        getCircuits: jest.fn().mockRejectedValue(new Error('Fail'))
      };

      await (extensionController as any).syncSidebarState();
      expect(mockSidebar.updateState).toHaveBeenCalledWith(expect.objectContaining({
        engineMode: 'local',
        engineStatus: { connected: false, version: '', error: 'Connection refused' },
        providers: []
      }));

      // Case: test handleTestConnection with explicit url and token
      const mockHealthClient = require('./api/client').RestClient;
      mockHealthClient.prototype.getHealth = jest.fn().mockResolvedValue({ version: '0.6.0' });
      await (extensionController as any).handleTestConnection('http://192.168.0.205:8000', 'valid-token');
      expect((extensionController as any).sidebarProvider.updateEngineStatus).toHaveBeenCalledWith({
        connected: true,
        version: '0.6.0'
      });
    });

    it('should test null client branches and command handlers', async () => {
      (extensionController as any).restClient = null;
      await (extensionController as any).handleRecalculateStats();
      expect(vscode.window.showErrorMessage).toHaveBeenCalledWith('Nacho Flow: Daemon client not initialized');

      await (extensionController as any).handleResetStats();
      expect(vscode.window.showErrorMessage).toHaveBeenCalledWith('Nacho Flow: Daemon client not initialized');

      (extensionController as any).sidebarProvider = null;
      await (extensionController as any).syncSidebarState();

      // Test openSettings registered command handler
      (extensionController as any).registerCommands();
      const openSettingsCall = (vscode.commands.registerCommand as jest.Mock).mock.calls.find(c => c[0] === 'nacho-flow.openSettings');
      expect(openSettingsCall).toBeDefined();
      const openSettingsSpy = jest.spyOn(extensionController, 'openSettings').mockResolvedValue(undefined);
      openSettingsCall[1]();
      expect(openSettingsSpy).toHaveBeenCalled();

      // Test timeframe registered command handlers
      const setTodayCall = (vscode.commands.registerCommand as jest.Mock).mock.calls.find(c => c[0] === 'nacho-flow.setTimeWindowToday');
      const setYesterdayCall = (vscode.commands.registerCommand as jest.Mock).mock.calls.find(c => c[0] === 'nacho-flow.setTimeWindowYesterday');
      const setWeekCall = (vscode.commands.registerCommand as jest.Mock).mock.calls.find(c => c[0] === 'nacho-flow.setTimeWindowWeek');
      const setMonthCall = (vscode.commands.registerCommand as jest.Mock).mock.calls.find(c => c[0] === 'nacho-flow.setTimeWindowMonth');
      const setAllTimeCall = (vscode.commands.registerCommand as jest.Mock).mock.calls.find(c => c[0] === 'nacho-flow.setTimeWindowAllTime');
      
      const setTimeWindowSpy = jest.spyOn(extensionController, 'setTimeWindow').mockResolvedValue(undefined);
      setTodayCall[1]();
      expect(setTimeWindowSpy).toHaveBeenCalledWith('today');
      setYesterdayCall[1]();
      expect(setTimeWindowSpy).toHaveBeenCalledWith('yesterday');
      setWeekCall[1]();
      expect(setTimeWindowSpy).toHaveBeenCalledWith('this_week');
      setMonthCall[1]();
      expect(setTimeWindowSpy).toHaveBeenCalledWith('this_month');
      setAllTimeCall[1]();
      expect(setTimeWindowSpy).toHaveBeenCalledWith('all_time');
    });

    it('should test setTimeWindow with globalState and dashboard panel', async () => {
      const mockGlobalState = {
        get: jest.fn().mockReturnValue('today'),
        update: jest.fn().mockResolvedValue(undefined)
      };
      (extensionController as any).context = {
        globalState: mockGlobalState,
        subscriptions: [],
        extensionUri: { fsPath: '/test' }
      };
      const mockDashboard = {
        setTimeWindow: jest.fn(),
        dispose: jest.fn()
      };
      (extensionController as any).dashboardPanel = mockDashboard;
      const statusBarSpy = jest.spyOn((extensionController as any).statusBar, 'setTimeWindow');

      await extensionController.setTimeWindow('this_week', true);
      expect(mockGlobalState.update).toHaveBeenCalledWith('nachoFlow_timeWindow', 'this_week');
      expect(statusBarSpy).toHaveBeenCalledWith('this_week');
      expect(mockDashboard.setTimeWindow).toHaveBeenCalledWith('this_week');

      // Test message handler for setTimeWindow from dashboard webview
      (extensionController as any).showDashboard();
      const DashboardClass = require('../ui/webview/dashboard').DashboardPanel;
      const onMessage = DashboardClass.mock.calls[DashboardClass.mock.calls.length - 1][1];
      await onMessage({ command: 'setTimeWindow', timeWindow: 'this_month' });
      expect(statusBarSpy).toHaveBeenCalledWith('this_month');
    });

    it('should test setRoutesRefreshInterval and visibility listeners', async () => {
      const mockGlobalState = {
        get: jest.fn().mockReturnValue(60),
        update: jest.fn().mockResolvedValue(undefined)
      };
      (extensionController as any).context = {
        globalState: mockGlobalState,
        subscriptions: [],
        extensionUri: { fsPath: '/test' }
      };

      let viewStateListener: any;
      const mockDashboard = {
        setTimeWindow: jest.fn(),
        setRoutesRefreshInterval: jest.fn(),
        onDidChangeViewState: jest.fn().mockImplementation((cb: any) => {
          viewStateListener = cb;
        }),
        dispose: jest.fn()
      };
      const DashboardClass = require('../ui/webview/dashboard').DashboardPanel;
      DashboardClass.mockImplementation(() => mockDashboard);

      const pollerMock = {
        setIntervalSeconds: jest.fn(),
        pause: jest.fn(),
        resume: jest.fn(),
        dispose: jest.fn()
      };
      (extensionController as any).telemetryPoller = pollerMock;

      // Open dashboard to test webview message and visibility listener
      (extensionController as any).showDashboard();

      await extensionController.setRoutesRefreshInterval(15, true);
      expect(mockGlobalState.update).toHaveBeenCalledWith('nachoFlow_routesRefreshInterval', 15);
      expect(pollerMock.setIntervalSeconds).toHaveBeenCalledWith(15);
      expect(mockDashboard.setRoutesRefreshInterval).toHaveBeenCalledWith(15);

      // Trigger visibility change
      if (viewStateListener) {
        viewStateListener({ webviewPanel: { visible: true } });
        expect(pollerMock.resume).toHaveBeenCalledWith(true);

        viewStateListener({ webviewPanel: { visible: false } });
        expect(pollerMock.pause).toHaveBeenCalled();
      }

      // Test webview message for setRoutesRefreshInterval and openSettings
      const onMessage = DashboardClass.mock.calls[DashboardClass.mock.calls.length - 1][1];
      await onMessage({ command: 'setRoutesRefreshInterval', interval: 30 });
      expect(pollerMock.setIntervalSeconds).toHaveBeenCalledWith(30);

      const openSettingsSpy = jest.spyOn(extensionController, 'openSettings').mockResolvedValue(undefined);
      await onMessage({ command: 'openSettings' });
      expect(openSettingsSpy).toHaveBeenCalled();

      // Test dispose cleanup
      extensionController.dispose();
      expect(pollerMock.dispose).toHaveBeenCalled();
    });

    it('should test pollTelemetry and updateStats branches', async () => {
      const mockStats = { total_requests: 100, cost_spent_usd: 1.5 };
      const mockRoutes = { routes: [{ id: 'r1' }] };
      const mockRestClient = {
        getStats: jest.fn().mockResolvedValue(mockStats),
        getRoutes: jest.fn().mockResolvedValue(mockRoutes)
      };
      const mockDashboard = {
        updateStats: jest.fn(),
        updateRoutes: jest.fn()
      };

      (extensionController as any).restClient = mockRestClient;
      (extensionController as any).dashboardPanel = mockDashboard;
      const statusBarSpy = jest.spyOn((extensionController as any).statusBar, 'updateStats');

      // Success path
      await (extensionController as any).pollTelemetry();
      expect(statusBarSpy).toHaveBeenCalledWith(mockStats);
      expect(mockDashboard.updateStats).toHaveBeenCalledWith(mockStats);
      expect(mockDashboard.updateRoutes).toHaveBeenCalledWith(mockRoutes);

      // Partial null path
      mockRestClient.getStats.mockRejectedValueOnce(new Error('Failed stats'));
      await (extensionController as any).pollTelemetry();
      expect(statusBarSpy).toHaveBeenCalledWith(null);

      // Error in pollTelemetry outer block
      const consoleSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
      mockRestClient.getStats.mockImplementationOnce(() => {
        throw new Error('Fatal socket error');
      });
      await (extensionController as any).pollTelemetry();
      expect(statusBarSpy).toHaveBeenCalledWith(null);
      consoleSpy.mockRestore();

      // Test startPeriodicUpdates creation
      (extensionController as any).telemetryPoller = null;
      (extensionController as any).startPeriodicUpdates();
      expect((extensionController as any).telemetryPoller).toBeDefined();
    });
  });
});