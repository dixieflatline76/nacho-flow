//go:build ignore

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"time"

	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

const (
	estimatedAvoidedTokensPerIntervention = 8000
	estimatedLocalTokensPerSecond         = 35
)

type WindowExpected struct {
	Requests            int64   `json:"requests"`
	TokensTotal         int64   `json:"tokens_total"`
	TokensLocal         int64   `json:"tokens_local"`
	CostSpentUSD        float64 `json:"cost_spent_usd"`
	CostSavedUSD        float64 `json:"cost_saved_usd"`
	CostReductionPct    float64 `json:"cost_reduction_pct"`
	CKInterventions     int64   `json:"ck_interventions"`
	CKAvoidedTokens     int64   `json:"ck_avoided_tokens"`
	CKAvoidedGPUSecs    int64   `json:"ck_avoided_gpu_seconds"`
	CKStage1LocalHeals  int64   `json:"ck_stage1_local_heals"`
	CKStage2Escalations int64   `json:"ck_stage2_cloud_escalations"`
	FDTriggers          int64   `json:"fd_triggers"`
}

type FixtureExpected struct {
	ReferenceTime string                    `json:"reference_time"`
	Today         WindowExpected            `json:"today"`
	Yesterday     WindowExpected            `json:"yesterday"`
	ThisWeek      WindowExpected            `json:"this_week"`
	ThisMonth     WindowExpected            `json:"this_month"`
	AllTime       WindowExpected            `json:"all_time"`
	ByDay         map[string]WindowExpected `json:"by_day"`
}

func reductionPct(saved, spent float64) float64 {
	denom := saved + spent
	if denom > 0 {
		return (saved / denom) * 100.0
	}
	return 0.0
}

func addRecordToExpected(w *WindowExpected, rec telemetry.TurnRecord) {
	w.Requests++
	w.TokensTotal += int64(rec.Tokens)
	if rec.IsLocal {
		w.TokensLocal += int64(rec.Tokens)
	}
	w.CostSpentUSD += rec.CostSpentUSD
	w.CostSavedUSD += rec.CostSavedUSD

	if rec.CycleBreakerTriggered {
		w.CKInterventions++
		w.CKAvoidedTokens += estimatedAvoidedTokensPerIntervention
		w.CKAvoidedGPUSecs += estimatedAvoidedTokensPerIntervention / estimatedLocalTokensPerSecond
		if rec.IsFallback {
			w.CKStage2Escalations++
		} else {
			w.CKStage1LocalHeals++
		}
	}
	if rec.FairyDusted {
		w.FDTriggers++
	}
}

func finalizeWindow(w *WindowExpected) {
	w.CostReductionPct = reductionPct(w.CostSavedUSD, w.CostSpentUSD)
}

func main() {
	seedFlag := flag.Uint64("seed", 42, "PRNG seed for deterministic output")
	daysFlag := flag.Int("days", 30, "Number of days of history to generate")
	refTimeFlag := flag.String("ref-time", "2026-08-31T12:00:00Z", "Reference UTC time for today")
	outTurnsFlag := flag.String("out-turns", "pkg/telemetry/testdata/historical_turns.json", "Output path for TurnRecord fixture JSON")
	outExpectedFlag := flag.String("out-expected", "pkg/telemetry/testdata/historical_expected.json", "Output path for FixtureExpected JSON")
	flag.Parse()

	refTime, err := time.Parse(time.RFC3339, *refTimeFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid ref-time %q: %v\n", *refTimeFlag, err)
		os.Exit(1)
	}
	refTime = refTime.UTC()

	rng := rand.New(rand.NewPCG(*seedFlag, 0))

	todayKey := refTime.Format("2006-01-02")
	yesterdayKey := refTime.AddDate(0, 0, -1).Format("2006-01-02")
	monthPrefix := refTime.Format("2006-01")

	weekday := int(refTime.Weekday())
	if weekday == 0 { // Sunday
		weekday = 7
	}
	daysFromMonday := weekday - 1
	monday := refTime.AddDate(0, 0, -daysFromMonday)
	mondayKey := monday.Format("2006-01-02")

	var allRecords []telemetry.TurnRecord
	byDay := make(map[string]WindowExpected)

	var reqCounter int

	// Generate records going backwards from (daysFlag - 1) days ago up to today
	for d := *daysFlag - 1; d >= 0; d-- {
		dayDate := refTime.AddDate(0, 0, -d)
		dayKey := dayDate.Format("2006-01-02")
		midnight := time.Date(dayDate.Year(), dayDate.Month(), dayDate.Day(), 0, 0, 0, 0, time.UTC)

		turnsToday := 5 + int(rng.IntN(15)) // 5 to 19 turns
		ckToday := int(rng.IntN(3))         // 0 to 2 interventions

		var dayExp WindowExpected

		for t := 0; t < turnsToday; t++ {
			reqCounter++
			tokens := 5000 + int(rng.IntN(45000)) // 5k to 50k tokens
			isLocal := rng.IntN(2) == 0
			isCK := t < ckToday
			isFallback := isCK && (rng.IntN(2) == 0)
			isFD := t%8 == 3

			var costSpent, costSaved float64
			var tierName, model, provider string

			if isLocal {
				tierName = "tier_1_local"
				model = "gemma-4-12b-qat"
				provider = "local_vllm"
				costSpent = 0.0
				costSaved = float64(tokens) * 0.000004 // $4/1M saved
			} else {
				tierName = "tier_2_cloud_coder"
				model = "deepseek-v4-pro"
				provider = "openrouter"
				costSpent = float64(tokens) * 0.000002 // $2/1M spent
				costSaved = float64(tokens) * 0.000002 // $2/1M saved
			}

			// Deterministic timestamp within the day
			secOffset := rng.Int64N(86400)
			recTime := midnight.Add(time.Duration(secOffset) * time.Second)

			rec := telemetry.TurnRecord{
				Timestamp:             recTime,
				RequestID:             fmt.Sprintf("fix-req-%05d", reqCounter),
				SessionID:             fmt.Sprintf("sess-%03d", reqCounter%10),
				Tokens:                tokens,
				SelectedTier:          tierName,
				TargetModel:           model,
				Provider:              provider,
				IsLocal:               isLocal,
				IsFallback:            isFallback,
				LatencyMs:             50.0 + float64(rng.IntN(400)),
				StatusCode:            200,
				CostSpentUSD:          costSpent,
				CostSavedUSD:          costSaved,
				CycleBreakerTriggered: isCK,
				FairyDusted:           isFD,
				FairyDustEntry: func() string {
					if isFD {
						return "tactical_review"
					}
					return ""
				}(),
			}
			if isCK {
				rec.CycleBreakerReason = "repetition_ngram_loop"
				rec.CycleProseTokens = 350
				rec.CycleMaxNgramFreq = 8
			}

			allRecords = append(allRecords, rec)
			addRecordToExpected(&dayExp, rec)
		}

		finalizeWindow(&dayExp)
		byDay[dayKey] = dayExp
	}

	// Synthesize Expected windows from byDay buckets (matching daemon logic)
	var todayExp, yesterdayExp, weekExp, monthExp, allTimeExp WindowExpected

	if b, ok := byDay[todayKey]; ok {
		todayExp = b
	}
	if b, ok := byDay[yesterdayKey]; ok {
		yesterdayExp = b
	}

	for k, b := range byDay {
		// All time
		allTimeExp.Requests += b.Requests
		allTimeExp.TokensTotal += b.TokensTotal
		allTimeExp.TokensLocal += b.TokensLocal
		allTimeExp.CostSpentUSD += b.CostSpentUSD
		allTimeExp.CostSavedUSD += b.CostSavedUSD
		allTimeExp.CKInterventions += b.CKInterventions
		allTimeExp.CKAvoidedTokens += b.CKAvoidedTokens
		allTimeExp.CKAvoidedGPUSecs += b.CKAvoidedGPUSecs
		allTimeExp.CKStage1LocalHeals += b.CKStage1LocalHeals
		allTimeExp.CKStage2Escalations += b.CKStage2Escalations
		allTimeExp.FDTriggers += b.FDTriggers

		// Week: Monday through Today
		if k >= mondayKey && k <= todayKey {
			weekExp.Requests += b.Requests
			weekExp.TokensTotal += b.TokensTotal
			weekExp.TokensLocal += b.TokensLocal
			weekExp.CostSpentUSD += b.CostSpentUSD
			weekExp.CostSavedUSD += b.CostSavedUSD
			weekExp.CKInterventions += b.CKInterventions
			weekExp.CKAvoidedTokens += b.CKAvoidedTokens
			weekExp.CKAvoidedGPUSecs += b.CKAvoidedGPUSecs
			weekExp.CKStage1LocalHeals += b.CKStage1LocalHeals
			weekExp.CKStage2Escalations += b.CKStage2Escalations
			weekExp.FDTriggers += b.FDTriggers
		}

		// Month: Current Month through Today
		if len(k) >= 7 && k[:7] == monthPrefix && k <= todayKey {
			monthExp.Requests += b.Requests
			monthExp.TokensTotal += b.TokensTotal
			monthExp.TokensLocal += b.TokensLocal
			monthExp.CostSpentUSD += b.CostSpentUSD
			monthExp.CostSavedUSD += b.CostSavedUSD
			monthExp.CKInterventions += b.CKInterventions
			monthExp.CKAvoidedTokens += b.CKAvoidedTokens
			monthExp.CKAvoidedGPUSecs += b.CKAvoidedGPUSecs
			monthExp.CKStage1LocalHeals += b.CKStage1LocalHeals
			monthExp.CKStage2Escalations += b.CKStage2Escalations
			monthExp.FDTriggers += b.FDTriggers
		}
	}

	finalizeWindow(&weekExp)
	finalizeWindow(&monthExp)
	finalizeWindow(&allTimeExp)

	expectedOutput := FixtureExpected{
		ReferenceTime: *refTimeFlag,
		Today:         todayExp,
		Yesterday:     yesterdayExp,
		ThisWeek:      weekExp,
		ThisMonth:     monthExp,
		AllTime:       allTimeExp,
		ByDay:         byDay,
	}

	// Write TurnRecord JSON
	if err := os.MkdirAll(filepath.Dir(*outTurnsFlag), 0750); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create dir: %v\n", err)
		os.Exit(1)
	}

	turnsJSON, err := json.MarshalIndent(allRecords, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal turns: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*outTurnsFlag, turnsJSON, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write %q: %v\n", *outTurnsFlag, err)
		os.Exit(1)
	}

	// Write Expected JSON
	expJSON, err := json.MarshalIndent(expectedOutput, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal expected: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*outExpectedFlag, expJSON, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write %q: %v\n", *outExpectedFlag, err)
		os.Exit(1)
	}

	fmt.Printf("✓ Successfully generated %d TurnRecords into %s\n", len(allRecords), *outTurnsFlag)
	fmt.Printf("✓ Successfully generated expected metrics into %s\n", *outExpectedFlag)
}
