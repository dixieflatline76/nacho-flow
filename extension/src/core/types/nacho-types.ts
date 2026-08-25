// Route Record
export interface TurnRecord {
	timestamp: string;
	request_id: string;
	tokens: number;
	has_images: boolean;
	has_tools: boolean;
	keywords: string[];
	selected_tier: string;
	target_model: string;
	provider: string;
	is_local: boolean;
	is_fallback: boolean;
	latency_ms: number;
	status_code: number;
	is_retry: boolean;
	cost_saved_usd: number;
}

// Circuit Information
export interface CircuitInfo {
	provider: string;
	name: string;
	state: 'closed' | 'open' | 'half-open';
	failures: number;
	failure_threshold: number;
	cooldown_seconds: number;
	is_available: boolean;
}

// Configuration
export interface ProviderConfig {
	base_url: string;
	api_key?: string;
	type?: 'local' | 'cloud';
	headers?: Record<string, string>;
}

export interface Tier {
	name: string;
	model: string;
	provider: string;
	when: string;
	strip_images?: boolean;
	reasoning_effort?: string;
	max_context?: number;
}

export interface Config {
	port: number;
	auth_token?: string;
	providers: Record<string, ProviderConfig>;
	tiers: Tier[];
	default_tier: Tier;
}

// Stats
export interface TierMetrics {
	tier1_local_free: number;
	tier2_cloud_coder: number;
	tier3_cloud_reasoning: number;
	tier4_cloud_vision: number;
	explicit_override: number;
	fallbacks: number;
}

export interface StatsSnapshot {
	started_at: string;
	total_requests: number;
	tier_breakdown: TierMetrics;
	total_tokens_routed_locally: number;
	estimated_cost_saved_usd: number;
}

// Pricing
export interface ModelPricing {
	prompt_cost_per_million: number;
	completion_cost_per_million: number;
}

// Tuning Result
export interface TuningResult {
	optimal_threshold: number;
	friction_keywords: string[];
	synthesized_rule: string;
	current_cost_usd: number;
	projected_cost_usd: number;
	projected_savings_usd: number;
	retries_eliminated: number;
	total_sample_turns: number;
}