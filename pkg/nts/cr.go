package nts

import "bytes"

// ResolveCarriageReturnsInPlace normalizes terminal carriage return overwrites
// in-place within the provided byte slice b. Standalone \r rewinds the write
// cursor to the beginning of the current line, simulating terminal overwrite behavior,
// while \r\n is cleanly normalized to \n.
// Operates with zero heap allocations (0 B/op) maintaining w <= r.
func ResolveCarriageReturnsInPlace(b []byte) []byte {
	firstCR := bytes.IndexByte(b, '\r')
	if firstCR == -1 {
		return b
	}

	w := firstCR
	r := firstCR
	n := len(b)
	lineStart := 0

	// Find the start of the line where the first \r occurred
	for i := firstCR - 1; i >= 0; i-- {
		if b[i] == '\n' {
			lineStart = i + 1
			break
		}
	}

	for r < n {
		if b[r] == '\r' {
			if r+1 < n && b[r+1] == '\n' {
				// CRLF -> normalize to \n
				b[w] = '\n'
				w++
				r += 2
				lineStart = w
			} else {
				// Standalone \r -> rewind write cursor to start of current line
				w = lineStart
				r++
			}
		} else if b[r] == '\n' {
			b[w] = '\n'
			w++
			r++
			lineStart = w
		} else {
			b[w] = b[r]
			w++
			r++
		}
	}

	return b[:w]
}
