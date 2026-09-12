package nts

import (
	"bytes"
	"strconv"
)

// CollapseDuplicatesInPlace detects consecutive duplicate lines and collapses them
// into a summary marker if the repetition meets or exceeds threshold AND the collapse
// strictly shrinks the output (droppedBytes > noticeLen).
// Operates in-place with zero heap allocations (0 B/op) and guaranteed w <= r.
func CollapseDuplicatesInPlace(b []byte, threshold int) []byte {
	if threshold <= 1 || len(b) == 0 {
		return b
	}

	w := 0
	r := 0
	n := len(b)

	var lastLine []byte
	repeatCount := 1

	for r < n {
		// Find end of current line
		lineStart := r
		lineEnd := r
		for lineEnd < n && b[lineEnd] != '\n' {
			lineEnd++
		}

		hasNewline := lineEnd < n && b[lineEnd] == '\n'
		currLine := b[lineStart:lineEnd]

		// Advance r past current line (and its newline if present)
		if hasNewline {
			r = lineEnd + 1
		} else {
			r = lineEnd
		}

		// Check if current line is identical to lastLine
		// Only consider non-empty lines with meaningful content (> 3 bytes)
		isRepeat := lastLine != nil && len(currLine) > 3 && bytes.Equal(currLine, lastLine)

		if isRepeat {
			repeatCount++
		} else {
			// Process any pending repeats from the previous block
			w = flushRepeats(b, w, lastLine, repeatCount, threshold)

			// Write the current line
			copy(b[w:], currLine)
			w += len(currLine)
			if hasNewline {
				b[w] = '\n'
				w++
			}

			lastLine = currLine
			repeatCount = 1
		}
	}

	// Flush trailing repeats if any
	w = flushRepeats(b, w, lastLine, repeatCount, threshold)

	return b[:w]
}

func flushRepeats(b []byte, w int, line []byte, repeatCount int, threshold int) int {
	if repeatCount <= 1 {
		return w
	}

	collapsedRepeats := repeatCount - 1
	droppedBytes := collapsedRepeats * (len(line) + 1)

	// Calculate notice length without allocations
	var numBuf [16]byte
	numStr := strconv.AppendInt(numBuf[:0], int64(collapsedRepeats), 10)
	noticeLen := len("  [... identical line repeated ") + len(numStr) + len(" times ...]\n")

	// Strict compaction invariant: only emit notice if it strictly shrinks data
	if repeatCount >= threshold && droppedBytes > noticeLen {
		w = appendCollapseNotice(b, w, numStr)
	} else {
		// Re-emit lines that were held back
		for k := 0; k < collapsedRepeats; k++ {
			copy(b[w:], line)
			w += len(line)
			b[w] = '\n'
			w++
		}
	}

	return w
}

func appendCollapseNotice(b []byte, w int, numStr []byte) int {
	prefix := []byte("  [... identical line repeated ")
	suffix := []byte(" times ...]\n")

	copy(b[w:], prefix)
	w += len(prefix)

	copy(b[w:], numStr)
	w += len(numStr)

	copy(b[w:], suffix)
	w += len(suffix)

	return w
}
