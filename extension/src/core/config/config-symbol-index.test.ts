import { parseConfigSymbolIndex, ProviderUsageRef } from './config-symbol-index';

describe('ConfigSymbolIndex', () => {
  const sampleYaml = `# =============================================================================
# NACHO FLOW PROFILE TEST
# =============================================================================
version: "1.1.0"
port: 8000

providers:
  llamacpp_local:
    base_url: "http://127.0.0.1:8080/v1"
    type: "local"

  openrouter_cloud:
    base_url: 'https://openrouter.ai/api/v1' # OpenRouter API
    type: cloud

# Global thought slap config
kickstart:
  thought_slap:
    enabled: true
    rescue_tier: "Tier 2: Reasoning" # Global rescue fallback

tiers:
  # --- Fast Coding Workhorse ---
  - name: "Tier 1: Fast Workhorse"
    provider: llamacpp_local # local GPU
    model: "qwen3.8-27b"
    reasoning_effort: low
    when: "Tokens < 50000 && !HasImages"
    thought_slap:
      enabled: true
      rescue_tier: 'Tier 2: Reasoning'

  # --- Reasoning Tier ---
  - name: 'Tier 2: Reasoning'
    provider: "openrouter_cloud"
    model: 'z-ai/glm-5.3-flash'
    when: 'Tokens >= 50000'

default_tier:
  name: "Default Catch-All"
  provider: openrouter_cloud

fairy_dust:
  enabled: true
  entries:
    - name: "Code Review"
      provider: "openrouter_cloud"
`;

  it('should parse an empty or whitespace document into empty collections', () => {
    const emptyIndex = parseConfigSymbolIndex('');
    expect(emptyIndex.providers.size).toBe(0);
    expect(emptyIndex.tiers.size).toBe(0);
    expect(emptyIndex.defaultTier).toBeUndefined();
    expect(emptyIndex.fairyDustEntries.size).toBe(0);
    expect(emptyIndex.providerUsages).toHaveLength(0);
    expect(emptyIndex.rescueTierUsages).toHaveLength(0);

    const wsIndex = parseConfigSymbolIndex('   \n\n  \t  \n');
    expect(wsIndex.providers.size).toBe(0);
    expect(wsIndex.fairyDustEntries.size).toBe(0);
  });

  it('should extract provider definitions with accurate ranges and metadata', () => {
    const index = parseConfigSymbolIndex(sampleYaml);

    expect(index.providers.has('llamacpp_local')).toBe(true);
    expect(index.providers.has('openrouter_cloud')).toBe(true);

    const localP = index.providers.get('llamacpp_local')!;
    expect(localP.id).toBe('llamacpp_local');
    expect(localP.type).toBe('local');
    expect(localP.baseUrl).toBe('http://127.0.0.1:8080/v1');
    expect(localP.idRange.startLine).toBe(7);
    expect(localP.baseUrlRange?.startLine).toBe(8);

    const cloudP = index.providers.get('openrouter_cloud')!;
    expect(cloudP.id).toBe('openrouter_cloud');
    expect(cloudP.type).toBe('cloud');
    expect(cloudP.baseUrl).toBe('https://openrouter.ai/api/v1');
  });

  it('should extract tiers with accurate names, providers, models, when expressions, and default tier', () => {
    const index = parseConfigSymbolIndex(sampleYaml);

    expect(index.tiers.has('Tier 1: Fast Workhorse')).toBe(true);
    expect(index.tiers.has('Tier 2: Reasoning')).toBe(true);

    const t1 = index.tiers.get('Tier 1: Fast Workhorse')!;
    expect(t1.name).toBe('Tier 1: Fast Workhorse');
    expect(t1.provider).toBe('llamacpp_local');
    expect(t1.model).toBe('qwen3.8-27b');
    expect(t1.reasoningEffort).toBe('low');
    expect(t1.when).toBe('Tokens < 50000 && !HasImages');
    expect(t1.isDefaultTier).toBe(false);

    const t2 = index.tiers.get('Tier 2: Reasoning')!;
    expect(t2.name).toBe('Tier 2: Reasoning');
    expect(t2.provider).toBe('openrouter_cloud');
    expect(t2.model).toBe('z-ai/glm-5.3-flash');
    expect(t2.when).toBe('Tokens >= 50000');
    expect(t2.isDefaultTier).toBe(false);

    expect(index.defaultTier).toBeDefined();
    expect(index.defaultTier?.name).toBe('Default Catch-All');
    expect(index.defaultTier?.provider).toBe('openrouter_cloud');
    expect(index.defaultTier?.isDefaultTier).toBe(true);
  });

  it('should record all provider usages across tiers, default_tier, and fairy_dust', () => {
    const index = parseConfigSymbolIndex(sampleYaml);

    // 1 in Tier 1, 1 in Tier 2, 1 in default_tier, 1 in fairy_dust = 4 total
    expect(index.providerUsages).toHaveLength(4);

    const usagesByProvider = index.providerUsages.map((u: ProviderUsageRef) => ({
      providerId: u.providerId,
      context: u.context,
      owner: u.ownerName
    }));

    expect(usagesByProvider).toContainEqual({
      providerId: 'llamacpp_local',
      context: 'tier',
      owner: 'Tier 1: Fast Workhorse'
    });
    expect(usagesByProvider).toContainEqual({
      providerId: 'openrouter_cloud',
      context: 'tier',
      owner: 'Tier 2: Reasoning'
    });
    expect(usagesByProvider).toContainEqual({
      providerId: 'openrouter_cloud',
      context: 'default_tier',
      owner: 'Default Catch-All'
    });
    expect(usagesByProvider).toContainEqual({
      providerId: 'openrouter_cloud',
      context: 'fairy_dust',
      owner: 'Code Review'
    });
  });

  it('should record all rescue_tier usages across global and tier thought_slap configs', () => {
    const index = parseConfigSymbolIndex(sampleYaml);

    expect(index.rescueTierUsages).toHaveLength(2);

    expect(index.rescueTierUsages).toContainEqual(
      expect.objectContaining({
        tierName: 'Tier 2: Reasoning',
        context: 'global_thought_slap'
      })
    );

    expect(index.rescueTierUsages).toContainEqual(
      expect.objectContaining({
        tierName: 'Tier 2: Reasoning',
        context: 'tier_thought_slap',
        ownerTierName: 'Tier 1: Fast Workhorse'
      })
    );
  });

  it('should ensure value ranges strictly exclude quotes and trailing comments for safe rename', () => {
    const index = parseConfigSymbolIndex(sampleYaml);
    const t1 = index.tiers.get('Tier 1: Fast Workhorse')!;
    
    // In sampleYaml:
    // line 23:   - name: "Tier 1: Fast Workhorse"
    // The exact inner value range should contain "Tier 1: Fast Workhorse" without quotes
    const lines = sampleYaml.split('\n');
    const nameLine = lines[t1.nameValueRange.startLine];
    const slicedName = nameLine.slice(t1.nameValueRange.startCharacter, t1.nameValueRange.endCharacter);
    expect(slicedName).toBe('Tier 1: Fast Workhorse');

    // provider with trailing comment: provider: llamacpp_local # local GPU
    const t1Usage = index.providerUsages.find((u: ProviderUsageRef) => u.ownerName === 'Tier 1: Fast Workhorse')!;
    const provLine = lines[t1Usage.valueRange.startLine];
    const slicedProv = provLine.slice(t1Usage.valueRange.startCharacter, t1Usage.valueRange.endCharacter);
    expect(slicedProv).toBe('llamacpp_local');
  });

  it('should support block ranges that encompass leading comment banners for tiers', () => {
    const index = parseConfigSymbolIndex(sampleYaml);
    const t1 = index.tiers.get('Tier 1: Fast Workhorse')!;
    const lines = sampleYaml.split('\n');

    // The block range startLine should be at or before line 22 ('  # --- Fast Coding Workhorse ---')
    expect(t1.blockRange.startLine).toBeLessThanOrEqual(22);
    expect(lines[t1.blockRange.startLine]).toContain('# --- Fast Coding Workhorse ---');
  });

  it('should support tier items with dash on its own line followed by name on next line', () => {
    const yamlWithSeparateDash = `
tiers:
  -
    name: "Tier Separate"
    provider: "local"
`;
    const index = parseConfigSymbolIndex(yamlWithSeparateDash);
    expect(index.tiers.has('Tier Separate')).toBe(true);
    expect(index.tiers.get('Tier Separate')?.provider).toBe('local');
  });

  it('should return empty rename edits on empty text or unknown symbol', () => {
    const emptyIndex = parseConfigSymbolIndex('');
    expect(emptyIndex.getRenameEdits('anything')).toEqual([]);

    const populatedIndex = parseConfigSymbolIndex(sampleYaml);
    expect(populatedIndex.getRenameEdits('nonexistent_symbol')).toEqual([]);
  });

  it('should parse default_tier without explicit name, extracting model and reasoning_effort', () => {
    const unnamedDefaultYaml = `
providers:
  anthropic:
    base_url: "https://api.anthropic.com"
    api_key: "sk-ant-test"
    type: "anthropic"

default_tier:
  provider: "anthropic"
  model: "claude-3-5-sonnet-20241022"
  reasoning_effort: "high"
  rescue_tier: "rescue_target"
`;
    const index = parseConfigSymbolIndex(unnamedDefaultYaml);
    expect(index.defaultTier).toBeDefined();
    expect(index.defaultTier?.name).toBeUndefined();
    expect(index.defaultTier?.isDefaultTier).toBe(true);
    expect(index.defaultTier?.provider).toBe('anthropic');
    expect(index.defaultTier?.model).toBe('claude-3-5-sonnet-20241022');
    expect(index.defaultTier?.reasoningEffort).toBe('high');
    expect(index.defaultTier?.modelValueRange).toBeDefined();

    const prov = index.providers.get('anthropic');
    expect(prov?.apiKey).toBe('sk-ant-test');

    expect(index.rescueTierUsages).toContainEqual(
      expect.objectContaining({
        tierName: 'rescue_target',
        context: 'tier_thought_slap'
      })
    );

    // Also assert explicit name on default_tier is preserved
    const namedDefaultYaml = `
default_tier:
  name: "My Explicit Fallback"
  provider: "anthropic"
`;
    const namedIndex = parseConfigSymbolIndex(namedDefaultYaml);
    expect(namedIndex.defaultTier?.name).toBe('My Explicit Fallback');
    expect(namedIndex.defaultTier?.isDefaultTier).toBe(true);
  });

  it('should extract fairy_dust entries with names, providers, models, and ranges', () => {
    const fdYaml = `
fairy_dust:
  enabled: true
  entries:
    # Strategic Architecture Review
    - name: "Strategic Architecture Review"
      model: "z-ai/glm-5.3-flash"
      provider: "openrouter_cloud"
      frequency: 60
    # Secondary Review
    - name: "Tactical Code Review"
      model: "claude-sonnet-4"
      provider: "anthropic"
`;
    const index = parseConfigSymbolIndex(fdYaml);
    expect(index.fairyDustEntries.size).toBe(2);

    const strat = index.fairyDustEntries.get('Strategic Architecture Review')!;
    expect(strat).toBeDefined();
    expect(strat.name).toBe('Strategic Architecture Review');
    expect(strat.model).toBe('z-ai/glm-5.3-flash');
    expect(strat.provider).toBe('openrouter_cloud');
    expect(strat.nameValueRange).toBeDefined();
    expect(strat.modelValueRange).toBeDefined();
    expect(strat.providerValueRange).toBeDefined();
    expect(strat.blockRange).toBeDefined();

    const tact = index.fairyDustEntries.get('Tactical Code Review')!;
    expect(tact).toBeDefined();
    expect(tact.name).toBe('Tactical Code Review');
    expect(tact.model).toBe('claude-sonnet-4');
    expect(tact.provider).toBe('anthropic');

    // Test separate line - then name:
    const separateDashYaml = `
fairy_dust:
  entries:
    -
      name: "Separate Line Entry"
      provider: "anthropic"
`;
    const sepIndex = parseConfigSymbolIndex(separateDashYaml);
    expect(sepIndex.fairyDustEntries.has('Separate Line Entry')).toBe(true);

    // Test getRenameEdits for tier name
    const populatedIndex = parseConfigSymbolIndex(sampleYaml);
    const tierEdits = populatedIndex.getRenameEdits('Tier 2: Reasoning');
    expect(tierEdits.length).toBeGreaterThan(0);
  });

  it('should handle multi-line prompt: > with markdown bullets and prose without misparsing entries or overwriting model', () => {
    const multiLinePromptYaml = `
fairy_dust:
  enabled: true
  entries:
    # Strategic Reviewer
    - name: "Strategic Reviewer"
      provider: "anthropic"
      model: "claude-3-7-sonnet"
      prompt: >
        You are a principal architect reviewing the agent plan.
        - Check that no files are deleted without permission
        - Ensure that model: deepseek-coder is not hardcoded
        - Verify provider: llamacpp fallbacks are respected
        End of review guidelines.
    # Second Entry
    - provider: "openrouter"
      model: "mistral-large"
`;
    const index = parseConfigSymbolIndex(multiLinePromptYaml);
    // Should have exactly 2 entries, NOT extra entries from "- Check that..."
    expect(index.fairyDustEntries.size).toBe(2);

    const first = index.fairyDustEntries.get('Strategic Reviewer')!;
    expect(first).toBeDefined();
    expect(first.provider).toBe('anthropic');
    expect(first.model).toBe('claude-3-7-sonnet'); // Not overwritten by 'deepseek-coder' prose!

    // Second unnamed entry should be anchored to its '-' line, not the preceding '# Second Entry' comment line
    const second = index.fairyDustEntries.get('entry_1')!;
    expect(second).toBeDefined();
    expect(second.provider).toBe('openrouter');
    expect(second.model).toBe('mistral-large');
    expect(second.nameValueRange).toBeDefined();

    const lines = multiLinePromptYaml.split('\n');
    const secondDashLineIdx = lines.findIndex((l) => l.includes('- provider: "openrouter"'));
    expect(second.nameValueRange?.startLine).toBe(secondDashLineIdx);
    // blockRange should start at the preceding comment
    expect(second.blockRange.startLine).toBe(secondDashLineIdx - 1);
  });
});

