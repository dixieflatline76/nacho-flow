// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package zeroalloc

// BitSet512 is a 64-byte stack-allocated bitset for tracking message indices 0..511.
// Guarantees 0 B/op and 0 allocs/op for high-frequency classification.
type BitSet512 struct {
	bits [8]uint64
}

// Set marks the bit at idx as true. No-op if idx is out of bounds (idx < 0 or idx >= 512).
func (s *BitSet512) Set(idx int) {
	if idx >= 0 && idx < 512 {
		s.bits[idx>>6] |= 1 << (uint(idx) & 63)
	}
}

// Clear marks the bit at idx as false. No-op if idx is out of bounds (idx < 0 or idx >= 512).
func (s *BitSet512) Clear(idx int) {
	if idx >= 0 && idx < 512 {
		s.bits[idx>>6] &^= 1 << (uint(idx) & 63)
	}
}

// Has reports whether the bit at idx is set. Returns false if idx is out of bounds.
func (s *BitSet512) Has(idx int) bool {
	if idx >= 0 && idx < 512 {
		return (s.bits[idx>>6] & (1 << (uint(idx) & 63))) != 0
	}
	return false
}

// IsEmpty reports whether all 512 bits are clear.
func (s *BitSet512) IsEmpty() bool {
	return (s.bits[0] | s.bits[1] | s.bits[2] | s.bits[3] |
		s.bits[4] | s.bits[5] | s.bits[6] | s.bits[7]) == 0
}
