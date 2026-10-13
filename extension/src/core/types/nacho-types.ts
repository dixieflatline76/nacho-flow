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
	cost_spent_usd?: number;
	nts_tokens_saved?: number;
	nts_bytes_saved?: number;
	thought_slap_attempts?: number;
	thought_slap_outcome?: string;
	thought_slap_tier?: string;
	session_thought_slap_count?: number;
	thought_slap_extra_tokens?: number;
	thought_slap_extra_cost?: number;
	session_kickstarted?: boolean;
	fairy_dusted?: boolean;
	fairy_dust_entry?: string;
	cycle_breaker_triggered?: boolean;
	cycle_breaker_reason?: string;
	cycle_max_ngram_freq?: number;
	cycle_content_tokens?: number;
	cycle_prose_tokens?: number;
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

export interface ThoughtSlapMetrics {
	total_slaps: number;
	recovered: number;
	exhausted: number;
	extra_tokens: number;
	extra_cost: number;
}

export interface StatsSnapshot {
	started_at: string;
	total_requests: number;
	tier_breakdown: TierMetrics;
	total_tokens_routed_locally: number;
	estimated_cost_saved_usd: number;
	total_cost_spent_usd?: number;
	cost_reduction_pct?: number;
	cycle_killer?: any;
	fairy_dust?: any;
	thought_slap?: ThoughtSlapMetrics;
	total_nts_tokens_saved?: number;
	total_nts_bytes_saved?: number;
	total_nts_compacted_turns?: number;
	windows?: Record<string, any>;
}

// Pricing
export interface ModelPricing {
	prompt_cost_per_million: number;
	completion_cost_per_million: number;
}

// Tier Tuning Result
export interface TierTuningResult {
	tier_name: string;
	optimal_threshold: number;
	optimal_retries: number;
	friction_keywords: string[];
	restrict_images: boolean;
	restrict_tools: boolean;
	preserved_clauses?: string[];
	original_rule?: string;
	synthesized_rule: string;
}

// Tuning Result
export interface TuningResult {
	tiers: TierTuningResult[];
	current_cost_usd: number;
	projected_cost_usd: number;
	projected_savings_usd: number;
	retries_eliminated: number;
	total_sample_turns: number;
	total_sessions?: number;
	avg_turns_per_session?: number;
	escalation_rate?: number;
}

// Dashboard SSOT Snapshot State
export interface DashboardEngineState {
	mode: 'local' | 'remote';
	isOnline: boolean;
	activeProfile: string;
	profileLabel: string;
	isRemote: boolean;
	version?: string;
	offlineReason?: string;
}

export interface ProfileInfo {
	id: string;
	label: string;
	number: number;
	isWorkspace: boolean;
	uri?: any;
}

export interface DashboardSnapshot {
	timestamp: number;
	engine: DashboardEngineState;
	stats: any | null;
	routes: any | null;
	circuits: any | null;
	deals: any | null;
	config: any | null;
	optimization?: any | null;
	timeWindow?: string;
	routesRefreshInterval?: number;
	localVramGB?: number;
	clients?: { clients: string[]; counts: Record<string, number> } | null;
	selectedClient?: string;
}