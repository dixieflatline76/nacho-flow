package router

import (
	"encoding/json"
	"strings"
	"testing"
)

// 1. Hermes / Nous / Qwen Format
func TestNormalize_HermesXMLFormat(t *testing.T) {
	raw := "I will check the status:\n<tool_call>\n{\"name\": \"get_status\", \"arguments\": {\"service\": \"nacho-flow\"}}\n</tool_call>"

	cleaned, calls, ok := NormalizeMarkdownToolCalls(raw)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if len(calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "get_status" {
		t.Errorf("Expected 'get_status', got '%s'", calls[0].Function.Name)
	}
	if !strings.Contains(calls[0].Function.Arguments, "nacho-flow") {
		t.Errorf("Expected arguments to contain 'nacho-flow', got '%s'", calls[0].Function.Arguments)
	}
	if strings.Contains(cleaned, "<tool_call>") {
		t.Errorf("Expected <tool_call> tags stripped from cleaned text")
	}
}

// 2. Mistral [TOOL_CALLS] Array Format
func TestNormalize_MistralToolCallsArray(t *testing.T) {
	raw := "[TOOL_CALLS] [{\"name\": \"read_file\", \"arguments\": {\"path\": \"config.go\"}}, {\"name\": \"read_file\", \"arguments\": {\"path\": \"proxy.go\"}}]"

	cleaned, calls, ok := NormalizeMarkdownToolCalls(raw)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if len(calls) != 2 {
		t.Fatalf("Expected 2 calls, got %d", len(calls))
	}
	if calls[0].Function.Name != "read_file" || calls[1].Function.Name != "read_file" {
		t.Errorf("Expected both to be 'read_file'")
	}
	if strings.Contains(cleaned, "[TOOL_CALLS]") {
		t.Errorf("Expected [TOOL_CALLS] token stripped")
	}
}

// 3. Mistral [TOOL_CALLS] Single Object Format
func TestNormalize_MistralToolCallsSingle(t *testing.T) {
	raw := "Executing operation: [TOOL_CALLS] {\"name\": \"list_dir\", \"arguments\": {\"path\": \"./cmd\"}}"

	_, calls, ok := NormalizeMarkdownToolCalls(raw)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if len(calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "list_dir" {
		t.Errorf("Expected 'list_dir', got '%s'", calls[0].Function.Name)
	}
}

// 4. Llama 3 <function=name>{...}</function> Format
func TestNormalize_Llama3FunctionTag(t *testing.T) {
	raw := "<function=write_file>{\"path\": \"test.txt\", \"content\": \"hello world\"}</function>"

	_, calls, ok := NormalizeMarkdownToolCalls(raw)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if len(calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "write_file" {
		t.Errorf("Expected 'write_file', got '%s'", calls[0].Function.Name)
	}
	if !strings.Contains(calls[0].Function.Arguments, "hello world") {
		t.Errorf("Expected content in arguments, got '%s'", calls[0].Function.Arguments)
	}
}

// 5. Llama 3.1 Python Tag Format with keyword arguments
func TestNormalize_Llama3PythonTag(t *testing.T) {
	raw := "<|python_tag|>brave_search.call(query=\"Golang 1.25 release date\", limit=5)<|eom_id|>"

	_, calls, ok := NormalizeMarkdownToolCalls(raw)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if len(calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "brave_search" {
		t.Errorf("Expected 'brave_search', got '%s'", calls[0].Function.Name)
	}
	if !strings.Contains(calls[0].Function.Arguments, "Golang 1.25") {
		t.Errorf("Expected arguments to contain query, got '%s'", calls[0].Function.Arguments)
	}
}

// 6. Claude XML Style <invoke name="...">
func TestNormalize_ClaudeXMLInvoke(t *testing.T) {
	raw := `<function_calls>
<invoke name="search_code">
<parameter name="query">func ServeHTTP</parameter>
<parameter name="path">pkg/server</parameter>
</invoke>
</function_calls>`

	_, calls, ok := NormalizeMarkdownToolCalls(raw)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if len(calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "search_code" {
		t.Errorf("Expected 'search_code', got '%s'", calls[0].Function.Name)
	}
	if !strings.Contains(calls[0].Function.Arguments, "func ServeHTTP") {
		t.Errorf("Expected arguments to contain 'func ServeHTTP', got '%s'", calls[0].Function.Arguments)
	}
}

// 7. ReAct Action / Action Input Format
func TestNormalize_ReActFormat(t *testing.T) {
	raw := "Thought: I need to run unit tests first.\nAction: execute_command\nAction Input: {\"cmd\": \"go test -race ./...\"}"

	cleaned, calls, ok := NormalizeMarkdownToolCalls(raw)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if len(calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "execute_command" {
		t.Errorf("Expected 'execute_command', got '%s'", calls[0].Function.Name)
	}
	if !strings.Contains(cleaned, "Thought: I need to run unit tests first.") {
		t.Errorf("Expected thought preserved in cleaned text, got '%s'", cleaned)
	}
}

// 8. DeepSeek-R1 CoT Reasoning + Markdown Tool Fence
func TestNormalize_DeepSeekR1_ReasoningAndMarkdown(t *testing.T) {
	raw := "<think>\nLet's check the proxy implementation to verify the auth logic.\n</think>\n```json\n{\n  \"name\": \"view_file\",\n  \"arguments\": {\"path\": \"pkg/server/proxy.go\"}\n}\n```"

	cleaned, calls, ok := NormalizeMarkdownToolCalls(raw)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if len(calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "view_file" {
		t.Errorf("Expected 'view_file', got '%s'", calls[0].Function.Name)
	}
	if !strings.Contains(cleaned, "<think>") || !strings.Contains(cleaned, "Let's check the proxy implementation") {
		t.Errorf("Expected <think> reasoning block preserved, got: %s", cleaned)
	}
	if strings.Contains(cleaned, "```json") {
		t.Errorf("Expected JSON code block removed from cleaned text")
	}
}

// 9. 4-Backtick Markdown Fence
func TestNormalize_FourBackticksMarkdown(t *testing.T) {
	raw := "````tool\n{\n  \"name\": \"run_linter\",\n  \"arguments\": {\"fast\": true}\n}\n````"

	_, calls, ok := NormalizeMarkdownToolCalls(raw)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if len(calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "run_linter" {
		t.Errorf("Expected 'run_linter', got '%s'", calls[0].Function.Name)
	}
}

// 10. Stringified JSON in Arguments
func TestNormalize_StringifiedArguments(t *testing.T) {
	raw := "```json\n{\n  \"name\": \"update_config\",\n  \"arguments\": \"{\\\"port\\\": 8000}\"\n}\n```"

	_, calls, ok := NormalizeMarkdownToolCalls(raw)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if len(calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Arguments != "{\"port\": 8000}" {
		t.Errorf("Expected stringified JSON preserved, got '%s'", calls[0].Function.Arguments)
	}
}

// 11. Malformed JSON Fallback
func TestNormalize_MalformedJSON_Fallback(t *testing.T) {
	raw := "Some text with broken json:\n```json\n{ not valid json ::: }\n```"

	cleaned, calls, ok := NormalizeMarkdownToolCalls(raw)
	if ok {
		t.Errorf("Expected ok to be false for broken json, got true")
	}
	if len(calls) > 0 {
		t.Errorf("Expected 0 calls, got %d", len(calls))
	}
	if cleaned != raw {
		t.Errorf("Expected raw text untouched, got '%s'", cleaned)
	}
}

// 12. Pure Prose (Negative Test)
func TestNormalize_PureProse(t *testing.T) {
	raw := "Here is how to design a distributed lock in Go using etcd or Redis."

	cleaned, calls, ok := NormalizeMarkdownToolCalls(raw)
	if ok {
		t.Errorf("Expected ok to be false for pure text, got true")
	}
	if len(calls) > 0 {
		t.Errorf("Expected 0 calls, got %d", len(calls))
	}
	if cleaned != raw {
		t.Errorf("Expected raw text untouched, got '%s'", cleaned)
	}
}

// 13. Edge Formats: Parameters key & Nested tool_call key
func TestNormalize_EdgeFormats(t *testing.T) {
	// Parameters key instead of arguments
	raw1 := "```json\n{\"name\": \"test_params\", \"parameters\": {\"param1\": 123}}\n```"
	_, calls1, ok1 := NormalizeMarkdownToolCalls(raw1)
	if !ok1 || len(calls1) != 1 || calls1[0].Function.Name != "test_params" {
		t.Errorf("Failed to parse parameters key format: %+v", calls1)
	}

	// Nested tool_call key
	raw2 := "```json\n{\"tool_call\": {\"name\": \"nested_func\", \"arguments\": {\"a\": true}}}\n```"
	_, calls2, ok2 := NormalizeMarkdownToolCalls(raw2)
	if !ok2 || len(calls2) != 1 || calls2[0].Function.Name != "nested_func" {
		t.Errorf("Failed to parse nested tool_call key format: %+v", calls2)
	}

	// Tool call with function map but missing arguments
	raw3 := "```json\n{\"function\": {\"name\": \"no_args_func\"}}\n```"
	_, calls3, ok3 := NormalizeMarkdownToolCalls(raw3)
	if !ok3 || len(calls3) != 1 || calls3[0].Function.Arguments != "{}" {
		t.Errorf("Expected empty JSON object for missing arguments: %+v", calls3)
	}
}

// 14. Python Args Parser Edge Cases
func TestNormalize_PythonArgs_EdgeCases(t *testing.T) {
	raw := `<|python_tag|>custom_tool.call(flag_true=true, flag_false=false, null_val=null, number=42.5, single_quote='hello')`
	_, calls, ok := NormalizeMarkdownToolCalls(raw)
	if !ok || len(calls) != 1 {
		t.Fatalf("Expected 1 call from python tag, got: %+v", calls)
	}
	if calls[0].Function.Name != "custom_tool" {
		t.Errorf("Expected name custom_tool, got %s", calls[0].Function.Name)
	}
	var parsedArgs map[string]interface{}
	if err := json.Unmarshal([]byte(calls[0].Function.Arguments), &parsedArgs); err != nil {
		t.Fatalf("Failed to parse converted python arguments: %v", err)
	}
	if parsedArgs["flag_true"] != true || parsedArgs["flag_false"] != false {
		t.Errorf("Boolean parsing error in python args: %+v", parsedArgs)
	}
	if parsedArgs["single_quote"] != "hello" {
		t.Errorf("Single quote string parsing error: %+v", parsedArgs)
	}
}

// ---------------------------------------------------------------------------
// Go Micro-Benchmarks (Nanosecond & Allocation Accuracy)
// ---------------------------------------------------------------------------

func BenchmarkNormalize_PureProse_FastBailout(b *testing.B) {
	raw := "This is a standard text response explaining concurrency patterns in Go with channels and mutexes."
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_, _, _ = NormalizeMarkdownToolCalls(raw)
	}
}

func BenchmarkNormalize_HermesXML_FullNormalization(b *testing.B) {
	raw := "I will inspect the file:\n<tool_call>\n{\"name\": \"read_file\", \"arguments\": {\"path\": \"pkg/server/proxy.go\"}}\n</tool_call>"
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_, _, _ = NormalizeMarkdownToolCalls(raw)
	}
}

func BenchmarkNormalize_DeepSeekR1_ReasoningAndToolCall(b *testing.B) {
	raw := "<think>\nAnalyzing the issue with proxy latency.\n</think>\n```json\n{\n  \"name\": \"search_code\",\n  \"arguments\": {\"pattern\": \"atomic.Pointer\"}\n}\n```"
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_, _, _ = NormalizeMarkdownToolCalls(raw)
	}
}

func BenchmarkNormalize_Mistral_ArrayCalls(b *testing.B) {
	raw := "[TOOL_CALLS] [{\"name\": \"read_file\", \"arguments\": {\"path\": \"a.go\"}}, {\"name\": \"read_file\", \"arguments\": {\"path\": \"b.go\"}}]"
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_, _, _ = NormalizeMarkdownToolCalls(raw)
	}
}
