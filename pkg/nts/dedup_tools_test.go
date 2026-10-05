package nts_test

import (
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/nts"
)

func TestToolDedup_CategoryGeneric_DuplicateReplaced(t *testing.T) {
	ctx := nts.NewToolDedupContext(200)

	output := strings.Repeat("git status: working tree clean; nothing to commit.\n", 10)
	if len(output) < 200 {
		t.Fatalf("expected output len >= 200, got %d", len(output))
	}

	// First time: should NOT be modified
	res1, red1, changed1 := ctx.Deduplicate(output, "execute_command", false)
	if changed1 {
		t.Fatalf("expected first execution to not be changed")
	}
	if res1 != output {
		t.Fatalf("expected unchanged output on first execution")
	}
	if red1.BytesSaved != 0 {
		t.Fatalf("expected 0 bytes saved on first execution")
	}

	// Second time: SHOULD be replaced with reference
	res2, red2, changed2 := ctx.Deduplicate(output, "execute_command", false)
	if !changed2 {
		t.Fatalf("expected second execution to be replaced as duplicate")
	}
	res2Str, ok := res2.(string)
	if !ok {
		t.Fatalf("expected string result")
	}
	if !strings.Contains(res2Str, "[Output identical to previous command execution above") {
		t.Fatalf("unexpected replacement string: %s", res2Str)
	}
	if red2.BytesSaved <= 0 {
		t.Fatalf("expected positive bytes saved, got %d", red2.BytesSaved)
	}
}

func TestToolDedup_CategoryFileRead_Immune(t *testing.T) {
	ctx := nts.NewToolDedupContext(200)

	content := strings.Repeat("package main\n\nfunc main() {}\n", 20)

	// First read
	_, _, changed1 := ctx.Deduplicate(content, "read_file", false)
	if changed1 {
		t.Fatalf("first read should not be changed")
	}

	// Second read: MUST BE IMMUNE (file-read lane handles stale reads, not command output dedup)
	res2, _, changed2 := ctx.Deduplicate(content, "read_file", false)
	if changed2 {
		t.Fatalf("file read tool MUST be 100%% immune to generic tool output deduplication")
	}
	if res2 != content {
		t.Fatalf("expected content to remain identical")
	}
}

func TestToolDedup_CategoryFileWrite_Immune(t *testing.T) {
	ctx := nts.NewToolDedupContext(200)

	content := strings.Repeat("File successfully written to disk.\n", 10)

	ctx.Deduplicate(content, "write_to_file", false)
	res2, _, changed2 := ctx.Deduplicate(content, "write_to_file", false)
	if changed2 {
		t.Fatalf("file write tool MUST be 100%% immune to generic tool output deduplication")
	}
	if res2 != content {
		t.Fatalf("expected content to remain identical")
	}
}

func TestToolDedup_BelowMinBytes_Skipped(t *testing.T) {
	ctx := nts.NewToolDedupContext(200)

	shortOutput := "Directory created successfully."

	ctx.Deduplicate(shortOutput, "bash", false)
	res2, _, changed2 := ctx.Deduplicate(shortOutput, "bash", false)
	if changed2 {
		t.Fatalf("short outputs (< 200 bytes) should not be deduplicated")
	}
	if res2 != shortOutput {
		t.Fatalf("expected short output to remain identical")
	}
}

func TestToolDedup_ToolError_Immune(t *testing.T) {
	ctx := nts.NewToolDedupContext(200)

	errorOutput := "Error: command failed with exit status 1\n" + strings.Repeat("Traceback (most recent call last):\n  File 'app.py', line 10\n", 5)

	ctx.Deduplicate(errorOutput, "bash", true)
	res2, _, changed2 := ctx.Deduplicate(errorOutput, "bash", true)
	if changed2 {
		t.Fatalf("tool errors MUST be immune from deduplication")
	}
	if res2 != errorOutput {
		t.Fatalf("expected error output to remain identical")
	}
}

func TestToolDedup_ContentBlocks_Handled(t *testing.T) {
	ctx := nts.NewToolDedupContext(200)

	output := strings.Repeat("Build succeeded in 1.42s.\n", 15)

	block1 := []interface{}{
		map[string]interface{}{
			"type": "text",
			"text": output,
		},
	}
	block2 := []interface{}{
		map[string]interface{}{
			"type": "text",
			"text": output,
		},
	}

	_, _, changed1 := ctx.Deduplicate(block1, "bash", false)
	if changed1 {
		t.Fatalf("first execution should not be changed")
	}

	res2, red2, changed2 := ctx.Deduplicate(block2, "bash", false)
	if !changed2 {
		t.Fatalf("expected second block execution to be deduplicated")
	}
	if red2.BytesSaved <= 0 {
		t.Fatalf("expected bytes saved > 0")
	}

	block2Slice := res2.([]interface{})
	part := block2Slice[0].(map[string]interface{})
	if !strings.Contains(part["text"].(string), "[Output identical to previous command execution above") {
		t.Fatalf("unexpected block text: %v", part["text"])
	}
}

func TestToolDedup_NilContext_Bypass(t *testing.T) {
	var ctx *nts.ToolDedupContext

	output := strings.Repeat("Hello world\n", 50)
	res, _, changed := ctx.Deduplicate(output, "bash", false)
	if changed || res != output {
		t.Fatalf("nil context should return unmodified content")
	}
}

func TestToolDedup_EdgeCases(t *testing.T) {
	// Default min bytes
	ctx := nts.NewToolDedupContext(0)

	// Empty tool name
	res, _, changed := ctx.Deduplicate("some string", "", false)
	if changed || res != "some string" {
		t.Fatalf("empty tool name should bypass")
	}

	// Unsupported content type
	res2, _, changed2 := ctx.Deduplicate(12345, "bash", false)
	if changed2 || res2 != 12345 {
		t.Fatalf("unsupported content type should bypass")
	}

	// Multiple blocks in list, not text
	mixedBlocks := []interface{}{
		map[string]interface{}{"type": "image"},
	}
	_, _, changed3 := ctx.Deduplicate(mixedBlocks, "bash", false)
	if changed3 {
		t.Fatalf("non-text blocks should bypass")
	}
}

