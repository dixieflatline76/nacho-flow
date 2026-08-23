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

	// Python args with raw unstructured text
	rawUnstructured := `<|python_tag|>unstructured_tool(just some raw query text without equals)`
	_, calls2, ok2 := NormalizeMarkdownToolCalls(rawUnstructured)
	if !ok2 || len(calls2) != 1 {
		t.Fatalf("Expected fallback raw_args wrap for unstructured python args")
	}
	if !strings.Contains(calls2[0].Function.Arguments, "raw_args") {
		t.Errorf("Expected raw_args in arguments: %s", calls2[0].Function.Arguments)
	}
}

// 15. Unclosed and Malformed Tags Fallbacks
func TestNormalize_UnclosedTagsAndEdgeExtractors(t *testing.T) {
	// Unclosed <tool_call>
	raw1 := "<tool_call>{\"name\": \"unclosed\"}"
	_, _, ok1 := NormalizeMarkdownToolCalls(raw1)
	if ok1 {
		t.Errorf("Expected false for unclosed <tool_call>")
	}

	// Unclosed <function=name>
	raw2 := "<function=unclosed>{\"a\": 1}"
	_, _, ok2 := NormalizeMarkdownToolCalls(raw2)
	if ok2 {
		t.Errorf("Expected false for unclosed <function>")
	}

	// ReAct single-line action input without trailing newline
	raw3 := "Action: single_line_tool\nAction Input: my_single_argument"
	_, calls3, ok3 := NormalizeMarkdownToolCalls(raw3)
	if !ok3 || len(calls3) != 1 || calls3[0].Function.Name != "single_line_tool" {
		t.Errorf("Failed to parse single line ReAct input: %+v", calls3)
	}

	// Balanced JSON with escaped quotes inside string
	raw4 := `{"name": "escaped_quotes", "arguments": "{\"query\": \"hello \\\"world\\\"\"}"}`
	_, _, ok4 := NormalizeMarkdownToolCalls("<tool_call>" + raw4 + "</tool_call>")
	if !ok4 {
		t.Errorf("Failed to parse balanced JSON with escaped quotes")
	}
}

// 16. Unit Tests for internal serialization & validation helpers
func TestNormalize_InternalHelpers(t *testing.T) {
	// serializeArgs with non-JSON string
	res1 := serializeArgs("plain_string_value")
	if res1 != `"plain_string_value"` {
		t.Errorf("Expected quoted JSON string, got: %s", res1)
	}

	// serializeArgs with valid JSON string
	res2 := serializeArgs(`{"valid":true}`)
	if res2 != `{"valid":true}` {
		t.Errorf("Expected unchanged JSON string, got: %s", res2)
	}

	// serializeArgs with map
	res3 := serializeArgs(map[string]int{"count": 5})
	if res3 != `{"count":5}` {
		t.Errorf("Expected serialized map, got: %s", res3)
	}

	// isValidJSON with empty and whitespace
	if isValidJSON("") || isValidJSON("   ") {
		t.Errorf("Expected false for empty JSON strings")
	}

	// pythonArgsToJSON with empty string
	if pythonArgsToJSON("") != "{}" || pythonArgsToJSON("   ") != "{}" {
		t.Errorf("Expected empty JSON object for empty python args")
	}

	// pythonArgsToJSON with valid JSON
	if pythonArgsToJSON(`{"already":"json"}`) != `{"already":"json"}` {
		t.Errorf("Expected raw pass-through for already valid JSON")
	}

	// parseMapToToolCall with invalid/empty map
	if _, ok := parseMapToToolCall(map[string]interface{}{"unknown_key": "val"}, 1); ok {
		t.Errorf("Expected false for map with unknown keys")
	}

	// parseSingleToolCall with invalid JSON
	if _, ok := parseSingleToolCall("not_json", 1); ok {
		t.Errorf("Expected false for invalid JSON in parseSingleToolCall")
	}
}

// 12. Bare JSON Output (Direct Ollama / Qwen response without code fences)
func TestNormalize_BareJSON_OllamaOutput(t *testing.T) {
	raw := "{\n  \"name\": \"read_file\",\n  \"arguments\": {\n    \"path\": \"docs/VSCODE_EXTENSION_SPEC.md\"\n  }\n}"

	cleaned, calls, ok := NormalizeMarkdownToolCalls(raw)
	if !ok {
		t.Fatalf("Expected ok to be true for bare JSON output, got false")
	}
	if len(calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "read_file" {
		t.Errorf("Expected 'read_file', got '%s'", calls[0].Function.Name)
	}
	if !strings.Contains(calls[0].Function.Arguments, "VSCODE_EXTENSION_SPEC.md") {
		t.Errorf("Expected arguments to contain 'VSCODE_EXTENSION_SPEC.md', got '%s'", calls[0].Function.Arguments)
	}
	if strings.TrimSpace(cleaned) != "" {
		t.Errorf("Expected empty cleaned content for pure tool JSON, got '%s'", cleaned)
	}
}

// 13. Conversational Prefix + Bare JSON Output (Qwen conversational response)
func TestNormalize_ConversationalPrefix_BareJSON(t *testing.T) {
	raw := "Sure, I will read the file located at docs/VSCODE_EXTENSION_SPEC.md. Here is how I will call the function:\n\n{\n  \"name\": \"read_file\",\n  \"arguments\": {\n    \"path\": \"docs/VSCODE_EXTENSION_SPEC.md\"\n  }\n}"

	cleaned, calls, ok := NormalizeMarkdownToolCalls(raw)
	if !ok {
		t.Fatalf("Expected ok to be true for conversational bare JSON output, got false")
	}
	if len(calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "read_file" {
		t.Errorf("Expected 'read_file', got '%s'", calls[0].Function.Name)
	}
	if !strings.Contains(cleaned, "Sure, I will read the file") {
		t.Errorf("Expected conversational text preserved in cleaned output, got '%s'", cleaned)
	}
}

// 14. Bare JSON with parameters, nested function, and array format
func TestNormalize_BareJSON_Variations(t *testing.T) {
	// Format with parameters
	rawParams := "{\"name\": \"search\", \"parameters\": {\"q\": \"golang\"}}"
	_, calls1, ok1 := NormalizeMarkdownToolCalls(rawParams)
	if !ok1 || len(calls1) != 1 || calls1[0].Function.Name != "search" {
		t.Errorf("Failed to parse bare JSON with parameters: %+v", calls1)
	}

	// Format with nested function object
	rawFn := "{\"function\": {\"name\": \"exec_cmd\", \"arguments\": {\"cmd\": \"ls\"}}}"
	_, calls2, ok2 := NormalizeMarkdownToolCalls(rawFn)
	if !ok2 || len(calls2) != 1 || calls2[0].Function.Name != "exec_cmd" {
		t.Errorf("Failed to parse bare JSON with nested function: %+v", calls2)
	}

	// Format with array of tool calls
	rawArr := "[{\"name\": \"t1\", \"arguments\": {\"a\": 1}}, {\"name\": \"t2\", \"arguments\": {\"b\": 2}}]"
	_, calls3, ok3 := NormalizeMarkdownToolCalls(rawArr)
	if !ok3 || len(calls3) != 2 {
		t.Errorf("Failed to parse bare JSON array: %+v", calls3)
	}

	// Format with array using parameters and nested function
	rawArrParams := "[{\"name\": \"t1\", \"parameters\": {\"a\": 1}}]"
	_, calls4, ok4 := NormalizeMarkdownToolCalls(rawArrParams)
	if !ok4 || len(calls4) != 1 {
		t.Errorf("Failed to parse bare JSON array with parameters: %+v", calls4)
	}

	rawArrFn := "[{\"function\": {\"name\": \"t1\", \"arguments\": {\"a\": 1}}}]"
	_, calls5, ok5 := NormalizeMarkdownToolCalls(rawArrFn)
	if !ok5 || len(calls5) != 1 {
		t.Errorf("Failed to parse bare JSON array with nested function: %+v", calls5)
	}
}

// 15. Strategy Names & Direct Parser Invocation
func TestStrategyNamesAndDirectExecution(t *testing.T) {
	pipeline := NewDefaultPipeline()
	if len(pipeline.parsers) != 8 {
		t.Fatalf("Expected 8 registered parsers in default pipeline, got %d", len(pipeline.parsers))
	}

	for _, parser := range pipeline.parsers {
		name := parser.Name()
		if name == "" {
			t.Errorf("Expected non-empty name for parser %T", parser)
		}

		// Verify try-and-fail-fast behavior with unmatched content
		rem, calls, matched := parser.Parse("Non-matching plain prose text without tags.", 1)
		if matched || len(calls) > 0 || rem != "Non-matching plain prose text without tags." {
			t.Errorf("Parser %s failed to gracefully reject unmatched text", name)
		}
	}
}

// 16. Edge Cases for Balanced JSON Extractor
func TestExtractBalancedJSON_EdgeCases(t *testing.T) {
	// StartIdx beyond string length
	if _, _, _, ok := extractBalancedJSON("{}", 10); ok {
		t.Errorf("Expected false for startIdx beyond length")
	}

	// Leading non-JSON tokens
	if _, _, _, ok := extractBalancedJSON("xyz", 0); ok {
		t.Errorf("Expected false for non-JSON tokens")
	}

	// Whitespace only
	if _, _, _, ok := extractBalancedJSON("   ", 0); ok {
		t.Errorf("Expected false for whitespace only")
	}

	// Unclosed brackets
	if _, _, _, ok := extractBalancedJSON("{ unclosed", 0); ok {
		t.Errorf("Expected false for unclosed bracket")
	}
}

// 17. Pipeline edge cases
func TestNormalizerPipeline_EmptyAndNoMatch(t *testing.T) {
	pipeline := NewDefaultPipeline()

	// Empty string
	if _, _, ok := pipeline.Normalize(""); ok {
		t.Errorf("Expected false for empty content")
	}

	// Content with candidate token '<' but no matching parser
	if _, _, ok := pipeline.Normalize("This is <not a tool tag> at all."); ok {
		t.Errorf("Expected false for unmatched candidate token")
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
