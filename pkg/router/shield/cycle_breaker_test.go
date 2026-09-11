package shield

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
)

func TestCycleBreaker_Disabled(t *testing.T) {
	disabled := false
	prompt := "Custom override"
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:          &disabled,
		CorrectionPrompt: prompt,
		MaxRetries:       2,
	})

	if cb.IsEnabled() {
		t.Errorf("expected IsEnabled to return false")
	}
	if cb.CorrectionPrompt() != prompt {
		t.Errorf("expected CorrectionPrompt %q, got %q", prompt, cb.CorrectionPrompt())
	}
	if cb.MaxRetries() != 2 {
		t.Errorf("expected MaxRetries 2, got %d", cb.MaxRetries())
	}

	// Process infinite repetition while disabled
	for i := 0; i < 20; i++ {
		triggered, reason := cb.ProcessDelta("Let's do this! Checking now! Proceeding! Actually, wait... ", false)
		if triggered {
			t.Fatalf("expected cycle breaker to not trigger when disabled, got reason: %s", reason)
		}
	}
}

func TestCycleBreaker_NgramRepetitionDetection(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:             &enabled,
		MaxProseTokens:      800,
		RepetitionWindow:    6,
		RepetitionThreshold: 3,
	})

	phrase := "Let's do this! Checking now! Proceeding! Actually wait... "

	triggered, reason := cb.ProcessDelta(phrase, false)
	if triggered {
		t.Fatalf("first repetition should not trigger, got %s", reason)
	}

	triggered, reason = cb.ProcessDelta(phrase, false)
	if triggered {
		t.Fatalf("second repetition should not trigger, got %s", reason)
	}

	// Third repetition should trigger instantly!
	triggered, reason = cb.ProcessDelta(phrase, false)
	if !triggered {
		t.Fatalf("expected cycle breaker to trigger on 3rd repetition")
	}
	if reason != "ngram_repetition_loop_detected" {
		t.Fatalf("expected ngram_repetition_loop_detected, got %s", reason)
	}
}

func TestCycleBreaker_ProseTokenCeiling(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:             &enabled,
		MaxProseTokens:      50,
		RepetitionWindow:    6,
		RepetitionThreshold: 5, // High threshold so ngram loop doesn't fire first
	})

	// Send unique words - should NOT trigger because maxNgramFreq stays 0
	for i := 0; i < 100; i++ {
		word := fmt.Sprintf("uniqueWord%d ", i)
		triggered, reason := cb.ProcessDelta(word, false)
		if triggered {
			t.Fatalf("unique words should never trigger cooperative budget check, got reason: %s at word %d", reason, i)
		}
	}

	if cb.ProseTokens() <= 50 {
		t.Fatalf("expected ProseTokens() > 50 (budget ceiling), got %d", cb.ProseTokens())
	}
	if cb.MaxNgramFreq() > 1 {
		t.Fatalf("expected MaxNgramFreq() <= 1 for all unique words, got %d", cb.MaxNgramFreq())
	}
}

func TestCycleBreaker_CooperativeProseBudget(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:             &enabled,
		MaxProseTokens:      200,
		RepetitionWindow:    6,
		RepetitionThreshold: 100, // Set very high so ngram loop never fires
	})

	// Send semi-repetitive content: same phrase interspersed with unique words
	// This will build up N-gram frequency >= 2 but below repetition threshold
	var triggered bool
	var reason string
	for round := 0; round < 50; round++ {
		cb.ProcessDelta(fmt.Sprintf("unique prefix number %d ", round), false)
		triggered, reason = cb.ProcessDelta("the quick brown fox jumps over ", false)
		if triggered {
			break
		}
	}

	if !triggered {
		t.Fatalf("expected cooperative budget to trigger on semi-repetitive content exceeding budget")
	}
	if reason != "prose_budget_exceeded_with_repetition" {
		t.Fatalf("expected prose_budget_exceeded_with_repetition, got %s", reason)
	}
}

func TestCycleBreaker_ThinkingTokenBudget(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:                     &enabled,
		MaxThinkingTokens:           60,
		ThinkingRepetitionThreshold: 10, // High threshold so loop doesn't fire first
	})

	// Send unique words - should NOT trigger cooperative budget
	for i := 0; i < 100; i++ {
		word := fmt.Sprintf("thinkingStepNumber%d ", i)
		triggered, reason := cb.ProcessDelta(word, true)
		if triggered {
			t.Fatalf("unique thinking words should never trigger cooperative budget, got reason: %s at word %d", reason, i)
		}
	}

	if cb.ThinkingTokens() <= 60 {
		t.Fatalf("expected ThinkingTokens() > 60 (budget ceiling), got %d", cb.ThinkingTokens())
	}
	if cb.MaxThinkingNgramFreq() > 1 {
		t.Fatalf("expected MaxThinkingNgramFreq() <= 1 for all unique thinking words, got %d", cb.MaxThinkingNgramFreq())
	}
	if cb.ProseTokens() != 0 {
		t.Fatalf("expected ProseTokens() == 0 during thinking, got %d", cb.ProseTokens())
	}
}

func TestCycleBreaker_CooperativeThinkingBudget(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:                     &enabled,
		MaxThinkingTokens:           200,
		RepetitionWindow:            6,
		ThinkingRepetitionThreshold: 100, // Set very high so loop never fires
	})

	var triggered bool
	var reason string
	for round := 0; round < 50; round++ {
		cb.ProcessDelta(fmt.Sprintf("unique thinking step %d ", round), true)
		triggered, reason = cb.ProcessDelta("let me reconsider this approach carefully ", true)
		if triggered {
			break
		}
	}

	if !triggered {
		t.Fatalf("expected cooperative thinking budget to trigger on semi-repetitive content exceeding budget")
	}
	if reason != "thinking_budget_exceeded_with_repetition" {
		t.Fatalf("expected thinking_budget_exceeded_with_repetition, got %s", reason)
	}
}

func TestCycleBreaker_ThinkingRepetitionLoop(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:                     &enabled,
		MaxThinkingTokens:           5000,
		RepetitionWindow:            6,
		ThinkingRepetitionThreshold: 5,
	})

	phrase := "Wait let me check the types again right now. "

	// First 4 repetitions should NOT trigger (threshold is 5)
	for i := 1; i <= 4; i++ {
		triggered, reason := cb.ProcessDelta(phrase, true)
		if triggered {
			t.Fatalf("repetition %d should not trigger, got %s", i, reason)
		}
	}

	// 5th repetition should trigger!
	triggered, reason := cb.ProcessDelta(phrase, true)
	if !triggered {
		t.Fatalf("expected cycle breaker to trigger on 5th thinking repetition")
	}
	if reason != "thinking_repetition_loop_detected" {
		t.Fatalf("expected thinking_repetition_loop_detected, got %s", reason)
	}
}

func TestCycleBreaker_DualLaneIsolation(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:                     &enabled,
		MaxProseTokens:              1000,
		MaxThinkingTokens:           1000,
		RepetitionWindow:            6,
		RepetitionThreshold:         3,
		ThinkingRepetitionThreshold: 5,
	})

	phrase := "Let us explore all the possibilities thoroughly. "

	// Emit 4x in thinking mode (under 5x threshold)
	for i := 0; i < 4; i++ {
		triggered, reason := cb.ProcessDelta(phrase, true)
		if triggered {
			t.Fatalf("thinking repetition should not trigger at 4x, got %s", reason)
		}
	}

	// Emit 2x in prose mode (under 3x threshold)
	for i := 0; i < 2; i++ {
		triggered, reason := cb.ProcessDelta(phrase, false)
		if triggered {
			t.Fatalf("prose repetition should not trigger at 2x, got %s", reason)
		}
	}

	// Neither lane triggered because their N-gram tables are completely isolated!
	if cb.ThinkingTokens() == 0 || cb.ProseTokens() == 0 {
		t.Fatalf("expected non-zero token counts in both lanes, got thinking=%d, prose=%d",
			cb.ThinkingTokens(), cb.ProseTokens())
	}
}

func TestCycleBreaker_ResetClearsBothLanes(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:                     &enabled,
		MaxProseTokens:              50,
		MaxThinkingTokens:           50,
		RepetitionWindow:            4,
		RepetitionThreshold:         3,
		ThinkingRepetitionThreshold: 3,
	})

	// Build up repeated N-grams in both lanes
	cb.ProcessDelta("repeat this word phrase repeat this word phrase ", false)
	cb.ProcessDelta("thinking about this word phrase thinking about this word phrase ", true)

	if cb.ProseTokens() == 0 || cb.ThinkingTokens() == 0 {
		t.Fatalf("expected non-zero counts before reset")
	}
	if cb.MaxNgramFreq() < 2 {
		t.Fatalf("expected MaxNgramFreq() >= 2 before reset, got %d", cb.MaxNgramFreq())
	}
	if cb.MaxThinkingNgramFreq() < 2 {
		t.Fatalf("expected MaxThinkingNgramFreq() >= 2 before reset, got %d", cb.MaxThinkingNgramFreq())
	}

	cb.Reset()

	if cb.ProseTokens() != 0 {
		t.Fatalf("expected 0 prose tokens after reset, got %d", cb.ProseTokens())
	}
	if cb.ThinkingTokens() != 0 {
		t.Fatalf("expected 0 thinking tokens after reset, got %d", cb.ThinkingTokens())
	}
	if cb.MaxNgramFreq() != 0 {
		t.Fatalf("expected 0 MaxNgramFreq after reset, got %d", cb.MaxNgramFreq())
	}
	if cb.MaxThinkingNgramFreq() != 0 {
		t.Fatalf("expected 0 MaxThinkingNgramFreq after reset, got %d", cb.MaxThinkingNgramFreq())
	}

	// Verify post-reset: sending unique tokens exceeding budget (50 tokens) does NOT trigger,
	// confirming that stale maxNgramFreq was cleanly erased.
	for i := 0; i < 60; i++ {
		triggered, reason := cb.ProcessDelta(fmt.Sprintf("postResetUniqueWord%d ", i), false)
		if triggered {
			t.Fatalf("should not trigger after reset on unique words exceeding budget, got %s", reason)
		}
	}
}

func TestCycleBreaker_LocalityGuard_DistantRepetitionsDoNotTrigger(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:             &enabled,
		MaxProseTokens:      4000,
		RepetitionWindow:    6,
		RepetitionThreshold: 3,
	})

	phrase := "if we have a queen at "

	// Occurrence 1
	triggered, reason := cb.ProcessDelta(phrase, false)
	if triggered {
		t.Fatalf("unexpected trigger on occurrence 1: %s", reason)
	}

	// 60 unique words in between (> defaultMaxLoopDistance of 48)
	for i := 0; i < 60; i++ {
		cb.ProcessDelta(fmt.Sprintf("uniqueContextWordA%d ", i), false)
	}

	// Occurrence 2
	triggered, reason = cb.ProcessDelta(phrase, false)
	if triggered {
		t.Fatalf("unexpected trigger on occurrence 2: %s", reason)
	}

	// Another 60 unique words in between
	for i := 0; i < 60; i++ {
		cb.ProcessDelta(fmt.Sprintf("uniqueContextWordB%d ", i), false)
	}

	// Occurrence 3 - In old global map, this triggered at count=3 even though separated by 120 words!
	// With locality check, distance > 48 resets consecutive count, so it must NOT trigger!
	triggered, reason = cb.ProcessDelta(phrase, false)
	if triggered {
		t.Fatalf("expected locality guard to prevent trigger on distant repetition, got %s", reason)
	}
}

func TestCycleBreaker_NQueensAlgorithmicProse(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:             &enabled,
		MaxProseTokens:      4000,
		RepetitionWindow:    6,
		RepetitionThreshold: 3,
	})

	// Exact pattern from N-Queens solver explanation where the model analyses separate board placements
	sections := []string{
		"If we have a queen at row zero column zero then the entire diagonal is attacked. Let us continue exploring the remaining possibilities across the chessboard systematically to find all valid non-attacking configurations.\n\n",
		"Next scenario to consider: If we have a queen at row one column two then column attacks are avoided. We can safely branch to row three and evaluate whether the remaining squares are defensible.\n\n",
		"Finally in the third iteration: If we have a queen at row two column four we observe an intersection along the negative diagonal, forcing a backtrack.\n\n",
	}

	for i, s := range sections {
		triggered, reason := cb.ProcessDelta(s, false)
		if triggered {
			t.Fatalf("N-Queens algorithmic explanation triggered cycle breaker at section %d: %s", i+1, reason)
		}
	}
}

func TestCycleBreaker_ToolLane_RepetitionDetection(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:             &enabled,
		MaxToolTokens:       8192,
		RepetitionWindow:    6,
		RepetitionThreshold: 3,
	})

	phrase := "replace target directory with destination directory systematically "

	triggered, reason := cb.ProcessToolDelta(phrase)
	if triggered {
		t.Fatalf("first repetition should not trigger, got %s", reason)
	}

	triggered, reason = cb.ProcessToolDelta(phrase)
	if triggered {
		t.Fatalf("second repetition should not trigger, got %s", reason)
	}

	// Third repetition in tool arguments triggers tool_repetition_loop_detected
	triggered, reason = cb.ProcessToolDelta(phrase)
	if !triggered {
		t.Fatalf("third repetition should trigger tool repetition loop")
	}
	if reason != "tool_repetition_loop_detected" {
		t.Fatalf("expected reason tool_repetition_loop_detected, got %s", reason)
	}
	if cb.ToolTokens() == 0 {
		t.Errorf("expected non-zero ToolTokens, got %d", cb.ToolTokens())
	}
	if cb.MaxToolNgramFreq() < 3 {
		t.Errorf("expected MaxToolNgramFreq >= 3, got %d", cb.MaxToolNgramFreq())
	}
}

func TestCycleBreaker_ToolLane_LargeCodePayload_NoFalsePositive(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:             &enabled,
		MaxProseTokens:      100, // tight prose budget
		MaxThinkingTokens:   100, // tight thinking budget
		MaxToolTokens:       8192,
		RepetitionWindow:    6,
		RepetitionThreshold: 3,
	})

	// Realistic Go code file written to a file via tool arguments
	code := `package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Config struct {
	Port         int           ` + "`json:\"port\"`" + `
	ReadTimeout  time.Duration ` + "`json:\"read_timeout\"`" + `
	WriteTimeout time.Duration ` + "`json:\"write_timeout\"`" + `
}

type Server struct {
	cfg    *Config
	server *http.Server
	mux    *http.ServeMux
}

func NewServer(cfg *Config) (*Server, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration cannot be nil")
	}
	mux := http.NewServeMux()
	s := &Server{
		cfg: cfg,
		mux: mux,
		server: &http.Server{
			Addr:         fmt.Sprintf(":%d", cfg.Port),
			Handler:      mux,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
		},
	}
	s.registerRoutes()
	return s, nil
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/healthz", s.handleHealthCheck)
	s.mux.HandleFunc("/api/v1/status", s.handleStatusReport)
	s.mux.HandleFunc("/api/v1/config", s.handleGetConfiguration)
}

func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(` + "`{\"status\":\"healthy\"}`" + `))
}

func (s *Server) handleStatusReport(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	select {
	case <-ctx.Done():
		http.Error(w, "request timed out during status evaluation", http.StatusGatewayTimeout)
		return
	default:
		payload := map[string]any{
			"uptime": time.Since(time.Now()),
			"active": true,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}
}

func (s *Server) handleGetConfiguration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "invalid request method", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.cfg)
}
`

	triggered, reason := cb.ProcessToolDelta(code)
	if triggered {
		t.Fatalf("expected large tool payload with unique identifiers not to trigger, got %s", reason)
	}
	// Tool lane tokens should be tracked, prose and thinking should remain zero
	if cb.ToolTokens() == 0 {
		t.Errorf("expected ToolTokens > 0, got %d", cb.ToolTokens())
	}
	if cb.ProseTokens() != 0 {
		t.Errorf("expected ProseTokens == 0, got %d", cb.ProseTokens())
	}
	if cb.ThinkingTokens() != 0 {
		t.Errorf("expected ThinkingTokens == 0, got %d", cb.ThinkingTokens())
	}
}

func TestCycleBreaker_ToolLane_BudgetExceededWithRepetition(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:             &enabled,
		MaxToolTokens:       50,
		RepetitionWindow:    5,
		RepetitionThreshold: 5, // high threshold, won't trip loop
	})

	// Send repeating phrase twice to get max freq >= 2
	phrase := "modify update replace execute apply "
	cb.ProcessToolDelta(phrase)
	cb.ProcessToolDelta(phrase)

	// Send additional unique tokens to breach maxToolTokens (50)
	var sb strings.Builder
	for i := 0; i < 60; i++ {
		sb.WriteString(fmt.Sprintf("uniqueParameterWord%d ", i))
	}
	triggered, reason := cb.ProcessToolDelta(sb.String())
	if !triggered {
		t.Fatalf("expected budget exceeded with repetition to trigger")
	}
	if reason != "tool_budget_exceeded_with_repetition" {
		t.Fatalf("expected tool_budget_exceeded_with_repetition, got %s", reason)
	}
}

func TestCycleBreaker_ToolLane_IsolationAndReset(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled: &enabled,
	})

	cb.ProcessToolDelta("some tool argument delta content")
	if cb.ToolTokens() == 0 {
		t.Errorf("expected ToolTokens > 0")
	}

	cb.Reset()
	if cb.ToolTokens() != 0 {
		t.Errorf("expected ToolTokens == 0 after Reset, got %d", cb.ToolTokens())
	}
	if cb.MaxToolNgramFreq() != 0 {
		t.Errorf("expected MaxToolNgramFreq == 0 after Reset, got %d", cb.MaxToolNgramFreq())
	}

	// Test disabled cb does not process
	disabled := false
	cbDisabled := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled: &disabled,
	})
	trig, r := cbDisabled.ProcessToolDelta("repeat repeat repeat repeat repeat repeat repeat")
	if trig || r != "" {
		t.Errorf("expected disabled cb not to trigger, got %v %s", trig, r)
	}
}

func TestCycleBreaker_ToolLane_TableDrivenTests(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:             &enabled,
		MaxToolTokens:       4096,
		MaxWriteTokens:      32768,
		RepetitionWindow:    6,
		RepetitionThreshold: 3,
	})

	// Realistic Go table-driven unit test file (like N-Queens board_test.go)
	tableTestCode := `package board_test

import (
	"testing"
	"github.com/example/nqueens/pkg/board"
)

func TestSolveNQueens(t *testing.T) {
	tests := []struct {
		name     string
		n        int
		expected int
		hasError bool
	}{
		{name: "n=1 single queen", n: 1, expected: 1, hasError: false},
		{name: "n=2 no solution", n: 2, expected: 0, hasError: false},
		{name: "n=3 no solution", n: 3, expected: 0, hasError: false},
		{name: "n=4 two solutions", n: 4, expected: 2, hasError: false},
		{name: "n=5 ten solutions", n: 5, expected: 10, hasError: false},
		{name: "n=6 four solutions", n: 6, expected: 4, hasError: false},
		{name: "n=7 forty solutions", n: 7, expected: 40, hasError: false},
		{name: "n=8 standard chessboard", n: 8, expected: 92, hasError: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			solutions, err := board.Solve(tc.n)
			if (err != nil) != tc.hasError {
				t.Fatalf("expected error: %v, got: %v", tc.hasError, err)
			}
			if len(solutions) != tc.expected {
				t.Fatalf("expected %d solutions, got %d", tc.expected, len(solutions))
			}
		})
	}
}
`
	// In ToolCategoryFileWrite, table-driven unit tests must NEVER trigger tool repetition!
	triggered, reason := cb.ProcessToolDelta(tableTestCode, ToolCategoryFileWrite)
	if triggered {
		t.Fatalf("expected table-driven unit test to NOT trigger repetition in file write category, but got: %s", reason)
	}
}

func TestCycleBreaker_ToolLane_WindowsPowerShellWrite(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:             &enabled,
		MaxToolTokens:       4096,
		MaxWriteTokens:      32768,
		RepetitionWindow:    6,
		RepetitionThreshold: 3,
	})

	// PowerShell script with repetitive assertions/objects
	psContent := `$tests = @(
		@{ Name = "Test1"; Status = "Pass"; Count = 10; Enabled = $true },
		@{ Name = "Test2"; Status = "Pass"; Count = 20; Enabled = $true },
		@{ Name = "Test3"; Status = "Pass"; Count = 30; Enabled = $true },
		@{ Name = "Test4"; Status = "Pass"; Count = 40; Enabled = $true }
	)
	$tests | ForEach-Object {
		Assert-MockCalled -CommandName Write-Host -Times $_.Count
	}
`
	triggered, reason := cb.ProcessToolDelta(psContent, ToolCategoryFileWrite)
	if triggered {
		t.Fatalf("expected PowerShell test write to NOT trigger repetition in file write category, got: %s", reason)
	}
}

func TestCycleBreaker_ToolLane_LargeSourceFile_32kCeiling(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:        &enabled,
		MaxToolTokens:  4096,
		MaxWriteTokens: 32768,
	})

	// Generate ~10,000 tokens (~40,000 bytes) of file write content
	var largeSb strings.Builder
	for i := 0; i < 500; i++ {
		largeSb.WriteString(fmt.Sprintf("func HandleItem%d(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, \"item %d\") }\n", i, i))
	}
	largeCode := largeSb.String()

	triggered, reason := cb.ProcessToolDelta(largeCode, ToolCategoryFileWrite)
	if triggered {
		t.Fatalf("expected 10k token file write to pass under 32k ceiling, got: %s", reason)
	}

	// Exceed 32,768 tokens (~131,072 bytes)
	var runawaySb strings.Builder
	for i := 0; i < 3000; i++ {
		runawaySb.WriteString(fmt.Sprintf("func RunawayItem%d(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, \"item %d\") }\n", i, i))
	}
	triggered, reason = cb.ProcessToolDelta(runawaySb.String(), ToolCategoryFileWrite)
	if !triggered {
		t.Fatalf("expected runaway file write exceeding 32k ceiling to trigger")
	}
	if reason != "write_budget_exceeded" {
		t.Fatalf("expected reason 'write_budget_exceeded', got: %s", reason)
	}
}

func TestCycleBreaker_ToolLane_DegenerateCommandLoop_Severed(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:             &enabled,
		MaxToolTokens:       4096,
		RepetitionWindow:    6,
		RepetitionThreshold: 3,
	})

	// Degenerate repeating pipeline in command category
	pipeline := "echo test | sed 's/test/prod/' | awk '{print $1}' && "
	cb.ProcessToolDelta(pipeline, ToolCategoryCommand)
	cb.ProcessToolDelta(pipeline, ToolCategoryCommand)
	triggered, reason := cb.ProcessToolDelta(pipeline, ToolCategoryCommand)

	if !triggered {
		t.Fatalf("expected degenerate repeating command pipeline to be caught in command category")
	}
	if reason != "tool_repetition_loop_detected" {
		t.Fatalf("expected tool_repetition_loop_detected, got: %s", reason)
	}
}

func TestCycleBreaker_ToolLane_ValidTestCommand_NotTriggered(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:             &enabled,
		MaxToolTokens:       4096,
		RepetitionWindow:    6,
		RepetitionThreshold: 3,
	})

	commands := []string{
		"go test -v -race ./pkg/board/...",
		"dotnet test --filter FullyQualifiedName~NQueens.Tests",
		"swift test --filter BoardTests",
		"mvn test -Dtest=BoardTest",
		"cargo test --package nqueens --test board_test",
	}

	for _, cmd := range commands {
		triggered, reason := cb.ProcessToolDelta(cmd, ToolCategoryCommand)
		if triggered {
			t.Fatalf("expected valid command %q to not trigger, got %s", cmd, reason)
		}
	}
}

func TestCycleBreaker_ProcessToolDelta_FileWrite_ZeroAllocs(t *testing.T) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:        &enabled,
		MaxToolTokens:  4096,
		MaxWriteTokens: 32768,
	})

	chunk := `{"name": "board_test.go", "content": "func TestBoard(t *testing.T) { ... }"}`
	allocs := testing.AllocsPerRun(1000, func() {
		cb.ProcessToolDelta(chunk, ToolCategoryFileWrite)
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocs on ToolCategoryFileWrite fast path, got %f", allocs)
	}
}

func BenchmarkCycleBreaker_ProcessToolDelta_FileWrite(b *testing.B) {
	enabled := true
	cb := NewCycleBreaker(&contract.CycleBreakerConfig{
		Enabled:        &enabled,
		MaxToolTokens:  4096,
		MaxWriteTokens: 32768000,
	})

	chunk := `{"name": "board_test.go", "content": "func TestBoard(t *testing.T) { ... }"}`
	b.ReportAllocs()
	for b.Loop() {
		cb.ProcessToolDelta(chunk, ToolCategoryFileWrite)
	}
}

func TestCycleBreaker_PoolAcquireRelease_CleanState(t *testing.T) {
	enabled := true
	cfg1 := &contract.CycleBreakerConfig{
		Enabled:        &enabled,
		MaxProseTokens: 1000,
		MaxWriteTokens: 20000,
	}

	cb1 := GetCycleBreaker(cfg1)
	cb1.ProcessDelta("word1 word2 word3 word4 word5 word6", false)
	cb1.ProcessDelta("thinking step 1 2 3 4 5", true)
	cb1.ProcessToolDelta("cmd arg1 arg2", ToolCategoryCommand)

	if cb1.ProseTokens() == 0 || cb1.ThinkingTokens() == 0 || cb1.ToolTokens() == 0 {
		t.Fatalf("expected non-zero counters before release")
	}

	PutCycleBreaker(cb1)

	// Re-acquire from pool with different config
	disabled := false
	cfg2 := &contract.CycleBreakerConfig{
		Enabled:        &disabled,
		MaxProseTokens: 500,
		MaxWriteTokens: 16000,
	}
	cb2 := GetCycleBreaker(cfg2)
	defer PutCycleBreaker(cb2)

	if cb2.IsEnabled() {
		t.Errorf("expected cb2.IsEnabled() == false")
	}
	if cb2.MaxWriteTokens() != 16000 {
		t.Errorf("expected MaxWriteTokens == 16000, got %d", cb2.MaxWriteTokens())
	}
	if cb2.ProseTokens() != 0 {
		t.Errorf("expected ProseTokens == 0 after pool reset, got %d", cb2.ProseTokens())
	}
	if cb2.ThinkingTokens() != 0 {
		t.Errorf("expected ThinkingTokens == 0 after pool reset, got %d", cb2.ThinkingTokens())
	}
	if cb2.ToolTokens() != 0 {
		t.Errorf("expected ToolTokens == 0 after pool reset, got %d", cb2.ToolTokens())
	}
	if cb2.MaxNgramFreq() != 0 {
		t.Errorf("expected MaxNgramFreq == 0 after pool reset, got %d", cb2.MaxNgramFreq())
	}
}

func TestCycleBreaker_Reset_ZeroAllocations(t *testing.T) {
	enabled := true
	cb := GetCycleBreaker(&contract.CycleBreakerConfig{Enabled: &enabled})
	defer PutCycleBreaker(cb)

	// Populate entries in all lanes
	cb.ProcessDelta("alpha beta gamma delta epsilon zeta eta theta", false)
	cb.ProcessDelta("think one think two think three think four", true)
	cb.ProcessToolDelta("exec command arg1 arg2 arg3", ToolCategoryCommand)

	allocs := testing.AllocsPerRun(1000, func() {
		cb.Reset()
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocs on cb.Reset() in-place map clear, got %f", allocs)
	}
}

func TestCycleBreaker_PutCycleBreaker_NilSafe(t *testing.T) {
	// Should not panic on nil
	PutCycleBreaker(nil)
}

func BenchmarkCycleBreaker_PoolAcquireRelease(b *testing.B) {
	enabled := true
	cfg := &contract.CycleBreakerConfig{
		Enabled:        &enabled,
		MaxProseTokens: 4096,
		MaxWriteTokens: 32768,
	}

	b.ReportAllocs()
	for b.Loop() {
		cb := GetCycleBreaker(cfg)
		PutCycleBreaker(cb)
	}
}
