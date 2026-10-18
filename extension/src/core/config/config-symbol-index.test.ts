import { parseConfigSymbolIndex } from './config-symbol-index';

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
    expect(emptyIndex.providerUsages).toHaveLength(0);
    expect(emptyIndex.rescueTierUsages).toHaveLength(0);

    const wsIndex = parseConfigSymbolIndex('   \n\n  \t  \n');
    expect(wsIndex.providers.size).toBe(0);
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

    const usagesByProvider = index.providerUsages.map((u) => ({
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
    const t1Usage = index.providerUsages.find((u) => u.ownerName === 'Tier 1: Fast Workhorse')!;
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
});
