package nts_test

import (
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/nts"
)

func TestCollapseVerboseTests_Go(t *testing.T) {
	input := `=== RUN   TestPassingA
--- PASS: TestPassingA (0.00s)
=== RUN   TestPassingB
=== RUN   TestPassingB/Sub1
    --- PASS: TestPassingB/Sub1 (0.00s)
--- PASS: TestPassingB (0.00s)
=== RUN   TestCrasher
panic: runtime error: invalid memory address
goroutine 1 [running]:
FAIL	mypackage	0.012s
=== RUN   TestFailing
--- FAIL: TestFailing (0.01s)
    main_test.go:42: assertion failed
FAIL
`
	out := nts.CollapseVerboseTestsInPlace([]byte(input))
	outStr := string(out)

	// Passing test RUN lines must be dropped
	if strings.Contains(outStr, "=== RUN   TestPassingA") {
		t.Errorf("expected === RUN TestPassingA to be dropped: %s", outStr)
	}
	if strings.Contains(outStr, "=== RUN   TestPassingB/Sub1") {
		t.Errorf("expected === RUN TestPassingB/Sub1 to be dropped")
	}

	// Passing PASS markers must be preserved
	if !strings.Contains(outStr, "--- PASS: TestPassingA") {
		t.Errorf("expected --- PASS: TestPassingA to be preserved")
	}
	if !strings.Contains(outStr, "--- PASS: TestPassingB/Sub1") {
		t.Errorf("expected subtest PASS to be preserved")
	}

	// CRITICAL INVARIANT: Crashed test without matching PASS must preserve its === RUN line
	if !strings.Contains(outStr, "=== RUN   TestCrasher") {
		t.Errorf("expected === RUN TestCrasher to be PRESERVED due to panic: %s", outStr)
	}
	if !strings.Contains(outStr, "panic: runtime error") {
		t.Errorf("expected panic message to be preserved")
	}

	// Failing test must preserve --- FAIL and assertion output
	if !strings.Contains(outStr, "--- FAIL: TestFailing") || !strings.Contains(outStr, "assertion failed") {
		t.Errorf("expected failure and assertion trace to be preserved")
	}
}

func TestCollapseVerboseTests_Cargo(t *testing.T) {
	input := `running 4 tests
test board::tests::test_new_board ... ok
test board::tests::test_is_safe ... ok
test board::tests::test_failing ... FAILED
test board::tests::test_ignored ... ignored

failures:

---- board::tests::test_failing stdout ----
thread 'board::tests::test_failing' panicked at 'assertion failed: 5 == 8'

failures:
    board::tests::test_failing

test result: FAILED. 2 passed; 1 failed; 1 ignored; 0 measured; 0 filtered out; finished in 0.05s
`
	out := nts.CollapseVerboseTestsInPlace([]byte(input))
	outStr := string(out)

	if strings.Contains(outStr, "test board::tests::test_new_board ... ok") {
		t.Errorf("expected passing cargo test to be dropped: %s", outStr)
	}
	if strings.Contains(outStr, "test board::tests::test_is_safe ... ok") {
		t.Errorf("expected passing cargo test to be dropped")
	}

	if !strings.Contains(outStr, "test board::tests::test_failing ... FAILED") {
		t.Errorf("expected failing cargo test to be preserved")
	}
	if !strings.Contains(outStr, "panicked at 'assertion failed: 5 == 8'") {
		t.Errorf("expected panic assertion to be preserved")
	}
	if !strings.Contains(outStr, "test result: FAILED. 2 passed; 1 failed") {
		t.Errorf("expected cargo test result summary to be preserved")
	}
}

func TestCollapseVerboseTests_Pytest(t *testing.T) {
	input := `tests/test_board.py::test_new_board PASSED               [ 25%]
tests/test_board.py::test_is_safe PASSED                 [ 50%]
tests/test_board.py::test_conflict FAILED                [ 75%]
tests/test_board.py::test_skip SKIPPED                   [100%]

=================================== FAILURES ===================================
________________________________ test_conflict _________________________________
assert 5 == 8
=========================== short test summary info ============================
FAILED tests/test_board.py::test_conflict - assert 5 == 8
========================= 1 failed, 2 passed in 0.24s ==========================
`
	out := nts.CollapseVerboseTestsInPlace([]byte(input))
	outStr := string(out)

	if strings.Contains(outStr, "tests/test_board.py::test_new_board PASSED") {
		t.Errorf("expected passing pytest line to be dropped: %s", outStr)
	}
	if strings.Contains(outStr, "tests/test_board.py::test_is_safe PASSED") {
		t.Errorf("expected passing pytest line to be dropped")
	}

	if !strings.Contains(outStr, "tests/test_board.py::test_conflict FAILED") {
		t.Errorf("expected failing pytest test to be preserved")
	}
	if !strings.Contains(outStr, "assert 5 == 8") {
		t.Errorf("expected assertion failure trace to be preserved")
	}
	if !strings.Contains(outStr, "1 failed, 2 passed in 0.24s") {
		t.Errorf("expected pytest summary to be preserved")
	}
}

func TestCollapseVerboseTests_Jest(t *testing.T) {
	input := ` PASS  src/board.test.ts
    ✓ should initialize board (12 ms)
    ✓ should place queen safely (4 ms)

 FAIL  src/solver.test.ts
    ✓ should solve small board (8 ms)
    ✕ should solve 8-queens (25 ms)

  ● Solver › should solve 8-queens
    expect(received).toBe(expected)

Test Suites: 1 failed, 1 passed, 2 total
Tests:       1 failed, 3 passed, 4 total
`
	out := nts.CollapseVerboseTestsInPlace([]byte(input))
	outStr := string(out)

	if strings.Contains(outStr, "✓ should initialize board") {
		t.Errorf("expected passing jest assertions to be dropped: %s", outStr)
	}
	if strings.Contains(outStr, "✓ should place queen safely") {
		t.Errorf("expected passing jest assertions to be dropped")
	}

	if !strings.Contains(outStr, "✕ should solve 8-queens") {
		t.Errorf("expected failed jest assertion to be preserved")
	}
	if !strings.Contains(outStr, "expect(received).toBe(expected)") {
		t.Errorf("expected diff to be preserved")
	}
	if !strings.Contains(outStr, "Tests:       1 failed, 3 passed, 4 total") {
		t.Errorf("expected jest summary to be preserved")
	}
}

func TestCollapseVerboseTests_Bypass(t *testing.T) {
	nonTest := "commit eae9473\nAuthor: Karl\nDate: Mon Oct 5\n\n    fix(server): great feature\n"
	out := nts.CollapseVerboseTestsInPlace([]byte(nonTest))
	if string(out) != nonTest {
		t.Errorf("non-test output should pass through byte-identical")
	}
}

func BenchmarkTestCollapse_Bypass(b *testing.B) {
	data := []byte("On branch main\nYour branch is up to date with 'origin/main'.\nnothing to commit, working tree clean\n")
	b.ReportAllocs()
	for b.Loop() {
		_ = nts.CollapseVerboseTestsInPlace(data)
	}
}

func BenchmarkTestCollapse_Cargo(b *testing.B) {
	var sb strings.Builder
	sb.WriteString("running 50 tests\n")
	for i := 0; i < 50; i++ {
		sb.WriteString("test module::tests::test_num_")
		sb.WriteString(string(rune('A' + (i % 26))))
		sb.WriteString(" ... ok\n")
	}
	sb.WriteString("test result: ok. 50 passed; 0 failed\n")
	raw := []byte(sb.String())
	buf := make([]byte, len(raw))
	b.ReportAllocs()
	for b.Loop() {
		copy(buf, raw)
		_ = nts.CollapseVerboseTestsInPlace(buf)
	}
}

func BenchmarkTestCollapse_Go(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 50; i++ {
		sb.WriteString("=== RUN   TestNum")
		sb.WriteString(string(rune('A' + (i % 26))))
		sb.WriteString("\n--- PASS: TestNum")
		sb.WriteString(string(rune('A' + (i % 26))))
		sb.WriteString(" (0.00s)\n")
	}
	sb.WriteString("PASS\nok  mypkg 0.123s\n")
	raw := []byte(sb.String())

	buf := make([]byte, len(raw))
	b.ReportAllocs()
	for b.Loop() {
		copy(buf, raw)
		_ = nts.CollapseVerboseTestsInPlace(buf)
	}
}
