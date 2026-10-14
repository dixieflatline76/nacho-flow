package server

import (
	"encoding/json"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/provider"
)

var (
	_ provider.LLMProvider            = (*testOpenRouterProvider)(nil)
	_ provider.WireDeclaratorProvider = (*testOpenRouterProvider)(nil)
	_ provider.LLMProvider            = (*testCloudProvider)(nil)
	_ provider.WireDeclaratorProvider = (*testCloudProvider)(nil)
)

type testOpenRouterProvider struct{}

func (m *testOpenRouterProvider) ID() string      { return "openrouter" }
func (m *testOpenRouterProvider) Name() string    { return "OpenRouter" }
func (m *testOpenRouterProvider) BaseURL() string { return "https://openrouter.ai/api/v1" }
func (m *testOpenRouterProvider) IsLocal() bool   { return false }
func (m *testOpenRouterProvider) WireDeclarator() *provider.WireDeclarator {
	return &provider.WireDeclarator{
		Dialect:           provider.ReasoningDialectNested,
		ForbidsEffortNone: true,
		StripFlatOnNested: true,
	}
}

type testCloudProvider struct{}

func (m *testCloudProvider) ID() string      { return "openai" }
func (m *testCloudProvider) Name() string    { return "OpenAI" }
func (m *testCloudProvider) BaseURL() string { return "https://api.openai.com/v1" }
func (m *testCloudProvider) IsLocal() bool   { return false }
func (m *testCloudProvider) WireDeclarator() *provider.WireDeclarator {
	return provider.DefaultWireDeclarator
}

func TestModifyTopLevelPayload_OpenRouter_InjectsNestedReasoning(t *testing.T) {
	body := []byte(`{"model":"old-model","messages":[{"role":"user","content":"hi"}]}`)
	tier := &contract.Tier{
		Model:           "glm-5.3-flash",
		ReasoningEffort: "high",
	}
	p := &testOpenRouterProvider{}

	out := modifyTopLevelPayload(body, tier, p)

	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("Failed to parse output JSON: %v, raw: %s", err, string(out))
	}

	if parsed["model"] != "glm-5.3-flash" {
		t.Errorf("Expected model 'glm-5.3-flash', got %v", parsed["model"])
	}

	// Must have nested reasoning.effort = "high"
	reasoning, ok := parsed["reasoning"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected 'reasoning' to be object, got %T: %v", parsed["reasoning"], parsed["reasoning"])
	}
	if reasoning["effort"] != "high" {
		t.Errorf("Expected reasoning.effort 'high', got %v", reasoning["effort"])
	}

	// Must NOT have flat reasoning_effort
	if _, exists := parsed["reasoning_effort"]; exists {
		t.Errorf("Expected NO flat 'reasoning_effort' field, found %v", parsed["reasoning_effort"])
	}
}

func TestModifyTopLevelPayload_OpenRouter_StripsFlatAndInjectsNested(t *testing.T) {
	body := []byte(`{"model":"old-model","reasoning_effort":"low","messages":[{"role":"user","content":"hi"}]}`)
	tier := &contract.Tier{
		Model:           "glm-5.3-flash",
		ReasoningEffort: "high",
	}
	p := &testOpenRouterProvider{}

	out := modifyTopLevelPayload(body, tier, p)

	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("Failed to parse output JSON: %v, raw: %s", err, string(out))
	}

	// Flat reasoning_effort MUST be removed
	if _, exists := parsed["reasoning_effort"]; exists {
		t.Errorf("Expected flat 'reasoning_effort' to be stripped, but found: %v", parsed["reasoning_effort"])
	}

	// Nested reasoning.effort MUST be "high"
	reasoning, ok := parsed["reasoning"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected 'reasoning' to be object, got %T: %v", parsed["reasoning"], parsed["reasoning"])
	}
	if reasoning["effort"] != "high" {
		t.Errorf("Expected reasoning.effort 'high', got %v", reasoning["effort"])
	}
}

func TestModifyTopLevelPayload_OpenRouter_ExistingNestedReasoning(t *testing.T) {
	body := []byte(`{"model":"old-model","reasoning":{"effort":"low"},"messages":[{"role":"user","content":"hi"}]}`)
	tier := &contract.Tier{
		Model:           "glm-5.3-flash",
		ReasoningEffort: "high",
	}
	p := &testOpenRouterProvider{}

	out := modifyTopLevelPayload(body, tier, p)

	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("Failed to parse output JSON: %v, raw: %s", err, string(out))
	}

	reasoning, ok := parsed["reasoning"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected 'reasoning' object, got %T: %v", parsed["reasoning"], parsed["reasoning"])
	}
	if reasoning["effort"] != "high" {
		t.Errorf("Expected reasoning.effort updated to 'high', got %v", reasoning["effort"])
	}
}

func TestModifyTopLevelPayload_OpenRouter_BothFlatAndNested(t *testing.T) {
	body := []byte(`{"model":"old-model","reasoning":{"effort":"low"},"reasoning_effort":"medium","messages":[{"role":"user","content":"hi"}]}`)
	tier := &contract.Tier{
		Model:           "glm-5.3-flash",
		ReasoningEffort: "high",
	}
	p := &testOpenRouterProvider{}

	out := modifyTopLevelPayload(body, tier, p)

	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("Failed to parse output JSON: %v, raw: %s", err, string(out))
	}

	if _, exists := parsed["reasoning_effort"]; exists {
		t.Errorf("Expected flat reasoning_effort to be stripped when both present, but found: %v", parsed["reasoning_effort"])
	}

	reasoning, ok := parsed["reasoning"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected 'reasoning' object, got %T: %v", parsed["reasoning"], parsed["reasoning"])
	}
	if reasoning["effort"] != "high" {
		t.Errorf("Expected reasoning.effort updated to 'high', got %v", reasoning["effort"])
	}
}

func TestModifyTopLevelPayload_OpenRouter_PassthroughWhenUnconfigured(t *testing.T) {
	// Ground Truth Principle: Tier has reasoning_effort == "" -> Inject nothing!
	body := []byte(`{"model":"old-model","messages":[{"role":"user","content":"hi"}]}`)
	tier := &contract.Tier{
		Model:           "glm-5.3-flash",
		ReasoningEffort: "",
	}
	p := &testOpenRouterProvider{}

	out := modifyTopLevelPayload(body, tier, p)

	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("Failed to parse output JSON: %v, raw: %s", err, string(out))
	}

	if _, exists := parsed["reasoning"]; exists {
		t.Errorf("Expected NO reasoning injection when tier.ReasoningEffort is empty, got: %v", parsed["reasoning"])
	}
	if _, exists := parsed["reasoning_effort"]; exists {
		t.Errorf("Expected NO reasoning_effort injection when tier.ReasoningEffort is empty, got: %v", parsed["reasoning_effort"])
	}
}

func TestModifyTopLevelPayload_Standard_PreservesFlatSyntax(t *testing.T) {
	body := []byte(`{"model":"old-model","messages":[{"role":"user","content":"hi"}]}`)
	tier := &contract.Tier{
		Model:           "gpt-4o",
		ReasoningEffort: "high",
	}
	p := &testCloudProvider{}

	out := modifyTopLevelPayload(body, tier, p)

	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("Failed to parse output JSON: %v, raw: %s", err, string(out))
	}

	if parsed["reasoning_effort"] != "high" {
		t.Errorf("Expected flat reasoning_effort 'high', got %v", parsed["reasoning_effort"])
	}
	if _, exists := parsed["reasoning"]; exists {
		t.Errorf("Expected NO nested reasoning object for standard cloud, got %v", parsed["reasoning"])
	}
}

func TestModifyTopLevelPayload_Standard_UpdatesFlatSyntax(t *testing.T) {
	body := []byte(`{"model":"old-model","reasoning_effort":"low","messages":[{"role":"user","content":"hi"}]}`)
	tier := &contract.Tier{
		Model:           "gpt-4o",
		ReasoningEffort: "high",
	}
	p := &testCloudProvider{}

	out := modifyTopLevelPayload(body, tier, p)

	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("Failed to parse output JSON: %v, raw: %s", err, string(out))
	}

	if parsed["reasoning_effort"] != "high" {
		t.Errorf("Expected flat reasoning_effort updated to 'high', got %v", parsed["reasoning_effort"])
	}
	if _, exists := parsed["reasoning"]; exists {
		t.Errorf("Expected NO nested reasoning object for standard cloud, got %v", parsed["reasoning"])
	}
}

func BenchmarkModifyTopLevelPayload_OpenRouter_InjectReasoning(b *testing.B) {
	body := []byte(`{"model":"old-model","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	tier := &contract.Tier{
		Model:           "glm-5.3-flash",
		ReasoningEffort: "high",
	}
	p := &testOpenRouterProvider{}

	b.ReportAllocs()
	for b.Loop() {
		_ = modifyTopLevelPayload(body, tier, p)
	}
}

func BenchmarkModifyTopLevelPayload_OpenRouter_StripFlatAndInject(b *testing.B) {
	body := []byte(`{"model":"old-model","reasoning_effort":"low","messages":[{"role":"user","content":"hi"}]}`)
	tier := &contract.Tier{
		Model:           "glm-5.3-flash",
		ReasoningEffort: "high",
	}
	p := &testOpenRouterProvider{}

	b.ReportAllocs()
	for b.Loop() {
		_ = modifyTopLevelPayload(body, tier, p)
	}
}

func BenchmarkModifyTopLevelPayload_Standard_InjectReasoning(b *testing.B) {
	body := []byte(`{"model":"old-model","messages":[{"role":"user","content":"hi"}]}`)
	tier := &contract.Tier{
		Model:           "gpt-4o",
		ReasoningEffort: "high",
	}
	p := &testCloudProvider{}

	b.ReportAllocs()
	for b.Loop() {
		_ = modifyTopLevelPayload(body, tier, p)
	}
}

func BenchmarkModifyTopLevelPayload_Unchanged_ZeroAlloc(b *testing.B) {
	body := []byte(`{"model":"glm-5.3-flash","messages":[{"role":"user","content":"hi"}]}`)
	tier := &contract.Tier{
		Model:           "glm-5.3-flash",
		ReasoningEffort: "",
	}
	p := &testOpenRouterProvider{}

	b.ReportAllocs()
	for b.Loop() {
		_ = modifyTopLevelPayload(body, tier, p)
	}
}

func TestDeleteMemberSpan_AllBranches(t *testing.T) {
	// Branch 1: Preceding comma
	b1 := []byte(`{"a":1,"b":2}`)
	d1Start, d1End := deleteMemberSpan(b1, 7, 12, 0, len(b1)-1)
	res1 := append(append([]byte{}, b1[:d1Start]...), b1[d1End:]...)
	if string(res1) != `{"a":1}` {
		t.Errorf("expected '{\"a\":1}', got %s", string(res1))
	}

	// Branch 2: First member (succeeding comma)
	b2 := []byte(`{"b":2,"a":1}`)
	d2Start, d2End := deleteMemberSpan(b2, 1, 6, 0, len(b2)-1)
	res2 := append(append([]byte{}, b2[:d2Start]...), b2[d2End:]...)
	if string(res2) != `{"a":1}` {
		t.Errorf("expected '{\"a\":1}', got %s", string(res2))
	}

	// Branch 3: Sole member (no comma)
	b3 := []byte(`{"b":2}`)
	d3Start, d3End := deleteMemberSpan(b3, 1, 6, 0, len(b3)-1)
	res3 := append(append([]byte{}, b3[:d3Start]...), b3[d3End:]...)
	if string(res3) != `{}` {
		t.Errorf("expected '{}', got %s", string(res3))
	}
}

func TestModifyTopLevelPayload_EdgeCases(t *testing.T) {
	tier := &contract.Tier{Model: "glm-5.3-flash", ReasoningEffort: "high"}
	p := &testOpenRouterProvider{}

	// Non-object
	if string(modifyTopLevelPayload([]byte(`[]`), tier, p)) != `[]` {
		t.Errorf("expected non-object to return untouched")
	}

	// Empty / invalid
	if string(modifyTopLevelPayload([]byte(`   `), tier, p)) != `   ` {
		t.Errorf("expected whitespace to return untouched")
	}

	// Unclosed object
	if string(modifyTopLevelPayload([]byte(`{`), tier, p)) != `{` {
		t.Errorf("expected unclosed object to return untouched")
	}

	// Invalid member start
	if string(modifyTopLevelPayload([]byte(`{123}`), tier, p)) != `{123}` {
		t.Errorf("expected invalid object to return untouched")
	}

	// First member is flat reasoning_effort
	bFirst := []byte(`{"reasoning_effort":"low","model":"old"}`)
	outFirst := modifyTopLevelPayload(bFirst, tier, p)
	var parsedFirst map[string]interface{}
	if err := json.Unmarshal(outFirst, &parsedFirst); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}
	if _, exists := parsedFirst["reasoning_effort"]; exists {
		t.Errorf("Expected flat reasoning_effort to be removed when it was first member")
	}

	// Duplicate reasoning keys and duplicate reasoning_effort keys
	bDup := []byte(`{"reasoning":{"effort":"low"},"reasoning":{"effort":"low"},"reasoning_effort":"low","reasoning_effort":"low"}`)
	outDup := modifyTopLevelPayload(bDup, tier, p)
	var parsedDup map[string]interface{}
	if err := json.Unmarshal(outDup, &parsedDup); err != nil {
		t.Fatalf("Failed to parse: %v (raw: %s)", err, string(outDup))
	}
	if _, exists := parsedDup["reasoning_effort"]; exists {
		t.Errorf("Expected all duplicate flat reasoning_effort to be removed")
	}
}

func TestModifyTopLevelPayload_OpenRouter_StripsClientNone_WhenTierUnset(t *testing.T) {
	body := []byte(`{"model":"glm-5.3-flash","reasoning_effort":"none","messages":[{"role":"user","content":"hi"}]}`)
	tier := &contract.Tier{Model: "glm-5.3-flash", ReasoningEffort: ""}
	p := &testOpenRouterProvider{}

	out := modifyTopLevelPayload(body, tier, p)
	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}
	if _, exists := parsed["reasoning_effort"]; exists {
		t.Errorf("Expected reasoning_effort: none to be stripped")
	}
	if _, exists := parsed["reasoning"]; exists {
		t.Errorf("Expected reasoning to not exist")
	}
}

func TestModifyTopLevelPayload_OpenRouter_StripsNestedNone_WhenTierUnset(t *testing.T) {
	body := []byte(`{"model":"glm-5.3-flash","reasoning":{"effort":"none"},"messages":[{"role":"user","content":"hi"}]}`)
	tier := &contract.Tier{Model: "glm-5.3-flash", ReasoningEffort: ""}
	p := &testOpenRouterProvider{}

	out := modifyTopLevelPayload(body, tier, p)
	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}
	if _, exists := parsed["reasoning"]; exists {
		t.Errorf("Expected reasoning: {effort: none} to be stripped")
	}
}

func TestModifyTopLevelPayload_AdjacentTailDeletion_BothNone(t *testing.T) {
	// Tests both "reasoning": {"effort": "none"} and "reasoning_effort": "none" at the end of the JSON object
	body := []byte(`{"model":"glm-5.3-flash","messages":[{"role":"user","content":"hi"}],"reasoning":{"effort":"none"},"reasoning_effort":"none"}`)
	tier := &contract.Tier{Model: "glm-5.3-flash", ReasoningEffort: ""}
	p := &testOpenRouterProvider{}

	out := modifyTopLevelPayload(body, tier, p)
	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("Failed to parse output JSON: %v, raw: %s", err, string(out))
	}
	if _, exists := parsed["reasoning_effort"]; exists {
		t.Errorf("Expected tail reasoning_effort to be stripped")
	}
	if _, exists := parsed["reasoning"]; exists {
		t.Errorf("Expected tail reasoning to be stripped")
	}
	if parsed["model"] != "glm-5.3-flash" {
		t.Errorf("Expected model preserved")
	}
}

func TestModifyTopLevelPayload_AdjacentHeadDeletion_BothNone(t *testing.T) {
	// Tests both reasoning fields at the beginning of the object
	body := []byte(`{"reasoning":{"effort":"none"},"reasoning_effort":"none","model":"glm-5.3-flash","messages":[{"role":"user","content":"hi"}]}`)
	tier := &contract.Tier{Model: "glm-5.3-flash", ReasoningEffort: ""}
	p := &testOpenRouterProvider{}

	out := modifyTopLevelPayload(body, tier, p)
	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("Failed to parse output JSON: %v, raw: %s", err, string(out))
	}
	if _, exists := parsed["reasoning_effort"]; exists {
		t.Errorf("Expected head reasoning_effort to be stripped")
	}
	if _, exists := parsed["reasoning"]; exists {
		t.Errorf("Expected head reasoning to be stripped")
	}
	if parsed["model"] != "glm-5.3-flash" {
		t.Errorf("Expected model preserved")
	}
}

func TestModifyTopLevelPayload_SoleMembers_BothNone(t *testing.T) {
	// Tests object containing only reasoning fields: {"reasoning":{"effort":"none"},"reasoning_effort":"none"}
	body := []byte(`{"reasoning":{"effort":"none"},"reasoning_effort":"none"}`)
	tier := &contract.Tier{Model: "glm-5.3-flash", ReasoningEffort: ""}
	p := &testOpenRouterProvider{}

	out := modifyTopLevelPayload(body, tier, p)
	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("Failed to parse output JSON: %v, raw: %s", err, string(out))
	}
	if _, exists := parsed["reasoning_effort"]; exists {
		t.Errorf("Expected reasoning_effort stripped")
	}
	if _, exists := parsed["reasoning"]; exists {
		t.Errorf("Expected reasoning stripped")
	}
}

func TestModifyTopLevelPayload_PartialReasoning_PreservesValidFields(t *testing.T) {
	// When reasoning has other fields (e.g. max_tokens: 1000) and effort: none,
	// only effort: none is stripped from the nested object, leaving max_tokens.
	body := []byte(`{"model":"glm-5.3-flash","reasoning":{"effort":"none","max_tokens":1000},"reasoning_effort":"none"}`)
	tier := &contract.Tier{Model: "glm-5.3-flash", ReasoningEffort: ""}
	p := &testOpenRouterProvider{}

	out := modifyTopLevelPayload(body, tier, p)
	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("Failed to parse output JSON: %v, raw: %s", err, string(out))
	}
	if _, exists := parsed["reasoning_effort"]; exists {
		t.Errorf("Expected flat reasoning_effort: none to be stripped")
	}
	reasoning, ok := parsed["reasoning"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected reasoning object preserved, got %T: %v", parsed["reasoning"], parsed["reasoning"])
	}
	if _, exists := reasoning["effort"]; exists {
		t.Errorf("Expected nested effort: none to be stripped from reasoning object")
	}
	if reasoning["max_tokens"] != float64(1000) {
		t.Errorf("Expected max_tokens 1000 preserved, got %v", reasoning["max_tokens"])
	}
}

func TestModifyTopLevelPayload_Standard_PreservesClientNone(t *testing.T) {
	// Standard Cloud / Local does NOT forbid effort "none".
	// When tier.ReasoningEffort == "", it must leave client payload untouched.
	body := []byte(`{"model":"gpt-4o","reasoning_effort":"none"}`)
	tier := &contract.Tier{Model: "gpt-4o", ReasoningEffort: ""}
	p := &testCloudProvider{}

	out := modifyTopLevelPayload(body, tier, p)
	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}
	if parsed["reasoning_effort"] != "none" {
		t.Errorf("Expected reasoning_effort 'none' preserved for standard cloud provider")
	}
}

func TestModifyTopLevelPayload_NilTargetProvider_DefaultsSafely(t *testing.T) {
	// When targetProvider is nil, DefaultWireDeclarator is used safely without panic.
	body := []byte(`{"model":"gpt-4o","reasoning_effort":"none"}`)
	tier := &contract.Tier{Model: "gpt-4o", ReasoningEffort: ""}

	out := modifyTopLevelPayload(body, tier, nil)
	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}
	if parsed["reasoning_effort"] != "none" {
		t.Errorf("Expected reasoning_effort preserved when targetProvider is nil")
	}
}

func BenchmarkModifyTopLevelPayload_OpenRouter_StripNone(b *testing.B) {
	body := []byte(`{"model":"glm-5.3-flash","reasoning_effort":"none","messages":[{"role":"user","content":"hi"}]}`)
	tier := &contract.Tier{Model: "glm-5.3-flash", ReasoningEffort: ""}
	p := &testOpenRouterProvider{}

	b.ReportAllocs()
	for b.Loop() {
		_ = modifyTopLevelPayload(body, tier, p)
	}
}

