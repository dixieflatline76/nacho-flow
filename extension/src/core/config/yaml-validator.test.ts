import { validateNachoConfigYaml } from './yaml-validator';

describe('NachoYamlValidator', () => {
  it('should pass on a valid, complete Nacho Flow YAML configuration', () => {
    const validYaml = `
version: "1.1.0"
port: 8000

providers:
  llamacpp_local:
    base_url: "http://127.0.0.1:8080/v1"
    type: "openai_compatible"
    local: true
  openrouter_cloud:
    base_url: "https://openrouter.ai/api/v1"
    type: "openrouter"

kickstart:
  thought_slap:
    enabled: true
    rescue_tier: "Tier 2: Reasoning"

tiers:
  - name: "Tier 1: Fast Workhorse"
    provider: llamacpp_local
    model: "qwen3.8-27b"
    when: "Tokens < 50000 && !HasImages"
    thought_slap:
      rescue_tier: "Tier 2: Reasoning"

  - name: "Tier 2: Reasoning"
    provider: openrouter_cloud
    model: "z-ai/glm-5.3-flash"
    when: "Tokens >= 50000"

default_tier:
  name: "Default Catch-All"
  provider: openrouter_cloud
`;

    const issues = validateNachoConfigYaml(validYaml);
    const errors = issues.filter((i) => i.severity === 'Error');
    expect(errors).toHaveLength(0);
  });

  // Guardrail 1: Minimal partial stubs like "port: 8080\n" must pass without error
  it('should pass on minimal partial stubs without tiers or providers (Guardrail 1)', () => {
    const minimalStub = 'port: 8080\n';
    const issues = validateNachoConfigYaml(minimalStub);
    const errors = issues.filter((i) => i.severity === 'Error');
    expect(errors).toHaveLength(0);
  });

  it('should flag empty providers mapping as MISSING_PROVIDERS', () => {
    const emptyProviders = `
port: 8000
providers:
`;
    const issues = validateNachoConfigYaml(emptyProviders);
    expect(issues.some((i) => i.ruleId === 'MISSING_PROVIDERS')).toBe(true);
  });

  it('should flag missing providers when tiers are defined without providers', () => {
    const tiersWithoutProv = `
tiers:
  - name: "Tier 1"
    provider: "unknown"
`;
    const issues = validateNachoConfigYaml(tiersWithoutProv);
    expect(issues.some((i) => i.ruleId === 'MISSING_PROVIDERS')).toBe(true);
  });

  it('should flag invalid provider type', () => {
    const invalidType = `
providers:
  my_prov:
    base_url: "http://localhost:8000"
    type: "unsupported_type"
`;
    const issues = validateNachoConfigYaml(invalidType);
    const issue = issues.find((i) => i.ruleId === 'INVALID_PROVIDER_TYPE');
    expect(issue).toBeDefined();
    expect(issue?.message).toBe('provider "my_prov": type is required and must be one of openai_compatible, anthropic, openrouter');
    expect(issue?.quickFixes?.length).toBeGreaterThan(0);
  });

  it('should flag missing base_url in provider', () => {
    const missingBase = `
providers:
  my_prov:
    type: "openai_compatible"
`;
    const issues = validateNachoConfigYaml(missingBase);
    expect(issues.some((i) => i.ruleId === 'MISSING_BASE_URL')).toBe(true);
    expect(issues.find((i) => i.ruleId === 'MISSING_BASE_URL')?.message).toBe('provider "my_prov": base_url is required for type openai_compatible');
  });

  it('should flag unknown provider references in tiers, default_tier, and fairy_dust', () => {
    const unknownProv = `
providers:
  llamacpp:
    base_url: "http://localhost:8080/v1"
    type: "openai_compatible"
    local: true

tiers:
  - name: "Tier 1"
    provider: "llama_cluster" # Typo for llamacpp

default_tier:
  name: "Catch-All"
  provider: "openrouter_ghost"

fairy_dust:
  entries:
    - name: "Reviewer"
      provider: "anthropic_ghost"
`;
    const issues = validateNachoConfigYaml(unknownProv);
    const provErrors = issues.filter((i) => i.ruleId === 'UNKNOWN_PROVIDER_REF');
    expect(provErrors).toHaveLength(3);

    const t1Error = provErrors.find((e) => e.message.includes('llama_cluster'));
    expect(t1Error).toBeDefined();
    // Quick fix suggests closest match: llamacpp
    expect(t1Error?.quickFixes?.some((qf) => qf.replacement.includes('llamacpp'))).toBe(true);
  });

  it('should flag unknown rescue_tier references in global and tier thought_slap', () => {
    const unknownRescue = `
providers:
  llamacpp:
    base_url: "http://localhost:8080/v1"
    type: "openai_compatible"
    local: true

kickstart:
  thought_slap:
    rescue_tier: "NonExistentTier"

tiers:
  - name: "Tier 1"
    provider: llamacpp
    thought_slap:
      rescue_tier: "AnotherGhostTier"
`;
    const issues = validateNachoConfigYaml(unknownRescue);
    const rescueErrors = issues.filter((i) => i.ruleId === 'UNKNOWN_RESCUE_TIER');
    expect(rescueErrors).toHaveLength(2);
  });

  it('should flag removed v1.5 legacy keys with migration guidance', () => {
    const legacyYaml = `
providers:
  llamacpp:
    base_url: "http://localhost:8080/v1"
    type: "openai_compatible"
    local: true

kickstart:
  enabled: true
  threshold: 5

tiers:
  - name: "Tier 1"
    provider: llamacpp
    when: "SessionKickstarted == true || SessionKickstartCount > 2"
`;
    const issues = validateNachoConfigYaml(legacyYaml);
    const legacyErrors = issues.filter((i) => i.ruleId === 'REMOVED_LEGACY_KEY');
    expect(legacyErrors.length).toBeGreaterThanOrEqual(2);
    expect(legacyErrors.some((e) => e.message.includes('SessionReadSlapped'))).toBe(true);
    expect(legacyErrors.some((e) => e.message.includes('kickstart.read_slap'))).toBe(true);
  });

  it('should flag invalid when expression with unknown variables or syntax errors', () => {
    const invalidWhen = `
providers:
  llamacpp:
    base_url: "http://localhost:8080/v1"
    type: "openai_compatible"
    local: true

tiers:
  - name: "Tier 1"
    provider: llamacpp
    when: "Tokens < 50000 && UnknownVariable == 123"

  - name: "Tier 2"
    provider: llamacpp
    when: "Tokens < " # Unfinished trailing operator
`;
    const issues = validateNachoConfigYaml(invalidWhen);
    const exprErrors = issues.filter((i) => i.ruleId === 'INVALID_WHEN_EXPR');
    expect(exprErrors.length).toBeGreaterThanOrEqual(2);
    expect(exprErrors.some((e) => e.message.includes('UnknownVariable'))).toBe(true);
  });

  it('should flag tab indentation in YAML as SYNTAX_INDENT', () => {
    const tabYaml = "providers:\n\tlocal:\n\t\tbase_url: http://localhost:8080\n\t\ttype: local\n";
    const issues = validateNachoConfigYaml(tabYaml);
    expect(issues.some((i) => i.ruleId === 'SYNTAX_INDENT')).toBe(true);
  });

  it('should warn on unreachable tiers after catch-all when: true', () => {
    const unreachableYaml = `
providers:
  llamacpp:
    base_url: "http://localhost:8080/v1"
    type: "local"

tiers:
  - name: "Tier 1: Catch-All"
    provider: llamacpp
    when: "true"

  - name: "Tier 2: Shadowed"
    provider: llamacpp
    when: "Tokens > 50000"
`;
    const issues = validateNachoConfigYaml(unreachableYaml);
    const unreachableWarnings = issues.filter((i) => i.ruleId === 'UNREACHABLE_TIER');
    expect(unreachableWarnings).toHaveLength(1);
    expect(unreachableWarnings[0].severity).toBe('Warning');
    expect(unreachableWarnings[0].message).toContain('Tier 2: Shadowed');
  });

  it('should handle empty or whitespace text gracefully', () => {
    expect(validateNachoConfigYaml('')).toHaveLength(0);
    expect(validateNachoConfigYaml('   \n\t\n  ')).toHaveLength(0);
  });

  it('should handle premature closing parentheses in when expression', () => {
    const prematureParenYaml = `
providers:
  llamacpp:
    base_url: "http://localhost:8080/v1"
    type: "local"

tiers:
  - name: "Tier Bad Parens"
    provider: llamacpp
    when: ")Tokens < 50000("
`;
    const issues = validateNachoConfigYaml(prematureParenYaml);
    expect(issues.some((i) => i.ruleId === 'INVALID_WHEN_EXPR' && i.message.includes('unbalanced parentheses'))).toBe(true);
  });

  // Phase 1B Guardrail 1: Auto-Tuner v3 when expressions and string literal masking
  it('should validate Auto-Tuner v3 when expressions and mask string literals cleanly', () => {
    const autoTunerYaml = `
providers:
  llamacpp:
    base_url: "http://localhost:8080/v1"
    type: "openai_compatible"
    local: true

tiers:
  - name: "Tier 1: Complex Expression"
    provider: llamacpp
    when: "!any(Keywords, { # in ['debug', 'refactor'] }) && ClientID == 'cline'"

  - name: "Tier 2: Thought Slap & Cooling Down"
    provider: llamacpp
    when: "SessionThoughtSlapCount > 0 && !IsModelCoolingDown(\\"qwen-32b\\")"

  - name: "Tier 3: Tools & Context"
    provider: llamacpp
    when: "InteractiveTool in ['write_to_file', 'replace_file_content'] && CycleBreakerReason != ''"
`;
    const issues = validateNachoConfigYaml(autoTunerYaml);
    const errors = issues.filter((i) => i.severity === 'Error');
    expect(errors).toHaveLength(0);
  });

  it('should preserve exact character offsets for quickFixes when string literals exist in when expression', () => {
    const yamlWithUnknownAndStrings = `
providers:
  llamacpp:
    base_url: "http://localhost:8080/v1"
    type: "openai_compatible"
    local: true

tiers:
  - name: "Tier Test"
    provider: llamacpp
    when: "ClientID == 'cline' && Tokns > 100"
`;
    const issues = validateNachoConfigYaml(yamlWithUnknownAndStrings);
    const exprError = issues.find((i) => i.ruleId === 'INVALID_WHEN_EXPR' && i.message.includes('Tokns'));
    expect(exprError).toBeDefined();
    expect(exprError?.quickFixes).toBeDefined();
    expect(exprError?.quickFixes?.[0].replacement).toBe('Tokens');

    const lines = yamlWithUnknownAndStrings.split('\n');
    const tierLineIndex = lines.findIndex((l) => l.includes('Tokns'));
    const expectedCol = lines[tierLineIndex].indexOf('Tokns');
    expect(exprError?.quickFixes?.[0].range.startLine).toBe(tierLineIndex);
    expect(exprError?.quickFixes?.[0].range.startCharacter).toBe(expectedCol);
    expect(exprError?.quickFixes?.[0].range.endCharacter).toBe(expectedCol + 'Tokns'.length);
  });

  it('should validate CRLF (\r\n) YAML documents with inline comments on providers: and clean tab quickFixes', () => {
    const crlfYaml = [
      'providers: # top-level providers comment',
      '\tllamacpp:',
      '    base_url: "http://localhost:8080/v1"',
      '    type: "openai_compatible"',
      '    local: true',
      'tiers:',
      '  - name: "Tier 1"',
      '    provider: llamacpp',
      '    when: "Tokens > 100"'
    ].join('\r\n');

    const issues = validateNachoConfigYaml(crlfYaml);
    // Should NOT report EMPTY_PROVIDERS despite inline comment + CRLF
    expect(issues.find((i) => i.ruleId === 'EMPTY_PROVIDERS')).toBeUndefined();

    // Tab quickFix replacement should not include a trailing \r
    const tabIssue = issues.find((i) => i.ruleId === 'SYNTAX_INDENT');
    expect(tabIssue).toBeDefined();
    expect(tabIssue?.quickFixes?.[0].replacement.endsWith('\r')).toBe(false);
    expect(tabIssue?.quickFixes?.[0].range.endCharacter).toBe('  llamacpp:'.length - 1);
  });

  describe('Provider Taxonomy & Validation Table (§1.3)', () => {
    it('flags type: local with exact removed message', () => {
      const yaml = `
providers:
  ollama:
    base_url: "http://127.0.0.1:11434"
    type: "local"
`;
      const issues = validateNachoConfigYaml(yaml);
      const issue = issues.find((i) => i.ruleId === 'REMOVED_PROVIDER_TYPE');
      expect(issue).toBeDefined();
      expect(issue?.message).toBe('provider "ollama": type "local" was removed; use type: openai_compatible with local: true');
    });

    it('flags type: cloud with exact removed message', () => {
      const yaml = `
providers:
  openrouter:
    base_url: "https://openrouter.ai/api/v1"
    type: "cloud"
`;
      const issues = validateNachoConfigYaml(yaml);
      const issue = issues.find((i) => i.ruleId === 'REMOVED_PROVIDER_TYPE');
      expect(issue).toBeDefined();
      expect(issue?.message).toBe('provider "openrouter": type "cloud" was removed; use type: openai_compatible, or type: openrouter for OpenRouter');
    });

    it('flags missing type with exact message', () => {
      const yaml = `
providers:
  ollama:
    base_url: "http://127.0.0.1:11434"
`;
      const issues = validateNachoConfigYaml(yaml);
      const issue = issues.find((i) => i.ruleId === 'INVALID_PROVIDER_TYPE');
      expect(issue).toBeDefined();
      expect(issue?.message).toBe('provider "ollama": type is required and must be one of openai_compatible, anthropic, openrouter');
    });

    it('flags openai_compatible without base_url with exact message', () => {
      const yaml = `
providers:
  ollama:
    type: "openai_compatible"
    local: true
`;
      const issues = validateNachoConfigYaml(yaml);
      const issue = issues.find((i) => i.ruleId === 'MISSING_BASE_URL');
      expect(issue).toBeDefined();
      expect(issue?.message).toBe('provider "ollama": base_url is required for type openai_compatible');
    });

    it('flags local: true on anthropic with exact message', () => {
      const yaml = `
providers:
  claude:
    type: "anthropic"
    local: true
`;
      const issues = validateNachoConfigYaml(yaml);
      const issue = issues.find((i) => i.ruleId === 'INVALID_PROVIDER_LOCAL');
      expect(issue).toBeDefined();
      expect(issue?.message).toBe('provider "claude": local is only valid for type openai_compatible');
    });

    it('flags local: true on openrouter with exact message', () => {
      const yaml = `
providers:
  or:
    type: "openrouter"
    local: true
`;
      const issues = validateNachoConfigYaml(yaml);
      const issue = issues.find((i) => i.ruleId === 'INVALID_PROVIDER_LOCAL');
      expect(issue).toBeDefined();
      expect(issue?.message).toBe('provider "or": local is only valid for type openai_compatible');
    });

    it('flags base_url when not absolute http(s) URL with exact message', () => {
      const yaml = `
providers:
  my_prov:
    type: "openai_compatible"
    base_url: "not-a-valid-url"
`;
      const issues = validateNachoConfigYaml(yaml);
      const issue = issues.find((i) => i.ruleId === 'INVALID_BASE_URL');
      expect(issue).toBeDefined();
      expect(issue?.message).toBe('provider "my_prov": base_url must be an absolute http(s) URL');
    });

    it('allows openrouter and anthropic without base_url', () => {
      const yaml = `
providers:
  or:
    type: "openrouter"
    api_key: "sk-or-test"
  claude:
    type: "anthropic"
    api_key: "sk-ant-test"
`;
      const issues = validateNachoConfigYaml(yaml);
      const errors = issues.filter((i) => i.severity === 'Error');
      expect(errors).toHaveLength(0);
    });
  });
});

