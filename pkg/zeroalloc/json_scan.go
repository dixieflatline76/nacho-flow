package zeroalloc

import (
	"bytes"
)

// Edit represents a byte range [Start, End) to be replaced by Repl.
type Edit struct {
	Start, End int
	Repl       []byte
}

// SkipValue returns the end offset (exclusive) of the JSON value starting at i.
// Returns ok=false if the value is malformed or truncated.
func SkipValue(b []byte, i int) (int, bool) {
	// Skip leading whitespace
	for i < len(b) && isWhitespace(b[i]) {
		i++
	}
	if i >= len(b) {
		return 0, false
	}

	c := b[i]
	switch c {
	case '"':
		return skipString(b, i)
	case '{', '[':
		return skipBracketContainer(b, i)
	case 't':
		if i+4 <= len(b) && string(b[i:i+4]) == "true" {
			if i+4 == len(b) || isDelimiter(b[i+4]) {
				return i + 4, true
			}
		}
		return 0, false
	case 'f':
		if i+5 <= len(b) && string(b[i:i+5]) == "false" {
			if i+5 == len(b) || isDelimiter(b[i+5]) {
				return i + 5, true
			}
		}
		return 0, false
	case 'n':
		if i+4 <= len(b) && string(b[i:i+4]) == "null" {
			if i+4 == len(b) || isDelimiter(b[i+4]) {
				return i + 4, true
			}
		}
		return 0, false
	default:
		if c == '-' || (c >= '0' && c <= '9') {
			return skipNumber(b, i)
		}
		return 0, false
	}
}

func skipString(b []byte, start int) (int, bool) {
	if start >= len(b) || b[start] != '"' {
		return 0, false
	}
	i := start + 1
	for i < len(b) {
		c := b[i]
		if c == '\\' {
			if i+1 >= len(b) {
				return 0, false // truncated escape
			}
			i += 2 // skip escaped character (e.g. \", \\)
			continue
		}
		if c == '"' {
			return i + 1, true
		}
		if c < 0x20 {
			// Unescaped control character inside string is invalid JSON
			return 0, false
		}
		i++
	}
	return 0, false
}

func skipBracketContainer(b []byte, start int) (int, bool) {
	if start >= len(b) {
		return 0, false
	}
	// Fixed-size depth stack (128 levels) for zero allocations
	var stack [128]byte
	depth := 0

	open := b[start]
	if open != '{' && open != '[' {
		return 0, false
	}
	stack[depth] = open
	depth++

	i := start + 1
	for i < len(b) && depth > 0 {
		c := b[i]
		switch c {
		case '"':
			endStr, ok := skipString(b, i)
			if !ok {
				return 0, false
			}
			i = endStr
			continue
		case '{', '[':
			if depth >= len(stack) {
				return 0, false // max nesting exceeded
			}
			stack[depth] = c
			depth++
		case '}':
			if depth == 0 || stack[depth-1] != '{' {
				return 0, false // mismatched bracket
			}
			depth--
		case ']':
			if depth == 0 || stack[depth-1] != '[' {
				return 0, false // mismatched bracket
			}
			depth--
		}
		i++
	}

	if depth != 0 {
		return 0, false // unclosed container
	}
	return i, true
}

func skipNumber(b []byte, start int) (int, bool) {
	i := start
	hasDigit := false

	if i < len(b) && b[i] == '-' {
		i++
	}
	for i < len(b) && (b[i] >= '0' && b[i] <= '9') {
		hasDigit = true
		i++
	}
	if !hasDigit {
		return 0, false
	}

	if i < len(b) && b[i] == '.' {
		i++
		hasFraction := false
		for i < len(b) && (b[i] >= '0' && b[i] <= '9') {
			hasFraction = true
			i++
		}
		if !hasFraction {
			return 0, false
		}
	}

	if i < len(b) && (b[i] == 'e' || b[i] == 'E') {
		i++
		if i < len(b) && (b[i] == '+' || b[i] == '-') {
			i++
		}
		hasExp := false
		for i < len(b) && (b[i] >= '0' && b[i] <= '9') {
			hasExp = true
			i++
		}
		if !hasExp {
			return 0, false
		}
	}

	if i < len(b) && !isDelimiter(b[i]) {
		return 0, false
	}
	return i, true
}

func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func isDelimiter(c byte) bool {
	return isWhitespace(c) || c == ',' || c == ']' || c == '}' || c == ':'
}

// ObjectField finds key among the direct members of the object spanning [objStart, objEnd).
// Keys are compared as raw bytes without unescaping; callers pass plain ASCII keys.
func ObjectField(b []byte, objStart, objEnd int, key string) (keyStart, valStart, valEnd int, ok bool) {
	if objStart < 0 || objEnd > len(b) || objStart >= objEnd {
		return 0, 0, 0, false
	}

	i := objStart
	for i < objEnd && isWhitespace(b[i]) {
		i++
	}
	if i >= objEnd || b[i] != '{' {
		return 0, 0, 0, false
	}
	i++ // step past '{'

	keyBytes := []byte(key)

	for i < objEnd {
		for i < objEnd && isWhitespace(b[i]) {
			i++
		}
		if i >= objEnd {
			return 0, 0, 0, false
		}
		if b[i] == '}' {
			return 0, 0, 0, false
		}
		if b[i] == ',' {
			i++
			continue
		}
		if b[i] != '"' {
			return 0, 0, 0, false
		}

		kStart := i
		kEnd, okStr := skipString(b, i)
		if !okStr || kEnd > objEnd {
			return 0, 0, 0, false
		}
		i = kEnd

		// Expect ':'
		for i < objEnd && isWhitespace(b[i]) {
			i++
		}
		if i >= objEnd || b[i] != ':' {
			return 0, 0, 0, false
		}
		i++ // step past ':'

		for i < objEnd && isWhitespace(b[i]) {
			i++
		}
		if i >= objEnd {
			return 0, 0, 0, false
		}

		vStart := i
		vEnd, okVal := SkipValue(b, i)
		if !okVal || vEnd > objEnd {
			return 0, 0, 0, false
		}
		i = vEnd

		// Check if key matches (excluding enclosing quotes)
		rawKey := b[kStart+1 : kEnd-1]
		if bytes.Equal(rawKey, keyBytes) {
			return kStart, vStart, vEnd, true
		}
	}

	return 0, 0, 0, false
}

// NextElement iterates array elements without allocating: pass the position after '[' or after
// the previous element; returns the element span and the position to continue from.
func NextElement(b []byte, pos, arrEnd int) (start, end, next int, ok bool) {
	if pos < 0 || arrEnd > len(b) || pos >= arrEnd {
		return 0, 0, 0, false
	}

	i := pos
	for i < arrEnd && isWhitespace(b[i]) {
		i++
	}
	if i >= arrEnd || b[i] == ']' {
		return 0, 0, 0, false
	}
	if b[i] == ',' {
		i++
		for i < arrEnd && isWhitespace(b[i]) {
			i++
		}
		if i >= arrEnd || b[i] == ']' {
			return 0, 0, 0, false
		}
	}

	start = i
	end, okVal := SkipValue(b, i)
	if !okVal || end > arrEnd {
		return 0, 0, 0, false
	}
	return start, end, end, true
}

// SpliceInPlace applies sorted, non-overlapping edits in b.
// Returns ok=false, leaving b untouched, if:
//   - any edit has Start < 0, End > len(b), or Start > End
//   - edits are not sorted (edits[i].Start < edits[i-1].End)
//   - any replacement is longer than its span (len(Repl) > End - Start)
func SpliceInPlace(b []byte, edits []Edit) ([]byte, bool) {
	if len(edits) == 0 {
		return b, true
	}

	// 1. Validation pass (ensures no mutations occur on invalid inputs)
	lastEnd := 0
	for _, e := range edits {
		if e.Start < 0 || e.End > len(b) || e.Start > e.End {
			return nil, false
		}
		if e.Start < lastEnd {
			return nil, false // unsorted or overlapping
		}
		if len(e.Repl) > (e.End - e.Start) {
			return nil, false // replacement exceeds span
		}
		lastEnd = e.End
	}

	// 2. Single forward pass: write cursor w <= read cursor r at all times
	w, r := 0, 0
	for _, e := range edits {
		w += copy(b[w:], b[r:e.Start])
		w += copy(b[w:], e.Repl)
		r = e.End
	}
	w += copy(b[w:], b[r:])
	return b[:w], true
}

// SpliceInto writes src with edits applied into dst (append semantics). src is never modified.
// Validates edits identically to SpliceInPlace (sorted, non-overlapping, in bounds).
// Returns nil, false on validation failure.
func SpliceInto(dst, src []byte, edits []Edit) ([]byte, bool) {
	// Validation pass
	lastEnd := 0
	for _, e := range edits {
		if e.Start < 0 || e.End > len(src) || e.Start > e.End {
			return nil, false
		}
		if e.Start < lastEnd {
			return nil, false // unsorted or overlapping
		}
		lastEnd = e.End
	}

	r := 0
	for _, e := range edits {
		dst = append(dst, src[r:e.Start]...)
		dst = append(dst, e.Repl...)
		r = e.End
	}
	return append(dst, src[r:]...), true
}
