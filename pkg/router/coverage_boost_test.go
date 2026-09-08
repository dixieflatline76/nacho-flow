package router

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestClassifier_XMLCommandExtraction_ShellWrite(t *testing.T) {
	classifier := NewClassifier().(*RequestClassifier)

	// Cline-style assistant message using XML execution tag followed by user confirmation
	body := []byte(`{
		"model": "claude-3-5-sonnet",
		"messages": [
			{"role": "user", "content": "Fix the bug in main.go"},
			{"role": "assistant", "content": "<execute_command><command>sed -i 's/foo/bar/g' main.go</command></execute_command>"},
			{"role": "user", "content": "Command executed successfully"}
		]
	}`)

	reqCtx, err := classifier.Classify(body)
	if err != nil {
		t.Fatalf("Classify failed: %v", err)
	}
	if !reqCtx.HasShellWrite {
		t.Errorf("Expected HasShellWrite=true for XML sed command, got false")
	}
	if !reqCtx.HasToolProgress {
		t.Errorf("Expected HasToolProgress=true for Cline user response, got false")
	}

	// Test alternative XML tags: <CommandLine>, <cmd>, <script> inside <run_command>, <bash>, <terminal>
	tests := []struct {
		tag       string
		childTag  string
		cmd       string
		expectHit bool
	}{
		{"run_command", "CommandLine", "touch newfile.txt", true},
		{"bash", "cmd", "rm -rf /tmp/scratch", true},
		{"terminal", "script", "mv old.txt new.txt", true},
		{"execute_command", "command", "cat main.go", false}, // read only
	}

	for _, tc := range tests {
		xml := "<" + tc.tag + "><" + tc.childTag + ">" + tc.cmd + "</" + tc.childTag + "></" + tc.tag + ">"
		res := extractXMLCommand(xml)
		if res != tc.cmd {
			t.Errorf("extractXMLCommand(%q) = %q, expected %q", xml, res, tc.cmd)
		}
	}
}

func TestExtractSupportedInteractiveTool(t *testing.T) {
	// Non-map tools
	toolsWithInvalid := []interface{}{
		"not a map",
		123,
		nil,
	}
	if res := ExtractSupportedInteractiveTool(toolsWithInvalid); res != "" {
		t.Errorf("Expected empty string for invalid tool elements, got %q", res)
	}

	// Various supported names
	cases := []struct {
		tool     interface{}
		expected string
	}{
		{map[string]interface{}{"function": map[string]interface{}{"name": "ask_question"}}, "ask_question"},
		{map[string]interface{}{"function": map[string]interface{}{"name": "user_prompt"}}, "user_prompt"},
		{map[string]interface{}{"function": map[string]interface{}{"name": "interactive_input"}}, "interactive_input"},
		{map[string]interface{}{"name": "ask_followup_question"}, "ask_followup_question"},
		{map[string]interface{}{"function": map[string]interface{}{"name": "read_file"}}, ""},
	}

	for _, tc := range cases {
		res := ExtractSupportedInteractiveTool([]interface{}{tc.tool})
		if res != tc.expected {
			t.Errorf("ExtractSupportedInteractiveTool(%v) = %q, expected %q", tc.tool, res, tc.expected)
		}
	}

	if res := ExtractSupportedInteractiveTool(nil); res != "" {
		t.Errorf("Expected empty string for nil tools, got %q", res)
	}
}

func TestExtractCommandFromRaw_EdgeCases(t *testing.T) {
	if res := extractCommandFromRaw(nil); res != "" {
		t.Errorf("Expected empty for nil raw, got %q", res)
	}
	if res := extractCommandFromRaw(json.RawMessage("   ")); res != "" {
		t.Errorf("Expected empty for whitespace, got %q", res)
	}

	// Quoted JSON string of JSON
	quoted := json.RawMessage(`"{\"cmd\": \"dir\"}"`)
	if res := extractCommandFromRaw(quoted); res != "dir" {
		t.Errorf("Expected 'dir' from quoted JSON, got %q", res)
	}

	// Non-JSON or array
	if res := extractCommandFromRaw(json.RawMessage(`"just a string"`)); res != "" {
		t.Errorf("Expected empty for non-object, got %q", res)
	}
	if res := extractCommandFromRaw(json.RawMessage(`[1, 2, 3]`)); res != "" {
		t.Errorf("Expected empty for array, got %q", res)
	}

	// Various field names
	if res := extractCommandFromRaw(json.RawMessage(`{"CommandLine": "go test"}`)); res != "go test" {
		t.Errorf("Expected 'go test', got %q", res)
	}
	if res := extractCommandFromRaw(json.RawMessage(`{"script": "npm test"}`)); res != "npm test" {
		t.Errorf("Expected 'npm test', got %q", res)
	}
	if res := extractCommandFromRaw(json.RawMessage(`{"other": "data"}`)); res != "" {
		t.Errorf("Expected empty for unknown field, got %q", res)
	}
}

func TestDetectShellWrite_DetailedEdges(t *testing.T) {
	cases := []struct {
		cmd      string
		expected bool
	}{
		{"", false},
		{"   ", false},
		{"diff <(cat a.txt) <(cat b.txt)", false}, // process substitution
		{"cmd &> /dev/null", false},               // descriptor redirect
		{"echo error >&2", false},                 // redirect to stderr
		{"echo log >&1", false},                   // redirect to stdout
		{"count >= 10", false},                    // >= operator
		{"echo append >> output.log", true},       // >> append
		{"echo null > /dev/zero", false},          // dev zero
		{"ps > $null", false},                     // powershell null
		{"ps | out-file output.txt", true},        // pipe to out-file
		{"Get-Process | set-content p.txt", true}, // pipe to set-content
		{"Get-Process | add-content p.txt", true}, // pipe to add-content
		{"cat data | dd of=out.bin", true},        // pipe to dd
		{"cat << 'EOF' > generated.py", true},     // heredoc with redirect
		{"cat << 'EOF' | tee generated.py", true}, // heredoc with tee
		{"cat << 'EOF'\njust text\nEOF", false},   // bare heredoc
		{"git checkout -- file.go", true},
		{"git restore file.go", true},
		{"git restore\tfile.go", true},
		{"git apply patch.diff", true},
		{"git apply\tpatch.diff", true},
		{"tar -xvf archive.tar", true},
		{"tar xvf archive.tar", true},
		{"copy-item a b", true},
		{"move-item a b", true},
		{"remove-item a", true},
		{"rename file1.txt file2.txt", true},
		{"rd /s /q temp", true},
		{"truncate -s 0 log.txt", true},
		{"install -m 755 bin /usr/local/bin", true},
		{"unzip package.zip", true},
		{"gunzip file.gz", true},
	}

	for _, tc := range cases {
		res := detectShellWrite(tc.cmd)
		if res != tc.expected {
			t.Errorf("detectShellWrite(%q) = %v, expected %v", tc.cmd, res, tc.expected)
		}
	}
}

func TestClassifyPayload_LegacyAndUntypedToolFormats(t *testing.T) {
	classifier := NewClassifier().(*RequestClassifier)

	// 1. Legacy OpenAI FunctionCall format
	bodyLegacy := []byte(`{
		"model": "gpt-4",
		"messages": [
			{"role": "user", "content": "Run tests"},
			{
				"role": "assistant",
				"function_call": {
					"name": "bash",
					"arguments": "{\"command\": \"sed -i 's/x/y/g' file.txt\"}"
				}
			},
			{
				"role": "tool",
				"content": "ok"
			}
		]
	}`)
	reqCtx, err := classifier.Classify(bodyLegacy)
	if err != nil {
		t.Fatalf("Classify legacy failed: %v", err)
	}
	if !reqCtx.HasShellWrite {
		t.Errorf("Expected HasShellWrite=true for legacy function_call, got false")
	}

	// 2. Direct Arguments on tool_calls instead of Function.Arguments
	bodyDirectArgs := []byte(`{
		"model": "gpt-4",
		"messages": [
			{"role": "user", "content": "Update file"},
			{
				"role": "assistant",
				"tool_calls": [
					{
						"id": "tc-1",
						"type": "function",
						"name": "execute_command",
						"arguments": "{\"command\": \"touch created.txt\"}"
					}
				]
			},
			{
				"role": "tool",
				"tool_call_id": "tc-1",
				"content": "File created"
			}
		]
	}`)
	reqCtx2, err := classifier.Classify(bodyDirectArgs)
	if err != nil {
		t.Fatalf("Classify direct args failed: %v", err)
	}
	if !reqCtx2.HasShellWrite {
		t.Errorf("Expected HasShellWrite=true for direct tool_calls arguments, got false")
	}

	// 3. Tool response without tool_call_id when hasAnyWriteCall is set
	classifier.SetKickstartWriteTools([]string{"write_to_file", "replace_in_file"})
	bodyNoToolID := []byte(`{
		"model": "gpt-4",
		"messages": [
			{"role": "user", "content": "Write code"},
			{
				"role": "assistant",
				"tool_calls": [
					{
						"id": "tc-write",
						"type": "function",
						"function": {
							"name": "write_to_file",
							"arguments": "{\"path\": \"app.go\"}"
						}
					}
				]
			},
			{
				"role": "tool",
				"content": "Wrote 50 lines"
			}
		]
	}`)
	reqCtx3, err := classifier.Classify(bodyNoToolID)
	if err != nil {
		t.Fatalf("Classify no tool_call_id failed: %v", err)
	}
	if !reqCtx3.HasWriteProgress {
		t.Errorf("Expected HasWriteProgress=true when tool message has no tool_call_id, got false")
	}

	// 4. Anthropic content parts with tool_result without tool_use_id
	bodyAnthropicNoID := []byte(`{
		"model": "claude-3-opus",
		"messages": [
			{"role": "user", "content": "Build"},
			{
				"role": "assistant",
				"content": [
					{
						"type": "tool_use",
						"id": "tu-1",
						"name": "bash",
						"input": {"command": "sed -i 's/1/2/g' num.txt"}
					}
				]
			},
			{
				"role": "user",
				"content": [
					{
						"type": "tool_result",
						"text": "File updated successfully"
					}
				]
			}
		]
	}`)
	reqCtx4, err := classifier.Classify(bodyAnthropicNoID)
	if err != nil {
		t.Fatalf("Classify Anthropic no ID failed: %v", err)
	}
	if !reqCtx4.HasShellWrite {
		t.Errorf("Expected HasShellWrite=true for Anthropic tool_result without ID, got false")
	}
}

func TestClassifyContent_UnmarshalEdges(t *testing.T) {
	// 1. classifyContent UnmarshalJSON with empty or invalid byte
	var cc classifyContent
	if err := cc.UnmarshalJSON([]byte{}); err != nil {
		t.Errorf("Unexpected error on empty content: %v", err)
	}
	if err := cc.UnmarshalJSON([]byte("123")); err != nil {
		t.Errorf("Unexpected error on number content: %v", err)
	}

	// 2. classifyContentPart with nested string and array p.Content
	var part1 classifyContentPart
	part1JSON := []byte(`{"type": "text", "text": "initial", "content": "appended string"}`)
	if err := json.Unmarshal(part1JSON, &part1); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if part1.Text != "initial appended string" {
		t.Errorf("Expected concatenated text 'initial appended string', got %q", part1.Text)
	}

	var part2 classifyContentPart
	part2JSON := []byte(`{"type": "text", "text": "prefix", "content": [{"type": "text", "text": "block1"}, {"type": "text", "text": "block2"}]}`)
	if err := json.Unmarshal(part2JSON, &part2); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if part2.Text != "prefix block1 block2" {
		t.Errorf("Expected concatenated text 'prefix block1 block2', got %q", part2.Text)
	}

	// 3. classifyToolSchema with empty or non-object
	var ts classifyToolSchema
	if err := ts.UnmarshalJSON([]byte{}); err != nil {
		t.Errorf("Unexpected error on empty tool schema: %v", err)
	}
	if err := ts.UnmarshalJSON([]byte(`"not an object"`)); err != nil {
		t.Errorf("Unexpected error on string tool schema: %v", err)
	}
}

func TestSanitizeToolCallArguments_AdditionalBranches(t *testing.T) {
	// 1. Malformed JSON containing <<<<<<< SEARCH
	malformedWithDiff := "not-json\n<<<<<<< SEARCH\n:10:\nline\n=======\nline2\n>>>>>>> REPLACE"
	sanitized := SanitizeToolCallArguments("apply_diff", malformedWithDiff)
	if strings.Contains(sanitized, ":10:") {
		t.Errorf("Expected :10: line number stripped from malformed diff, got: %s", sanitized)
	}

	// 2. Malformed JSON without <<<<<<< SEARCH
	malformedNoDiff := `invalid json content`
	if res := SanitizeToolCallArguments("patch", malformedNoDiff); res != malformedNoDiff {
		t.Errorf("Expected untouched output for malformed non-diff, got: %s", res)
	}

	// 3. JSON containing non-string fields and diff field
	mixedJSON := `{"count": 42, "enabled": true, "diff": "<<<<<<< SEARCH\n:5:\nold\n=======\nnew\n>>>>>>> REPLACE"}`
	sanitizedMixed := SanitizeToolCallArguments("patch", mixedJSON)
	if strings.Contains(sanitizedMixed, ":5:") {
		t.Errorf("Expected :5: stripped from mixed JSON, got: %s", sanitizedMixed)
	}
	if !strings.Contains(sanitizedMixed, `"count":42`) && !strings.Contains(sanitizedMixed, `"count": 42`) {
		t.Errorf("Expected non-string fields preserved, got: %s", sanitizedMixed)
	}

	// 4. JSON containing clean diff (no changes made)
	cleanDiffJSON := `{"diff": "<<<<<<< SEARCH\nclean line\n=======\nnew line\n>>>>>>> REPLACE"}`
	sanitizedClean := SanitizeToolCallArguments("patch", cleanDiffJSON)
	if !strings.Contains(sanitizedClean, "clean line") {
		t.Errorf("Expected clean diff preserved, got: %s", sanitizedClean)
	}
}

func TestSessionTracker_ExhaustiveEdges(t *testing.T) {
	st := NewSessionTracker(100 * time.Millisecond)

	// Blank sessionKey guards
	st.RecordKickstartFailure("")
	if count := st.GetKickstartCount(""); count != 0 {
		t.Errorf("Expected 0 for blank session kickstart count, got %d", count)
	}
	if count := st.GetKickstartCount("nonexistent"); count != 0 {
		t.Errorf("Expected 0 for nonexistent session kickstart count, got %d", count)
	}

	// RecordKickstartState guards and behaviors
	if cnt, kick := st.RecordKickstartState("", false, 3, 0); cnt != 0 || kick {
		t.Errorf("Expected 0/false for blank session in RecordKickstartState")
	}
	if cnt, kick := st.RecordKickstartState("sess-test", false, 0, 0); cnt != 0 || kick {
		t.Errorf("Expected 0/false for threshold <= 0 in RecordKickstartState")
	}
	if cnt, kick := st.RecordKickstartState("nonexistent", false, 3, 0); cnt != 0 || kick {
		t.Errorf("Expected 0/false for nonexistent session in RecordKickstartState")
	}

	st.RecordTurn("sess-test", HashPrompt("prompt"), false)
	st.RecordKickstartFailure("sess-test")
	// Increment kickstart state
	cnt, isKick := st.RecordKickstartState("sess-test", false, 1, 0)
	if cnt != 1 || !isKick {
		t.Errorf("Expected kickstart count=1, isKick=true, got cnt=%d, isKick=%v", cnt, isKick)
	}
	st.RecordKickstartFailure("sess-test")
	if count := st.GetKickstartCount("sess-test"); count != 1 {
		t.Errorf("Expected 1, got %d", count)
	}
	// Tool progress resets kickstart state
	cnt, _ = st.RecordKickstartState("sess-test", true, 1, 0)
	if cnt != 0 {
		t.Errorf("Expected tool progress to reset kickstart count to 0, got %d", cnt)
	}

	// RecordCycleKill with blank / defaults / custom floor
	st.RecordCycleKill("", "model-1", 0)
	st.RecordCycleKill("sess-cycle", "", -1, 5)
	if !st.IsModelCoolingDown("sess-cycle", "model-1") {
		// model-1 was not added, should be false
	}
	st.RecordCycleKill("sess-cycle", "model-1", 50*time.Millisecond, 4)
	if !st.IsModelCoolingDown("sess-cycle", "model-1") {
		t.Errorf("Expected model-1 to be cooling down")
	}
	if models := st.GetCoolingDownModels("sess-cycle"); len(models) == 0 || models[0] != "model-1" {
		t.Errorf("Expected ['model-1'] in cooling down models, got %v", models)
	}

	// Test cooldown expiration
	time.Sleep(60 * time.Millisecond)
	if st.IsModelCoolingDown("sess-cycle", "model-1") {
		t.Errorf("Expected model-1 cooldown to have expired")
	}
	if models := st.GetCoolingDownModels("sess-cycle"); len(models) != 0 {
		t.Errorf("Expected empty cooling down models after expiration, got %v", models)
	}
	if st.IsModelCoolingDown("", "model-1") || st.IsModelCoolingDown("sess-cycle", "") {
		t.Errorf("Expected false for blank args in IsModelCoolingDown")
	}
	if st.GetCoolingDownModels("") != nil || st.GetCoolingDownModels("missing-sess") != nil {
		t.Errorf("Expected nil for blank/missing session in GetCoolingDownModels")
	}

	// RecordWriteProgress
	if res := st.RecordWriteProgress("", true); res != 0 {
		t.Errorf("Expected 0 for blank session write progress, got %d", res)
	}
	if res := st.RecordWriteProgress("missing", true); res != 0 {
		t.Errorf("Expected 0 for missing session write progress, got %d", res)
	}
	st.RecordTurn("sess-write", HashPrompt("prompt"), false)
	if res := st.RecordWriteProgress("sess-write", false); res != 0 {
		t.Errorf("Expected 0 for writeProgress=false, got %d", res)
	}
	if res := st.RecordWriteProgress("sess-write", true); res != 1 {
		t.Errorf("Expected 1 after write progress, got %d", res)
	}

	// CheckFairyDust edge cases
	if _, ok := st.CheckFairyDust("", "entry", 1, 1); ok {
		t.Errorf("Expected false for blank session")
	}
	if _, ok := st.CheckFairyDust("sess-write", "", 1, 1); ok {
		t.Errorf("Expected false for blank entry name")
	}
	if _, ok := st.CheckFairyDust("sess-write", "entry", 0, 1); ok {
		t.Errorf("Expected false for frequency <= 0")
	}
	if _, ok := st.CheckFairyDust("missing", "entry", 1, 1); ok {
		t.Errorf("Expected false for missing session")
	}

	// Modulo mismatch
	if _, ok := st.CheckFairyDust("sess-write", "entry", 5, 1); ok {
		t.Errorf("Expected false when WriteProgressCount %% frequency != 0")
	}

	// Trigger and maxPerSession enforcement
	cnt, ok := st.CheckFairyDust("sess-write", "entry", 1, 1)
	if !ok || cnt != 1 {
		t.Errorf("Expected trigger with count=1, got ok=%v, cnt=%d", ok, cnt)
	}
	// Second trigger should fail due to maxPerSession=1
	cnt2, ok2 := st.CheckFairyDust("sess-write", "entry", 1, 1)
	if ok2 || cnt2 != 1 {
		t.Errorf("Expected blocked by maxPerSession, got ok=%v, cnt=%d", ok2, cnt2)
	}
}

func TestDirective_LargeBufferAndShieldVariants(t *testing.T) {
	// 1. Text > 512 bytes with @nacho: directive to exercise heap buffer branch
	longPrefix := strings.Repeat("This is a verbose prompt detailing architecture specifications. ", 15)
	fullPrompt := longPrefix + "@nacho:raw please handle this quickly."
	if len(fullPrompt) <= 512 {
		t.Fatalf("Prompt length must exceed 512, got %d", len(fullPrompt))
	}

	flags, clean := ScanDirectives(fullPrompt)
	if flags != FeatureRawPassThrough {
		t.Errorf("Expected FeatureRawPassThrough, got %v", flags)
	}
	if strings.Contains(clean, "@nacho:raw") {
		t.Errorf("Expected @nacho:raw stripped from clean output")
	}

	// 2. Shield disable variants
	variants := []string{"@nacho:no-shield", "@nacho:noshield", "@nacho:shield-off", "@nacho:shield=off"}
	for _, v := range variants {
		flags, _ := ScanDirectives("Fix bug " + v)
		if flags.Has(FeatureShieldEnabled) {
			t.Errorf("Expected FeatureShieldEnabled masked out for %s", v)
		}
	}
}

func TestToolNormalizer_ReActSingleLineStringAndMistral(t *testing.T) {
	parser := &ReActParser{}

	// Single line string input with trailing newline
	content1 := "Thinking...\nAction: web_search\nAction Input: latest go release notes\nDone."
	cleaned1, calls1, ok1 := parser.Parse(content1, 1)
	if !ok1 || len(calls1) != 1 {
		t.Fatalf("Expected 1 ReAct call, got %d", len(calls1))
	}
	if calls1[0].Function.Name != "web_search" || !strings.Contains(calls1[0].Function.Arguments, "latest go release notes") {
		t.Errorf("Unexpected function name or arguments: %+v", calls1[0])
	}
	if strings.Contains(cleaned1, "Action: web_search") {
		t.Errorf("Expected action removed from cleaned content: %s", cleaned1)
	}

	// Single line string input without trailing newline (EOF)
	content2 := "Action: shell\nAction Input: ls -la"
	_, calls2, ok2 := parser.Parse(content2, 2)
	if !ok2 || len(calls2) != 1 {
		t.Fatalf("Expected 1 ReAct call at EOF, got %d", len(calls2))
	}
	if calls2[0].Function.Name != "shell" || !strings.Contains(calls2[0].Function.Arguments, "ls -la") {
		t.Errorf("Unexpected function name or arguments: %+v", calls2[0])
	}

	// Mistral lowercase token
	mistralParser := &MistralTokenParser{}
	contentMistral := "Here is the tool: [tool_calls] [{\"name\": \"run\", \"arguments\": {\"x\": 1}}]"
	_, mistralCalls, okMistral := mistralParser.Parse(contentMistral, 1)
	if !okMistral || len(mistralCalls) != 1 {
		t.Fatalf("Expected 1 Mistral tool call from lowercase token, got %d", len(mistralCalls))
	}
}
