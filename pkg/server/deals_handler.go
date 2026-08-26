package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

// DealsResponse represents the JSON payload returned by GET /api/v1/deals.
type DealsResponse struct {
	BenchmarkModel    string              `json:"benchmark_model"`
	BenchmarkCostPerM float64             `json:"benchmark_cost_per_m"`
	DealsCount        int                 `json:"deals_count"`
	Deals             []contract.DealInfo `json:"deals"`
	LastSynced        string              `json:"last_synced"`
}

// handleAPIDeals handles GET /api/v1/deals.
func (s *Server) handleAPIDeals(w http.ResponseWriter, r *http.Request) {
	if s.setCORS(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg := s.GetConfig()
	benchmarkRate := contract.DefaultBenchmarkPricePerMillion
	if s.oracle != nil {
		if p, found := s.oracle.GetPrice(contract.ProviderOpenRouter, contract.DefaultBenchmarkModel); found && p.PromptCostPerMillion > 0 {
			benchmarkRate = p.PromptCostPerMillion
		}
	}

	var deals []contract.DealInfo
	if s.oracle != nil {
		deals = s.oracle.GetDeals(cfg.Deals, benchmarkRate, contract.DefaultDealsLimit)
	}
	if deals == nil {
		deals = []contract.DealInfo{}
	}

	lastSynced := time.Now().UTC().Format(time.RFC3339)
	if s.oracle != nil {
		if syncTime := s.oracle.LastSynced(); !syncTime.IsZero() {
			lastSynced = syncTime.Format(time.RFC3339)
		}
	}

	resp := DealsResponse{
		BenchmarkModel:    contract.DefaultBenchmarkModel,
		BenchmarkCostPerM: benchmarkRate,
		DealsCount:        len(deals),
		Deals:             deals,
		LastSynced:        lastSynced,
	}

	w.Header().Set(contract.HeaderContentType, contract.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
