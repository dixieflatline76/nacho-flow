package router

import (
	"strings"
	"testing"
)

func TestNormalizeMarkdownToolCalls_SingleJSONFence(t *testing.T) {
	raw := "I will inspect the files for you.\n```json\n{\n  \"name\": \"list_files\",\n  \"arguments\": {\n    \"path\": \"./cmd\"\n  }\n}\n```\nLet me know if you need more."

	cleanedText, toolCalls, ok := NormalizeMarkdownToolCalls(raw)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if len(toolCalls) != 1 {
		t.Fatalf("Expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].Function.Name != "list_files" {
		t.Errorf("Expected function name 'list_files', got '%s'", toolCalls[0].Function.Name)
	}
	if !strings.Contains(toolCalls[0].Function.Arguments, "./cmd") {
		t.Errorf("Expected arguments to contain './cmd', got '%s'", toolCalls[0].Function.Arguments)
	}
	if strings.Contains(cleanedText, "```json") {
		t.Errorf("Expected code block to be stripped from message text, got: %s", cleanedText)
	}
}

func TestNormalizeMarkdownToolCalls_XMLToolCall(t *testing.T) {
	raw := "Executing operation:\n<tool_call>\n{\"name\": \"run_command\", \"arguments\": {\"cmd\": \"go test ./...\"}}\n</tool_call>"

	cleanedText, toolCalls, ok := NormalizeMarkdownToolCalls(raw)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if len(toolCalls) != 1 {
		t.Fatalf("Expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].Function.Name != "run_command" {
		t.Errorf("Expected 'run_command', got '%s'", toolCalls[0].Function.Name)
	}
	if strings.Contains(cleanedText, "<tool_call>") {
		t.Errorf("Expected <tool_call> tag to be removed from cleaned text")
	}
}

func TestNormalizeMarkdownToolCalls_ArrayOfCalls(t *testing.T) {
	raw := "```json\n[\n  {\"name\": \"read_file\", \"arguments\": {\"path\": \"a.go\"}},\n  {\"name\": \"read_file\", \"arguments\": {\"path\": \"b.go\"}}\n]\n```"

	_, toolCalls, ok := NormalizeMarkdownToolCalls(raw)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if len(toolCalls) != 2 {
		t.Fatalf("Expected 2 tool calls, got %d", len(toolCalls))
	}
	if toolCalls[0].Function.Name != "read_file" || toolCalls[1].Function.Name != "read_file" {
		t.Errorf("Expected both calls to be 'read_file'")
	}
}

func TestNormalizeMarkdownToolCalls_FunctionFormat(t *testing.T) {
	raw := "```json\n{\n  \"function\": {\n    \"name\": \"search_code\",\n    \"arguments\": {\"pattern\": \"TODO\"}\n  }\n}\n```"

	_, toolCalls, ok := NormalizeMarkdownToolCalls(raw)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if len(toolCalls) != 1 {
		t.Fatalf("Expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].Function.Name != "search_code" {
		t.Errorf("Expected 'search_code', got '%s'", toolCalls[0].Function.Name)
	}
	if !strings.Contains(toolCalls[0].Function.Arguments, "TODO") {
		t.Errorf("Expected arguments to contain 'TODO', got '%s'", toolCalls[0].Function.Arguments)
	}
}

func TestNormalizeMarkdownToolCalls_ParametersFormat(t *testing.T) {
	raw := "```json\n{\n  \"name\": \"fetch_url\",\n  \"parameters\": {\"url\": \"https://spicebox.dev\"}\n}\n```"

	_, toolCalls, ok := NormalizeMarkdownToolCalls(raw)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if len(toolCalls) != 1 {
		t.Fatalf("Expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].Function.Name != "fetch_url" {
		t.Errorf("Expected 'fetch_url', got '%s'", toolCalls[0].Function.Name)
	}
	if !strings.Contains(toolCalls[0].Function.Arguments, "spicebox.dev") {
		t.Errorf("Expected parameters mapped to arguments containing 'spicebox.dev', got '%s'", toolCalls[0].Function.Arguments)
	}
}

func TestNormalizeMarkdownToolCalls_NestedComplexArgs(t *testing.T) {
	raw := "```json\n{\n  \"name\": \"batch_edit\",\n  \"arguments\": {\n    \"files\": [\"a.go\", \"b.go\"],\n    \"options\": {\"dry_run\": true, \"flags\": [1, 2, 3]}\n  }\n}\n```"

	_, toolCalls, ok := NormalizeMarkdownToolCalls(raw)
	if !ok {
		t.Fatalf("Expected ok to be true, got false")
	}
	if len(toolCalls) != 1 {
		t.Fatalf("Expected 1 tool call, got %d", len(toolCalls))
	}
	if !strings.Contains(toolCalls[0].Function.Arguments, "dry_run") || !strings.Contains(toolCalls[0].Function.Arguments, "a.go") {
		t.Errorf("Expected nested arguments preserved, got '%s'", toolCalls[0].Function.Arguments)
	}
}

func TestNormalizeMarkdownToolCalls_MalformedJSON_GracefulFallback(t *testing.T) {
	raw := "Here is some broken code:\n```json\n{ this is not valid json : 123 }\n```"

	cleanedText, toolCalls, ok := NormalizeMarkdownToolCalls(raw)
	if ok {
		t.Errorf("Expected ok to be false for malformed JSON, got true")
	}
	if len(toolCalls) > 0 {
		t.Errorf("Expected 0 tool calls, got %d", len(toolCalls))
	}
	if cleanedText != raw {
		t.Errorf("Expected unmodified raw text, got '%s'", cleanedText)
	}
}

func TestNormalizeMarkdownToolCalls_NonToolJSON_Ignored(t *testing.T) {
	raw := "Here is a data payload example:\n```json\n{\n  \"status\": 200,\n  \"user_count\": 42,\n  \"active\": true\n}\n```"

	cleanedText, toolCalls, ok := NormalizeMarkdownToolCalls(raw)
	if ok {
		t.Errorf("Expected ok to be false for standard non-tool JSON data, got true")
	}
	if len(toolCalls) > 0 {
		t.Errorf("Expected 0 tool calls, got %d", len(toolCalls))
	}
	if cleanedText != raw {
		t.Errorf("Expected text to be unmodified, got: %s", cleanedText)
	}
}

func TestNormalizeMarkdownToolCalls_PureProse_ReturnsFalse(t *testing.T) {
	raw := "This is a regular explanation of how Go channels work with mutexes."

	cleanedText, toolCalls, ok := NormalizeMarkdownToolCalls(raw)
	if ok {
		t.Errorf("Expected ok to be false for pure prose, got true")
	}
	if len(toolCalls) > 0 {
		t.Errorf("Expected 0 tool calls, got %d", len(toolCalls))
	}
	if cleanedText != raw {
		t.Errorf("Expected text to be unmodified, got: %s", cleanedText)
	}
}
