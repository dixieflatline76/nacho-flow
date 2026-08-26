package router

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeDiff_LineGutters(t *testing.T) {
	input := `<<<<<<< SEARCH
168 | 		const benchmark = dealsData.benchmark_cost_per_m || 3.00;
169 | 		const dealCards = deals.map(deal => {
=======
		const benchmark = 3.00;
>>>>>>> REPLACE`

	expected := `<<<<<<< SEARCH
		const benchmark = dealsData.benchmark_cost_per_m || 3.00;
		const dealCards = deals.map(deal => {
=======
		const benchmark = 3.00;
>>>>>>> REPLACE`

	result := SanitizeDiff(input)
	if result != expected {
		t.Errorf("SanitizeDiff(LineGutters) failed:\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestSanitizeDiff_ColonPrefix(t *testing.T) {
	input := `<<<<<<< SEARCH
:168:
		const benchmark = dealsData.benchmark_cost_per_m || 3.00;
=======
		const benchmark = 3.00;
>>>>>>> REPLACE`

	expected := `<<<<<<< SEARCH
		const benchmark = dealsData.benchmark_cost_per_m || 3.00;
=======
		const benchmark = 3.00;
>>>>>>> REPLACE`

	result := SanitizeDiff(input)
	if result != expected {
		t.Errorf("SanitizeDiff(ColonPrefix) failed:\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestSanitizeDiff_StandardColon(t *testing.T) {
	input := `<<<<<<< SEARCH
168: 		const benchmark = dealsData.benchmark_cost_per_m || 3.00;
169: 		const dealCards = deals.map(deal => {
=======
		const benchmark = 3.00;
>>>>>>> REPLACE`

	expected := `<<<<<<< SEARCH
		const benchmark = dealsData.benchmark_cost_per_m || 3.00;
		const dealCards = deals.map(deal => {
=======
		const benchmark = 3.00;
>>>>>>> REPLACE`

	result := SanitizeDiff(input)
	if result != expected {
		t.Errorf("SanitizeDiff(StandardColon) failed:\nExpected:\n%s\nGot:\n%s", expected, result)
	}
}

func TestSanitizeDiff_CleanDiffNoOp(t *testing.T) {
	clean := `<<<<<<< SEARCH
func test() {
    return 1
}
=======
func test() {
    return 2
}
>>>>>>> REPLACE`

	result := SanitizeDiff(clean)
	if result != clean {
		t.Errorf("expected clean diff to be unchanged, got:\n%s", result)
	}
}

func TestSanitizeToolCallArguments_JSON(t *testing.T) {
	args := map[string]interface{}{
		"path": "extension/resources/webview/dashboard.js",
		"diff": "<<<<<<< SEARCH\n:168:\n\t\tconst benchmark = 3.00;\n=======\n\t\tconst benchmark = 4.00;\n>>>>>>> REPLACE",
	}
	bytes, _ := json.Marshal(args)

	sanitized := SanitizeToolCallArguments("apply_diff", string(bytes))
	if strings.Contains(sanitized, ":168:") {
		t.Errorf("expected :168: to be stripped from tool arguments, got: %s", sanitized)
	}
	if !strings.Contains(sanitized, "const benchmark = 3.00;") {
		t.Errorf("expected clean benchmark line in tool arguments, got: %s", sanitized)
	}
}

func TestSanitizeToolCallArguments_NonDiffFastBailout(t *testing.T) {
	args := `{"path": "pkg/server/proxy.go"}`
	sanitized := SanitizeToolCallArguments("read_file", args)
	if sanitized != args {
		t.Errorf("expected non-diff tool call to pass through untouched, got: %s", sanitized)
	}
}
