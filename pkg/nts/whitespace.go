package nts

// NormalizeWhitespaceInPlace strips trailing spaces/tabs on each line, collapses
// consecutive runs of blank lines down to a single blank line, and cleans excessive
// trailing newlines at the end of the buffer.
// Operates in-place with zero heap allocations (0 B/op) maintaining w <= r.
func NormalizeWhitespaceInPlace(b []byte) []byte {
	w := 0
	r := 0
	n := len(b)
	consecutiveBlankLines := 0

	for r < n {
		lineStart := r
		for r < n && b[r] != '\n' {
			r++
		}
		lineEnd := r
		hasNewline := r < n && b[r] == '\n'
		if hasNewline {
			r++
		}

		// Trim trailing spaces and tabs from the line
		for lineEnd > lineStart && (b[lineEnd-1] == ' ' || b[lineEnd-1] == '\t') {
			lineEnd--
		}

		lineLen := lineEnd - lineStart
		if lineLen == 0 {
			// Blank line
			consecutiveBlankLines++
			if consecutiveBlankLines <= 1 && hasNewline {
				b[w] = '\n'
				w++
			}
		} else {
			consecutiveBlankLines = 0
			copy(b[w:], b[lineStart:lineEnd])
			w += lineLen
			if hasNewline {
				b[w] = '\n'
				w++
			}
		}
	}

	// Trim trailing blank lines at the end of the output (preserve at most 1 trailing \n)
	for w > 1 && b[w-1] == '\n' && b[w-2] == '\n' {
		w--
	}

	return b[:w]
}
