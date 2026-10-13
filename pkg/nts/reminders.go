package nts

import (
	"strings"
)

// RemindersUnchangedNotice is the contextual, non-positional tombstone injected
// in place of repetitive, byte-identical REMINDERS tables.
const RemindersUnchangedNotice = "\n\n[REMINDERS unchanged — same as the last full list above]\n"

const (
	envDetailsOpenTag  = "<environment_details>"
	envDetailsCloseTag = "</environment_details>"
	remindersMarker    = "REMINDERS"
)

// DeduplicateRemindersContext tracks prefix-causal state across chronological messages
// within a single request execution. It is strictly local to the request to ensure zero
// shared-state concurrency and zero cross-request race conditions.
type DeduplicateRemindersContext struct {
	LastReminderBytes    string
	OccurrencesSinceFull int
	RefreshInterval      int
}

// NewDeduplicateRemindersContext creates a fresh context for traversing messages within a request.
func NewDeduplicateRemindersContext(refreshInterval int) *DeduplicateRemindersContext {
	if refreshInterval <= 0 {
		refreshInterval = 10
	}
	return &DeduplicateRemindersContext{
		RefreshInterval: refreshInterval,
	}
}

// CompactReminders inspects text for an <environment_details> block containing REMINDERS.
// If the table matches the previous full table and the refresh interval has not elapsed,
// it replaces the table with RemindersUnchangedNotice.
func (ctx *DeduplicateRemindersContext) CompactReminders(text string) (string, ReductionResult, bool) {
	if ctx == nil {
		return text, ReductionResult{}, false
	}

	openIdx := strings.Index(text, envDetailsOpenTag)
	if openIdx == -1 {
		return text, ReductionResult{}, false
	}

	closeIdx := strings.Index(text[openIdx:], envDetailsCloseTag)
	if closeIdx == -1 {
		return text, ReductionResult{}, false
	}
	closeIdx += openIdx

	envContent := text[openIdx+len(envDetailsOpenTag) : closeIdx]
	remIdx := strings.Index(envContent, remindersMarker)
	if remIdx == -1 {
		return text, ReductionResult{}, false
	}

	// Advance past the REMINDERS header line
	newlineRel := strings.IndexByte(envContent[remIdx:], '\n')
	if newlineRel == -1 {
		return text, ReductionResult{}, false
	}
	tableStartInEnv := remIdx + newlineRel + 1

	rawTable := envContent[tableStartInEnv:]
	trimmedTable := strings.TrimRight(rawTable, " \t\r\n")
	if len(trimmedTable) == 0 {
		return text, ReductionResult{}, false
	}

	// Compare against the previous raw table
	if ctx.LastReminderBytes != "" && trimmedTable == ctx.LastReminderBytes && ctx.OccurrencesSinceFull < ctx.RefreshInterval {
		// Replace table with contextual tombstone
		var sb strings.Builder
		sb.Grow(len(text) - len(rawTable) + len(RemindersUnchangedNotice))
		sb.WriteString(text[:openIdx+len(envDetailsOpenTag)+tableStartInEnv])
		sb.WriteString(RemindersUnchangedNotice)
		sb.WriteString(text[closeIdx:])

		origBytes := len(text)
		reducedText := sb.String()
		reducedBytes := len(reducedText)
		savedBytes := origBytes - reducedBytes
		if savedBytes < 0 {
			savedBytes = 0
		}

		ctx.OccurrencesSinceFull++
		return reducedText, ReductionResult{
			OriginalBytes: origBytes,
			ReducedBytes:  reducedBytes,
			BytesSaved:    savedBytes,
			TokensSaved:   (savedBytes + 3) / 4,
		}, true
	}

	// Baseline table or refresh interval elapsed: update state and emit full table
	ctx.LastReminderBytes = trimmedTable
	ctx.OccurrencesSinceFull = 1
	return text, ReductionResult{}, false
}
