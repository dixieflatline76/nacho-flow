package contract

// Application metadata constants.
const (
	AppName = "nacho-flow"
)

// Version is the application version, optionally populated at build time via -ldflags.
var Version = "0.5.0-dev"

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
)

// Standard API endpoint routes.
const (
	PathHealth          = "/health"
	PathV1Health        = "/v1/health"
	PathStats           = "/v1/stats"
	PathModels          = "/v1/models"
	PathChatCompletions = "/v1/chat/completions"
	PathCompletions     = "/v1/completions"
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
)
