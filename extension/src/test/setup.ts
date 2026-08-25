// Mock VS Code API for tests
jest.mock('vscode', () => ({
  // Mock only the parts of the VS Code API that we use
  // This is a minimal mock - expand as needed
  window: {
    showInformationMessage: jest.fn(),
    showErrorMessage: jest.fn(),
    createStatusBarItem: jest.fn().mockReturnValue({
      show: jest.fn(),
      hide: jest.fn(),
      dispose: jest.fn()
    }),
    createWebviewPanel: jest.fn().mockReturnValue({
      webview: {
        html: '',
        onDidReceiveMessage: jest.fn(),
        postMessage: jest.fn()
      },
      onDidDispose: jest.fn(),
      reveal: jest.fn()
    })
  },
  commands: {
    registerCommand: jest.fn()
  },
  workspace: {
    getConfiguration: jest.fn().mockReturnValue({
      get: jest.fn()
    }),
    onDidChangeConfiguration: jest.fn()
  },
  SecretStorage: jest.fn(),
  EventEmitter: jest.fn().mockImplementation(() => ({
    fire: jest.fn(),
    event: jest.fn()
  }))
}), { virtual: true });