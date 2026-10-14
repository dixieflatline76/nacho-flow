package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

// TestCapturedClineTurn15_Replay verifies the real-world payload captured from Cline on Turn 15.
// In the production failure, Cline sent "reasoning_effort": "none" to z-ai/glm-5.3-flash on OpenRouter.
func TestCapturedClineTurn15_Replay(t *testing.T) {
	payloadPath := filepath.Join("..", "..", "scratch", "captured_cline_run", "turn15_request_payload.json")
	payloadBytes, err := os.ReadFile(payloadPath)
	if err != nil {
		t.Skipf("Captured Turn 15 payload not found at %s: %v", payloadPath, err)
		return
	}

	p := &testOpenRouterProvider{}

	// Scenario A: When tier has ReasoningEffort: "" (e.g. unconfigured or dropped by FairyDustEntry schema omission)
	// Under the Wire Declarator fix, OpenRouter forbids effort "none", so "reasoning_effort": "none" MUST be stripped.
	tierUnset := &contract.Tier{
		Model:           "z-ai/glm-5.3-flash",
		ReasoningEffort: "",
	}

	outA := modifyTopLevelPayload(payloadBytes, tierUnset, p)

	var parsedA map[string]interface{}
	if err := json.Unmarshal(outA, &parsedA); err != nil {
		t.Fatalf("Failed to unmarshal output for Scenario A: %v", err)
	}

	if parsedA["model"] != "z-ai/glm-5.3-flash" {
		t.Errorf("Expected model 'z-ai/glm-5.3-flash', got %v", parsedA["model"])
	}

	// In Scenario A, under the Wire Declarator fix, flat reasoning_effort: "none" MUST be stripped:
	if re, exists := parsedA["reasoning_effort"]; exists {
		t.Errorf("[BUG REPRODUCED] Scenario A leaked raw 'reasoning_effort': %v to OpenRouter (expected stripped)", re)
	}
	if _, exists := parsedA["reasoning"]; exists {
		t.Errorf("Scenario A: unexpected reasoning field injected when tier effort is unset")
	}

	// Scenario B: When tier has ReasoningEffort: "high" (e.g. properly propagated Fairy Dust)
	tierHigh := &contract.Tier{
		Model:           "z-ai/glm-5.3-flash",
		ReasoningEffort: "high",
	}

	outB := modifyTopLevelPayload(payloadBytes, tierHigh, p)

	var parsedB map[string]interface{}
	if err := json.Unmarshal(outB, &parsedB); err != nil {
		t.Fatalf("Failed to unmarshal output for Scenario B: %v", err)
	}

	if _, exists := parsedB["reasoning_effort"]; exists {
		t.Errorf("Scenario B: flat reasoning_effort was not stripped")
	}

	reasoning, ok := parsedB["reasoning"].(map[string]interface{})
	if !ok {
		t.Fatalf("Scenario B: expected 'reasoning' object, got %T: %v", parsedB["reasoning"], parsedB["reasoning"])
	}
	if reasoning["effort"] != "high" {
		t.Errorf("Scenario B: expected reasoning.effort 'high', got %v", reasoning["effort"])
	}
}
