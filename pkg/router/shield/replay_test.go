package shield

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestZooCodeNQueensCrashReplay tests that the exact Turn 7 failure payload from Zoo Code v3.80.0
// (where Gemma 4 emitted an N-Queens plan in prose ending with "Are you satisfied with this plan...")
// is successfully rescued and converted into an ask_followup_question tool call.
func TestZooCodeNQueensCrashReplay(t *testing.T) {
	// Exact Turn 7 response text from the real-world Zoo Code crash dump:
	rawAssistantProse := `I have outlined the technical plan for your Go CLI N-Queens solver below. Since we are currently in **Architect** mode, this plan will serve as our blueprint before we move into implementation.

### Technical Plan: ANSI Colored N-Queens Solver

#### 1. Architecture Overview
*   **Project Structure**: A standard Go project layout.
    *   ` + "`main.go`" + `: Handles CLI flag parsing (e.g., -n 8), invokes the solver, and passes results to the renderer.
    *   ` + "`internal/solver`" + `: Contains the backtracking logic.
    *   ` + "`internal/ui`" + `: Contains the ANSI rendering engine for terminal graphics.

#### 2. Solver Logic (` + "`internal/solver`" + `)
*   **Algorithm**: Recursive Backtracking.
*   **Efficiency**: We'll use three boolean arrays to keep track of occupied columns and diagonals (O(1) lookup).
    *   ` + "`cols`" + `: [N] tracks if a column is taken.
    *   ` + "`diag1`" + `: [2N-1] tracks main diagonals.
    *   ` + "`diag2`" + `: [2N-1] tracks anti-diagonals.
*   **Output**: The solver will return a 1D slice []int.

#### 3. Graphics & UI (` + "`internal/ui`" + `)
*   **ANSI Rendering**: We will use standard ANSI escape sequences to provide color without external dependencies.
*   **Colors**: Bright Red for Queens, Blue for board borders, and Gray for empty cells.

#### Updated Todo List:
[x] Project Initialization (Go modules) 
[-] Implement N-Queens Solver logic
[-] Implement CLI argument parsing
[-] Implement ANSI Color Rendering engine
[-] Create sample output and verify colors
[-] Finalize documentation and help messages

**Are you satisfied with this plan, or would you like to make any adjustments before we proceed to the implementation phase in Code Mode?**`

	mgr := NewDefaultShieldManager()

	// Zoo Code declared interactive tool: "ask_followup_question"
	call, ok := mgr.EvaluateAndSynthesize(rawAssistantProse, "ask_followup_question")
	if !ok || call == nil {
		t.Fatalf("CRITICAL: Shield failed to rescue Turn 7 of Zoo Code N-Queens crash trace!")
	}

	if call.Function.Name != "ask_followup_question" {
		t.Fatalf("expected function name 'ask_followup_question', got '%s'", call.Function.Name)
	}

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		t.Fatalf("failed to parse synthesized json args: %v", err)
	}

	q, ok := args["question"].(string)
	if !ok || !strings.Contains(q, "Are you satisfied with this plan") {
		t.Fatalf("expected question argument to contain 'Are you satisfied with this plan', got: %v", args["question"])
	}

	if !strings.HasPrefix(call.ID, "call_autowrap_") {
		t.Fatalf("expected call ID to have deterministic prefix 'call_autowrap_', got: %s", call.ID)
	}
}
