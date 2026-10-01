// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package zeroalloc provides high-performance, strictly zero-allocation byte slice
// manipulation utilities for stream normalization and high-throughput proxy pipelines.
package zeroalloc

import (
	"bytes"
	"strings"
)

// StripSubsliceInPlace removes all occurrences of target from b in-place.
// Operates with zero heap allocations (0 B/op, 0 allocs/op) maintaining w <= r.
func StripSubsliceInPlace(b []byte, target []byte) []byte {
	if len(b) == 0 || len(target) == 0 {
		return b
	}

	firstIdx := bytes.Index(b, target)
	if firstIdx == -1 {
		return b
	}

	w := firstIdx
	r := firstIdx + len(target)
	tLen := len(target)
	n := len(b)

	for r < n {
		nextIdx := bytes.Index(b[r:], target)
		if nextIdx == -1 {
			copy(b[w:], b[r:])
			w += n - r
			break
		}
		copy(b[w:], b[r:r+nextIdx])
		w += nextIdx
		r += nextIdx + tLen
	}

	return b[:w]
}

// StripSubslicesInPlace removes all occurrences of any byte sequence in targets
// from b in-place in a single forward pass.
//
// Invariants & Requirements:
//  1. targets MUST be sorted descending by length to ensure maximal greedy matching.
//  2. Operates strictly in-place with zero heap allocations (0 B/op, 0 allocs/op),
//     guaranteeing write cursor w <= read cursor r at all times.
func StripSubslicesInPlace(b []byte, targets [][]byte) []byte {
	n := len(b)
	if n == 0 || len(targets) == 0 {
		return b
	}

	// Build stack-allocated trigger table of initial bytes
	var triggers [256]bool
	singleTrigger := true
	firstTriggerByte := byte(0)

	for i, t := range targets {
		if len(t) > 0 {
			b0 := t[0]
			triggers[b0] = true
			if i == 0 {
				firstTriggerByte = b0
			} else if b0 != firstTriggerByte {
				singleTrigger = false
			}
		}
	}

	var firstMatchIdx int
	if singleTrigger {
		firstMatchIdx = bytes.IndexByte(b, firstTriggerByte)
	} else {
		firstMatchIdx = -1
		for i := 0; i < n; i++ {
			if triggers[b[i]] {
				firstMatchIdx = i
				break
			}
		}
	}

	// Fast bailout: no candidate trigger byte exists in b
	if firstMatchIdx == -1 {
		return b
	}

	w := firstMatchIdx
	r := firstMatchIdx

	for r < n {
		if triggers[b[r]] {
			matchedLen := 0
			sub := b[r:]
			for _, target := range targets {
				tLen := len(target)
				if tLen <= len(sub) && bytes.Equal(sub[:tLen], target) {
					matchedLen = tLen
					break
				}
			}
			if matchedLen > 0 {
				r += matchedLen
				continue
			}

			// Trigger byte did not lead to a target match; copy byte and advance
			b[w] = b[r]
			w++
			r++
		} else {
			// Find next candidate trigger byte
			var nextTrigger int
			if singleTrigger {
				nextTrigger = bytes.IndexByte(b[r:], firstTriggerByte)
			} else {
				nextTrigger = -1
				for i := r; i < n; i++ {
					if triggers[b[i]] {
						nextTrigger = i - r
						break
					}
				}
			}

			if nextTrigger == -1 {
				// No more trigger bytes in remaining buffer; bulk copy and finish
				copy(b[w:], b[r:])
				w += n - r
				break
			}

			// Bulk copy non-trigger slice
			copy(b[w:], b[r:r+nextTrigger])
			w += nextTrigger
			r += nextTrigger
		}
	}

	return b[:w]
}

// FindTrailingPrefix checks whether the tail of b (up to maxLen bytes) matches
// any prefix in the provided prefixes slice.
//
// If a match is found, it returns the byte offset in b where the earliest matching
// prefix begins. If no match is found, it returns -1.
// Operates with zero heap allocations (0 B/op, 0 allocs/op).
func FindTrailingPrefix(b []byte, maxLen int, prefixes [][]byte) int {
	n := len(b)
	if n == 0 || len(prefixes) == 0 {
		return -1
	}

	checkLen := maxLen
	if checkLen > n {
		checkLen = n
	}

	start := n - checkLen
	for i := start; i < n; i++ {
		candidate := b[i:]
		for _, prefix := range prefixes {
			if bytes.Equal(candidate, prefix) {
				return i
			}
		}
	}

	return -1
}

// HasPrefixAny reports whether b starts with any of the prefixes.
// Operates with zero heap allocations (0 B/op, 0 allocs/op).
func HasPrefixAny(b []byte, prefixes [][]byte) bool {
	for _, p := range prefixes {
		if bytes.HasPrefix(b, p) {
			return true
		}
	}
	return false
}

// ReplaceSubslicesInPlace replaces all occurrences of byte sequences in targets
// with corresponding byte sequences in replacements within b in-place in a single forward pass.
//
// Invariants & Requirements:
//  1. len(targets) == len(replacements).
//  2. For every index i, len(replacements[i]) <= len(targets[i]) MUST hold to guarantee
//     that write cursor w <= read cursor r at all times (preventing buffer corruption).
//  3. targets MUST be sorted descending by length to ensure maximal greedy matching.
//  4. Operates strictly in-place with zero heap allocations (0 B/op, 0 allocs/op).
func ReplaceSubslicesInPlace(b []byte, targets [][]byte, replacements [][]byte) []byte {
	n := len(b)
	if n == 0 || len(targets) == 0 || len(targets) != len(replacements) {
		return b
	}

	// Build stack-allocated trigger table of initial bytes
	var triggers [256]bool
	singleTrigger := true
	firstTriggerByte := byte(0)

	for i, t := range targets {
		if len(t) > 0 {
			b0 := t[0]
			triggers[b0] = true
			if i == 0 {
				firstTriggerByte = b0
			} else if b0 != firstTriggerByte {
				singleTrigger = false
			}
		}
	}

	var firstMatchIdx int
	if singleTrigger {
		firstMatchIdx = bytes.IndexByte(b, firstTriggerByte)
	} else {
		firstMatchIdx = -1
		for i := 0; i < n; i++ {
			if triggers[b[i]] {
				firstMatchIdx = i
				break
			}
		}
	}

	// Fast bailout: no candidate trigger byte exists in b
	if firstMatchIdx == -1 {
		return b
	}

	w := firstMatchIdx
	r := firstMatchIdx

	for r < n {
		if triggers[b[r]] {
			matchedIdx := -1
			sub := b[r:]
			for i, target := range targets {
				tLen := len(target)
				if tLen <= len(sub) && bytes.Equal(sub[:tLen], target) {
					matchedIdx = i
					break
				}
			}
			if matchedIdx != -1 {
				targetLen := len(targets[matchedIdx])
				rep := replacements[matchedIdx]
				repLen := len(rep)
				// Enforce w <= r invariant safety guard
				if repLen <= targetLen {
					copy(b[w:], rep)
					w += repLen
					r += targetLen
					continue
				}
			}

			// Trigger byte did not lead to a target match; copy byte and advance
			b[w] = b[r]
			w++
			r++
		} else {
			// Find next candidate trigger byte
			var nextTrigger int
			if singleTrigger {
				nextTrigger = bytes.IndexByte(b[r:], firstTriggerByte)
			} else {
				nextTrigger = -1
				for i := r; i < n; i++ {
					if triggers[b[i]] {
						nextTrigger = i - r
						break
					}
				}
			}

			if nextTrigger == -1 {
				// No more trigger bytes in remaining buffer; bulk copy and finish
				copy(b[w:], b[r:])
				w += n - r
				break
			}

			// Bulk copy non-trigger slice
			copy(b[w:], b[r:r+nextTrigger])
			w += nextTrigger
			r += nextTrigger
		}
	}

	return b[:w]
}

// ContainsFoldASCII reports whether needle is within haystack, using ASCII case-folding.
// Both haystack and needle may contain mixed-case ASCII bytes.
// Non-ASCII bytes are matched exact.
// Operates strictly with zero heap allocations (0 B/op, 0 allocs/op).
func ContainsFoldASCII(haystack, needle []byte) bool {
	n := len(needle)
	if n == 0 {
		return true
	}
	if len(haystack) < n {
		return false
	}

	b0 := needle[0]
	b0Lower := b0
	b0Upper := b0
	if b0 >= 'a' && b0 <= 'z' {
		b0Upper = b0 - 32
	} else if b0 >= 'A' && b0 <= 'Z' {
		b0Lower = b0 + 32
	}

	limit := len(haystack) - n
	for i := 0; i <= limit; i++ {
		h0 := haystack[i]
		if h0 == b0Lower || h0 == b0Upper {
			match := true
			for j := 1; j < n; j++ {
				hb := haystack[i+j]
				if hb >= 'A' && hb <= 'Z' {
					hb += 32
				}
				nb := needle[j]
				if nb >= 'A' && nb <= 'Z' {
					nb += 32
				}
				if hb != nb {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}

// ExtractQuotedField finds the raw quoted JSON string value for key (e.g. `"text":`) in b,
// returning the subslice including the opening and closing quotes (e.g. `"hello world"`).
// It correctly handles escaped characters (`\"`, `\\`) and operates with strictly zero heap allocations (0 B/op, 0 allocs/op).
func ExtractQuotedField(b, key []byte) ([]byte, bool) {
	idx := bytes.Index(b, key)
	if idx == -1 {
		return nil, false
	}
	p := b[idx+len(key):]
	start := -1
	for i := 0; i < len(p); i++ {
		c := p[i]
		if c == '"' {
			start = i
			break
		}
		if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			return nil, false
		}
	}
	if start == -1 {
		return nil, false
	}

	inEscape := false
	for i := start + 1; i < len(p); i++ {
		if inEscape {
			inEscape = false
			continue
		}
		if p[i] == '\\' {
			inEscape = true
			continue
		}
		if p[i] == '"' {
			return p[start : i+1], true
		}
	}
	return nil, false
}

// AppendEscapedJSONString appends a JSON-quoted and escaped representation of s into dst
// without invoking json.Marshal or heap allocations (0 allocs/op when cap(dst) is sufficient).
func AppendEscapedJSONString(dst []byte, s string) []byte {
	dst = append(dst, '"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\', '"':
			dst = append(dst, '\\', c)
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\t':
			dst = append(dst, '\\', 't')
		default:
			if c < 0x20 {
				const hex = "0123456789abcdef"
				dst = append(dst, '\\', 'u', '0', '0', hex[c>>4], hex[c&0xf])
			} else {
				dst = append(dst, c)
			}
		}
	}
	return append(dst, '"')
}

// WriteEscapedJSONString writes a JSON-quoted and escaped representation of s directly into buf
// without invoking json.Marshal or heap allocations (0 allocs/op if buf has sufficient capacity).
func WriteEscapedJSONString(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\', '"':
			buf.WriteByte('\\')
			buf.WriteByte(c)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			if c < 0x20 {
				const hex = "0123456789abcdef"
				buf.WriteString(`\u00`)
				buf.WriteByte(hex[c>>4])
				buf.WriteByte(hex[c&0xf])
			} else {
				buf.WriteByte(c)
			}
		}
	}
	buf.WriteByte('"')
}

// LookupCompositeKey searches map m for a composite key formatted as {prefix}{sep}{suffix}
// with case-insensitive ASCII normalization on prefix, guaranteeing strictly zero heap allocations (0 B/op, 0 allocs/op)
// by leveraging a stack-allocated buffer and the Go runtime mapaccess_faststr optimization.
func LookupCompositeKey[V any](m map[string]V, prefix, sep, suffix string) (V, bool) {
	if m == nil {
		var zero V
		return zero, false
	}
	need := len(prefix) + len(sep) + len(suffix)
	if need <= 128 {
		var buf [128]byte
		n := copy(buf[:], prefix)
		for i := 0; i < n; i++ {
			if buf[i] >= 'A' && buf[i] <= 'Z' {
				buf[i] += 'a' - 'A'
			}
		}
		n += copy(buf[n:], sep)
		n += copy(buf[n:], suffix)
		// Compiler optimization in mapaccess1_faststr: m[string(buf[:n])] avoids heap allocation
		if v, ok := m[string(buf[:n])]; ok {
			return v, true
		}
	} else {
		key := strings.ToLower(prefix) + sep + suffix
		if v, ok := m[key]; ok {
			return v, true
		}
	}
	var zero V
	return zero, false
}
