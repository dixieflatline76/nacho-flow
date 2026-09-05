package router

import (
	"strings"
	"sync"
	"testing"
)

func TestScanTrailingMessages_CleanHistory(t *testing.T) {
	clf := &RequestClassifier{}
	messages := []interface{}{
		map[string]interface{}{"role": "user", "content": "Build me an app"},
		map[string]interface{}{"role": "assistant", "content": "Sure, let me help."},
	}
	errors, progress, _, _, _ := clf.scanTrailingMessages(messages)
	if errors != 0 {
		t.Errorf("expected 0 errors, got %d", errors)
	}
	if progress {
		t.Errorf("expected no tool progress")
	}
}

func TestScanTrailingMessages_ZooCodeMissingTool(t *testing.T) {
	clf := &RequestClassifier{}
	messages := []interface{}{
		map[string]interface{}{"role": "assistant", "content": "Let me think about this..."},
		map[string]interface{}{"role": "user", "content": "[ERROR] You did not use a tool in your previous response! Please retry with a tool use."},
	}
	errors, progress, _, _, _ := clf.scanTrailingMessages(messages)
	if errors != 1 {
		t.Errorf("expected 1 error, got %d", errors)
	}
	if progress {
		t.Errorf("expected no tool progress on error-only history")
	}
}

func TestScanTrailingMessages_SchemaParameterError(t *testing.T) {
	clf := &RequestClassifier{}
	messages := []interface{}{
		map[string]interface{}{"role": "user", "content": "Missing value for required parameter 'follow_up'. Please retry with complete response."},
	}
	errors, _, _, _, _ := clf.scanTrailingMessages(messages)
	if errors != 1 {
		t.Errorf("expected 1 error, got %d", errors)
	}
}

func TestScanTrailingMessages_ConsecutiveErrors(t *testing.T) {
	clf := &RequestClassifier{}
	messages := []interface{}{
		map[string]interface{}{"role": "user", "content": "Normal message"},
		map[string]interface{}{"role": "user", "content": "[ERROR] You did not use a tool"},
		map[string]interface{}{"role": "user", "content": "The tool execution failed"},
	}
	errors, _, _, _, _ := clf.scanTrailingMessages(messages)
	if errors != 2 {
		t.Errorf("expected 2 consecutive errors, got %d", errors)
	}
}

func TestScanTrailingMessages_ToolProgressDetection(t *testing.T) {
	clf := &RequestClassifier{}
	messages := []interface{}{
		map[string]interface{}{"role": "user", "content": "Please proceed"},
		map[string]interface{}{"role": "assistant", "content": "Writing file..."},
		map[string]interface{}{"role": "tool", "content": "File written successfully"},
	}
	errors, progress, _, _, _ := clf.scanTrailingMessages(messages)
	if errors != 0 {
		t.Errorf("expected 0 errors, got %d", errors)
	}
	if !progress {
		t.Errorf("expected tool progress to be detected")
	}
}

func TestScanTrailingMessages_DiffMismatchError(t *testing.T) {
	clf := &RequestClassifier{}
	messages := []interface{}{
		map[string]interface{}{"role": "user", "content": "<error_details>\nNo sufficiently similar match found (79% similar, needs 100%)\n</error_details>"},
	}
	errors, _, _, _, _ := clf.scanTrailingMessages(messages)
	if errors != 1 {
		t.Errorf("expected 1 error, got %d", errors)
	}
}

func TestScanTrailingMessages_ClineDiffEditToolRoleError(t *testing.T) {
	clf := &RequestClassifier{}
	messages := []interface{}{
		map[string]interface{}{"role": "assistant", "content": "Writing tsconfig.json..."},
		map[string]interface{}{
			"role":    "tool",
			"content": `{"query":"edit:c:\\project\\tsconfig.json","result":"","error":"Editor operation failed: Parameter ` + "`old_text`" + ` is required when editing an existing file without ` + "`insert_line`" + `","success":false}`,
		},
	}
	errors, progress, _, _, _ := clf.scanTrailingMessages(messages)
	if errors != 1 {
		t.Errorf("expected 1 error for Cline tool-role diff rejection, got %d", errors)
	}
	if progress {
		t.Errorf("expected no tool progress on failed tool execution")
	}
}

func TestScanTrailingMessages_ClineDiffEditUserRoleError(t *testing.T) {
	clf := &RequestClassifier{}
	messages := []interface{}{
		map[string]interface{}{"role": "assistant", "content": "Editing file..."},
		map[string]interface{}{"role": "user", "content": "Editor operation failed: Parameter `old_text` is required when editing an existing file"},
	}
	errors, _, _, _, _ := clf.scanTrailingMessages(messages)
	if errors != 1 {
		t.Errorf("expected 1 error for Cline user-role diff rejection, got %d", errors)
	}
}

func TestScanTrailingMessages_PlanModeIsNotAnErrorByDefault(t *testing.T) {
	clf := &RequestClassifier{}
	messages := []interface{}{
		map[string]interface{}{"role": "assistant", "content": "Let me edit package.json"},
		map[string]interface{}{
			"role":    "user",
			"content": "file modifications are blocked in plan mode",
		},
	}
	errors, _, _, _, _ := clf.scanTrailingMessages(messages)
	if errors != 0 {
		t.Errorf("expected 0 errors for plan mode discussion, got %d", errors)
	}
}

func TestScanTrailingMessages_CustomErrorSignatures(t *testing.T) {
	clf := &RequestClassifier{}
	customSigs := []string{"CUSTOM_AGENT_FAILURE", "MY_LINTER_ERROR"}
	clf.SetErrorSignatures(customSigs)

	if sigs := clf.GetErrorSignatures(); len(sigs) != 2 || sigs[0] != "CUSTOM_AGENT_FAILURE" {
		t.Fatalf("unexpected signatures: %v", sigs)
	}

	messages := []interface{}{
		map[string]interface{}{"role": "user", "content": "Warning: MY_LINTER_ERROR detected in file"},
	}
	errors, _, _, _, _ := clf.scanTrailingMessages(messages)
	if errors != 1 {
		t.Errorf("expected 1 error with custom signature, got %d", errors)
	}

	// Reset to defaults with empty slice
	clf.SetErrorSignatures(nil)
	if sigs := clf.GetErrorSignatures(); len(sigs) != len(defaultAgentErrorSignatures) {
		t.Errorf("expected default signatures after reset, got %d", len(sigs))
	}
}

func TestScanTrailingMessages_ConcurrentAccess(t *testing.T) {
	clf := &RequestClassifier{}
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				clf.SetErrorSignatures([]string{"ERR_A", "ERR_B"})
			} else {
				clf.SetErrorSignatures(nil)
			}
		}(i)
		go func() {
			defer wg.Done()
			_ = clf.GetErrorSignatures()
			messages := []interface{}{
				map[string]interface{}{"role": "user", "content": "ERR_A happened"},
			}
			_, _, _, _, _ = clf.scanTrailingMessages(messages)
		}()
	}
	wg.Wait()
}

func TestScanTrailingMessages_ClineToolSuccess(t *testing.T) {
	clf := &RequestClassifier{}
	messages := []interface{}{
		map[string]interface{}{"role": "assistant", "content": "Creating package.json"},
		map[string]interface{}{
			"role":    "tool",
			"content": `{"query":"edit:package.json","result":"File created successfully at: package.json","success":true}`,
		},
	}
	errors, progress, _, _, _ := clf.scanTrailingMessages(messages)
	if errors != 0 {
		t.Errorf("expected 0 errors on successful tool execution, got %d", errors)
	}
	if !progress {
		t.Errorf("expected tool progress on successful tool execution")
	}
}

// --- HasTestProgress & Dual-Signal tests ---

func TestScanTrailingMessages_GoTestOutput_Failing(t *testing.T) {
	clf := &RequestClassifier{}

	// Simulate: assistant ran "go test ./..." and tool result contains Go test output with FAIL
	messages := []interface{}{
		map[string]interface{}{
			"role": "assistant",
			"tool_calls": []interface{}{
				map[string]interface{}{
					"id": "call-1",
					"function": map[string]interface{}{
						"name":      "execute_command",
						"arguments": `{"command":"go test ./..."}`,
					},
				},
			},
		},
		map[string]interface{}{
			"role":         "tool",
			"tool_call_id": "call-1",
			"content":      "ok  \tgithub.com/foo/bar/pkg/engine\t0.452s\n--- FAIL: TestDealCard (0.00s)\nFAIL\tgithub.com/foo/bar/pkg/cards\t0.123s",
		},
	}

	_, _, _, pass, fail := clf.scanTrailingMessages(messages)
	if !pass {
		t.Errorf("expected pass=true (found 'ok  \\t'), got false")
	}
	if !fail {
		t.Errorf("expected fail=true (found '--- FAIL:'), got false")
	}
	testProgress := pass && !fail
	if testProgress {
		t.Errorf("expected testProgress=false when suite contains FAIL, got true")
	}
}

func TestScanTrailingMessages_GoTestOutput_Passing(t *testing.T) {
	clf := &RequestClassifier{}

	// Simulate: assistant ran "go test ./..." and tool result contains clean passing output
	messages := []interface{}{
		map[string]interface{}{
			"role": "assistant",
			"tool_calls": []interface{}{
				map[string]interface{}{
					"id": "call-1p",
					"function": map[string]interface{}{
						"name":      "execute_command",
						"arguments": `{"command":"go test ./..."}`,
					},
				},
			},
		},
		map[string]interface{}{
			"role":         "tool",
			"tool_call_id": "call-1p",
			"content":      "--- PASS: TestSolveNQueens (0.00s)\nPASS\nok  \tgithub.com/foo/bar/pkg/solver\t0.123s",
		},
	}

	_, _, _, pass, fail := clf.scanTrailingMessages(messages)
	if !pass {
		t.Errorf("expected pass=true, got false")
	}
	if fail {
		t.Errorf("expected fail=false, got true")
	}
	testProgress := pass && !fail
	if !testProgress {
		t.Errorf("expected testProgress=true on clean passing test run, got false")
	}
}

func TestScanTrailingMessages_CompilerErrors(t *testing.T) {
	clf := &RequestClassifier{}

	// Simulate: compiler error in tool result
	messages := []interface{}{
		map[string]interface{}{
			"role": "assistant",
			"tool_calls": []interface{}{
				map[string]interface{}{
					"id": "call-2",
					"function": map[string]interface{}{
						"name":      "execute_command",
						"arguments": `{"command":"go build ./..."}`,
					},
				},
			},
		},
		map[string]interface{}{
			"role":         "tool",
			"tool_call_id": "call-2",
			"content":      "./pkg/engine/engine.go:45:2: undefined: CardSuit\n./pkg/engine/hand.go:12:15: cannot use deck (variable of type []Card) as type Deck",
		},
	}

	_, _, _, pass, fail := clf.scanTrailingMessages(messages)
	if pass {
		t.Errorf("expected pass=false, got true")
	}
	if !fail {
		t.Errorf("expected fail=true for compiler error (undefined:, cannot use), got false")
	}
	testProgress := pass && !fail
	if testProgress {
		t.Errorf("expected testProgress=false when compiler errors exist, got true")
	}
}

func TestScanTrailingMessages_TestFileRead_NoImmunity(t *testing.T) {
	clf := &RequestClassifier{}

	// Simulate: assistant reads a _test.go file — reading test files must NOT grant test progress immunity
	messages := []interface{}{
		map[string]interface{}{
			"role": "assistant",
			"tool_calls": []interface{}{
				map[string]interface{}{
					"id": "call-3",
					"function": map[string]interface{}{
						"name":      "read_file",
						"arguments": `{"path":"pkg/oracle/oracle_test.go"}`,
					},
				},
			},
		},
		map[string]interface{}{
			"role":         "tool",
			"tool_call_id": "call-3",
			"content":      "package oracle\n\nimport \"testing\"\n\nfunc TestBasicStrategy(t *testing.T) {",
		},
	}

	_, _, _, pass, fail := clf.scanTrailingMessages(messages)
	if pass || fail {
		t.Errorf("expected pass=false and fail=false when reading test files, got pass=%v fail=%v", pass, fail)
	}
	testProgress := pass && !fail
	if testProgress {
		t.Errorf("expected testProgress=false when reading a _test.go file, got true")
	}
}

func TestScanTrailingMessages_NormalReadNoTestProgress(t *testing.T) {
	clf := &RequestClassifier{}

	// Simulate: assistant reads a normal .go file — should NOT trigger test progress
	messages := []interface{}{
		map[string]interface{}{
			"role": "assistant",
			"tool_calls": []interface{}{
				map[string]interface{}{
					"id": "call-4",
					"function": map[string]interface{}{
						"name":      "read_file",
						"arguments": `{"path":"pkg/engine/engine.go"}`,
					},
				},
			},
		},
		map[string]interface{}{
			"role":         "tool",
			"tool_call_id": "call-4",
			"content":      "package engine\n\ntype Engine struct {\n\tDeck []Card\n}",
		},
	}

	_, _, _, pass, fail := clf.scanTrailingMessages(messages)
	if pass || fail {
		t.Errorf("expected pass=false and fail=false when reading normal file, got pass=%v fail=%v", pass, fail)
	}
	testProgress := pass && !fail
	if testProgress {
		t.Errorf("expected testProgress=false when reading a normal .go file, got true (false positive)")
	}
}

func TestScanTrailingMessages_JestOutput(t *testing.T) {
	clf := &RequestClassifier{}

	// Simulate: clean Jest test output in tool result
	messages := []interface{}{
		map[string]interface{}{
			"role": "assistant",
			"tool_calls": []interface{}{
				map[string]interface{}{
					"id": "call-5",
					"function": map[string]interface{}{
						"name":      "execute_command",
						"arguments": `{"command":"npx jest --coverage"}`,
					},
				},
			},
		},
		map[string]interface{}{
			"role":         "tool",
			"tool_call_id": "call-5",
			"content":      "PASS src/blackjack.test.js\nTests: 0 failed, 12 passed, 12 total",
		},
	}

	_, _, _, pass, fail := clf.scanTrailingMessages(messages)
	if !pass {
		t.Errorf("expected pass=true, got false")
	}
	if fail {
		t.Errorf("expected fail=false, got true")
	}
	testProgress := pass && !fail
	if !testProgress {
		t.Errorf("expected testProgress=true on clean passing Jest run, got false")
	}
}

// --- detectTestSignals Unit Tests ---

func TestDetectTestSignals(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		wantPass bool
		wantFail bool
	}{
		{
			name:     "Go test clean pass",
			text:     "--- PASS: TestFoo (0.01s)\nPASS\nok  \tpkg/foo\t0.02s",
			wantPass: true,
			wantFail: false,
		},
		{
			name:     "Go test failure",
			text:     "--- FAIL: TestFoo (0.01s)\nFAIL\nFAIL\tpkg/foo\t0.02s",
			wantPass: false,
			wantFail: true,
		},
		{
			name:     "Go mixed pass and fail (pass before fail)",
			text:     "--- PASS: TestA (0.00s)\n--- FAIL: TestB (0.01s)\nFAIL",
			wantPass: true,
			wantFail: true,
		},
		{
			name:     "Go compiler error",
			text:     "./main.go:10:2: undefined: Foo\n./main.go:12:5: syntax error",
			wantPass: false,
			wantFail: true,
		},
		{
			name:     "Jest clean pass",
			text:     "PASS src/index.test.ts\n0 failed, 5 passed\npassed\n",
			wantPass: true,
			wantFail: false,
		},
		{
			name:     "Jest failure",
			text:     "FAIL src/index.test.ts\n1 failed, 4 passed\nfailed\n",
			wantPass: true,
			wantFail: true,
		},
		{
			name:     "Pytest clean pass",
			text:     "tests/test_x.py . [100%]\n=== 1 passed in 0.04s ===",
			wantPass: true,
			wantFail: false,
		},
		{
			name:     "Pytest failure",
			text:     "tests/test_x.py F [100%]\n=== FAILURES ===\nFAILED tests/test_x.py::test_f\nTraceback (most recent call last):",
			wantPass: false,
			wantFail: true,
		},
		{
			name:     "Cargo clean pass",
			text:     "running 5 tests\ntest result: ok. 5 passed; 0 failed;",
			wantPass: true,
			wantFail: false,
		},
		{
			name:     "Cargo failure",
			text:     "running 5 tests\ntest result: FAILED. 4 passed; 1 failed;\nerror[E0425]: cannot find value",
			wantPass: false,
			wantFail: true,
		},
		{
			name:     "CLI command failure",
			text:     "Command failed with exit code 1",
			wantPass: false,
			wantFail: true,
		},
		{
			name:     "Non-test prose",
			text:     "I have examined the code and everything looks good.",
			wantPass: false,
			wantFail: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPass, gotFail := detectTestSignals(tt.text)
			if gotPass != tt.wantPass || gotFail != tt.wantFail {
				t.Errorf("detectTestSignals(%q) = (%v, %v), want (%v, %v)",
					tt.text, gotPass, gotFail, tt.wantPass, tt.wantFail)
			}
		})
	}
}

func BenchmarkDetectTestSignals(b *testing.B) {
	sample := strings.Repeat("some code line\n", 200) +
		"--- FAIL: TestSolveNQueens (0.01s)\n" +
		strings.Repeat("more output line\n", 200) +
		"FAIL\tpkg/solver\t0.05s\n"

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p, f := detectTestSignals(sample)
		if !f || p {
			b.Fatalf("unexpected result: pass=%v fail=%v", p, f)
		}
	}
}
