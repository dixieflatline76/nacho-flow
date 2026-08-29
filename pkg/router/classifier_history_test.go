package router

import "testing"

func TestScanTrailingMessages_CleanHistory(t *testing.T) {
	messages := []interface{}{
		map[string]interface{}{"role": "user", "content": "Build me an app"},
		map[string]interface{}{"role": "assistant", "content": "Sure, let me help."},
	}
	errors, progress := scanTrailingMessages(messages)
	if errors != 0 {
		t.Errorf("expected 0 errors, got %d", errors)
	}
	if progress {
		t.Errorf("expected no tool progress")
	}
}

func TestScanTrailingMessages_ZooCodeMissingTool(t *testing.T) {
	messages := []interface{}{
		map[string]interface{}{"role": "assistant", "content": "Let me think about this..."},
		map[string]interface{}{"role": "user", "content": "[ERROR] You did not use a tool in your previous response! Please retry with a tool use."},
	}
	errors, progress := scanTrailingMessages(messages)
	if errors != 1 {
		t.Errorf("expected 1 error, got %d", errors)
	}
	if progress {
		t.Errorf("expected no tool progress on error-only history")
	}
}

func TestScanTrailingMessages_SchemaParameterError(t *testing.T) {
	messages := []interface{}{
		map[string]interface{}{"role": "user", "content": "Missing value for required parameter 'follow_up'. Please retry with complete response."},
	}
	errors, _ := scanTrailingMessages(messages)
	if errors != 1 {
		t.Errorf("expected 1 error, got %d", errors)
	}
}

func TestScanTrailingMessages_ConsecutiveErrors(t *testing.T) {
	messages := []interface{}{
		map[string]interface{}{"role": "user", "content": "Normal message"},
		map[string]interface{}{"role": "user", "content": "[ERROR] You did not use a tool"},
		map[string]interface{}{"role": "user", "content": "The tool execution failed"},
	}
	errors, _ := scanTrailingMessages(messages)
	if errors != 2 {
		t.Errorf("expected 2 consecutive errors, got %d", errors)
	}
}

func TestScanTrailingMessages_ToolProgressDetection(t *testing.T) {
	messages := []interface{}{
		map[string]interface{}{"role": "user", "content": "Please proceed"},
		map[string]interface{}{"role": "assistant", "content": "Writing file..."},
		map[string]interface{}{"role": "tool", "content": "File written successfully"},
	}
	errors, progress := scanTrailingMessages(messages)
	if errors != 0 {
		t.Errorf("expected 0 errors, got %d", errors)
	}
	if !progress {
		t.Errorf("expected tool progress to be detected")
	}
}

func TestScanTrailingMessages_DiffMismatchError(t *testing.T) {
	messages := []interface{}{
		map[string]interface{}{"role": "user", "content": "<error_details>\nNo sufficiently similar match found (79% similar, needs 100%)\n</error_details>"},
	}
	errors, _ := scanTrailingMessages(messages)
	if errors != 1 {
		t.Errorf("expected 1 error, got %d", errors)
	}
}
