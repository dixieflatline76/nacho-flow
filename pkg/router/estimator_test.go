package router

import (
	"math"
	"sync"
	"testing"
)

func TestTokenEstimator_Baseline(t *testing.T) {
	e := NewTokenEstimator()
	if math.Abs(e.GetRatio()-3.2) > 1e-6 {
		t.Fatalf("Expected initial ratio 3.2, got %f", e.GetRatio())
	}

	// 3200 chars / 3.2 = 1000 tokens
	tokens := e.Estimate(3200)
	if tokens != 1000 {
		t.Errorf("Expected 1000 tokens, got %d", tokens)
	}

	// Zero / negative charCount edge cases
	if e.Estimate(0) != 0 {
		t.Errorf("Expected 0 tokens for 0 chars, got %d", e.Estimate(0))
	}
	if e.Estimate(-5) != 0 {
		t.Errorf("Expected 0 tokens for negative chars, got %d", e.Estimate(-5))
	}
	if e.Estimate(1) != 1 {
		t.Errorf("Expected at least 1 token for 1 char, got %d", e.Estimate(1))
	}
}

func TestTokenEstimator_EMACalibration(t *testing.T) {
	e := NewTokenEstimator()

	// Feed observed ratio of 3.0 (3000 chars / 1000 tokens = 3.0)
	// updated = 3.2 * 0.8 + 3.0 * 0.2 = 2.56 + 0.6 = 3.16
	e.Calibrate(1000, 3000)
	if math.Abs(e.GetRatio()-3.16) > 1e-6 {
		t.Errorf("Expected ratio 3.16 after 1 step, got %f", e.GetRatio())
	}

	// Step 2: 3.16 * 0.8 + 3.0 * 0.2 = 2.528 + 0.6 = 3.128
	e.Calibrate(1000, 3000)
	if math.Abs(e.GetRatio()-3.128) > 1e-6 {
		t.Errorf("Expected ratio 3.128 after 2 steps, got %f", e.GetRatio())
	}

	// Invalid calibration values should be ignored
	curr := e.GetRatio()
	e.Calibrate(0, 3000)
	if e.GetRatio() != curr {
		t.Errorf("Expected ratio unchanged for 0 tokens, got %f", e.GetRatio())
	}
	e.Calibrate(1000, 0)
	if e.GetRatio() != curr {
		t.Errorf("Expected ratio unchanged for 0 chars, got %f", e.GetRatio())
	}
}

func TestTokenEstimator_Clamping(t *testing.T) {
	e := NewTokenEstimator()

	// Extreme low: 100 chars / 1000 tokens = 0.1 ratio -> clamped to 1.8
	// updated = 3.2 * 0.8 + 1.8 * 0.2 = 2.56 + 0.36 = 2.92
	e.Calibrate(1000, 100)
	if math.Abs(e.GetRatio()-2.92) > 1e-6 {
		t.Errorf("Expected clamped low ratio 2.92, got %f", e.GetRatio())
	}

	e2 := NewTokenEstimator()
	// Extreme high: 10000 chars / 1000 tokens = 10.0 ratio -> clamped to 5.0
	// updated = 3.2 * 0.8 + 5.0 * 0.2 = 2.56 + 1.0 = 3.56
	e2.Calibrate(1000, 10000)
	if math.Abs(e2.GetRatio()-3.56) > 1e-6 {
		t.Errorf("Expected clamped high ratio 3.56, got %f", e2.GetRatio())
	}
}

func TestTokenEstimator_ConcurrentRace(t *testing.T) {
	e := NewTokenEstimator()
	var wg sync.WaitGroup
	workers := 100

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				_ = e.Estimate(3000 + id)
				if j%10 == 0 {
					e.Calibrate(1000, 3000+id)
				}
			}
		}(i)
	}

	wg.Wait()
	if e.GetRatio() < MinCharsPerToken || e.GetRatio() > MaxCharsPerToken {
		t.Errorf("Ratio %f out of bounds after concurrent runs", e.GetRatio())
	}
}

func TestTokenEstimator_NilAndInvalidPointers(t *testing.T) {
	e := &TokenEstimator{}
	if e.GetRatio() != DefaultCharsPerToken {
		t.Errorf("Expected default ratio for nil pointer, got %f", e.GetRatio())
	}

	invalid := -1.0
	e.ratio.Store(&invalid)
	if e.GetRatio() != DefaultCharsPerToken {
		t.Errorf("Expected default ratio for negative ratio, got %f", e.GetRatio())
	}
}
