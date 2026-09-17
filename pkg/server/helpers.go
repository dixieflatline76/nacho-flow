package server

import (
	"net"
	"net/http"
	"strings"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

func singleJoiningSlash(a, b string) string {
	aslashes := strings.HasSuffix(a, "/")
	bslashes := strings.HasPrefix(b, "/")
	switch {
	case aslashes && bslashes:
		return a + b[1:]
	case !aslashes && !bslashes:
		return a + "/" + b
	}
	return a + b
}

// authenticateClient checks if the incoming request carries a valid Bearer token or API key.
func (s *Server) authenticateClient(r *http.Request) bool {
	expected := s.GetConfig().AuthToken
	if expected == "" {
		return true
	}

	authHeader := r.Header.Get(contract.HeaderAuthorization)
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == expected {
			return true
		}
	}

	// Also check X-API-Key or api-key headers
	if r.Header.Get(contract.HeaderXAPIKey) == expected || r.Header.Get(contract.HeaderAPIKey) == expected {
		return true
	}

	return false
}

// extractSessionKey extracts a consistent session identifier from HTTP headers or client IP.
// If x-session-id or session-id headers are absent, it extracts the host IP from RemoteAddr
// or proxy headers (X-Forwarded-For, X-Real-IP) rather than raw IP:port so that ephemeral
// client TCP ports do not break cross-turn retry counting and kickstart state.
func extractSessionKey(r *http.Request) string {
	if s := r.Header.Get("x-session-id"); s != "" {
		return s
	}
	if s := r.Header.Get("session-id"); s != "" {
		return s
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			return ip
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if ip := strings.TrimSpace(xri); ip != "" {
			return ip
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// mergeUniqueSlices merges base and custom string slices without duplicates, preserving order.
func mergeUniqueSlices(base, custom []string) []string {
	if len(custom) == 0 {
		return base
	}
	if len(base) == 0 {
		return custom
	}
	set := make(map[string]struct{}, len(base)+len(custom))
	result := make([]string, 0, len(base)+len(custom))
	for _, s := range base {
		trimmed := strings.TrimSpace(s)
		if trimmed != "" {
			if _, exists := set[trimmed]; !exists {
				set[trimmed] = struct{}{}
				result = append(result, trimmed)
			}
		}
	}
	for _, s := range custom {
		trimmed := strings.TrimSpace(s)
		if trimmed != "" {
			if _, exists := set[trimmed]; !exists {
				set[trimmed] = struct{}{}
				result = append(result, trimmed)
			}
		}
	}
	return result
}
