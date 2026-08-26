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
	"strings"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry/curation"
)

// GenerateCatalog pulls OpenRouter models, blends curated ratings, and generates catalog JSON.
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
			Architecture  struct {
				InputModalities []string `json:"input_modalities"`
			} `json:"architecture"`
			SupportedParameters []string `json:"supported_parameters"`
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

	curatedOverrides := GetCuratedOverrides()

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

		role := curation.RoleGeneral
		var recTiers []string
		name := strings.ToLower(m.ID)
		if strings.Contains(name, "coder") && hasTools {
			role = curation.RoleCodingWorkhorse
			recTiers = []string{contract.TierIDWorkhorse}
		} else if hasVision && (strings.Contains(name, "flash") || strings.Contains(name, "lite")) {
			role = curation.RoleVisionWorkhorse
			recTiers = []string{contract.TierIDVision}
		} else if strings.Contains(name, "r1") || strings.Contains(name, "reason") {
			role = curation.RoleDeepReasoner
			recTiers = []string{contract.TierIDFrontier}
		}

		profile := curation.ModelCuratedProfile{
			Name:             m.Name,
			TierRole:         role,
			ToolReliability:  85.0,
			RecommendedTiers: recTiers,
		}

		if override, exists := curatedOverrides[m.ID]; exists {
			profile = override
		}

		if profile.TierRole != curation.RoleGeneral || curatedOverrides[m.ID].Name != "" {
			catalog.Models[m.ID] = profile
		}
	}

	// Ensure curated overrides are always present even if missing from API response
	for id, override := range curatedOverrides {
		if _, exists := catalog.Models[id]; !exists {
			catalog.Models[id] = override
		}
	}

	return catalog, nil
}

// GetCuratedOverrides returns the baseline high-precision benchmarks for top frontier & coding workhorses.
func GetCuratedOverrides() map[string]curation.ModelCuratedProfile {
	return map[string]curation.ModelCuratedProfile{
		"google/gemini-2.5-flash": {
			Name:             "Google: Gemini 2.5 Flash",
			TierRole:         curation.RoleCodingWorkhorse,
			CodingIndex:      78.4,
			ToolReliability:  95.0,
			RecommendedTiers: []string{contract.TierIDVision, contract.TierIDWorkhorse},
		},
		"google/gemini-2.5-flash-lite": {
			Name:             "Google: Gemini 2.5 Flash Lite",
			TierRole:         curation.RoleVisionWorkhorse,
			CodingIndex:      68.1,
			ToolReliability:  90.0,
			RecommendedTiers: []string{contract.TierIDVision},
		},
		"qwen/qwen3-coder-480b": {
			Name:             "Qwen: Qwen 3 Coder 480B",
			TierRole:         curation.RoleCodingWorkhorse,
			CodingIndex:      82.0,
			ToolReliability:  96.0,
			RecommendedTiers: []string{contract.TierIDWorkhorse},
		},
		"deepseek/deepseek-r1": {
			Name:             "DeepSeek: R1 (Reasoning)",
			TierRole:         curation.RoleDeepReasoner,
			CodingIndex:      75.0,
			ToolReliability:  88.0,
			RecommendedTiers: []string{contract.TierIDFrontier},
		},
		"anthropic/claude-3.5-sonnet": {
			Name:             "Anthropic: Claude 3.5 Sonnet",
			TierRole:         curation.RoleCodingWorkhorse,
			CodingIndex:      92.0,
			ToolReliability:  99.0,
			RecommendedTiers: []string{contract.TierIDFrontier},
		},
	}
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
