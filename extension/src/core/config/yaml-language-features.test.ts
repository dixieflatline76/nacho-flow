import * as vscode from 'vscode';
import {
  isNachoConfigDocument,
  toVsCodeRange,
  NachoCompletionItemProvider,
  NachoHoverProvider,
  NachoDefinitionProvider,
  NachoReferenceProvider,
  NachoRenameProvider,
  NachoCodeActionProvider,
  NachoCodeLensProvider,
  NachoDocumentLinkProvider,
  registerYamlLanguageFeatures,
  REQUEST_CONTEXT_DOCS
} from './yaml-language-features';
import * as modelDiscovery from './model-discovery';

jest.mock('./model-discovery');

jest.mock('vscode', () => {
  class MockRange {
    public start: { line: number; character: number };
    public end: { line: number; character: number };
    constructor(
      public startLine: number,
      public startCharacter: number,
      public endLine: number,
      public endCharacter: number
    ) {
      this.start = { line: startLine, character: startCharacter };
      this.end = { line: endLine, character: endCharacter };
    }
    intersection(other: any) {
      if (this.startLine > other.endLine || this.endLine < other.startLine) return null;
      return this;
    }
    contains(pos: any) {
      return (
        pos.line >= this.startLine &&
        pos.line <= this.endLine &&
        pos.character >= this.startCharacter &&
        pos.character <= this.endCharacter
      );
    }
  }

  class MockPosition {
    constructor(public line: number, public character: number) {}
  }

  class MockLocation {
    constructor(public uri: any, public range: any) {}
  }

  class MockCompletionItem {
    public detail?: string;
    public documentation?: any;
    public insertText?: string;
    constructor(public label: string, public kind?: any) {}
  }

  class MockMarkdownString {
    public value: string = '';
    appendMarkdown(text: string) {
      this.value += text;
      return this;
    }
  }

  class MockHover {
    constructor(public contents: any, public range?: any) {}
  }

  class MockWorkspaceEdit {
    public edits: Array<{ uri: any; range: any; newText: string }> = [];
    replace(uri: any, range: any, newText: string) {
      this.edits.push({ uri, range, newText });
    }
  }

  class MockCodeAction {
    public edit?: MockWorkspaceEdit;
    public isPreferred?: boolean;
    public command?: any;
    constructor(public title: string, public kind?: any) {}
  }

  class MockCodeLens {
    constructor(public range: any, public command?: any) {}
  }

  class MockDocumentLink {
    constructor(public range: any, public target: any) {}
  }

  class MockDiagnostic {
    constructor(public range: any, public message: string, public severity: any) {}
  }

  return {
    Range: MockRange,
    Position: MockPosition,
    Location: MockLocation,
    CompletionItem: MockCompletionItem,
    MarkdownString: MockMarkdownString,
    Hover: MockHover,
    WorkspaceEdit: MockWorkspaceEdit,
    CodeAction: MockCodeAction,
    CodeLens: MockCodeLens,
    DocumentLink: MockDocumentLink,
    Diagnostic: MockDiagnostic,
    CompletionItemKind: {
      Reference: 18,
      Value: 12,
      EnumMember: 20,
      Variable: 6,
      Keyword: 14
    },
    CodeActionKind: {
      QuickFix: 'quickfix'
    },
    DiagnosticSeverity: {
      Error: 0,
      Warning: 1,
      Information: 2
    },
    Uri: {
      file: (f: string) => ({ fsPath: f, toString: () => f }),
      parse: (s: string) => ({ fsPath: s, toString: () => s })
    },
    Disposable: {
      from: (...items: any[]) => ({
        dispose: () => items.forEach((i) => i?.dispose?.())
      })
    },
    languages: {
      registerCompletionItemProvider: jest.fn().mockReturnValue({ dispose: jest.fn() }),
      registerHoverProvider: jest.fn().mockReturnValue({ dispose: jest.fn() }),
      registerDefinitionProvider: jest.fn().mockReturnValue({ dispose: jest.fn() }),
      registerReferenceProvider: jest.fn().mockReturnValue({ dispose: jest.fn() }),
      registerRenameProvider: jest.fn().mockReturnValue({ dispose: jest.fn() }),
      registerCodeActionsProvider: jest.fn().mockReturnValue({ dispose: jest.fn() }),
      registerCodeLensProvider: jest.fn().mockReturnValue({ dispose: jest.fn() }),
      registerDocumentLinkProvider: jest.fn().mockReturnValue({ dispose: jest.fn() })
    },
    commands: {
      registerCommand: jest.fn().mockReturnValue({ dispose: jest.fn() })
    },
    window: {
      activeTextEditor: undefined as any,
      showInformationMessage: jest.fn(),
      showWarningMessage: jest.fn(),
      showErrorMessage: jest.fn(),
      showQuickPick: jest.fn()
    },
    workspace: {
      onDidOpenTextDocument: jest.fn().mockReturnValue({ dispose: jest.fn() }),
      onDidChangeTextDocument: jest.fn().mockReturnValue({ dispose: jest.fn() }),
      onDidCloseTextDocument: jest.fn().mockReturnValue({ dispose: jest.fn() })
    }
  };
});

describe('YamlLanguageFeatures', () => {
  const sampleConfigYaml = `
providers:
  llamacpp_local:
    base_url: "http://127.0.0.1:8080/v1"
    type: "local"

tiers:
  - name: "Tier 1: Coding"
    provider: llamacpp_local
    model: "qwen-32b"
    reasoning_effort: "medium"
    when: "Tokens < 50000 && !HasImages"
    thought_slap:
      rescue_tier: "Tier 2: Reasoning"

  - name: "Tier 2: Reasoning"
    provider: llamacpp_local
    model: "glm-4"
    when: "Tokens >= 50000"

default_tier:
  name: "Default Catch-All"
  provider: llamacpp_local
`;

  function createMockDoc(text: string, fileName: string = '/test/profile1.yaml'): vscode.TextDocument {
    const lines = text.split('\n');
    return {
      uri: vscode.Uri.file(fileName),
      fileName,
      languageId: 'yaml',
      getText: (range?: any) => {
        if (!range) return text;
        const line = lines[range.startLine] || '';
        return line.slice(range.startCharacter, range.endCharacter);
      },
      lineAt: (lineNum: number) => ({
        text: lines[lineNum] || '',
        range: new vscode.Range(lineNum, 0, lineNum, (lines[lineNum] || '').length)
      }),
      getWordRangeAtPosition: (pos: any, regex?: RegExp) => {
        const line = lines[pos.line] || '';
        const wordRegex = regex || /[a-zA-Z0-9_]+/;
        let match: RegExpExecArray | null;
        const re = new RegExp(wordRegex.source, 'g');
        while ((match = re.exec(line)) !== null) {
          if (pos.character >= match.index && pos.character <= match.index + match[0].length) {
            return new vscode.Range(pos.line, match.index, pos.line, match.index + match[0].length);
          }
        }
        return undefined;
      }
    } as any;
  }

  beforeEach(() => {
    jest.clearAllMocks();
  });

  describe('isNachoConfigDocument', () => {
    it('should identify profile and config YAML files as Nacho documents', () => {
      expect(isNachoConfigDocument({ fileName: 'c:\\dev\\profile1.yaml' })).toBe(true);
      expect(isNachoConfigDocument({ fileName: '/home/user/profile_fast.yml' })).toBe(true);
      expect(isNachoConfigDocument({ fileName: 'C:/app/config.yaml' })).toBe(true);
      expect(isNachoConfigDocument({ fileName: '/root/.nacho/default.yaml' })).toBe(true);
      expect(isNachoConfigDocument({ fileName: '/root/profiles/coding.yaml' })).toBe(true);
      expect(isNachoConfigDocument({
        fileName: 'custom.yaml',
        getText: () => 'providers:\n  local:\ntiers:\n  - name: T1'
      })).toBe(true);
    });

    it('should reject non-yaml or unrelated files', () => {
      expect(isNachoConfigDocument({ fileName: 'main.go' })).toBe(false);
      expect(isNachoConfigDocument({ fileName: 'package.json' })).toBe(false);
      expect(isNachoConfigDocument({
        fileName: 'other.yaml',
        getText: () => 'hello: world\nfoo: bar'
      })).toBe(false);
    });
  });

  describe('NachoCompletionItemProvider', () => {
    const provider = new NachoCompletionItemProvider();

    it('should complete provider IDs after provider:', async () => {
      const doc = createMockDoc(sampleConfigYaml);
      const items = await provider.provideCompletionItems(doc, new vscode.Position(8, 14));
      expect(items.length).toBeGreaterThan(0);
      expect(items.some((i) => i.label === 'llamacpp_local')).toBe(true);
    });

    it('should complete rescue tier names after rescue_tier:', async () => {
      const doc = createMockDoc(sampleConfigYaml);
      const items = await provider.provideCompletionItems(doc, new vscode.Position(13, 19));
      expect(items.length).toBeGreaterThan(0);
      expect(items.some((i) => (i.label as string).includes('Tier 2: Reasoning'))).toBe(true);
    });

    it('should complete live discovered models after model: using discoverModels', async () => {
      (modelDiscovery.discoverModels as jest.Mock).mockResolvedValue({
        online: true,
        latencyMs: 12,
        models: ['qwen-32b-q8', 'qwen-coder-14b']
      });

      const doc = createMockDoc(sampleConfigYaml);
      const items = await provider.provideCompletionItems(doc, new vscode.Position(9, 11));
      expect(items.length).toBe(2);
      expect(items.some((i) => (i.label as string).includes('qwen-32b-q8'))).toBe(true);
      expect(modelDiscovery.discoverModels).toHaveBeenCalledWith('http://127.0.0.1:8080/v1');
    });

    it('should complete reasoning_effort enums', async () => {
      const doc = createMockDoc(sampleConfigYaml);
      const items = await provider.provideCompletionItems(doc, new vscode.Position(10, 22));
      expect(items.map((i) => i.label)).toEqual(['low', 'medium', 'high']);
    });

    it('should complete RequestContext variables and DSL keywords in when:', async () => {
      const doc = createMockDoc(sampleConfigYaml);
      const items = await provider.provideCompletionItems(doc, new vscode.Position(11, 15));
      expect(items.some((i) => i.label === 'Tokens')).toBe(true);
      expect(items.some((i) => i.label === 'HasImages')).toBe(true);
      expect(items.some((i) => i.label === 'any')).toBe(true);
    });

    it('should return empty completions on unrelated lines', async () => {
      const doc = createMockDoc(sampleConfigYaml);
      const items = await provider.provideCompletionItems(doc, new vscode.Position(0, 0));
      expect(items).toEqual([]);
    });
  });

  describe('NachoHoverProvider', () => {
    const hoverProvider = new NachoHoverProvider();

    it('should provide rich documentation for RequestContext DSL variables', () => {
      const doc = createMockDoc(sampleConfigYaml);
      const hover = hoverProvider.provideHover(doc, new vscode.Position(11, 13)); // hovering on "Tokens"
      expect(hover).toBeDefined();
      expect((hover?.contents as any).value).toContain('Tokens');
      expect((hover?.contents as any).value).toContain(REQUEST_CONTEXT_DOCS['Tokens']);
    });

    it('should provide hover details for provider definitions', () => {
      const doc = createMockDoc(sampleConfigYaml);
      const hover = hoverProvider.provideHover(doc, new vscode.Position(2, 5)); // "llamacpp_local"
      expect(hover).toBeDefined();
      expect((hover?.contents as any).value).toContain('Provider: `llamacpp_local`');
      expect((hover?.contents as any).value).toContain('http://127.0.0.1:8080/v1');
    });

    it('should provide hover details for provider usages', () => {
      const doc = createMockDoc(sampleConfigYaml);
      const hover = hoverProvider.provideHover(doc, new vscode.Position(8, 20)); // "llamacpp_local" usage
      expect(hover).toBeDefined();
      expect((hover?.contents as any).value).toContain('Provider Reference: `llamacpp_local`');
    });

    it('should provide hover details for rescue tier usages', () => {
      const doc = createMockDoc(sampleConfigYaml);
      const hover = hoverProvider.provideHover(doc, new vscode.Position(13, 25)); // "Tier 2: Reasoning"
      expect(hover).toBeDefined();
      expect((hover?.contents as any).value).toContain('Rescue Tier Reference');
    });

    it('should return null when hovering outside recognized tokens', () => {
      const doc = createMockDoc(sampleConfigYaml);
      const hover = hoverProvider.provideHover(doc, new vscode.Position(0, 0));
      expect(hover).toBeNull();
    });
  });

  describe('NachoDefinitionProvider', () => {
    const defProvider = new NachoDefinitionProvider();

    it('should jump from provider usage to provider definition', () => {
      const doc = createMockDoc(sampleConfigYaml);
      const loc = defProvider.provideDefinition(doc, new vscode.Position(8, 20)) as vscode.Location;
      expect(loc).toBeDefined();
      expect((loc.range as any).startLine).toBe(2); // "llamacpp_local:" definition line
    });

    it('should jump from rescue tier usage to tier definition', () => {
      const doc = createMockDoc(sampleConfigYaml);
      const loc = defProvider.provideDefinition(doc, new vscode.Position(13, 25)) as vscode.Location;
      expect(loc).toBeDefined();
      expect((loc.range as any).startLine).toBe(15); // Tier 2 definition line
    });

    it('should return null when not on a reference', () => {
      const doc = createMockDoc(sampleConfigYaml);
      const loc = defProvider.provideDefinition(doc, new vscode.Position(0, 0));
      expect(loc).toBeNull();
    });
  });

  describe('NachoReferenceProvider', () => {
    const refProvider = new NachoReferenceProvider();

    it('should find all references for a provider ID', () => {
      const doc = createMockDoc(sampleConfigYaml);
      const locations = refProvider.provideReferences(doc, new vscode.Position(2, 5));
      expect(locations.length).toBeGreaterThanOrEqual(3); // def + 2 tiers + default_tier
    });

    it('should find all references for a tier name', () => {
      const doc = createMockDoc(sampleConfigYaml);
      const locations = refProvider.provideReferences(doc, new vscode.Position(15, 15)); // Tier 2
      expect(locations.length).toBe(2); // def + rescue_tier usage
    });

    it('should return empty list when no symbol matches', () => {
      const doc = createMockDoc(sampleConfigYaml);
      const locations = refProvider.provideReferences(doc, new vscode.Position(0, 0));
      expect(locations).toEqual([]);
    });
  });

  describe('NachoRenameProvider', () => {
    const renameProvider = new NachoRenameProvider();

    it('should prepare rename on a valid provider ID', () => {
      const doc = createMockDoc(sampleConfigYaml);
      const prep = renameProvider.prepareRename(doc, new vscode.Position(2, 5));
      expect(prep.placeholder).toBe('llamacpp_local');
    });

    it('should throw an error on an invalid rename location', () => {
      const doc = createMockDoc(sampleConfigYaml);
      expect(() => renameProvider.prepareRename(doc, new vscode.Position(0, 0))).toThrow();
    });

    it('should provide atomic rename edits across the file for provider ID', () => {
      const doc = createMockDoc(sampleConfigYaml);
      const edit = renameProvider.provideRenameEdits(doc, new vscode.Position(2, 5), 'llama_9070xt');
      expect((edit as any).edits.length).toBeGreaterThanOrEqual(4); // definition + 3 usages
      expect((edit as any).edits.every((e: any) => e.newText === 'llama_9070xt')).toBe(true);
    });

    it('should provide atomic rename edits for tier name', () => {
      const doc = createMockDoc(sampleConfigYaml);
      const edit = renameProvider.provideRenameEdits(doc, new vscode.Position(15, 15), 'Tier 2: Advanced Reasoning');
      expect((edit as any).edits.length).toBe(2); // def + rescue_tier usage
      expect((edit as any).edits.every((e: any) => e.newText === 'Tier 2: Advanced Reasoning')).toBe(true);
    });
  });

  describe('NachoCodeActionProvider', () => {
    const actionProvider = new NachoCodeActionProvider();

    it('should provide QuickFix code actions on validation issues', () => {
      const invalidYaml = `
providers:
  llamacpp:
    base_url: "http://localhost:8080/v1"
    type: "local"

tiers:
  - name: "Tier 1"
    provider: unknown_prov
`;
      const doc = createMockDoc(invalidYaml);
      const actions = actionProvider.provideCodeActions(
        doc,
        new vscode.Range(8, 14, 8, 26),
        { diagnostics: [] } as any
      );
      expect(actions.length).toBeGreaterThan(0);
      expect(actions.some((a) => a.title.includes('llamacpp'))).toBe(true);
      expect(actions.some((a) => a.title.includes('Pick from defined providers'))).toBe(true);
    });
  });

  describe('NachoCodeLensProvider', () => {
    const lensProvider = new NachoCodeLensProvider();

    it('should provide lenses for providers and tiers', () => {
      const doc = createMockDoc(sampleConfigYaml);
      const lenses = lensProvider.provideCodeLenses(doc);
      expect(lenses.length).toBeGreaterThanOrEqual(4);
      expect(lenses.some((l) => l.command?.title.includes('reference'))).toBe(true);
      expect(lenses.some((l) => l.command?.title.includes('Ping Endpoint'))).toBe(true);
      expect(lenses.some((l) => l.command?.title.includes('Switch Provider'))).toBe(true);
      expect(lenses.some((l) => l.command?.title.includes('Pick Model'))).toBe(true);
    });
  });

  describe('NachoDocumentLinkProvider', () => {
    const linkProvider = new NachoDocumentLinkProvider();

    it('should produce clickable document links for provider baseUrl', () => {
      const doc = createMockDoc(sampleConfigYaml);
      const links = linkProvider.provideDocumentLinks(doc);
      expect(links.length).toBe(1);
      expect(links[0] && links[0].target?.toString()).toBe('http://127.0.0.1:8080/v1');
    });
  });

  describe('registerYamlLanguageFeatures and Commands', () => {
    it('should register providers, commands, and diagnostics', async () => {
      const context = { subscriptions: [] as any[] } as vscode.ExtensionContext;
      const mockDiagnostics = {
        set: jest.fn(),
        delete: jest.fn()
      } as any;

      const group = registerYamlLanguageFeatures(context, mockDiagnostics);
      expect(group).toBeDefined();
      expect(context.subscriptions.length).toBe(1);

      // Verify commands were registered
      expect(vscode.commands.registerCommand).toHaveBeenCalledWith('nacho-flow.pickProvider', expect.any(Function));
      expect(vscode.commands.registerCommand).toHaveBeenCalledWith('nacho-flow.pickRescueTier', expect.any(Function));
      expect(vscode.commands.registerCommand).toHaveBeenCalledWith('nacho-flow.pickModel', expect.any(Function));
      expect(vscode.commands.registerCommand).toHaveBeenCalledWith('nacho-flow.pingProviderEndpoint', expect.any(Function));

      // Test pingProviderEndpoint command
      const pingCommandCall = (vscode.commands.registerCommand as jest.Mock).mock.calls.find(
        (c) => c[0] === 'nacho-flow.pingProviderEndpoint'
      );
      expect(pingCommandCall).toBeDefined();
      const pingHandler = pingCommandCall[1];

      (modelDiscovery.discoverModels as jest.Mock).mockResolvedValueOnce({
        online: true,
        latencyMs: 15,
        models: ['m1', 'm2']
      });

      await pingHandler('http://127.0.0.1:8080/v1', 'llamacpp_local');
      expect(vscode.window.showInformationMessage).toHaveBeenCalledWith(
        expect.stringContaining('Online (15ms)')
      );

      // Test offline ping
      (modelDiscovery.discoverModels as jest.Mock).mockResolvedValueOnce({
        online: false,
        latencyMs: 1500,
        models: [],
        error: 'ECONNREFUSED'
      });
      await pingHandler('http://127.0.0.1:8080/v1', 'llamacpp_local');
      expect(vscode.window.showErrorMessage).toHaveBeenCalledWith(
        expect.stringContaining('Offline: ECONNREFUSED')
      );

      // Test pickProvider command
      const pickProvCall = (vscode.commands.registerCommand as jest.Mock).mock.calls.find(
        (c) => c[0] === 'nacho-flow.pickProvider'
      );
      const pickProvHandler = pickProvCall[1];

      const mockEdit = jest.fn().mockImplementation((cb: (builder: any) => void) => {
        cb({ replace: jest.fn() });
        return Promise.resolve(true);
      });
      const doc = createMockDoc(sampleConfigYaml);
      (vscode.window as any).activeTextEditor = {
        document: doc,
        selection: new vscode.Range(0, 0, 0, 0),
        edit: mockEdit
      };

      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: 'llamacpp_local',
        providerId: 'llamacpp_local'
      });
      await pickProvHandler({ tierName: 'Tier 1: Coding' });
      expect(mockEdit).toHaveBeenCalled();

      // Test pickRescueTier command
      const pickRescueCall = (vscode.commands.registerCommand as jest.Mock).mock.calls.find(
        (c) => c[0] === 'nacho-flow.pickRescueTier'
      );
      const pickRescueHandler = pickRescueCall[1];

      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: 'Tier 2: Reasoning',
        tierName: 'Tier 2: Reasoning'
      });
      await pickRescueHandler();
      expect(mockEdit).toHaveBeenCalled();

      // Test pickModel command
      const pickModelCall = (vscode.commands.registerCommand as jest.Mock).mock.calls.find(
        (c) => c[0] === 'nacho-flow.pickModel'
      );
      const pickModelHandler = pickModelCall[1];

      (modelDiscovery.discoverModels as jest.Mock).mockResolvedValueOnce({
        online: true,
        latencyMs: 10,
        models: ['qwen-32b', 'glm-4']
      });
      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: 'qwen-32b',
        modelId: 'qwen-32b'
      });
      await pickModelHandler({ tierName: 'Tier 1: Coding' });
      expect(mockEdit).toHaveBeenCalled();

      // Test live document listeners
      const openDocListeners = (vscode.workspace.onDidOpenTextDocument as jest.Mock).mock.calls;
      const changeDocListeners = (vscode.workspace.onDidChangeTextDocument as jest.Mock).mock.calls;
      const closeDocListeners = (vscode.workspace.onDidCloseTextDocument as jest.Mock).mock.calls;

      if (openDocListeners.length > 0) {
        openDocListeners[0][0](doc);
        expect(mockDiagnostics.set).toHaveBeenCalled();
      }
      if (changeDocListeners.length > 0) {
        changeDocListeners[0][0]({ document: doc });
        expect(mockDiagnostics.set).toHaveBeenCalled();
      }
      if (closeDocListeners.length > 0) {
        closeDocListeners[0][0](doc);
        expect(mockDiagnostics.delete).toHaveBeenCalledWith(doc.uri);
      }
    });
  });
});
