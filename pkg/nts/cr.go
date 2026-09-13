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

	// Find the start of the line where the first \r occurred using SIMD
	lastLF := bytes.LastIndexByte(b[:firstCR], '\n')
	if lastLF >= 0 {
		lineStart = lastLF + 1
	}

	for r < n {
		switch b[r] {
		case '\r':
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
		case '\n':
			b[w] = '\n'
			w++
			r++
			lineStart = w
		default:
			nextControl := bytes.IndexAny(b[r:], "\r\n")
			if nextControl == -1 {
				// No more \r or \n: bulk copy all remaining bytes and finish
				copy(b[w:], b[r:])
				w += n - r
				r = n
				break
			}
			copy(b[w:], b[r:r+nextControl])
			w += nextControl
			r += nextControl
		}
	}

	return b[:w]
}
