// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package agentregistry

import (
	"encoding/json"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/dixieflatline76/nacho-flow/data"
)

func TestDefaultRegistry_EmbeddedCatalog(t *testing.T) {
	reg := DefaultRegistry()
	if reg == nil {
		t.Fatal("expected DefaultRegistry() to return non-nil registry")
	}

	// Verify write tools across all agent profiles are loaded
	expectedWriteTools := []string{
		"editor", "write_to_file", "replace_in_file",
		"replace_file_content", "apply_diff", "edit_file",
		"create_file", "reapply", "insert_content", "modify_file",
		"write_file", "patch_file", "str_replace_editor", "text_editor",
		"save_file", "create_or_update_file", "put_file",
	}
	for _, tool := range expectedWriteTools {
		if !reg.IsWriteTool(tool) {
			t.Errorf("expected %q to be recognized as a write tool", tool)
		}
	}

	// Verify heuristic tools
	if !reg.IsWriteTool("custom_code_editor") {
		t.Errorf("expected custom_code_editor to be recognized via heuristic")
	}
	if !reg.IsWriteTool("edit_custom") {
		t.Errorf("expected edit_custom to be recognized via heuristic")
	}
	if reg.IsWriteTool("read_file") {
		t.Errorf("did not expect read_file to be recognized as write tool")
	}
	if reg.IsWriteTool("") {
		t.Errorf("did not expect empty string to be recognized as write tool")
	}

	// Verify known tools (read, command, write)
	if !reg.IsKnownTool("read_file") {
		t.Errorf("expected read_file to be a known tool")
	}
	if !reg.IsKnownTool("bash") {
		t.Errorf("expected bash to be a known tool")
	}
	if !reg.IsKnownTool("editor") {
		t.Errorf("expected editor to be a known tool")
	}
	if reg.IsKnownTool("unknown_random_tool") {
		t.Errorf("did not expect unknown_random_tool to be known")
	}
	if reg.IsKnownTool("") {
		t.Errorf("did not expect empty string to be known")
	}

	// Verify TagReplacer and ReasoningByteMarkers
	if reg.TagReplacer() == nil {
		t.Fatal("expected TagReplacer to be non-nil")
	}
	if replaced := reg.TagReplacer().Replace("<|channel|>thought"); replaced != "<think>" {
		t.Errorf("expected <|channel|>thought to be canonicalized to <think>, got %q", replaced)
	}
	if replaced := reg.TagReplacer().Replace("</thinking>"); replaced != "</think>" {
		t.Errorf("expected </thinking> to be canonicalized to </think>, got %q", replaced)
	}
	if replaced := reg.TagReplacer().Replace("<start_of_turn>"); replaced != "" {
		t.Errorf("expected <start_of_turn> to be stripped, got %q", replaced)
	}

	markers := reg.ReasoningByteMarkers()
	if len(markers) < 20 {
		t.Errorf("expected at least 20 reasoning byte markers, got %d", len(markers))
	}
}

func TestRegistry_DetectShellWrite(t *testing.T) {
	reg := DefaultRegistry()

	tests := []struct {
		cmd      string
		expected bool
	}{
		{"", false},
		{"   ", false},
		{"echo 'hello world'", false},
		{"ls -la", false},
		{"cat README.md", false},
		{"grep -rn 'foo' .", false},
		{"echo 'hi' > output.txt", true},
		{"echo 'hi' >> output.txt", true},
		{"echo 'test' | tee log.txt", true},
		{"echo 'test' |tee log.txt", true},
		{"cat src | dd of=dest.bin", true},
		{"cat src | out-file dest.txt", true},
		{"cat src | set-content dest.txt", true},
		{"cat src | add-content dest.txt", true},
		{"sed -i 's/foo/bar/g' main.go", true},
		{"sed 's/foo/bar/g' main.go", false},
		{"touch file.txt", true},
		{"mkdir -p /tmp/test", true},
		{"rm -rf /tmp/test", true},
		{"cp a.txt b.txt", true},
		{"mv a.txt b.txt", true},
		{"git checkout -- file.go", true},
		{"git restore file.go", true},
		{"git apply patch.diff", true},
		{"git status", false},
		{"git diff", false},
		{"go mod init mymod", true},
		{"go mod tidy", true},
		{"go test ./...", false},
		{"npm init -y", true},
		{"npm test", false},
		{"cargo new myapp", true},
		{"cargo test", false},
		{"New-Item -ItemType File test.txt", true},
		{"Copy-Item a.txt b.txt", true},
		{"echo 'command with > /dev/null' > /dev/null", false},
		{"echo 'command with > &1' > &1", false},
		{"echo 'a > b in string'", false},
		{"echo \"a > b in double quotes\"", false},
		{"cat << 'EOF' > generated.go\npackage main\nEOF", true},
		{"cat << 'EOF'\njust reading\nEOF", false},
		{"echo \\> text", false},
		{"cmd &> log.txt", false},
		{"cat <(echo hi)", false},
		{"cmd >&2", false},
		{"if [ $x >= 5 ]; then echo ok; fi", false},
		{"echo 'hi' >", false},
		{"cmd\r\ntouch file.txt", true},
		{"echo 'test'\r\nrm file.txt", true},
		{"git status\r\nls -la", false},
	}

	for _, tt := range tests {
		got := reg.DetectShellWrite(tt.cmd)
		if got != tt.expected {
			t.Errorf("DetectShellWrite(%q) = %v, expected %v", tt.cmd, got, tt.expected)
		}
	}
}

func TestRegistry_Reload(t *testing.T) {
	mockFS := fstest.MapFS{
		"agents/manifest.json": &fstest.MapFile{
			Data: []byte(`{"version":"1.0.0","agents":{"test":"1.0.0"},"shell":"1.0.0"}`),
		},
		"agents/test.json": &fstest.MapFile{
			Data: []byte(`{"id":"test","name":"Test Agent","version":"1.0.0","write_tools":["test_write"],"read_tools":["test_read"],"command_tools":["test_cmd"]}`),
		},
		"agents/README.md": &fstest.MapFile{
			Data: []byte(`Ignored non-json file`),
		},
		"agents/subdir/nested.json": &fstest.MapFile{
			Data: []byte(`Ignored directory entry`),
		},
		"shell.json": &fstest.MapFile{
			Data: []byte(`{"version":"1.0.0","unix":{"write_commands":["test_touch"],"write_pipes":["| test_tee"],"contextual_commands":{"test_sed":["-i"]},"redirections":[">"]},"windows":{"write_commands":[],"write_pipes":[],"contextual_commands":{},"redirections":[]}}`),
		},
		"reasoning.json": &fstest.MapFile{
			Data: []byte(`{"version":"1.0.0","canonical_open":{"<think>":["<test_think>"]},"canonical_close":{"</think>":["</test_think>"]},"strip_delimiters":["<test_strip>"],"reasoning_fields":["reasoning_content"],"extra_byte_markers":["extra_test"]}`),
		},
	}

	reg, err := NewRegistryFromFS(mockFS)
	if err != nil {
		t.Fatalf("unexpected error creating registry: %v", err)
	}

	if !reg.IsWriteTool("test_write") {
		t.Errorf("expected test_write to be a write tool")
	}
	if !reg.IsKnownTool("test_read") {
		t.Errorf("expected test_read to be known")
	}
	if !reg.DetectShellWrite("test_touch file.txt") {
		t.Errorf("expected test_touch to trigger shell write")
	}
	if reg.TagReplacer().Replace("<test_think>") != "<think>" {
		t.Errorf("expected <test_think> to be canonicalized")
	}

	// Reload with updated agent
	updatedFS := fstest.MapFS{
		"agents/manifest.json": &fstest.MapFile{
			Data: []byte(`{"version":"1.0.1","agents":{"test":"1.0.1"},"shell":"1.0.1","reasoning":"1.0.1"}`),
		},
		"agents/test.json": &fstest.MapFile{
			Data: []byte(`{"id":"test","name":"Test Agent","version":"1.0.1","write_tools":["new_write"],"read_tools":["new_read"],"command_tools":["new_cmd"]}`),
		},
		"shell.json": &fstest.MapFile{
			Data: []byte(`{"version":"1.0.1","unix":{"write_commands":["new_touch"],"write_pipes":[],"contextual_commands":{},"redirections":[]},"windows":{"write_commands":[],"write_pipes":[],"contextual_commands":{},"redirections":[]}}`),
		},
		"reasoning.json": &fstest.MapFile{
			Data: []byte(`{"version":"1.0.1","canonical_open":{"<think>":["<new_think>"]},"canonical_close":{"</think>":["</new_think>"]},"strip_delimiters":[],"reasoning_fields":["reasoning_content"],"extra_byte_markers":[]}`),
		},
	}

	if err := reg.Reload(updatedFS); err != nil {
		t.Fatalf("unexpected reload error: %v", err)
	}

	if !reg.IsWriteTool("new_write") {
		t.Errorf("expected new_write to be write tool after reload")
	}
	if !reg.DetectShellWrite("new_touch file.txt") {
		t.Errorf("expected new_touch to trigger shell write after reload")
	}
	if reg.TagReplacer().Replace("<new_think>") != "<think>" {
		t.Errorf("expected <new_think> to be canonicalized after reload")
	}
}

func TestRegistry_ErrorHandling(t *testing.T) {
	// Missing agents directory
	badFS1 := fstest.MapFS{
		"shell.json":     &fstest.MapFile{Data: []byte(`{}`)},
		"reasoning.json": &fstest.MapFile{Data: []byte(`{}`)},
	}
	if _, err := NewRegistryFromFS(badFS1); err == nil {
		t.Errorf("expected error for missing agents dir")
	}

	// Invalid manifest JSON
	badFS2 := fstest.MapFS{
		"agents/manifest.json": &fstest.MapFile{Data: []byte(`invalid json`)},
		"shell.json":           &fstest.MapFile{Data: []byte(`{}`)},
		"reasoning.json":       &fstest.MapFile{Data: []byte(`{}`)},
	}
	if _, err := NewRegistryFromFS(badFS2); err == nil {
		t.Errorf("expected error for invalid manifest JSON")
	}

	// Invalid agent JSON
	badFS3 := fstest.MapFS{
		"agents/manifest.json": &fstest.MapFile{Data: []byte(`{"version":"1.0.0"}`)},
		"agents/broken.json":   &fstest.MapFile{Data: []byte(`{invalid agent}`)},
		"shell.json":           &fstest.MapFile{Data: []byte(`{}`)},
		"reasoning.json":       &fstest.MapFile{Data: []byte(`{}`)},
	}
	if _, err := NewRegistryFromFS(badFS3); err == nil {
		t.Errorf("expected error for broken agent JSON")
	}

	// Missing shell.json
	badFS4 := fstest.MapFS{
		"agents/manifest.json": &fstest.MapFile{Data: []byte(`{"version":"1.0.0"}`)},
		"reasoning.json":       &fstest.MapFile{Data: []byte(`{}`)},
	}
	if _, err := NewRegistryFromFS(badFS4); err == nil {
		t.Errorf("expected error for missing shell.json")
	}

	// Invalid shell.json
	badFS5 := fstest.MapFS{
		"agents/manifest.json": &fstest.MapFile{Data: []byte(`{"version":"1.0.0"}`)},
		"shell.json":           &fstest.MapFile{Data: []byte(`{invalid shell}`)},
		"reasoning.json":       &fstest.MapFile{Data: []byte(`{}`)},
	}
	if _, err := NewRegistryFromFS(badFS5); err == nil {
		t.Errorf("expected error for invalid shell.json")
	}

	// Missing reasoning.json
	badFS6 := fstest.MapFS{
		"agents/manifest.json": &fstest.MapFile{Data: []byte(`{"version":"1.0.0"}`)},
		"shell.json":           &fstest.MapFile{Data: []byte(`{}`)},
	}
	if _, err := NewRegistryFromFS(badFS6); err == nil {
		t.Errorf("expected error for missing reasoning.json")
	}

	// Invalid reasoning.json
	badFS7 := fstest.MapFS{
		"agents/manifest.json": &fstest.MapFile{Data: []byte(`{"version":"1.0.0"}`)},
		"shell.json":           &fstest.MapFile{Data: []byte(`{}`)},
		"reasoning.json":       &fstest.MapFile{Data: []byte(`{invalid reasoning}`)},
	}
	if _, err := NewRegistryFromFS(badFS7); err == nil {
		t.Errorf("expected error for invalid reasoning.json")
	}

	// Reload with error
	reg := DefaultRegistry()
	if err := reg.Reload(badFS1); err == nil {
		t.Errorf("expected reload to fail with invalid FS")
	}
}

func BenchmarkRegistry_IsWriteTool(b *testing.B) {
	reg := DefaultRegistry()
	tool := "write_to_file"
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = reg.IsWriteTool(tool)
	}
}

func BenchmarkRegistry_DetectShellWrite(b *testing.B) {
	reg := DefaultRegistry()
	cmd := "echo 'package main' > main.go"
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = reg.DetectShellWrite(cmd)
	}
}

func BenchmarkRegistry_WriteToolsList(b *testing.B) {
	reg := DefaultRegistry()
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = reg.WriteToolsList()
	}
}

func TestRegistry_InteractiveToolsAndShieldHeuristics(t *testing.T) {
	reg := DefaultRegistry()

	// 1. Verify IsInteractiveTool
	interactive := []string{
		"ask_followup_question", "ask_question", "switch_mode", "user_prompt",
		"Ask_Followup_Question", " ASK_QUESTION ",
	}
	for _, tool := range interactive {
		if !reg.IsInteractiveTool(tool) {
			t.Errorf("expected %q to be recognized as interactive tool", tool)
		}
	}

	nonInteractive := []string{"read_file", "write_to_file", "bash", "unknown_tool", "", "   "}
	for _, tool := range nonInteractive {
		if reg.IsInteractiveTool(tool) {
			t.Errorf("did not expect %q to be recognized as interactive tool", tool)
		}
	}

	// 2. Verify List getters return non-empty slices
	if list := reg.InteractiveToolsList(); len(list) == 0 {
		t.Error("expected non-empty InteractiveToolsList")
	}
	if list := reg.ModeHeuristicsList(); len(list) == 0 {
		t.Error("expected non-empty ModeHeuristicsList")
	}
	if list := reg.QuestionHeuristicsList(); len(list) == 0 {
		t.Error("expected non-empty QuestionHeuristicsList")
	}
	if list := reg.ErrorSignaturesList(); len(list) == 0 {
		t.Error("expected non-empty ErrorSignaturesList")
	}

	// 3. Nil receiver safety
	var nilReg *Registry
	if nilReg.IsInteractiveTool("ask_question") {
		t.Error("expected nil registry to return false for IsInteractiveTool")
	}
	if nilReg.IsWriteTool("write_to_file") {
		t.Error("expected nil registry to return false for IsWriteTool")
	}
	if nilReg.IsKnownTool("read_file") {
		t.Error("expected nil registry to return false for IsKnownTool")
	}
	if nilReg.DetectShellWrite("touch file.txt") {
		t.Error("expected nil registry to return false for DetectShellWrite")
	}
	if nilReg.WriteToolsList() != nil {
		t.Error("expected nil registry to return nil for WriteToolsList")
	}
	if nilReg.WriteTagByteMarkers() != nil {
		t.Error("expected nil registry to return nil for WriteTagByteMarkers")
	}
	if nilReg.TagReplacer() != nil {
		t.Error("expected nil registry to return nil for TagReplacer")
	}
	if nilReg.ReasoningByteMarkers() != nil {
		t.Error("expected nil registry to return nil for ReasoningByteMarkers")
	}
	if nilReg.InteractiveToolsList() != nil {
		t.Error("expected nil registry to return nil for InteractiveToolsList")
	}
	if nilReg.ModeHeuristicsList() != nil {
		t.Error("expected nil registry to return nil for ModeHeuristicsList")
	}
	if nilReg.QuestionHeuristicsList() != nil {
		t.Error("expected nil registry to return nil for QuestionHeuristicsList")
	}
	if nilReg.ErrorSignaturesList() != nil {
		t.Error("expected nil registry to return nil for ErrorSignaturesList")
	}
	if nilReg.IsFileReadTool("read_file") {
		t.Error("expected nil registry to return false for IsFileReadTool")
	}
	if nilReg.FileReadToolsList() != nil {
		t.Error("expected nil registry to return nil for FileReadToolsList")
	}
	if nilReg.IsToolError("error: something failed", false) {
		t.Error("expected nil registry to return false for IsToolError when isError is false")
	}
	if !nilReg.IsToolError("error: something failed", true) {
		t.Error("expected nil registry to return true for IsToolError when isError is true")
	}
}

func TestRegistry_FileReadToolsAndIsToolError(t *testing.T) {
	reg := DefaultRegistry()

	// 1. FileReadTools tests
	readTools := []string{"read_file", "read_files", "view", "view_file", "cat", "open_file"}
	for _, tool := range readTools {
		if !reg.IsFileReadTool(tool) {
			t.Errorf("expected %s to be recognized as a file read tool", tool)
		}
		if !reg.IsFileReadTool("  " + strings.ToUpper(tool) + "  ") {
			t.Errorf("expected whitespace/cased %s to be recognized as a file read tool", tool)
		}
	}

	// Dynamic fallback
	if !reg.IsFileReadTool("custom_read_file") {
		t.Error("expected custom_read_file to be recognized via fallback")
	}

	// Non-file-read tools
	nonReadTools := []string{"list_files", "search_files", "read_dir", "find_files", "bash", "write_to_file", "", "   "}
	for _, tool := range nonReadTools {
		if reg.IsFileReadTool(tool) {
			t.Errorf("expected %q NOT to be recognized as a file read tool", tool)
		}
	}

	if list := reg.FileReadToolsList(); len(list) == 0 {
		t.Error("expected non-empty FileReadToolsList")
	}

	// 2. IsToolError tests
	// Explicit boolean error
	if !reg.IsToolError("", true) {
		t.Error("expected true when isError is true")
	}
	if !reg.IsToolError("all good", true) {
		t.Error("expected true when isError is true regardless of content")
	}

	// Catalog error signatures
	catalogErrors := []string{
		"Editor operation failed",
		"Unable to apply diff to file: syntax error",
		"Parameter 'old_str' is required",
		"File not found",
		"Missing value for required parameter",
		"The tool execution failed",
		"<error_details>something broke</error_details>",
	}
	for _, errStr := range catalogErrors {
		if !reg.IsToolError(errStr, false) {
			t.Errorf("expected catalog error %q to be recognized", errStr)
		}
	}

	// Generic error prefixes & substrings
	genericErrors := []string{
		"error: syntax error at line 5",
		"fatal: out of memory",
		"[syntax error] missing semicolon",
		"exit code: 1",
		"command execution was not successful",
		"not recognized as an internal or external command",
		"the process was terminated because not found",
		"unable to apply diff to target",
	}
	for _, errStr := range genericErrors {
		if !reg.IsToolError(errStr, false) {
			t.Errorf("expected generic error %q to be recognized", errStr)
		}
	}

	// Non-errors
	nonErrors := []string{
		"File saved successfully",
		"Compilation succeeded in 1.2s",
		"Reading 24 lines from file.go",
		"",
		"   ",
	}
	for _, okStr := range nonErrors {
		if reg.IsToolError(okStr, false) {
			t.Errorf("expected %q NOT to be recognized as tool error", okStr)
		}
	}
}

func BenchmarkRegistry_IsInteractiveTool(b *testing.B) {
	reg := DefaultRegistry()
	tool := "ask_followup_question"
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = reg.IsInteractiveTool(tool)
	}
}

func BenchmarkRegistry_IsFileReadTool(b *testing.B) {
	reg := DefaultRegistry()
	tool := "read_file"
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = reg.IsFileReadTool(tool)
	}
}

func BenchmarkRegistry_IsToolError(b *testing.B) {
	reg := DefaultRegistry()
	content := "Editor operation failed: file locked"
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = reg.IsToolError(content, false)
	}
}

func TestAgentRegistry_AllToolProfilesContract(t *testing.T) {
	reg := DefaultRegistry()

	entries, err := fs.ReadDir(data.CatalogFS, "agents")
	if err != nil {
		t.Fatalf("failed to read agents dir: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || entry.Name() == "manifest.json" {
			continue
		}

		raw, err := fs.ReadFile(data.CatalogFS, "agents/"+entry.Name())
		if err != nil {
			t.Fatalf("failed to read agent profile %s: %v", entry.Name(), err)
		}

		var profile AgentProfile
		if err := json.Unmarshal(raw, &profile); err != nil {
			t.Fatalf("failed to parse agent profile %s: %v", entry.Name(), err)
		}

		t.Run(profile.ID+"/FileReadTools", func(t *testing.T) {
			for _, tool := range profile.FileReadTools {
				if !reg.IsFileReadTool(tool) {
					t.Errorf("agent %s: expected %q to be recognized as file read tool", profile.ID, tool)
				}
				if reg.IsWriteTool(tool) {
					t.Errorf("agent %s: tool %q is a read tool, must NOT be recognized as write tool", profile.ID, tool)
				}
			}
		})

		t.Run(profile.ID+"/FileWriteTools", func(t *testing.T) {
			for _, tool := range profile.WriteTools {
				if !reg.IsWriteTool(tool) {
					t.Errorf("agent %s: expected %q to be recognized as write tool", profile.ID, tool)
				}
				if reg.IsFileReadTool(tool) {
					t.Errorf("agent %s: tool %q is a write tool, must NOT be recognized as read tool", profile.ID, tool)
				}
			}
		})

		t.Run(profile.ID+"/ErrorSignatures", func(t *testing.T) {
			for _, sig := range profile.ErrorSignatures {
				content := "Error context prefix: " + sig + " suffix"
				if !reg.IsToolError(content, false) {
					t.Errorf("agent %s: expected error signature %q to be detected", profile.ID, sig)
				}
			}
		})
	}
}
