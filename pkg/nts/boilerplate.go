package nts

import "bytes"

var (
	noticePrefix      = []byte("<notice>Making multiple related changes in a single apply_diff is more efficient")
	nodeWarningsHint  = []byte("(Use `node --trace-warnings ...` to show where the warning was created)")
	errorDetailsOpen  = []byte("<error_details>")
	errorDetailsClose = []byte("</error_details>")
	searchContentTag  = []byte("Search Content:")
)

// StripToolBoilerplateInPlace removes repetitive IDE extension notices and compacts
// verbose diff-match failure dumps in-place within byte slice b.
// Operates with zero heap allocations (0 B/op) maintaining w <= r.
func StripToolBoilerplateInPlace(b []byte) []byte {
	w := 0
	r := 0
	n := len(b)
	inErrorDetails := false

	for r < n {
		lineStart := r
		for r < n && b[r] != '\n' {
			r++
		}
		hasNewline := r < n && b[r] == '\n'
		if hasNewline {
			r++ // include newline
		}
		line := b[lineStart:r]

		// Check if line contains noticePrefix
		if bytes.Contains(line, noticePrefix) {
			// Skip this line entirely
			continue
		}

		// Check if line contains node trace-warnings boilerplate
		if bytes.Contains(line, nodeWarningsHint) {
			continue
		}

		// Track <error_details> block
		if bytes.Contains(line, errorDetailsOpen) {
			inErrorDetails = true
		}

		if inErrorDetails && bytes.HasPrefix(bytes.TrimSpace(line), searchContentTag) {
			// Skip lines until </error_details>
			for r < n {
				subStart := r
				for r < n && b[r] != '\n' {
					r++
				}
				if r < n && b[r] == '\n' {
					r++
				}
				subLine := b[subStart:r]
				if bytes.Contains(subLine, errorDetailsClose) {
					// Write </error_details>\n
					copy(b[w:], errorDetailsClose)
					w += len(errorDetailsClose)
					b[w] = '\n'
					w++
					inErrorDetails = false
					break
				}
			}
			continue
		}

		if bytes.Contains(line, errorDetailsClose) {
			inErrorDetails = false
		}

		// Copy line
		copy(b[w:], line)
		w += len(line)
	}

	return b[:w]
}
