import * as vscode from 'vscode';
import {
  isNachoConfigDocument,
  toVsCodeRange,
  replaceScalarRange,
  getAllRouteTargets,
  resolveRouteTarget,
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

jest.mock('./model-discovery', () => {
  const actual = jest.requireActual('./model-discovery');
  return {
    ...actual,
    discoverModels: jest.fn(),
    clearModelDiscoveryCache: jest.fn()
  };
});

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

  class MockSelection extends MockRange {
    constructor(
      public anchor: any,
      public active: any
    ) {
      super(anchor?.line || 0, anchor?.character || 0, active?.line || 0, active?.character || 0);
    }
  }

  return {
    Range: MockRange,
    Position: MockPosition,
    Selection: MockSelection,
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
    TextEditorRevealType: {
      Default: 0,
      InCenter: 1,
      InCenterIfOutsideViewport: 2,
      AtTop: 3
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
      registerCommand: jest.fn().mockReturnValue({ dispose: jest.fn() }),
      executeCommand: jest.fn().mockResolvedValue(undefined)
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
    type: "openai_compatible"

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

    it('should complete provider types after type: (including inside and after quotes)', async () => {
      const doc = createMockDoc(sampleConfigYaml);
      // Line 4 is `    type: "openai_compatible"`
      const itemsInside = await provider.provideCompletionItems(doc, new vscode.Position(4, 15));
      expect(itemsInside.map((i) => i.label)).toEqual(['openrouter', 'anthropic', 'openai_compatible']);

      // Cursor after closing quote (`    type: "openai_compatible"|`)
      const itemsAfter = await provider.provideCompletionItems(doc, new vscode.Position(4, 29));
      expect(itemsAfter.map((i) => i.label)).toEqual(['openrouter', 'anthropic', 'openai_compatible']);
    });

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
      expect(modelDiscovery.discoverModels).toHaveBeenCalledWith('http://127.0.0.1:8080/v1', { apiKey: undefined, providerType: 'openai_compatible' });
    });

    it('discovers live models when completing model inside a fairy_dust entry', async () => {
      const fdYaml = `providers:
  p1:
    base_url: "http://127.0.0.1:8080/v1"
tiers:
  - name: "Tier 1"
    provider: p1
fairy_dust:
  enabled: true
  entries:
    - name: "Tactical Review"
      provider: p1
      model: `;
      const fdDoc = createMockDoc(fdYaml);
      (modelDiscovery.discoverModels as jest.Mock).mockResolvedValueOnce({
        online: true,
        latencyMs: 12,
        models: ['fairy-model-1', 'fairy-model-2']
      });

      const lines = fdYaml.split('\n');
      const modelLineIdx = lines.findIndex((l) => l.includes('model: '));
      const items = await provider.provideCompletionItems(
        fdDoc,
        new vscode.Position(modelLineIdx, 13)
      );
      expect(items).toBeDefined();
      expect(items.some((c) => (c.label as string).includes('fairy-model-1'))).toBe(true);
    });

    it('should complete reasoning_effort enums', async () => {
      const doc = createMockDoc(sampleConfigYaml);
      const items = await provider.provideCompletionItems(doc, new vscode.Position(10, 22));
      expect(items.map((i) => i.label)).toEqual(['low', 'medium', 'high', 'max']);
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

    it('should provide rich documentation for configuration keys (fairy_dust, agent_shield, cycle_killer, nts, deals)', () => {
      const advancedConfigYaml = `
nts:
  enabled: true
  strip_ansi: true
cycle_killer:
  phrase_length: 6
agent_shield:
  tail_buffer_bytes: 256
deals:
  alert_threshold_pct: 30.0
fairy_dust:
  frequency: 10
`;
      const doc = createMockDoc(advancedConfigYaml);

      const ntsHover = hoverProvider.provideHover(doc, new vscode.Position(1, 2));
      expect(ntsHover).toBeDefined();
      expect((ntsHover?.contents as any).value).toContain('Neural Token Saver');

      const ansiHover = hoverProvider.provideHover(doc, new vscode.Position(3, 4));
      expect(ansiHover).toBeDefined();
      expect((ansiHover?.contents as any).value).toContain('strip_ansi');

      const cycleHover = hoverProvider.provideHover(doc, new vscode.Position(4, 2));
      expect(cycleHover).toBeDefined();
      expect((cycleHover?.contents as any).value).toContain('cycle_killer');

      const shieldHover = hoverProvider.provideHover(doc, new vscode.Position(6, 2));
      expect(shieldHover).toBeDefined();
      expect((shieldHover?.contents as any).value).toContain('agent_shield');

      const dealsHover = hoverProvider.provideHover(doc, new vscode.Position(8, 2));
      expect(dealsHover).toBeDefined();
      expect((dealsHover?.contents as any).value).toContain('deals');

      const fairyHover = hoverProvider.provideHover(doc, new vscode.Position(10, 2));
      expect(fairyHover).toBeDefined();
      expect((fairyHover?.contents as any).value).toContain('fairy_dust');
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

    it('should prepare rename on provider usage, tier name, rescue tier usage, and default tier name', async () => {
      const doc = createMockDoc(sampleConfigYaml);
      // Provider usage
      const pUsagePrep = renameProvider.prepareRename(doc, new vscode.Position(8, 20));
      expect(pUsagePrep.placeholder).toBe('llamacpp_local');

      // Tier name definition
      const tNamePrep = renameProvider.prepareRename(doc, new vscode.Position(15, 15));
      expect(tNamePrep.placeholder).toBe('Tier 2: Reasoning');

      // Rescue tier usage
      const rescuePrep = renameProvider.prepareRename(doc, new vscode.Position(13, 25));
      expect(rescuePrep.placeholder).toBe('Tier 2: Reasoning');

      // Default tier name
      const defTierPrep = renameProvider.prepareRename(doc, new vscode.Position(21, 15));
      expect(defTierPrep.placeholder).toBe('Default Catch-All');

      // Non-Nacho doc guard checks across all 8 providers
      const nonNacho = {
        uri: vscode.Uri.file('c:/other/random.txt'),
        fileName: 'c:/other/random.txt',
        languageId: 'plaintext',
        getText: () => 'hello',
        lineAt: () => ({ text: 'hello' })
      } as any;
      const emptyEdits = renameProvider.provideRenameEdits(nonNacho, new vscode.Position(0, 0), 'new');
      expect((emptyEdits as any).edits.length).toBe(0);

      const compP = new NachoCompletionItemProvider();
      expect(await compP.provideCompletionItems(nonNacho, new vscode.Position(0, 0))).toEqual([]);

      const hoverP = new NachoHoverProvider();
      expect(hoverP.provideHover(nonNacho, new vscode.Position(0, 0))).toBeNull();

      const defP = new NachoDefinitionProvider();
      expect(defP.provideDefinition(nonNacho, new vscode.Position(0, 0))).toBeNull();

      const refP = new NachoReferenceProvider();
      expect(refP.provideReferences(nonNacho, new vscode.Position(0, 0))).toEqual([]);

      const actP = new NachoCodeActionProvider();
      expect(actP.provideCodeActions(nonNacho, new vscode.Range(0, 0, 0, 0), {} as any)).toEqual([]);

      const lensP = new NachoCodeLensProvider();
      expect(lensP.provideCodeLenses(nonNacho)).toEqual([]);

      const linkP = new NachoDocumentLinkProvider();
      expect(linkP.provideDocumentLinks(nonNacho)).toEqual([]);
    });
  });

  describe('NachoCodeActionProvider', () => {
    const actionProvider = new NachoCodeActionProvider();

    it('should provide QuickFix code actions on validation issues', () => {
      const invalidYaml = `
providers:
  llamacpp:
    base_url: "http://localhost:8080/v1"
    type: "openai_compatible"

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

    it('should provide Pick Rescue Tier action when UNKNOWN_RESCUE_TIER diagnostic is present', () => {
      const invalidRescueYaml = `
providers:
  llamacpp:
    base_url: "http://localhost:8080/v1"
    type: "openai_compatible"

tiers:
  - name: "Tier 1"
    provider: llamacpp
    thought_slap:
      rescue_tier: "nonexistent_tier"
`;
      const doc = createMockDoc(invalidRescueYaml);
      const actions = actionProvider.provideCodeActions(
        doc,
        new vscode.Range(10, 19, 10, 37),
        { diagnostics: [] } as any
      );
      expect(actions.some((a) => a.title.includes('Pick from defined tiers'))).toBe(true);
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

    it('provides Ping Endpoint lenses for openrouter and anthropic when base_url is omitted', () => {
      const omittedDoc = createMockDoc(`
providers:
  or:
    type: "openrouter"
  claude:
    type: "anthropic"
`);
      const lenses = lensProvider.provideCodeLenses(omittedDoc);
      const pingLenses = lenses.filter((l) => l.command?.command === 'nacho-flow.pingProviderEndpoint');
      expect(pingLenses).toHaveLength(2);
      expect(pingLenses[0].command?.arguments).toEqual(['https://openrouter.ai/api/v1', 'or']);
      expect(pingLenses[1].command?.arguments).toEqual(['https://api.anthropic.com', 'claude']);
    });

    it('should use nacho-flow.findProviderReferences for both refCount > 0 and refCount === 0', () => {
      const doc = createMockDoc(sampleConfigYaml);
      const lenses = lensProvider.provideCodeLenses(doc);
      const refLenses = lenses.filter(
        (l) => l.command?.title.includes('reference')
      );
      expect(refLenses.length).toBeGreaterThanOrEqual(1);
      for (const lens of refLenses) {
        expect(lens.command?.command).toBe('nacho-flow.findProviderReferences');
        expect(lens.command?.arguments).toEqual(['llamacpp_local']);
      }

      // Test with 0 references provider
      const unusedDoc = createMockDoc(`
providers:
  unused_p:
    base_url: "http://127.0.0.1:8080/v1"
tiers: []
`);
      const unusedLenses = lensProvider.provideCodeLenses(unusedDoc);
      const zeroRefLens = unusedLenses.find((l) => l.command?.title.includes('0 references'));
      expect(zeroRefLens).toBeDefined();
      expect(zeroRefLens?.command?.command).toBe('nacho-flow.findProviderReferences');
      expect(zeroRefLens?.command?.arguments).toEqual(['unused_p']);
    });

    it('should provide Switch Provider and Pick Model lenses for default_tier', () => {
      const doc = createMockDoc(sampleConfigYaml);
      const lenses = lensProvider.provideCodeLenses(doc);
      const defaultTierLenses = lenses.filter(
        (l) => l.command?.arguments?.[0]?.isDefaultTier === true
      );
      expect(defaultTierLenses.length).toBe(2);
      expect(defaultTierLenses.some((l) => l.command?.title === '🔌 Switch Provider')).toBe(true);
      expect(defaultTierLenses.some((l) => l.command?.title === '🧠 Pick Model')).toBe(true);
    });

    it('should provide Switch Provider and Pick Model lenses for fairy_dust entries', () => {
      const fdDoc = createMockDoc(`
providers:
  p1:
    base_url: "http://127.0.0.1:8080/v1"
tiers:
  - name: "Tier 1"
    provider: p1
fairy_dust:
  enabled: true
  entries:
    - name: "Architecture Review"
      provider: p1
      model: "glm-5.3"
`);
      const lenses = lensProvider.provideCodeLenses(fdDoc);
      const fdLenses = lenses.filter(
        (l) => l.command?.arguments?.[0]?.fairyDustEntryName === 'Architecture Review'
      );
      expect(fdLenses.length).toBe(2);
      expect(fdLenses.some((l) => l.command?.title === '🔌 Switch Provider')).toBe(true);
      expect(fdLenses.some((l) => l.command?.title === '🧠 Pick Model')).toBe(true);
    });

    it('provides CodeLenses for default_tier without explicit name and does not pollute rescue_tier completions', async () => {
      const unnamedDefaultYaml = `
providers:
  p1:
    base_url: "http://127.0.0.1:8080/v1"
default_tier:
  provider: p1
  model: "model-1"
tiers:
  - name: "Tier 1"
    provider: p1
    rescue_tier: 
`;
      const doc = createMockDoc(unnamedDefaultYaml);
      const lenses = lensProvider.provideCodeLenses(doc);
      const defaultTierLenses = lenses.filter(
        (l) => l.command?.arguments?.[0]?.isDefaultTier === true
      );
      expect(defaultTierLenses.length).toBe(2);
      expect(defaultTierLenses[0].command?.arguments?.[0]?.tierName).toBe('default_tier');

      const compProvider = new NachoCompletionItemProvider();
      const completions = await compProvider.provideCompletionItems(
        doc,
        new vscode.Position(10, 16)
      );
      const proposedNames = completions.map((c) => c.label);
      expect(proposedNames).not.toContain('"default_tier"');
      expect(proposedNames).not.toContain('default_tier');
      expect(proposedNames).toContain('"Tier 1"');
    });

    it('should provide only Pick Rescue Tier lens for rescue_tier and not Switch Provider or Pick Model', () => {
      const rescueYaml = `
providers:
  p1:
    base_url: "http://127.0.0.1:8080/v1"
tiers:
  - name: "Tier 1: Haiku 5.5 (medium reasoning)"
    provider: p1
    model: "claude-3-5-haiku"
thought_slap:
  max_attempts: 1
  rescue_tier: "Tier 1: Haiku 5.5 (medium reasoning)"
`;
      const doc = createMockDoc(rescueYaml);
      const lenses = lensProvider.provideCodeLenses(doc);

      const rescueLenses = lenses.filter(
        (l) => l.command?.command === 'nacho-flow.pickRescueTier'
      );
      expect(rescueLenses.length).toBe(1);
      expect(rescueLenses[0].command?.title).toBe('🚑 Pick Rescue Tier');

      // Only the tier definition itself should have Switch Provider and Pick Model (not rescue_tier)
      const switchProviderLenses = lenses.filter(
        (l) =>
          l.command?.command === 'nacho-flow.pickProvider' &&
          l.command?.arguments?.[0]?.tierName === 'Tier 1: Haiku 5.5 (medium reasoning)'
      );
      expect(switchProviderLenses.length).toBe(1);

      const pickModelLenses = lenses.filter(
        (l) =>
          l.command?.command === 'nacho-flow.pickModel' &&
          l.command?.arguments?.[0]?.tierName === 'Tier 1: Haiku 5.5 (medium reasoning)'
      );
      expect(pickModelLenses.length).toBe(1);
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
        cb({ replace: jest.fn(), insert: jest.fn() });
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

      // Test pickProvider for default_tier
      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: 'openrouter',
        providerId: 'openrouter'
      });
      await pickProvHandler({ isDefaultTier: true });
      expect(mockEdit).toHaveBeenCalled();

      // Test pickModel for default_tier
      (modelDiscovery.discoverModels as jest.Mock).mockResolvedValueOnce({
        online: true,
        latencyMs: 10,
        models: ['claude-3-7-sonnet']
      });
      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: 'claude-3-7-sonnet',
        modelId: 'claude-3-7-sonnet'
      });
      await pickModelHandler({ isDefaultTier: true });
      expect(mockEdit).toHaveBeenCalled();

      // Test pickProvider and pickModel for fairy_dust entry
      const fdDoc = createMockDoc(`
providers:
  llamacpp_local:
    base_url: "http://127.0.0.1:8080/v1"
  openrouter:
    base_url: "https://openrouter.ai/api/v1"
tiers:
  - name: "Tier 1: Coding"
    provider: llamacpp_local
fairy_dust:
  enabled: true
  entries:
    - name: "Tactical Review"
      provider: llamacpp_local
      model: "qwen-32b"
`);
      (vscode.window as any).activeTextEditor = {
        document: fdDoc,
        selection: new vscode.Range(0, 0, 0, 0),
        edit: mockEdit
      };

      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: 'openrouter',
        providerId: 'openrouter'
      });
      await pickProvHandler({ fairyDustEntryName: 'Tactical Review' });
      expect(mockEdit).toHaveBeenCalled();

      (modelDiscovery.discoverModels as jest.Mock).mockResolvedValueOnce({
        online: true,
        latencyMs: 10,
        models: ['claude-3-7-sonnet']
      });
      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: 'claude-3-7-sonnet',
        modelId: 'claude-3-7-sonnet'
      });
      await pickModelHandler({ fairyDustEntryName: 'Tactical Review' });
      expect(mockEdit).toHaveBeenCalled();

      // Test pickProvider cursor fallback inside a fairy_dust entry
      const fdLines = fdDoc.getText().split('\n');
      const fdEntryLine = fdLines.findIndex((l) => l.includes('Tactical Review'));
      (vscode.window as any).activeTextEditor.selection = new vscode.Range(fdEntryLine, 0, fdEntryLine, 0);
      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: 'openrouter',
        providerId: 'openrouter'
      });
      await pickProvHandler();
      expect(mockEdit).toHaveBeenCalled();

      // Test pickModel cursor fallback inside a fairy_dust entry
      (modelDiscovery.discoverModels as jest.Mock).mockResolvedValueOnce({
        online: true,
        latencyMs: 10,
        models: ['claude-3-7-sonnet']
      });
      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: 'claude-3-7-sonnet',
        modelId: 'claude-3-7-sonnet'
      });
      await pickModelHandler();
      expect(mockEdit).toHaveBeenCalled();

      // Restore activeTextEditor to doc for remaining tests
      (vscode.window as any).activeTextEditor = {
        document: doc,
        selection: new vscode.Range(0, 0, 0, 0),
        edit: mockEdit
      };

      // Test findProviderReferences command with single and multiple references
      const findRefsCall = (vscode.commands.registerCommand as jest.Mock).mock.calls.find(
        (c) => c[0] === 'nacho-flow.findProviderReferences'
      );
      expect(findRefsCall).toBeDefined();
      const findRefsHandler = findRefsCall[1];

      // Single/multiple reference QuickPick jump
      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: 'Tier 1: Coding (Line 8)',
        usage: { valueRange: { startLine: 7, startCharacter: 14, endLine: 7, endCharacter: 28 } }
      });
      await findRefsHandler('llamacpp_local');
      expect(vscode.commands.executeCommand).toHaveBeenCalledWith(
        'editor.action.showReferences',
        expect.anything(),
        expect.anything(),
        expect.anything()
      );

      // Test pingProviderEndpoint command (online and offline)
      const pingCall = (vscode.commands.registerCommand as jest.Mock).mock.calls.find(
        (c) => c[0] === 'nacho-flow.pingProviderEndpoint'
      );
      expect(pingCall).toBeDefined();
      const offlinePingHandler = pingCall[1];

      (modelDiscovery.discoverModels as jest.Mock).mockResolvedValueOnce({
        online: false,
        latencyMs: 1500,
        models: [],
        error: 'Connection refused'
      });
      await offlinePingHandler('http://127.0.0.1:9999/v1', 'llamacpp_local');
      expect(vscode.window.showErrorMessage).toHaveBeenCalled();

      // Test pickProvider cursor-based fallback
      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: 'llamacpp_local',
        providerId: 'llamacpp_local'
      });
      await pickProvHandler();
      expect(mockEdit).toHaveBeenCalled();

      // Test pickModel cursor-based fallback
      (modelDiscovery.discoverModels as jest.Mock).mockResolvedValueOnce({
        online: true,
        latencyMs: 10,
        models: ['qwen-32b']
      });
      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: 'qwen-32b',
        modelId: 'qwen-32b'
      });
      await pickModelHandler();
      expect(mockEdit).toHaveBeenCalled();

      // Test pickRescueTier with empty config
      const activeEd = (vscode.window as any).activeTextEditor;
      activeEd.revealRange = jest.fn();
      const emptyDoc = createMockDoc('port: 8000\n');
      activeEd.document = emptyDoc;
      await pickRescueHandler();
      expect(vscode.window.showInformationMessage).toHaveBeenCalledWith('No tiers defined in this configuration.');

      // Test pickProvider with empty config (no providers)
      await pickProvHandler();
      expect(vscode.window.showInformationMessage).toHaveBeenCalledWith('No providers defined in this configuration.');

      // Test pickModel with empty config (no target)
      await pickModelHandler();
      expect(vscode.window.showWarningMessage).toHaveBeenCalledWith('No tier or fairy dust entry with a configured provider found at cursor.');
      activeEd.document = doc;

      // Test pickProvider fallback when target has no providerValueRange
      const noProvValRangeDoc = createMockDoc(`
providers:
  prov_any:
    base_url: "http://localhost:8080/v1"
tiers:
  - name: "Tier Cursor"
`);
      activeEd.document = noProvValRangeDoc;
      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: 'prov_any',
        providerId: 'prov_any'
      });
      await pickProvHandler({ tierName: 'Tier Cursor' });
      expect(mockEdit).toHaveBeenCalled();
      activeEd.document = doc;

      // Test findProviderReferences when provider has 0 usages
      const unusedProvDoc = createMockDoc(`
providers:
  unused_p:
    base_url: "http://localhost:8080/v1"
tiers: []
`);
      activeEd.document = unusedProvDoc;
      await findRefsHandler('unused_p');
      expect(vscode.window.showInformationMessage).toHaveBeenCalledWith("Provider 'unused_p' is unused (0 references).");
      activeEd.document = doc;

      // Test findProviderReferences with 1 usage (direct jump without quickpick)
      const singleUsageDoc = createMockDoc(`
providers:
  single_p:
    base_url: "http://localhost:8080/v1"
tiers:
  - name: "Tier 1"
    provider: single_p
`);
      activeEd.document = singleUsageDoc;
      await findRefsHandler('single_p');
      expect(activeEd.revealRange).toHaveBeenCalled();
      activeEd.document = doc;

      // Test findProviderReferences with multiple usages (QuickPick selection)
      const multiUsageDoc = createMockDoc(`
providers:
  multi_p:
    base_url: "http://localhost:8080/v1"
tiers:
  - name: "Tier 1"
    provider: multi_p
  - name: "Tier 2"
    provider: multi_p
`);
      activeEd.document = multiUsageDoc;
      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: 'Tier 2 (Line 7)',
        usage: { valueRange: { startLine: 6, startCharacter: 14, endLine: 6, endCharacter: 21 } }
      });
      await findRefsHandler('multi_p');
      expect(activeEd.revealRange).toHaveBeenCalled();

      // Test findProviderReferences when activeTextEditor is undefined but visibleTextEditors has doc
      (vscode.window as any).activeTextEditor = undefined;
      (vscode.window as any).visibleTextEditors = [activeEd];
      await findRefsHandler('llamacpp_local');
      (vscode.window as any).activeTextEditor = activeEd;
      activeEd.document = doc;

      // Test pingProviderEndpoint with no arguments (falls back to active doc)
      (modelDiscovery.discoverModels as jest.Mock).mockResolvedValueOnce({
        online: true,
        latencyMs: 15,
        models: ['m1', 'm2', 'm3', 'm4']
      });
      await offlinePingHandler();
      expect(vscode.window.showInformationMessage).toHaveBeenCalledWith(
        expect.stringContaining('🔌 [llamacpp_local] Online (15ms) — 4 models available: m1, m2, m3...')
      );

      // Test pingProviderEndpoint with no baseUrl available
      activeEd.document = emptyDoc;
      await offlinePingHandler();
      expect(vscode.window.showWarningMessage).toHaveBeenCalledWith('No provider base_url available to ping.');
      activeEd.document = doc;

      // Test pickModel failure cases
      const brokenProvDoc = createMockDoc(`
providers:
  no_url:
    type: "openai_compatible"
tiers:
  - name: "Tier broken"
    provider: no_url
`);
      activeEd.document = brokenProvDoc;
      await pickModelHandler({ tierName: 'Tier broken' });
      expect(vscode.window.showWarningMessage).toHaveBeenCalledWith("Provider 'no_url' has no base_url configured.");

      // Test pickModel discovery failure
      activeEd.document = doc;
      (modelDiscovery.discoverModels as jest.Mock).mockResolvedValueOnce({
        online: false,
        latencyMs: 100,
        models: [],
        error: 'Network Timeout'
      });
      await pickModelHandler({ tierName: 'Tier 1: Coding' });
      expect(vscode.window.showErrorMessage).toHaveBeenCalledWith(
        expect.stringContaining('Failed to discover models')
      );

      // Test live document listeners
      const openDocListeners = (vscode.workspace.onDidOpenTextDocument as jest.Mock).mock.calls;
      const changeDocListeners = (vscode.workspace.onDidChangeTextDocument as jest.Mock).mock.calls;
      const closeDocListeners = (vscode.workspace.onDidCloseTextDocument as jest.Mock).mock.calls;

      const issueDoc = createMockDoc('tiers:\n  - name: T1\n    when: Tokenss\n');
      if (openDocListeners.length > 0) {
        openDocListeners[0][0](issueDoc);
        expect(mockDiagnostics.set).toHaveBeenCalled();
      }
      if (changeDocListeners.length > 0) {
        changeDocListeners[0][0]({ document: issueDoc });
        expect(mockDiagnostics.set).toHaveBeenCalled();
      }
      if (closeDocListeners.length > 0) {
        closeDocListeners[0][0](issueDoc);
        expect(mockDiagnostics.delete).toHaveBeenCalledWith(issueDoc.uri);
      }
    });

    it('handles unresolved provider and rescue_tier hover/definitions', () => {
      const brokenDoc = createMockDoc(`
providers:
  valid_p:
    base_url: "http://127.0.0.1:8080/v1"
tiers:
  - name: "Tier 1"
    provider: ghost_provider
    thought_slap:
      rescue_tier: "Ghost Tier"
default_tier:
  name: "Fallback"
  provider: valid_p
`);
      const hoverProvider = new NachoHoverProvider();
      // Hover on unresolved provider
      const provHover = hoverProvider.provideHover(brokenDoc, new vscode.Position(6, 17));
      expect(provHover).not.toBeNull();
      expect((provHover?.contents as any).value).toContain('Provider `ghost_provider` is not defined');

      // Hover on unresolved rescue_tier
      const rescueHover = hoverProvider.provideHover(brokenDoc, new vscode.Position(8, 22));
      expect(rescueHover).not.toBeNull();
      expect((rescueHover?.contents as any).value).toContain('Tier `Ghost Tier` is not defined');

      // Hover on subsystem documentation keys
      const ntsDoc = createMockDoc('nts:\n  enabled: true\ncycle_killer:\n  enabled: true\n');
      const ntsHover = hoverProvider.provideHover(ntsDoc, new vscode.Position(0, 1));
      expect(ntsHover).not.toBeNull();
      expect((ntsHover?.contents as any).value).toContain('Neural Token Saver');

      const defProvider = new NachoDefinitionProvider();
      // Definition for rescue_tier pointing to default_tier
      const rescueToDefaultDoc = createMockDoc(`
providers:
  p1:
    base_url: "http://127.0.0.1:8080/v1"
tiers:
  - name: "Tier 1"
    provider: p1
    thought_slap:
      rescue_tier: "Fallback"
default_tier:
  name: "Fallback"
  provider: p1
`);
      const defLoc = defProvider.provideDefinition(rescueToDefaultDoc, new vscode.Position(8, 22));
      expect(defLoc).not.toBeNull();

      const refProvider = new NachoReferenceProvider();
      // References for provider usage
      const refsUsage = refProvider.provideReferences(rescueToDefaultDoc, new vscode.Position(6, 15));
      expect(refsUsage.length).toBeGreaterThanOrEqual(2);

      // References for rescue_tier usage
      const refsRescue = refProvider.provideReferences(rescueToDefaultDoc, new vscode.Position(8, 22));
      expect(refsRescue.length).toBeGreaterThanOrEqual(1);
    });

    it('identifies nacho config documents accurately', () => {
      expect(isNachoConfigDocument({ fileName: 'c:/foo/profile1.yaml', languageId: 'yaml' } as any)).toBe(true);
      expect(isNachoConfigDocument({ fileName: 'c:/foo/.nacho/app.yaml', languageId: 'yaml' } as any)).toBe(true);
      expect(isNachoConfigDocument({ fileName: 'c:/foo/other.json', languageId: 'json' } as any)).toBe(false);
    });

    it('replaces scalars quote-safely: bare string for already-quoted and double-quoted for unquoted', async () => {
      // 1. Direct replaceScalarRange tests
      const doubleQuotedDoc = createMockDoc('provider: "openrouter"');
      const doubleQuotedEdit = { replace: jest.fn() };
      replaceScalarRange(
        doubleQuotedDoc,
        doubleQuotedEdit as any,
        { startLine: 0, startCharacter: 11, endLine: 0, endCharacter: 21 },
        'llamacpp'
      );
      expect(doubleQuotedEdit.replace).toHaveBeenCalledWith(expect.any(vscode.Range), 'llamacpp');

      const singleQuotedDoc = createMockDoc("provider: 'openrouter'");
      const singleQuotedEdit = { replace: jest.fn() };
      replaceScalarRange(
        singleQuotedDoc,
        singleQuotedEdit as any,
        { startLine: 0, startCharacter: 11, endLine: 0, endCharacter: 21 },
        'llamacpp'
      );
      expect(singleQuotedEdit.replace).toHaveBeenCalledWith(expect.any(vscode.Range), 'llamacpp');

      const unquotedDoc = createMockDoc('provider: openrouter');
      const unquotedEdit = { replace: jest.fn() };
      replaceScalarRange(
        unquotedDoc,
        unquotedEdit as any,
        { startLine: 0, startCharacter: 10, endLine: 0, endCharacter: 20 },
        'llamacpp'
      );
      expect(unquotedEdit.replace).toHaveBeenCalledWith(expect.any(vscode.Range), '"llamacpp"');

      // 2. Interactive command tests (pickProvider, pickModel, pickRescueTier)
      const context = { subscriptions: [] as any[] } as vscode.ExtensionContext;
      const registeredCommands = new Map<string, Function>();
      (vscode.commands.registerCommand as jest.Mock).mockImplementation((name, fn) => {
        registeredCommands.set(name, fn);
        return { dispose: jest.fn() };
      });
      registerYamlLanguageFeatures(context);

      const pickProv = registeredCommands.get('nacho-flow.pickProvider')!;
      const pickModel = registeredCommands.get('nacho-flow.pickModel')!;
      const pickRescue = registeredCommands.get('nacho-flow.pickRescueTier')!;

      // Case A: Quoted provider in tier
      const quotedTierDoc = createMockDoc(`
providers:
  prov_a:
    base_url: "http://127.0.0.1:8080/v1"
  prov_b:
    base_url: "http://127.0.0.1:8081/v1"
tiers:
  - name: "Tier A"
    provider: "prov_a"
    model: "model_a"
    thought_slap:
      rescue_tier: "Tier A"
`);
      let lastReplacedText = '';
      const mockActiveEd = {
        document: quotedTierDoc,
        selection: new vscode.Range(0, 0, 0, 0),
        edit: jest.fn().mockImplementation((cb: (builder: any) => void) => {
          cb({
            replace: (_range: any, text: string) => {
              lastReplacedText = text;
            }
          });
          return Promise.resolve(true);
        })
      };
      (vscode.window as any).activeTextEditor = mockActiveEd;

      // Pick provider on quoted tier -> replaces with bare 'prov_b'
      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: 'prov_b',
        providerId: 'prov_b'
      });
      await pickProv({ tierName: 'Tier A' });
      expect(lastReplacedText).toBe('prov_b');

      // Pick model on quoted tier -> replaces with bare 'model_b'
      (modelDiscovery.discoverModels as jest.Mock).mockResolvedValueOnce({
        online: true,
        latencyMs: 10,
        models: ['model_b']
      });
      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: 'model_b',
        modelId: 'model_b'
      });
      await pickModel({ tierName: 'Tier A' });
      expect(lastReplacedText).toBe('model_b');

      // Pick rescue tier on cursor line of rescue_tier
      mockActiveEd.selection = new vscode.Range(11, 20, 11, 20) as any;
      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: 'Tier A',
        tierName: 'Tier A'
      });
      await pickRescue();
      expect(lastReplacedText).toBe('Tier A');

      // Case B: Unquoted provider in tier
      const unquotedTierDoc = createMockDoc(`
providers:
  prov_a:
    base_url: "http://127.0.0.1:8080/v1"
  prov_b:
    base_url: "http://127.0.0.1:8081/v1"
tiers:
  - name: "Tier B"
    provider: prov_a
    model: model_a
`);
      mockActiveEd.document = unquotedTierDoc;
      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: 'prov_b',
        providerId: 'prov_b'
      });
      await pickProv({ tierName: 'Tier B' });
      expect(lastReplacedText).toBe('"prov_b"');

      // Pick model on unquoted tier -> wraps in quotes
      (modelDiscovery.discoverModels as jest.Mock).mockResolvedValueOnce({
        online: true,
        latencyMs: 10,
        models: ['model_b']
      });
      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: 'model_b',
        modelId: 'model_b'
      });
      await pickModel({ tierName: 'Tier B' });
      expect(lastReplacedText).toBe('"model_b"');

      // Case C: QuickFix invocation with args.range on double quoted doc
      const docWithProviders = createMockDoc(`
providers:
  prov_a:
    base_url: "http://127.0.0.1:8080/v1"
  prov_b:
    base_url: "http://127.0.0.1:8081/v1"
tiers:
  - name: "Tier A"
    provider: "prov_a"
    rescue_tier: "Tier A"
`);
      mockActiveEd.document = docWithProviders;
      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: 'prov_b',
        providerId: 'prov_b'
      });
      await pickProv({ range: { startLine: 8, startCharacter: 15, endLine: 8, endCharacter: 21 } });
      expect(lastReplacedText).toBe('prov_b');

      // QuickFix invocation on rescue tier with args.range
      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: 'Tier B',
        tierName: 'Tier B'
      });
      await pickRescue({ range: { startLine: 9, startCharacter: 18, endLine: 9, endCharacter: 24 } });
      expect(lastReplacedText).toBe('Tier B');

      // Case D: Provider with omitted base_url (openrouter / anthropic)
      const omittedBaseDoc = createMockDoc(`
providers:
  my_or:
    type: "openrouter"
    api_key: "sk-or-test"
  my_claude:
    type: "anthropic"
    api_key: "sk-ant-test"
tiers:
  - name: "Tier OR"
    provider: my_or
    model: "anthropic/claude-sonnet-5"
  - name: "Tier Anthropic"
    provider: my_claude
    model: "claude-sonnet-5"
`);
      mockActiveEd.document = omittedBaseDoc;

      (modelDiscovery.discoverModels as jest.Mock).mockResolvedValueOnce({
        online: true,
        latencyMs: 15,
        models: ['anthropic/claude-opus-5']
      });
      (vscode.window.showQuickPick as jest.Mock).mockResolvedValueOnce({
        label: 'anthropic/claude-opus-5',
        modelId: 'anthropic/claude-opus-5'
      });
      await pickModel({ tierName: 'Tier OR' });
      expect(modelDiscovery.discoverModels).toHaveBeenCalledWith('https://openrouter.ai/api/v1', {
        apiKey: 'sk-or-test',
        providerType: 'openrouter'
      });
      expect(lastReplacedText).toBe('anthropic/claude-opus-5');
    });

    it('should extract all route targets and resolve route target correctly', () => {
      const { parseConfigSymbolIndex } = require('./config-symbol-index');
      const docYaml = `
providers:
  prov_1:
    base_url: "http://127.0.0.1:8080/v1"
tiers:
  - name: "Tier Alpha"
    provider: "prov_1"
    model: "model_alpha"
default_tier:
  name: "My Fallback"
  provider: "prov_1"
  model: "model_fallback"
fairy_dust:
  enabled: true
  entries:
    - name: "Reviewer One"
      provider: "prov_1"
      model: "model_reviewer"
`;
      const index = parseConfigSymbolIndex(docYaml);
      const targets = getAllRouteTargets(index);
      expect(targets.length).toBe(3);

      const tierTarget = targets.find((t) => t.commandArgs.tierName === 'Tier Alpha');
      expect(tierTarget).toBeDefined();
      expect(tierTarget?.provider).toBe('prov_1');
      expect(tierTarget?.model).toBe('model_alpha');

      const dtTarget = targets.find((t) => t.commandArgs.isDefaultTier);
      expect(dtTarget).toBeDefined();
      expect(dtTarget?.label).toBe('My Fallback');

      const fdTarget = targets.find((t) => t.commandArgs.fairyDustEntryName === 'Reviewer One');
      expect(fdTarget).toBeDefined();
      expect(fdTarget?.label).toBe('Reviewer One');

      // resolveRouteTarget tests
      expect(resolveRouteTarget(index, { isDefaultTier: true })).toEqual(dtTarget);
      expect(resolveRouteTarget(index, { fairyDustEntryName: 'Reviewer One' })).toEqual(fdTarget);
      expect(resolveRouteTarget(index, { tierName: 'Tier Alpha' })).toEqual(tierTarget);

      // Resolve by cursorLine inside Tier Alpha block
      expect(resolveRouteTarget(index, undefined, tierTarget!.blockRange.startLine)).toEqual(tierTarget);

      // CursorLine outside any block
      expect(resolveRouteTarget(index, undefined, 9999)).toBeUndefined();
      expect(resolveRouteTarget(index, undefined, undefined)).toBeUndefined();

      // Test default_tier without explicit name
      const unnamedDtYaml = `
providers:
  prov_1:
    base_url: "http://127.0.0.1:8080/v1"
default_tier:
  provider: "prov_1"
  model: "model_default"
`;
      const unnamedIndex = parseConfigSymbolIndex(unnamedDtYaml);
      const unnamedTargets = getAllRouteTargets(unnamedIndex);
      const unnamedDtTarget = unnamedTargets.find((t) => t.commandArgs.isDefaultTier);
      expect(unnamedDtTarget?.label).toBe('default_tier');
    });
  });
});

