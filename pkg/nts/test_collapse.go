package nts

import (
	"bytes"
	"slices"
	"sync"
)

var (
	goPassMarker   = []byte("--- PASS:")
	goSkipMarker   = []byte("--- SKIP:")
	cargoOkMarker  = []byte("... ok")
	pytestMarker   = []byte(" PASSED")
	jestCheckUTF8  = []byte("\xe2\x9c\x93 ") // ✓
	mochaCheckUTF8 = []byte("\xe2\x9c\x94 ") // ✔
	asciiCheckMark = []byte("\xe2\x88\x9a ") // √
)

var goTestHashPool = sync.Pool{
	New: func() interface{} {
		s := make([]uint64, 0, 128)
		return &s
	},
}

// CollapseVerboseTestsInPlace removes noisy passing test execution lines across Go,
// Rust (Cargo), Python (Pytest), and Node.js (Jest/Vitest/Mocha) with zero heap
// allocations on the hot path.
//
// Invariants strictly enforced:
// 1. All failure lines (FAIL, FAILED, ✕, ✖, stack traces, assertion diffs) are preserved verbatim.
// 2. All compiler errors, crash dumps, and panics (orphaned === RUN without PASS) are preserved verbatim.
// 3. All suite summary lines are preserved verbatim.
// 4. In-place memory mutation obeying w <= r.
func CollapseVerboseTestsInPlace(b []byte) []byte {
	n := len(b)
	if n == 0 {
		return b
	}

	// Ultra-fast bypass: exit immediately in 0 allocations if no pass markers exist
	hasGo := bytes.Contains(b, goPassMarker)
	hasCargo := bytes.Contains(b, cargoOkMarker)
	hasPytest := bytes.Contains(b, pytestMarker)
	hasJest := bytes.Contains(b, jestCheckUTF8) || bytes.Contains(b, mochaCheckUTF8) || bytes.Contains(b, asciiCheckMark)

	if !hasGo && !hasCargo && !hasPytest && !hasJest {
		return b
	}

	// Pass 1 for Go test output: collect hashes of tests that explicitly passed or skipped
	var goPassedHashes *[]uint64
	if hasGo {
		goPassedHashes = goTestHashPool.Get().(*[]uint64)
		*goPassedHashes = (*goPassedHashes)[:0]

		lineStart := 0
		for lineStart < n {
			lineEnd := bytes.IndexByte(b[lineStart:], '\n')
			var line []byte
			if lineEnd == -1 {
				line = b[lineStart:]
				lineStart = n
			} else {
				line = b[lineStart : lineStart+lineEnd]
				lineStart += lineEnd + 1
			}

			trimmed := bytes.TrimLeft(line, " \t")
			if bytes.HasPrefix(trimmed, goPassMarker) || bytes.HasPrefix(trimmed, goSkipMarker) {
				var rest []byte
				if bytes.HasPrefix(trimmed, goPassMarker) {
					rest = trimmed[len(goPassMarker):]
				} else {
					rest = trimmed[len(goSkipMarker):]
				}
				rest = bytes.TrimLeft(rest, " \t")
				nameEnd := bytes.IndexAny(rest, " (\r\n")
				var testName []byte
				if nameEnd == -1 {
					testName = rest
				} else {
					testName = rest[:nameEnd]
				}
				if len(testName) > 0 {
					h := fnv64Bytes(testName)
					*goPassedHashes = append(*goPassedHashes, h)
				}
			}
		}
		slices.Sort(*goPassedHashes)
	}

	// Pass 2: In-place two-cursor rewrite (w <= r)
	w := 0
	r := 0

	for r < n {
		nextLF := bytes.IndexByte(b[r:], '\n')
		var line []byte
		var lineLenWithLF int
		if nextLF == -1 {
			line = b[r:]
			lineLenWithLF = len(line)
		} else {
			line = b[r : r+nextLF]
			lineLenWithLF = nextLF + 1
		}

		trimmed := bytes.TrimLeft(line, " \t")
		drop := false

		// Check 1: Go test running lines
		if hasGo && goPassedHashes != nil && len(*goPassedHashes) > 0 {
			if bytes.HasPrefix(trimmed, []byte("=== RUN ")) ||
				bytes.HasPrefix(trimmed, []byte("=== PAUSE ")) ||
				bytes.HasPrefix(trimmed, []byte("=== CONT ")) ||
				bytes.HasPrefix(trimmed, []byte("=== NAME ")) {
				firstSpace := bytes.IndexByte(trimmed, ' ')
				afterEquiv := bytes.TrimLeft(trimmed[firstSpace:], " \t")
				secondSpace := bytes.IndexByte(afterEquiv, ' ')
				if secondSpace != -1 {
					testNameRaw := bytes.TrimLeft(afterEquiv[secondSpace:], " \t")
					nameEnd := bytes.IndexAny(testNameRaw, " \r\n")
					var testName []byte
					if nameEnd == -1 {
						testName = testNameRaw
					} else {
						testName = testNameRaw[:nameEnd]
					}
					if len(testName) > 0 {
						h := fnv64Bytes(testName)
						if _, found := slices.BinarySearch(*goPassedHashes, h); found {
							drop = true
						}
					}
				}
			}
		}

		// Check 2: Rust (cargo test) passing lines: "test <name> ... ok"
		if !drop && hasCargo {
			if bytes.HasPrefix(trimmed, []byte("test ")) && bytes.HasSuffix(bytes.TrimRight(line, " \r"), cargoOkMarker) {
				drop = true
			}
		}

		// Check 3: Python (pytest -v) passing lines: "<path>::<name> PASSED"
		if !drop && hasPytest {
			if bytes.Contains(trimmed, []byte("::")) && bytes.Contains(trimmed, pytestMarker) {
				// Ensure it's not a failure block
				if !bytes.Contains(trimmed, []byte("FAIL")) {
					drop = true
				}
			}
		}

		// Check 4: Node.js (Jest / Vitest / Mocha) passing assertions
		if !drop && hasJest {
			if bytes.HasPrefix(trimmed, jestCheckUTF8) ||
				bytes.HasPrefix(trimmed, mochaCheckUTF8) ||
				bytes.HasPrefix(trimmed, asciiCheckMark) {
				drop = true
			}
		}

		if drop {
			// Skip line without writing
			r += lineLenWithLF
			continue
		}

		// Keep line: copy to w
		if w != r {
			copy(b[w:w+lineLenWithLF], b[r:r+lineLenWithLF])
		}
		w += lineLenWithLF
		r += lineLenWithLF
	}

	if goPassedHashes != nil {
		goTestHashPool.Put(goPassedHashes)
	}

	return b[:w]
}

func fnv64Bytes(b []byte) uint64 {
	var h uint64 = 14695981039346656037
	for _, c := range b {
		h ^= uint64(c)
		h *= 1099511628211
	}
	return h
}
