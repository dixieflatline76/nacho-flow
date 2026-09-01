package router

import (
	"sync"
	"testing"
)

func TestScanTrailingMessages_CleanHistory(t *testing.T) {
	clf := &RequestClassifier{}
	messages := []interface{}{
		map[string]interface{}{"role": "user", "content": "Build me an app"},
		map[string]interface{}{"role": "assistant", "content": "Sure, let me help."},
	}
	errors, progress, _ := clf.scanTrailingMessages(messages)
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
	errors, progress, _ := clf.scanTrailingMessages(messages)
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
	errors, _, _ := clf.scanTrailingMessages(messages)
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
	errors, _, _ := clf.scanTrailingMessages(messages)
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
	errors, progress, _ := clf.scanTrailingMessages(messages)
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
	errors, _, _ := clf.scanTrailingMessages(messages)
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
	errors, progress, _ := clf.scanTrailingMessages(messages)
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
	errors, _, _ := clf.scanTrailingMessages(messages)
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
	errors, _, _ := clf.scanTrailingMessages(messages)
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
	errors, _, _ := clf.scanTrailingMessages(messages)
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
			_, _, _ = clf.scanTrailingMessages(messages)
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
	errors, progress, _ := clf.scanTrailingMessages(messages)
	if errors != 0 {
		t.Errorf("expected 0 errors on successful tool execution, got %d", errors)
	}
	if !progress {
		t.Errorf("expected tool progress on successful tool execution")
	}
}

