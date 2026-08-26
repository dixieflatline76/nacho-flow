package contract

// Application metadata constants.
const (
	AppName = "nacho-flow"
)

// Version is the application version, populated at build time via -ldflags into main.version.
var Version = "0.0.0"

// Standard HTTP headers used across nacho-flow and legacy spice compatibility.
// #nosec G101 - constants define HTTP header names, not hardcoded credentials
const (
	HeaderNachoRouterTier  = "x-nacho-router-tier"
	HeaderSpiceRouterTier  = "x-spice-router-tier"
	HeaderNachoTargetModel = "x-nacho-target-model"
	HeaderSpiceTargetModel = "x-spice-target-model"
	HeaderAuthorization    = "Authorization"
	HeaderAPIKey           = "api-key"
	HeaderXAPIKey          = "X-API-Key"
	HeaderContentType      = "Content-Type"
	HeaderContentLength    = "Content-Length"
	ContentTypeJSON        = "application/json"
	ContentTypeEventStream = "text/event-stream"
	AuthSchemeBearer       = "Bearer "
	HTTPProtocol           = "http"
)

// Standard API endpoint routes.
const (
	PathHealth          = "/health"
	PathV1Health        = "/v1/health"
	PathStats           = "/v1/stats"
	PathModels          = "/v1/models"
	PathChatCompletions = "/v1/chat/completions"
	PathCompletions     = "/v1/completions"

	// Management REST API endpoint routes (v0.6.0+).
	PathAPIInfo          = "/api/v1/info"
	PathAPIEvents        = "/api/v1/events"
	PathAPIRoutes        = "/api/v1/routes"
	PathAPICircuits      = "/api/v1/circuits"
	PathAPICircuitsReset = "/api/v1/circuits/reset"
	PathAPIPricing       = "/api/v1/pricing"
	PathAPIConfig        = "/api/v1/config"
	PathAPITune             = "/api/v1/tune"
	PathAPIDeals            = "/api/v1/deals"
	PathAPIStatsReset       = "/api/v1/stats/reset"
	PathAPIStatsRecalculate = "/api/v1/stats/recalculate"
)

// Provider names and API endpoint defaults.
const (
	ProviderOpenRouter   = "openrouter"
	ProviderOllama       = "ollama"
	OpenRouterProduction = "https://openrouter.ai"
	OpenRouterModelsPath = "/api/v1/models"
)

// Standard Tier and Role identifiers.
const (
	TierIDVision    = "tier_1_vision"
	TierIDLocalGPU  = "tier_2_local_gpu"
	TierIDWorkhorse = "tier_3_workhorse"
	TierIDFrontier  = "tier_4_frontier"
)

// Model capabilities and parameter tags.
const (
	ModalityImage = "image"
	ModalityText  = "text"
	ParamTools    = "tools"
)

// Model Deals & Pricing constants.
const (
	PricingNamespaceSeparator     = "::"
	TokensPerMillion              = 1_000_000.0
	DefaultDealsAlertThresholdPct = 30.0
	DefaultDealsMinCodingIndex    = 40.0
	DefaultDealsLimit             = 20
	DiscountFullFree              = 100.0
)

// Curated Catalog & OTA Defaults.
const (
	DefaultCatalogFileName   = "models.json"
	DefaultCatalogCacheDir   = ".nacho-flow/cache/curation"
	DefaultCatalogVersion    = "v1.0.0"
	FallbackCatalogVersion   = "v0.0.0"
	DefaultRemoteCatalogURL  = "https://raw.githubusercontent.com/dixieflatline76/nacho-flow/main/data/models.json"
)

// File system and environment variable defaults.
// #nosec G101 - constants define environment variable names, not hardcoded credentials
const (
	DefaultConfigFileName     = "config.yaml"
	DefaultStatsFileName      = "stats.json"
	DefaultTrafficLogFileName = "traffic.jsonl"
	DefaultRouterLogFileName  = "router.log"
	EnvVarPrefix              = "ENV_"
	GlobalAuthTokenEnv        = "NACHO_AUTH_TOKEN"
)

// Runtime fallback defaults.
const (
	DefaultBenchmarkModel           = "anthropic/claude-3.5-sonnet"
	DefaultBenchmarkPricePerMillion = 3.00
	DefaultServerPort               = 8000
	DefaultDaemonHost               = "127.0.0.1"
)
