package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry/curation"
)

// GenerateCatalog pulls OpenRouter models, blends verified Artificial Analysis ratings, and generates catalog JSON.
func GenerateCatalog(ctx context.Context, apiURL, version string) (*curation.CuratedCatalog, error) {
	if apiURL == "" {
		apiURL = fmt.Sprintf("%s%s", contract.OpenRouterProduction, contract.OpenRouterModelsPath)
	}
	if version == "" {
		version = contract.DefaultCatalogVersion
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api returned HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Data []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			ContextLength int    `json:"context_length"`
			Pricing       struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
			Architecture struct {
				InputModalities []string `json:"input_modalities"`
			} `json:"architecture"`
			SupportedParameters []string `json:"supported_parameters"`
			Benchmarks          *struct {
				ArtificialAnalysis *struct {
					CodingIndex  float64 `json:"coding_index"`
					AgenticIndex float64 `json:"agentic_index"`
				} `json:"artificial_analysis"`
			} `json:"benchmarks"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	catalog := &curation.CuratedCatalog{
		Version:     version,
		UpdatedAt:   time.Now().UTC(),
		Description: "Canonical benchmark and capability intelligence catalog for Nacho Flow",
		Models:      make(map[string]curation.ModelCuratedProfile),
	}

	for _, m := range payload.Data {
		hasVision := false
		for _, mod := range m.Architecture.InputModalities {
			if mod == contract.ModalityImage {
				hasVision = true
				break
			}
		}

		hasTools := false
		for _, p := range m.SupportedParameters {
			if p == contract.ParamTools {
				hasTools = true
				break
			}
		}

		var codingIdx, agenticIdx float64
		var benchSource, provURL string
		if m.Benchmarks != nil && m.Benchmarks.ArtificialAnalysis != nil {
			codingIdx = m.Benchmarks.ArtificialAnalysis.CodingIndex
			agenticIdx = m.Benchmarks.ArtificialAnalysis.AgenticIndex
			if codingIdx > 0 {
				benchSource = "artificial_analysis"
				provURL = "https://artificialanalysis.ai"
			}
		}

		toolReliability := 85.0
		if agenticIdx > 0 {
			toolReliability = agenticIdx
		}

		var promptCost, compCost float64
		if pVal, parseErr := strconv.ParseFloat(m.Pricing.Prompt, 64); parseErr == nil && pVal > 0 {
			promptCost = pVal * 1_000_000
		}
		if cVal, parseErr := strconv.ParseFloat(m.Pricing.Completion, 64); parseErr == nil && cVal > 0 {
			compCost = cVal * 1_000_000
		}

		role := curation.RoleGeneral
		var recTiers []string
		name := strings.ToLower(m.ID)
		if strings.Contains(name, "r1") || strings.Contains(name, "reason") || strings.Contains(name, "o1") || strings.Contains(name, "o3") || strings.Contains(name, "thinking") || strings.Contains(name, "opus") {
			role = curation.RoleDeepReasoner
			recTiers = []string{contract.TierIDFrontier}
		} else if hasTools && (codingIdx >= 65.0 || strings.Contains(name, "coder") || strings.Contains(name, "sonnet") || strings.Contains(name, "claude-3.5") || strings.Contains(name, "claude-3.7") || strings.Contains(name, "qwen3")) {
			role = curation.RoleCodingWorkhorse
			if promptCost >= 1.5 || codingIdx >= 80.0 || strings.Contains(name, "sonnet") {
				recTiers = []string{contract.TierIDFrontier, contract.TierIDWorkhorse}
			} else {
				recTiers = []string{contract.TierIDWorkhorse}
			}
		} else if hasVision && (strings.Contains(name, "flash") || strings.Contains(name, "lite") || strings.Contains(name, "vision") || strings.Contains(name, "gemma")) {
			role = curation.RoleVisionWorkhorse
			recTiers = []string{contract.TierIDVision}
		} else if codingIdx >= 80.0 {
			recTiers = []string{contract.TierIDFrontier}
		}

		isOpenWeights := false
		for _, fam := range []string{"qwen", "llama", "deepseek", "mistral", "phi", "gemma", "codellama", "starcoder", "command-r", "smollm", "granite", "falcon", "internlm"} {
			if strings.Contains(name, fam) {
				isOpenWeights = true
				break
			}
		}

		profile := curation.ModelCuratedProfile{
			Name:                     m.Name,
			TierRole:                 role,
			CodingIndex:              codingIdx,
			ToolReliability:          toolReliability,
			PromptCostPerMillion:     promptCost,
			CompletionCostPerMillion: compCost,
			RecommendedTiers:         recTiers,
			SupportsVision:           hasVision,
			SupportsTools:            hasTools,
			IsOpenWeights:            isOpenWeights,
			BenchmarkSource:          benchSource,
			ProvenanceURL:            provURL,
		}

		if profile.TierRole != curation.RoleGeneral || profile.CodingIndex > 0 {
			catalog.Models[m.ID] = profile
		}
	}

	return catalog, nil
}

func run(args []string, apiURL string) error {
	fs := flag.NewFlagSet("gen_catalog", flag.ContinueOnError)
	versionFlag := fs.String("version", contract.DefaultCatalogVersion, "Semver version for catalog")
	outputFlag := fs.String("out", "data/models.json", "Output file path")
	embedFlag := fs.String("embed-out", "pkg/telemetry/curation/models.json", "Embedded copy path")

	if err := fs.Parse(args); err != nil {
		return err
	}

	catalog, err := GenerateCatalog(context.Background(), apiURL, *versionFlag)
	if err != nil {
		return fmt.Errorf("failed to generate catalog: %w", err)
	}

	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize catalog: %w", err)
	}

	if *outputFlag != "" {
		_ = os.MkdirAll(filepath.Dir(*outputFlag), 0750)
		if err := os.WriteFile(*outputFlag, data, 0600); err != nil {
			return fmt.Errorf("failed to write to %s: %w", *outputFlag, err)
		}
	}

	if *embedFlag != "" {
		_ = os.MkdirAll(filepath.Dir(*embedFlag), 0750)
		if err := os.WriteFile(*embedFlag, data, 0600); err != nil {
			return fmt.Errorf("failed to write to %s: %w", *embedFlag, err)
		}
	}

	log.Printf("Successfully generated %d curated models into %s and %s", len(catalog.Models), *outputFlag, *embedFlag)
	return nil
}

var logFatal = log.Fatalf

func main() {
	if err := run(os.Args[1:], ""); err != nil {
		logFatal("Fatal error: %v", err)
	}
}
