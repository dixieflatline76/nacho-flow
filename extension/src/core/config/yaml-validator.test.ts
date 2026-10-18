import { validateNachoConfigYaml } from './yaml-validator';

describe('NachoYamlValidator', () => {
  it('should pass on a valid, complete Nacho Flow YAML configuration', () => {
    const validYaml = `
version: "1.1.0"
port: 8000

providers:
  llamacpp_local:
    base_url: "http://127.0.0.1:8080/v1"
    type: "local"
  openrouter_cloud:
    base_url: "https://openrouter.ai/api/v1"
    type: "cloud"

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
    expect(issue?.message).toContain('unsupported_type');
    expect(issue?.quickFixes?.length).toBeGreaterThan(0);
  });

  it('should flag missing base_url in provider', () => {
    const missingBase = `
providers:
  my_prov:
    type: "local"
`;
    const issues = validateNachoConfigYaml(missingBase);
    expect(issues.some((i) => i.ruleId === 'MISSING_BASE_URL')).toBe(true);
  });

  it('should flag unknown provider references in tiers, default_tier, and fairy_dust', () => {
    const unknownProv = `
providers:
  llamacpp:
    base_url: "http://localhost:8080/v1"
    type: "local"

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
    type: "local"

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
    type: "local"

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
    type: "local"

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
});
