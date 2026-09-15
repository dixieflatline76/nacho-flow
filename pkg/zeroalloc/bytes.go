// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package zeroalloc provides high-performance, strictly zero-allocation byte slice
// manipulation utilities for stream normalization and high-throughput proxy pipelines.
package zeroalloc

import (
	"bytes"
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
