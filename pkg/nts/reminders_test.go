package nts_test

import (
	"strings"
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/nts"
)

const sampleTable1 = `| # | Task | Status |
|---|------|--------|
| 1 | Setup project | Pending |
| 2 | Add router | Pending |`

const sampleTable2 = `| # | Task | Status |
|---|------|--------|
| 1 | Setup project | Done |
| 2 | Add router | Pending |`

func buildEnvText(table string) string {
	return `<environment_details>
# Current Time
2026-10-05T12:00:00Z

# Current Mode
<slug>code</slug>
====

REMINDERS

` + table + `
</environment_details>`
}

func TestDeduplicateRemindersContext_Basic(t *testing.T) {
	ctx := nts.NewDeduplicateRemindersContext(10)

	msg1 := buildEnvText(sampleTable1)
	msg2 := buildEnvText(sampleTable1)
	msg3 := buildEnvText(sampleTable1)

	// Turn 0: Baseline emit
	out1, _, modified1 := ctx.CompactReminders(msg1)
	if modified1 {
		t.Fatalf("expected Turn 0 to emit full baseline table, but was modified")
	}
	if out1 != msg1 {
		t.Fatalf("expected output identical to input on Turn 0")
	}

	// Turn 1: Duplicate table -> tombstone
	out2, res2, modified2 := ctx.CompactReminders(msg2)
	if !modified2 {
		t.Fatalf("expected Turn 1 to be deduplicated")
	}
	if !strings.Contains(out2, nts.RemindersUnchangedNotice) {
		t.Errorf("expected tombstone in Turn 1 output: %s", out2)
	}
	if strings.Contains(out2, "Setup project") {
		t.Errorf("expected raw table content removed in Turn 1")
	}
	if res2.BytesSaved <= 0 || res2.TokensSaved <= 0 {
		t.Errorf("expected positive savings, got: %+v", res2)
	}

	// Turn 2: Another duplicate -> tombstone
	out3, _, modified3 := ctx.CompactReminders(msg3)
	if !modified3 {
		t.Fatalf("expected Turn 2 to be deduplicated")
	}
	if !strings.Contains(out3, nts.RemindersUnchangedNotice) {
		t.Errorf("expected tombstone in Turn 2 output")
	}
}

func TestDeduplicateRemindersContext_TableMutation(t *testing.T) {
	ctx := nts.NewDeduplicateRemindersContext(10)

	msg1 := buildEnvText(sampleTable1)
	msg2 := buildEnvText(sampleTable1)
	msgMutated := buildEnvText(sampleTable2)
	msgAfterMutated := buildEnvText(sampleTable2)

	// Turn 0: Table 1 baseline
	ctx.CompactReminders(msg1)

	// Turn 1: Table 1 duplicate -> tombstone
	_, _, mod1 := ctx.CompactReminders(msg2)
	if !mod1 {
		t.Fatalf("expected Turn 1 to be deduplicated")
	}

	// Turn 2: Table mutated -> MUST emit full table!
	outMut, _, modMut := ctx.CompactReminders(msgMutated)
	if modMut {
		t.Fatalf("expected mutated table on Turn 2 to emit full table, but was modified")
	}
	if !strings.Contains(outMut, "Done") {
		t.Errorf("expected mutated table content preserved")
	}

	// Turn 3: Duplicate of mutated Table 2 -> tombstone
	outAfter, _, modAfter := ctx.CompactReminders(msgAfterMutated)
	if !modAfter {
		t.Fatalf("expected Turn 3 to deduplicate against mutated table")
	}
	if !strings.Contains(outAfter, nts.RemindersUnchangedNotice) {
		t.Errorf("expected tombstone in Turn 3 output")
	}
}

func TestDeduplicateRemindersContext_RefreshInterval(t *testing.T) {
	// Refresh interval K = 3:
	// Turn 0: Full
	// Turn 1: Tombstone (occurrences = 1)
	// Turn 2: Tombstone (occurrences = 2)
	// Turn 3: Full Refresh (occurrences = 0)
	// Turn 4: Tombstone (occurrences = 1)
	ctx := nts.NewDeduplicateRemindersContext(3)
	msg := buildEnvText(sampleTable1)

	// Turn 0: Full
	_, _, mod0 := ctx.CompactReminders(msg)
	if mod0 {
		t.Fatalf("Turn 0 should be full")
	}

	// Turn 1: Tombstone
	_, _, mod1 := ctx.CompactReminders(msg)
	if !mod1 {
		t.Fatalf("Turn 1 should be tombstone")
	}

	// Turn 2: Tombstone
	_, _, mod2 := ctx.CompactReminders(msg)
	if !mod2 {
		t.Fatalf("Turn 2 should be tombstone")
	}

	// Turn 3: Full refresh
	out3, _, mod3 := ctx.CompactReminders(msg)
	if mod3 {
		t.Fatalf("Turn 3 should be full refresh after K=3 turns, but was modified")
	}
	if !strings.Contains(out3, "Setup project") {
		t.Errorf("expected full table content in Turn 3 refresh")
	}

	// Turn 4: Tombstone again
	_, _, mod4 := ctx.CompactReminders(msg)
	if !mod4 {
		t.Fatalf("Turn 4 should be tombstone after refresh")
	}
}

func TestDeduplicateRemindersContext_NoEnvironmentDetails(t *testing.T) {
	ctx := nts.NewDeduplicateRemindersContext(10)

	regularText := "Just a normal user message asking to edit a file."
	out, _, mod := ctx.CompactReminders(regularText)
	if mod || out != regularText {
		t.Errorf("regular message should pass through untouched")
	}

	unclosed := "<environment_details>REMINDERS\n| # |"
	outUnclosed, _, modUnclosed := ctx.CompactReminders(unclosed)
	if modUnclosed || outUnclosed != unclosed {
		t.Errorf("unclosed tag should pass through untouched")
	}
}

func TestDeduplicateRemindersContext_EdgeCases(t *testing.T) {
	// Default refresh interval when <= 0
	ctxDef := nts.NewDeduplicateRemindersContext(0)
	if ctxDef == nil {
		t.Fatalf("expected non-nil context")
	}

	// Nil context
	var nilCtx *nts.DeduplicateRemindersContext
	outNil, _, modNil := nilCtx.CompactReminders("hello")
	if modNil || outNil != "hello" {
		t.Errorf("expected nil context to bypass")
	}

	// Environment details without REMINDERS header
	noHeader := "<environment_details>\n# Current Time\n2026-10-05\n</environment_details>"
	outNoHeader, _, modNoHeader := ctxDef.CompactReminders(noHeader)
	if modNoHeader || outNoHeader != noHeader {
		t.Errorf("environment details without REMINDERS should bypass")
	}
}

