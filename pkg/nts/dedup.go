package nts

import (
	"bytes"
	"strconv"
)

const (
	dedupNoticePrefix = "  [... identical line repeated "
	dedupNoticeSuffix = " times ...]"
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
		// Find end of current line using SIMD
		lineStart := r
		idx := bytes.IndexByte(b[r:], '\n')
		var lineEnd int
		hasNewline := idx >= 0
		if hasNewline {
			lineEnd = r + idx
			r = lineEnd + 1
		} else {
			lineEnd = n
			r = n
		}
		currLine := b[lineStart:lineEnd]

		// Check if current line is identical to lastLine.
		// Only consider non-empty lines with meaningful content (> 3 bytes)
		// and at least one alphanumeric character (preserving ASCII art, empty UI boxes, and borders).
		isRepeat := lastLine != nil && len(currLine) > 3 && hasAlphanumeric(currLine) && bytes.Equal(currLine, lastLine)

		if isRepeat {
			repeatCount++
		} else {
			// Process any pending repeats from the previous block
			w = flushRepeats(b, w, lastLine, repeatCount, threshold, true)

			// Write the current line: skip redundant self-copy if no prior compactions shifted cursor
			if w != lineStart {
				copy(b[w:], currLine)
				w += len(currLine)
				if hasNewline {
					b[w] = '\n'
					w++
				}
			} else {
				w += len(currLine)
				if hasNewline {
					w++
				}
			}

			lastLine = currLine
			repeatCount = 1
		}
	}

	// Flush trailing repeats if any
	trailingNewline := n > 0 && b[n-1] == '\n'
	w = flushRepeats(b, w, lastLine, repeatCount, threshold, trailingNewline)

	return b[:w]
}

func flushRepeats(b []byte, w int, line []byte, repeatCount int, threshold int, trailingNewline bool) int {
	if repeatCount <= 1 {
		return w
	}

	collapsedRepeats := repeatCount - 1
	droppedBytes := collapsedRepeats * (len(line) + 1)
	if !trailingNewline {
		droppedBytes = (collapsedRepeats-1)*(len(line)+1) + len(line)
	}

	// Calculate notice length without allocations
	var numBuf [16]byte
	numStr := strconv.AppendInt(numBuf[:0], int64(collapsedRepeats), 10)
	noticeLen := len(dedupNoticePrefix) + len(numStr) + len(dedupNoticeSuffix)
	if trailingNewline {
		noticeLen++
	}

	// Strict compaction invariant: only emit notice if it strictly shrinks data
	if repeatCount >= threshold && droppedBytes > noticeLen {
		w = appendCollapseNotice(b, w, numStr, trailingNewline)
	} else {
		// Re-emit lines that were held back
		for k := 0; k < collapsedRepeats; k++ {
			copy(b[w:], line)
			w += len(line)
			if (k < collapsedRepeats-1 || trailingNewline) && w < len(b) {
				b[w] = '\n'
				w++
			}
		}
	}

	return w
}

func appendCollapseNotice(b []byte, w int, numStr []byte, trailingNewline bool) int {
	w += copy(b[w:], dedupNoticePrefix)
	w += copy(b[w:], numStr)
	w += copy(b[w:], dedupNoticeSuffix)

	if trailingNewline && w < len(b) {
		b[w] = '\n'
		w++
	}

	return w
}

// hasAlphanumeric returns true if b contains at least one ASCII letter or digit.
func hasAlphanumeric(b []byte) bool {
	for _, c := range b {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			return true
		}
	}
	return false
}
