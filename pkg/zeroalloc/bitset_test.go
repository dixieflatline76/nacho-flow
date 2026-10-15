// Copyright (c) 2026 Karl Kwong / Spicebox. Licensed under AGPL-3.0.
// SPDX-License-Identifier: AGPL-3.0-or-later

package zeroalloc

import (
	"testing"
)

func TestBitSet512_OperationsAndBoundaries(t *testing.T) {
	var bs BitSet512

	if !bs.IsEmpty() {
		t.Fatalf("expected initial BitSet512 to be empty")
	}

	// Boundary indices to test
	testIndices := []int{0, 1, 63, 64, 65, 127, 128, 255, 256, 510, 511}

	for _, idx := range testIndices {
		if bs.Has(idx) {
			t.Errorf("expected idx %d to not be set initially", idx)
		}
		bs.Set(idx)
		if !bs.Has(idx) {
			t.Errorf("expected idx %d to be set after Set(%d)", idx, idx)
		}
		if bs.IsEmpty() {
			t.Errorf("expected BitSet512 to not be empty after Set(%d)", idx)
		}
	}

	// Out-of-bounds indices should not panic or set bits
	outOfBounds := []int{-100, -1, 512, 513, 1000}
	for _, oob := range outOfBounds {
		bs.Set(oob)
		if bs.Has(oob) {
			t.Errorf("expected out-of-bounds idx %d to return false for Has()", oob)
		}
		bs.Clear(oob) // Should be a no-op, no panic
	}

	// Verify all valid test indices are still set
	for _, idx := range testIndices {
		if !bs.Has(idx) {
			t.Errorf("expected idx %d to still be set", idx)
		}
	}

	// Clear each index one by one
	for _, idx := range testIndices {
		bs.Clear(idx)
		if bs.Has(idx) {
			t.Errorf("expected idx %d to be cleared after Clear(%d)", idx, idx)
		}
	}

	if !bs.IsEmpty() {
		t.Errorf("expected BitSet512 to be empty after clearing all set bits")
	}
}

func BenchmarkBitSet512(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		var bs BitSet512
		bs.Set(0)
		bs.Set(63)
		bs.Set(64)
		bs.Set(255)
		bs.Set(511)

		if !bs.Has(64) {
			b.Fatal("expected bit 64 to be set")
		}
		if bs.Has(100) {
			b.Fatal("expected bit 100 to not be set")
		}
		bs.Clear(64)
		if bs.Has(64) {
			b.Fatal("expected bit 64 to be cleared")
		}
		if bs.IsEmpty() {
			b.Fatal("expected not empty")
		}
	}
}
