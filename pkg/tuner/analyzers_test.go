package tuner

import (
	"testing"

	"github.com/dixieflatline76/nacho-flow/pkg/contract"
	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

func TestModalityRiskAnalyzer_EmptyRecords(t *testing.T) {
	policy := DefaultTuningPolicy()
	analyzer := NewModalityRiskAnalyzer(policy)

	restrictImages, restrictTools := analyzer.Analyze(nil, nil)
	if restrictImages || restrictTools {
		t.Errorf("Expected false on empty records")
	}

	// Records with only cloud turns (totalLocal == 0)
	cloudOnly := []telemetry.TurnRecord{
		{IsLocal: false, HasImages: true, HasTools: true, IsRetry: true},
	}
	rImg, rTools := analyzer.Analyze(cloudOnly, nil)
	if rImg || rTools {
		t.Errorf("Expected false when totalLocal == 0")
	}
}

func TestModalityRiskAnalyzer_LocalVisionSuccess(t *testing.T) {
	policy := DefaultTuningPolicy()
	analyzer := NewModalityRiskAnalyzer(policy)

	var records []telemetry.TurnRecord
	for i := 0; i < 50; i++ {
		records = append(records, telemetry.TurnRecord{
			IsLocal:   true,
			HasImages: false,
			IsRetry:   false,
		})
	}
	for i := 0; i < 50; i++ {
		records = append(records, telemetry.TurnRecord{
			IsLocal:   true,
			HasImages: true,
			IsRetry:   false,
		})
	}

	restrictImages, restrictTools := analyzer.Analyze(records, nil)
	if restrictImages {
		t.Errorf("Expected restrictImages=false when local vision succeeds 100%% of the time")
	}
	if restrictTools {
		t.Errorf("Expected restrictTools=false when local tools succeed")
	}
}

func TestModalityRiskAnalyzer_LocalVisionHighFriction(t *testing.T) {
	policy := DefaultTuningPolicy()
	analyzer := NewModalityRiskAnalyzer(policy)

	var records []telemetry.TurnRecord
	// 100 turns text-only: 5% retry rate
	for i := 0; i < 100; i++ {
		records = append(records, telemetry.TurnRecord{
			IsLocal:   true,
			HasImages: false,
			IsRetry:   i < 5,
		})
	}
	// 20 turns with images: 80% retry rate (severe friction)
	for i := 0; i < 20; i++ {
		records = append(records, telemetry.TurnRecord{
			IsLocal:   true,
			HasImages: true,
			IsRetry:   i < 16,
		})
	}

	restrictImages, _ := analyzer.Analyze(records, nil)
	if !restrictImages {
		t.Errorf("Expected restrictImages=true when local image turns have 80%% retry rate vs 5%% baseline")
	}
}

// Test Fix 1: Polluted 100% Baseline where 100% of traffic is images and fails 100%
func TestModalityRiskAnalyzer_Polluted100PercentBaseline(t *testing.T) {
	policy := DefaultTuningPolicy()
	analyzer := NewModalityRiskAnalyzer(policy)

	var records []telemetry.TurnRecord
	// 30 turns: ALL local, ALL images, ALL failed (100% baseline failure)
	for i := 0; i < 30; i++ {
		records = append(records, telemetry.TurnRecord{
			IsLocal:   true,
			HasImages: true,
			HasTools:  true,
			IsRetry:   true,
		})
	}

	rImg, rTools := analyzer.Analyze(records, nil)
	if !rImg {
		t.Errorf("Expected restrictImages=true for 100%% failure rate despite baseline also being 100%%")
	}
	if !rTools {
		t.Errorf("Expected restrictTools=true for 100%% failure rate despite baseline also being 100%%")
	}
}

func TestModalityRiskAnalyzer_ZeroBaselineHighImageAndToolFailures(t *testing.T) {
	policy := DefaultTuningPolicy()
	analyzer := NewModalityRiskAnalyzer(policy)

	var records []telemetry.TurnRecord
	// 50 clean text turns (0% baseline retries)
	for i := 0; i < 50; i++ {
		records = append(records, telemetry.TurnRecord{
			IsLocal:   true,
			HasImages: false,
			HasTools:  false,
			IsRetry:   false,
		})
	}
	// 20 image turns with 80% failure (> 50% threshold)
	for i := 0; i < 20; i++ {
		records = append(records, telemetry.TurnRecord{
			IsLocal:   true,
			HasImages: true,
			IsRetry:   i < 16,
		})
	}
	// 20 tool turns with 80% failure (> 50% threshold)
	for i := 0; i < 20; i++ {
		records = append(records, telemetry.TurnRecord{
			IsLocal:  true,
			HasTools: true,
			IsRetry:  i < 16,
		})
	}

	rImages, rTools := analyzer.Analyze(records, nil)
	if !rImages {
		t.Errorf("Expected restrictImages=true with 0 baseline and >50%% image failure")
	}
	if !rTools {
		t.Errorf("Expected restrictTools=true with 0 baseline and >50%% tool failure")
	}
}

func TestModalityRiskAnalyzer_LocalToolHighFriction(t *testing.T) {
	policy := DefaultTuningPolicy()
	analyzer := NewModalityRiskAnalyzer(policy)

	var records []telemetry.TurnRecord
	// 100 turns: 5% retry baseline
	for i := 0; i < 100; i++ {
		records = append(records, telemetry.TurnRecord{
			IsLocal:  true,
			HasTools: false,
			IsRetry:  i < 5,
		})
	}
	// 20 turns with tools: 90% retry rate
	for i := 0; i < 20; i++ {
		records = append(records, telemetry.TurnRecord{
			IsLocal:  true,
			HasTools: true,
			IsRetry:  i < 18,
		})
	}

	_, restrictTools := analyzer.Analyze(records, nil)
	if !restrictTools {
		t.Errorf("Expected restrictTools=true when local tool turns have high retry rate")
	}
}

func TestKeywordRiskAnalyzer_FrictionAndThreshold(t *testing.T) {
	policy := DefaultTuningPolicy()
	analyzer := NewKeywordRiskAnalyzer(policy)

	if res := analyzer.Analyze(nil); res != nil {
		t.Errorf("Expected nil on empty records")
	}

	// Cloud only records
	cloudOnly := []telemetry.TurnRecord{
		{IsLocal: false, Keywords: []string{"test"}},
	}
	if res := analyzer.Analyze(cloudOnly); res != nil {
		t.Errorf("Expected nil when totalLocal == 0")
	}

	var records []telemetry.TurnRecord
	// 200 normal turns: baseline retry ~ 5%
	for i := 0; i < 200; i++ {
		records = append(records, telemetry.TurnRecord{
			IsLocal:  true,
			Keywords: []string{"ui", "css"},
			IsRetry:  i < 10,
		})
	}
	// 15 turns with "deadlock": 100% retry
	for i := 0; i < 15; i++ {
		records = append(records, telemetry.TurnRecord{
			IsLocal:  true,
			Keywords: []string{"deadlock"},
			IsRetry:  true,
		})
	}
	// 3 turns with "rare_bug" (below min_occurrences = 10): 100% retry
	for i := 0; i < 3; i++ {
		records = append(records, telemetry.TurnRecord{
			IsLocal:  true,
			Keywords: []string{"rare_bug"},
			IsRetry:  true,
		})
	}

	highFriction := analyzer.Analyze(records)
	if len(highFriction) != 1 || highFriction[0] != "deadlock" {
		t.Errorf("Expected ['deadlock'], got: %v", highFriction)
	}
}

// Test Fix 1 for Keywords: Polluted 100% Baseline
func TestKeywordRiskAnalyzer_Polluted100PercentBaseline(t *testing.T) {
	policy := DefaultTuningPolicy()
	analyzer := NewKeywordRiskAnalyzer(policy)

	var records []telemetry.TurnRecord
	// 20 turns: ALL local, ALL containing "sql", ALL failed (100% baseline)
	for i := 0; i < 20; i++ {
		records = append(records, telemetry.TurnRecord{
			IsLocal:  true,
			Keywords: []string{"sql"},
			IsRetry:  true,
		})
	}

	highFriction := analyzer.Analyze(records)
	if len(highFriction) != 1 || highFriction[0] != "sql" {
		t.Errorf("Expected ['sql'] flagged under 100%% failure rate, got: %v", highFriction)
	}
}

func TestKeywordRiskAnalyzer_ZeroBaselineWithFriction(t *testing.T) {
	policy := DefaultTuningPolicy()
	analyzer := NewKeywordRiskAnalyzer(policy)

	var records []telemetry.TurnRecord
	// 50 clean turns (0% baseline)
	for i := 0; i < 50; i++ {
		records = append(records, telemetry.TurnRecord{
			IsLocal:  true,
			Keywords: []string{"clean"},
			IsRetry:  false,
		})
	}
	// 15 turns with "fail_kw" (100% fail)
	for i := 0; i < 15; i++ {
		records = append(records, telemetry.TurnRecord{
			IsLocal:  true,
			Keywords: []string{"fail_kw"},
			IsRetry:  true,
		})
	}

	highFriction := analyzer.Analyze(records)
	if len(highFriction) != 1 || highFriction[0] != "fail_kw" {
		t.Errorf("Expected ['fail_kw'], got: %v", highFriction)
	}
}

func TestContextCliffAnalyzer_RespectsMaxContext(t *testing.T) {
	policy := DefaultTuningPolicy()
	analyzer := NewContextCliffAnalyzer(policy)

	var records []telemetry.TurnRecord
	for i := 0; i < 500; i++ {
		records = append(records, telemetry.TurnRecord{
			Tokens:  1000 + i*100,
			IsLocal: true,
			IsRetry: false,
		})
	}

	tier := &contract.Tier{
		Name:       "Local GPU",
		MaxContext: 24000,
	}

	bestT, _, _ := analyzer.Sweep(records, tier, false, false, nil)
	if bestT != 24000 {
		t.Errorf("Expected bestT to reach MaxContext bound of 24000 on clean data, got: %d", bestT)
	}

	// Test fallback when tier is nil
	bestTNil, _, _ := analyzer.Sweep(records, nil, false, false, nil)
	if bestTNil != 32000 {
		t.Errorf("Expected default max bound 32000 for nil tier, got: %d", bestTNil)
	}

	// Test fallback when tier maxContext < 1000
	tinyTier := &contract.Tier{MaxContext: 500}
	bestTTiny, _, _ := analyzer.Sweep(records, tinyTier, false, false, nil)
	if bestTTiny != 1000 {
		t.Errorf("Expected 1000 minimum floor, got: %d", bestTTiny)
	}
}

func TestContextCliffAnalyzer_8kCliffDetection(t *testing.T) {
	policy := DefaultTuningPolicy()
	analyzer := NewContextCliffAnalyzer(policy)

	var records []telemetry.TurnRecord
	// 500 turns < 8k tokens: 0% retries
	for i := 0; i < 500; i++ {
		records = append(records, telemetry.TurnRecord{
			Tokens:  1000 + i*12,
			IsLocal: true,
			IsRetry: false,
		})
	}
	// 500 turns >= 8k tokens: 100% retries
	for i := 0; i < 500; i++ {
		records = append(records, telemetry.TurnRecord{
			Tokens:  8000 + i*16,
			IsLocal: true,
			IsRetry: true,
		})
	}

	bestT, projectedCost, projectedRetries := analyzer.Sweep(records, nil, false, false, nil)
	if bestT != 8000 {
		t.Errorf("Expected cliff at 8000, got %d", bestT)
	}
	if projectedRetries != 0 {
		t.Errorf("Expected 0 projected retries after routing >=8k to cloud, got %d", projectedRetries)
	}
	if projectedCost <= 0 {
		t.Errorf("Expected positive projected cloud cost, got %f", projectedCost)
	}
}
