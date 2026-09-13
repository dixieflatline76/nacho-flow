package nts

import "bytes"

// StripANSIInPlace removes ANSI escape sequences (CSI, OSC, and basic escapes)
// in-place within the provided byte slice b. It operates with zero heap allocations (0 B/op)
// by shifting non-escape bytes to the write cursor w <= r.
func StripANSIInPlace(b []byte) []byte {
	// Fast path: if there is no escape byte at all, return b immediately.
	firstEsc := bytes.IndexByte(b, 0x1b)
	if firstEsc == -1 {
		return b
	}

	w := firstEsc
	r := firstEsc
	n := len(b)

	for r < n {
		if b[r] == 0x1b {
			// Start of an escape sequence
			r++
			if r >= n {
				break
			}

			switch b[r] {
			case '[':
				// CSI sequence: ESC [ [params] [intermediates] final_byte
				// final_byte is in the range 0x40 - 0x7E ('@' - '~')
				r++
				for r < n && (b[r] < 0x40 || b[r] > 0x7E) {
					r++
				}
				if r < n {
					r++ // skip final byte
				}
			case ']':
				// OSC sequence: ESC ] [command] ; [payload] (BEL | ESC \)
				r++
				for r < n {
					if b[r] == 0x07 { // BEL
						r++
						break
					}
					if b[r] == 0x1b && r+1 < n && b[r+1] == '\\' {
						r += 2 // skip ESC \
						break
					}
					r++
				}
			case '(', ')', '*', '+':
				// Character set designator: ESC ( [charset]
				if r+1 < n {
					r += 2
				} else {
					r = n
				}
			default:
				// 2-character escape sequence: ESC <char>
				r++
			}
		} else {
			nextEsc := bytes.IndexByte(b[r:], 0x1b)
			if nextEsc == -1 {
				// No more escapes in remaining buffer: bulk copy remaining bytes and finish
				copy(b[w:], b[r:])
				w += n - r
				break
			}
			// Copy chunk up to next escape byte
			copy(b[w:], b[r:r+nextEsc])
			w += nextEsc
			r += nextEsc
		}
	}

	return b[:w]
}
