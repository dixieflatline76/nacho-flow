package shield

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTailBuffer_CapacityAndWrapAround(t *testing.T) {
	t.Run("Pool Get and Put", func(t *testing.T) {
		tb := GetTailBuffer()
		if tb == nil {
			t.Fatal("expected non-nil from pool")
		}
		tb.Append([]byte("pool data"))
		PutTailBuffer(tb)
		PutTailBuffer(nil) // safe no-op

		tb2 := GetTailBuffer()
		if tb2.Bytes() != nil {
			t.Fatal("expected reset buffer from pool")
		}
		PutTailBuffer(tb2)
	})

	t.Run("Empty Buffer", func(t *testing.T) {
		tb := NewTailBuffer(0) // tests default capacity fallback
		if tb.capacity != 256 {
			t.Fatalf("expected capacity 256, got %d", tb.capacity)
		}
		tb.Append(nil) // safe no-op
		if tb.Bytes() != nil {
			t.Fatalf("expected nil for empty buffer, got %s", string(tb.Bytes()))
		}
	})

	t.Run("Append less than capacity", func(t *testing.T) {
		tb := NewTailBuffer(64)
		input := []byte("hello world")
		tb.Append(input)

		out := tb.Bytes()
		if string(out) != "hello world" {
			t.Fatalf("expected 'hello world', got '%s'", string(out))
		}
	})

	t.Run("Append exact capacity", func(t *testing.T) {
		tb := NewTailBuffer(10)
		input := []byte("0123456789")
		tb.Append(input)

		out := tb.Bytes()
		if string(out) != "0123456789" {
			t.Fatalf("expected '0123456789', got '%s'", string(out))
		}
	})

	t.Run("Append with circular wrap-around", func(t *testing.T) {
		tb := NewTailBuffer(10)
		tb.Append([]byte("0123456789"))
		tb.Append([]byte("ABC")) // Overwrites "012", resulting in "3456789ABC"

		out := tb.Bytes()
		if string(out) != "3456789ABC" {
			t.Fatalf("expected '3456789ABC', got '%s'", string(out))
		}
	})

	t.Run("Multiple streaming chunks wrap-around", func(t *testing.T) {
		tb := NewTailBuffer(16)
		chunks := [][]byte{
			[]byte("First sentence. "),
			[]byte("Second sentence. "),
			[]byte("Are you satisfied?"),
		}
		for _, chunk := range chunks {
			tb.Append(chunk)
		}

		out := tb.Bytes()
		expected := "e you satisfied?" // Last 16 bytes of the full concatenated text
		if string(out) != expected {
			t.Fatalf("expected '%s', got '%s'", expected, string(out))
		}
	})

	t.Run("Reset clears buffer", func(t *testing.T) {
		tb := NewTailBuffer(32)
		tb.Append([]byte("some data"))
		tb.Reset()
		if tb.Bytes() != nil {
			t.Fatalf("expected nil after reset, got %s", string(tb.Bytes()))
		}
	})
}

func TestRuleEngine_Evaluations(t *testing.T) {
	questions := []string{
		"are you satisfied",
		"would you like",
		"should i",
		"do you approve",
		"please confirm",
		"let me know if",
		"how would you like to proceed",
	}
	modes := []string{
		"switch to code mode",
		"switch to architect mode",
		"ready to implement",
	}

	engine := NewRuleEngine(questions, modes)

	t.Run("Empty tail", func(t *testing.T) {
		matched, _ := engine.Evaluate(nil)
		if matched {
			t.Fatal("expected false for nil tail")
		}
		matched, _ = engine.Evaluate([]byte("   \n\t  "))
		if matched {
			t.Fatal("expected false for whitespace tail")
		}
	})

	t.Run("Default RuleEngine initialization", func(t *testing.T) {
		defEngine := NewRuleEngine(nil, nil)
		matched, intent := defEngine.Evaluate([]byte("are you satisfied with the result?"))
		if !matched || intent != "question" {
			t.Fatalf("expected question match on default engine, got %v, %s", matched, intent)
		}
	})

	t.Run("Trailing question mark", func(t *testing.T) {
		matched, intent := engine.Evaluate([]byte("Should we proceed with this architecture?"))
		if !matched || intent != "question" {
			t.Fatalf("expected matched=true, intent='question', got matched=%v, intent='%s'", matched, intent)
		}
	})

	t.Run("Trailing question mark with whitespace", func(t *testing.T) {
		matched, intent := engine.Evaluate([]byte("Can you confirm?  \n\n"))
		if !matched || intent != "question" {
			t.Fatalf("expected matched=true, intent='question', got matched=%v, intent='%s'", matched, intent)
		}
	})

	t.Run("Phrase match without trailing question mark", func(t *testing.T) {
		matched, intent := engine.Evaluate([]byte("Let me know if you would like any modifications before we begin."))
		if !matched || intent != "question" {
			t.Fatalf("expected matched=true, intent='question', got matched=%v, intent='%s'", matched, intent)
		}
	})

	t.Run("Mode switch phrase match", func(t *testing.T) {
		matched, intent := engine.Evaluate([]byte("Plan approved. I am now ready to implement."))
		if !matched || intent != "mode_switch" {
			t.Fatalf("expected matched=true, intent='mode_switch', got matched=%v, intent='%s'", matched, intent)
		}
	})

	t.Run("False Positive Guards - Normal Completions", func(t *testing.T) {
		nonQuestions := [][]byte{
			[]byte("I have completed the task and verified tests pass."),
			[]byte("```go\nfunc main() {\n  fmt.Println(\"ok\")\n}\n```"),
			[]byte("The files were created at /path/to/project."),
			[]byte("Error code 404: Not Found"),
		}

		for _, nq := range nonQuestions {
			matched, intent := engine.Evaluate(nq)
			if matched {
				t.Fatalf("expected no match for '%s', got matched=true with intent='%s'", string(nq), intent)
			}
		}
	})
}

func TestStrategy_SynthesizeArgs(t *testing.T) {
	t.Run("AskFollowupStrategy serialization", func(t *testing.T) {
		strat := &AskFollowupStrategy{Name: "ask_followup_question"}
		if strat.ToolName() != "ask_followup_question" {
			t.Fatalf("expected 'ask_followup_question', got '%s'", strat.ToolName())
		}

		// Empty name default
		emptyStrat := &AskFollowupStrategy{}
		if emptyStrat.ToolName() != "ask_followup_question" {
			t.Fatalf("expected default 'ask_followup_question', got '%s'", emptyStrat.ToolName())
		}

		content := "Are you satisfied with the N-Queens technical blueprint?"
		args, err := strat.SynthesizeArgs(content)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(args), &parsed); err != nil {
			t.Fatalf("failed to unmarshal synthesized args: %v", err)
		}

		if parsed["question"] != content {
			t.Fatalf("expected question to match content, got: %s", parsed["question"])
		}

		callID := strat.GenerateCallID(content)
		if !strings.HasPrefix(callID, "call_autowrap_") {
			t.Fatalf("expected call_autowrap_ prefix, got: %s", callID)
		}
	})

	t.Run("ModeSwitchStrategy serialization", func(t *testing.T) {
		strat := &ModeSwitchStrategy{Name: "switch_mode"}
		if strat.ToolName() != "switch_mode" {
			t.Fatalf("expected 'switch_mode', got '%s'", strat.ToolName())
		}

		// Empty name default
		emptyMode := &ModeSwitchStrategy{}
		if emptyMode.ToolName() != "switch_mode" {
			t.Fatalf("expected default 'switch_mode', got '%s'", emptyMode.ToolName())
		}

		content := "Ready to implement in code mode"
		args, err := strat.SynthesizeArgs(content)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var parsed map[string]string
		if err := json.Unmarshal([]byte(args), &parsed); err != nil {
			t.Fatalf("failed to unmarshal synthesized args: %v", err)
		}

		if parsed["mode_slug"] != "code" || parsed["reason"] != content {
			t.Fatalf("unexpected mode switch args: %v", parsed)
		}
	})
}

func TestShieldManager_ProcessTurn(t *testing.T) {
	mgr := NewDefaultShieldManager()
	mgr.RegisterStrategy(nil) // safe no-op

	if mgr.RuleEngine() == nil {
		t.Fatal("expected non-nil RuleEngine")
	}

	t.Run("Bailout when client declares no interactive tool", func(t *testing.T) {
		content := "Are you satisfied with this plan?"
		call, ok := mgr.EvaluateAndSynthesize(content, "")
		if ok || call != nil {
			t.Fatalf("expected bailout when interactive tool is empty, got call=%v", call)
		}
	})

	t.Run("Bailout when content is empty", func(t *testing.T) {
		call, ok := mgr.EvaluateAndSynthesize("", "ask_followup_question")
		if ok || call != nil {
			t.Fatalf("expected bailout when content is empty, got call=%v", call)
		}
	})

	t.Run("Bailout when content is pure completion with no question", func(t *testing.T) {
		call, ok := mgr.EvaluateAndSynthesize("All unit tests have passed successfully.", "ask_followup_question")
		if ok || call != nil {
			t.Fatalf("expected bailout for non-question completion, got call=%v", call)
		}
	})

	t.Run("Synthesizes ask_followup_question on trailing question", func(t *testing.T) {
		content := "I have generated the plan. Are you satisfied with this architecture?"
		call, ok := mgr.EvaluateAndSynthesize(content, "ask_followup_question")
		if !ok || call == nil {
			t.Fatal("expected successful tool call synthesis")
		}

		if call.Function.Name != "ask_followup_question" {
			t.Fatalf("expected function name 'ask_followup_question', got '%s'", call.Function.Name)
		}

		var args map[string]interface{}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			t.Fatalf("invalid json args: %v", err)
		}
		if args["question"] != content {
			t.Fatalf("expected question argument to equal content, got: %s", args["question"])
		}
	})

	t.Run("Fallback strategy for custom un-registered tool", func(t *testing.T) {
		content := "Do you approve of these changes?"
		call, ok := mgr.EvaluateAndSynthesize(content, "custom_ask_tool")
		if !ok || call == nil {
			t.Fatal("expected successful tool call synthesis for custom tool")
		}
		if call.Function.Name != "custom_ask_tool" {
			t.Fatalf("expected custom tool name, got '%s'", call.Function.Name)
		}
	})

	t.Run("Long content tail evaluation (>512 bytes)", func(t *testing.T) {
		longContent := strings.Repeat("Detailed planning specification paragraph. ", 30) + "\nAre you satisfied with this plan?"
		call, ok := mgr.EvaluateAndSynthesize(longContent, "ask_followup_question")
		if !ok || call == nil {
			t.Fatal("expected successful synthesis for long content")
		}
	})
}

// Benchmarks
func BenchmarkTailBuffer_Append(b *testing.B) {
	tb := NewTailBuffer(256)
	chunk := []byte("data: {\"choices\":[{\"delta\":{\"content\":\" streaming tokens flowing through proxy \"}}]}\n\n")
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		tb.Append(chunk)
	}
}

func BenchmarkRuleEngine_Evaluate(b *testing.B) {
	questions := []string{
		"are you satisfied", "would you like", "should i",
		"do you approve", "please confirm", "let me know if",
	}
	engine := NewRuleEngine(questions, nil)
	tail := []byte("I have summarized the technical design. Are you satisfied with this plan?")

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		engine.Evaluate(tail)
	}
}
