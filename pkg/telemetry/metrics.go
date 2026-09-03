package telemetry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// TierMetrics breaks down request counts across the multi-tiered model architecture.
type TierMetrics struct {
	Tier1LocalFree      int64 `json:"tier1_local_free"`
	Tier2CloudCoder     int64 `json:"tier2_cloud_coder"`
	Tier3CloudReasoning int64 `json:"tier3_cloud_reasoning"`
	Tier4CloudVision    int64 `json:"tier4_cloud_vision"`
	ExplicitOverride    int64 `json:"explicit_override"`
	Fallbacks           int64 `json:"fallbacks"`
}

// FairyDustMetrics captures proactive quality checkpoint telemetry.
type FairyDustMetrics struct {
	TotalTriggers int64 `json:"total_triggers"`
}

// TimeWindowMetrics captures aggregated volume, financial, and defense telemetry for a discrete timeframe.
type TimeWindowMetrics struct {
	Requests         int64              `json:"requests"`
	TokensTotal      int64              `json:"tokens_total"`
	TokensLocal      int64              `json:"tokens_local"`
	CostSpentUSD     float64            `json:"cost_spent_usd"`
	CostSavedUSD     float64            `json:"cost_saved_usd"`
	CostReductionPct float64            `json:"cost_reduction_pct"`
	CycleKiller      CycleKillerMetrics `json:"cycle_killer"`
	FairyDust        FairyDustMetrics   `json:"fairy_dust"`
}

// TimeWindowSnapshot holds pre-aggregated metrics across standard time horizons.
type TimeWindowSnapshot struct {
	Today     TimeWindowMetrics `json:"today"`
	Yesterday TimeWindowMetrics `json:"yesterday"`
	ThisWeek  TimeWindowMetrics `json:"this_week"`
	ThisMonth TimeWindowMetrics `json:"this_month"`
	AllTime   TimeWindowMetrics `json:"all_time"`
}

// CycleKillerMetrics captures cumulative defense telemetry for loop-severing interventions.
type CycleKillerMetrics struct {
	TotalInterventions     int64 `json:"total_interventions"`
	AvoidedRunawayTokens   int64 `json:"avoided_runaway_tokens"`
	AvoidedGPUSeconds      int64 `json:"avoided_gpu_seconds"`
	Stage1LocalHeals       int64 `json:"stage1_local_heals"`
	Stage2CloudEscalations int64 `json:"stage2_cloud_escalations"`
	SessionKickstarts      int64 `json:"session_kickstarts"`
	// LocalHealSuccessRatePct is computed on read in GetStats(), never stored to disk.
	// It is included here solely for JSON serialization to the /v1/stats REST endpoint.
	LocalHealSuccessRatePct float64 `json:"local_heal_success_rate_pct,omitempty"`
}

// estimatedAvoidedTokensPerIntervention is the empirical mean runaway token count
// observed in unmitigated benchmark runs (Gemma 4 12B QAT, nqueen-cline-4).
const estimatedAvoidedTokensPerIntervention = 8000

// estimatedLocalTokensPerSecond is the approximate local GPU inference throughput
// for 12B/14B QAT models on AMD ROCm, used to convert avoided tokens to GPU seconds.
const estimatedLocalTokensPerSecond = 35

// StatsSnapshot provides an immutable snapshot of proxy metrics for reporting.
type StatsSnapshot struct {
	StartedAt                string                       `json:"started_at"`
	TotalRequests            int64                        `json:"total_requests"`
	TierBreakdown            TierMetrics                  `json:"tier_breakdown"`
	TotalTokensRoutedLocally int64                        `json:"total_tokens_routed_locally"`
	TotalCostSpentUSD        float64                      `json:"total_cost_spent_usd"`
	EstimatedCostSavedUSD    float64                      `json:"estimated_cost_saved_usd"`
	CostReductionPct         float64                      `json:"cost_reduction_pct"`
	CycleKiller              CycleKillerMetrics           `json:"cycle_killer"`
	FairyDust                FairyDustMetrics             `json:"fairy_dust"`
	Windows                  TimeWindowSnapshot           `json:"windows"`
	DailyBuckets             map[string]TimeWindowMetrics `json:"daily_buckets,omitempty"`
}

// Observation encapsulates metrics captured from a single completed proxy request.
type Observation struct {
	Tier                      int
	TierName                  string
	Model                     string
	Provider                  string
	Tokens                    int
	CostSpent                 float64
	CostSaved                 float64
	IsLocal                   bool
	IsFallback                bool
	IsExplicitOverride        bool
	LatencyMs                 float64
	Keywords                  []string
	HasImages                 bool
	HasTools                  bool
	StatusCode                int
	IsRetry                   bool
	ForcedTier                string
	ForcedModel               string
	DirectiveUsed             string
	CycleBreakerTriggered     bool
	CycleBreakerReason        string
	CycleProseTokens          int
	CycleMaxNgramFreq         int
	CycleThinkingTokens       int
	CycleMaxThinkingNgramFreq int
	SessionKickstarted        bool
	CachedTokens              int
	UpstreamCost              float64
	FairyDusted               bool
	FairyDustEntry            string
	ObservedAt                time.Time
}

// StatsTracker processes proxy telemetry asynchronously via a dedicated background channel.
type StatsTracker struct {
	obsChan       chan Observation
	doneChan      chan struct{}
	mu            sync.RWMutex // guards stats, windows, and buckets
	sinkMu        sync.Mutex   // guards sink registration exclusively
	stats         StatsSnapshot
	allTimeTokens int64
	sinks         atomic.Pointer[[]ObservationSink]
	closed        bool

	activeDay   string
	activeWeek  string
	activeMonth string
}

// NewStatsTracker initializes and starts the background metrics aggregator.
func NewStatsTracker(bufferSize int) *StatsTracker {
	return NewStatsTrackerWithInitialSnapshot(bufferSize, StatsSnapshot{
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

// NewStatsTrackerWithInitialSnapshot initializes the tracker seeded with a previous snapshot.
func NewStatsTrackerWithInitialSnapshot(bufferSize int, initial StatsSnapshot) *StatsTracker {
	if bufferSize <= 0 {
		bufferSize = 50000
	}
	if initial.StartedAt == "" {
		initial.StartedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if initial.DailyBuckets == nil {
		initial.DailyBuckets = make(map[string]TimeWindowMetrics)
	}

	tracker := &StatsTracker{
		obsChan:  make(chan Observation, bufferSize),
		doneChan: make(chan struct{}),
		stats:    initial,
	}

	// Compute allTimeTokens from initial daily buckets or initialize
	for _, b := range initial.DailyBuckets {
		tracker.allTimeTokens += b.TokensTotal
	}
	if tracker.allTimeTokens < initial.TotalTokensRoutedLocally {
		tracker.allTimeTokens = initial.TotalTokensRoutedLocally
	}

	// Initialize AllTime baseline if restoring prior stats
	if tracker.stats.TotalRequests > 0 {
		allPct := reductionPct(tracker.stats.EstimatedCostSavedUSD, tracker.stats.TotalCostSpentUSD)
		tracker.stats.CostReductionPct = allPct
		tracker.stats.Windows.AllTime = TimeWindowMetrics{
			Requests:         tracker.stats.TotalRequests,
			TokensTotal:      tracker.allTimeTokens,
			TokensLocal:      tracker.stats.TotalTokensRoutedLocally,
			CostSpentUSD:     tracker.stats.TotalCostSpentUSD,
			CostSavedUSD:     tracker.stats.EstimatedCostSavedUSD,
			CostReductionPct: allPct,
			CycleKiller:      tracker.stats.CycleKiller,
		}
	}

	// If restoring from existing non-empty daily buckets, restore pre-aggregated windows once
	if len(initial.DailyBuckets) > 0 {
		tracker.restoreWindowsFromBuckets(time.Now().UTC())
	}

	emptySinks := make([]ObservationSink, 0)
	tracker.sinks.Store(&emptySinks)

	go tracker.worker()
	return tracker
}

// AddSink registers an ObservationSink using a dedicated registration mutex without locking the stats RWMutex.
func (s *StatsTracker) AddSink(sink ObservationSink) {
	s.sinkMu.Lock()
	defer s.sinkMu.Unlock()

	var current []ObservationSink
	if old := s.sinks.Load(); old != nil {
		current = append(current, *old...)
	}
	current = append(current, sink)
	s.sinks.Store(&current)
}

func (s *StatsTracker) restoreWindowsFromBuckets(now time.Time) {
	if s.stats.DailyBuckets == nil {
		s.stats.DailyBuckets = make(map[string]TimeWindowMetrics)
	}

	s.activeDay = now.Format("2006-01-02")
	year, week := now.ISOWeek()
	s.activeWeek = fmt.Sprintf("%d-W%02d", year, week)
	s.activeMonth = now.Format("2006-01")

	s.stats.Windows.Today = s.stats.DailyBuckets[s.activeDay]
	yesterdayKey := now.AddDate(0, 0, -1).Format("2006-01-02")
	s.stats.Windows.Yesterday = s.stats.DailyBuckets[yesterdayKey]

	weekday := int(now.Weekday())
	if weekday == 0 { // Sunday
		weekday = 7
	}
	daysFromMonday := weekday - 1
	monday := now.AddDate(0, 0, -daysFromMonday)
	mondayKey := monday.Format("2006-01-02")
	monthPrefix := s.activeMonth

	var weekMetrics TimeWindowMetrics
	var monthMetrics TimeWindowMetrics

	for k, b := range s.stats.DailyBuckets {
		if k >= mondayKey && k <= s.activeDay {
			addBucketToWindow(&weekMetrics, b)
		}
		if len(k) >= 7 && k[:7] == monthPrefix && k <= s.activeDay {
			addBucketToWindow(&monthMetrics, b)
		}
	}

	weekMetrics.CostReductionPct = reductionPct(weekMetrics.CostSavedUSD, weekMetrics.CostSpentUSD)
	monthMetrics.CostReductionPct = reductionPct(monthMetrics.CostSavedUSD, monthMetrics.CostSpentUSD)

	// Legacy migration: if the loaded snapshot was written by an older daemon build that did
	// not record cycle_killer inside daily buckets, all buckets will have zero CycleKiller
	// while the root accumulator may be nonzero. In that case we distribute the root
	// accumulator to all windows that had traffic. This is a one-time migration path;
	// modern daemon builds always write cycle_killer into every bucket, so isLegacySnapshot
	// will be false the next time the daemon saves and reloads stats.json.
	isLegacySnapshot := s.stats.CycleKiller.TotalInterventions > 0
	if isLegacySnapshot {
		for _, b := range s.stats.DailyBuckets {
			if b.CycleKiller.TotalInterventions > 0 {
				isLegacySnapshot = false
				break
			}
		}
	}
	if isLegacySnapshot {
		// Backfill the daily bucket with the most requests so this migration
		// becomes permanent after the next save/reload cycle. Without this,
		// isLegacySnapshot stays true on every restart because all buckets remain zero.
		var largestKey string
		var largestReqs int64
		for k, b := range s.stats.DailyBuckets {
			if b.Requests > largestReqs {
				largestReqs = b.Requests
				largestKey = k
			}
		}
		if largestKey != "" {
			bucket := s.stats.DailyBuckets[largestKey]
			bucket.CycleKiller = s.stats.CycleKiller
			s.stats.DailyBuckets[largestKey] = bucket
		}

		if s.stats.Windows.Yesterday.Requests > 0 {
			s.stats.Windows.Yesterday.CycleKiller = s.stats.CycleKiller
		}
		if weekMetrics.Requests > 0 {
			weekMetrics.CycleKiller = s.stats.CycleKiller
		}
		if monthMetrics.Requests > 0 {
			monthMetrics.CycleKiller = s.stats.CycleKiller
		}
	}

	s.stats.Windows.ThisWeek = weekMetrics
	s.stats.Windows.ThisMonth = monthMetrics

	allPct := reductionPct(s.stats.EstimatedCostSavedUSD, s.stats.TotalCostSpentUSD)
	s.stats.CostReductionPct = allPct
	s.stats.Windows.AllTime = TimeWindowMetrics{
		Requests:         s.stats.TotalRequests,
		TokensTotal:      s.allTimeTokens,
		TokensLocal:      s.stats.TotalTokensRoutedLocally,
		CostSpentUSD:     s.stats.TotalCostSpentUSD,
		CostSavedUSD:     s.stats.EstimatedCostSavedUSD,
		CostReductionPct: allPct,
		CycleKiller:      s.stats.CycleKiller,
		FairyDust:        s.stats.FairyDust,
	}
}

func (s *StatsTracker) pruneBucketsLocked(now time.Time) {
	cutoff := now.AddDate(0, 0, -31).Format("2006-01-02")
	for k := range s.stats.DailyBuckets {
		if k < cutoff {
			delete(s.stats.DailyBuckets, k)
		}
	}
}

func (s *StatsTracker) updateWindowsLocked(obs Observation, observedAt time.Time) {
	if s.stats.DailyBuckets == nil {
		s.stats.DailyBuckets = make(map[string]TimeWindowMetrics)
	}

	dayKey := observedAt.Format("2006-01-02")
	year, week := observedAt.ISOWeek()
	weekKey := fmt.Sprintf("%d-W%02d", year, week)
	monthKey := observedAt.Format("2006-01")

	tokens := int64(obs.Tokens)
	localTokens := int64(0)
	if obs.IsLocal {
		localTokens = tokens
	}

	// Rollover Sentinel Checks (O(1))
	if s.activeDay == "" {
		s.activeDay = dayKey
		s.activeWeek = weekKey
		s.activeMonth = monthKey
		s.stats.Windows.Today = s.stats.DailyBuckets[dayKey]
		yesterdayKey := observedAt.AddDate(0, 0, -1).Format("2006-01-02")
		s.stats.Windows.Yesterday = s.stats.DailyBuckets[yesterdayKey]
	} else if dayKey > s.activeDay {
		yesterdayKey := observedAt.AddDate(0, 0, -1).Format("2006-01-02")
		s.stats.Windows.Yesterday = s.stats.DailyBuckets[yesterdayKey]
		s.activeDay = dayKey
		s.stats.Windows.Today = s.stats.DailyBuckets[dayKey]

		if weekKey != s.activeWeek {
			s.activeWeek = weekKey
			s.stats.Windows.ThisWeek = TimeWindowMetrics{}
		}
		if monthKey != s.activeMonth {
			s.activeMonth = monthKey
			s.stats.Windows.ThisMonth = TimeWindowMetrics{}
		}

		s.pruneBucketsLocked(observedAt)
	}

	// Update persistent daily bucket
	bucket := s.stats.DailyBuckets[dayKey]
	addToWindow(&bucket, obs, tokens, localTokens)
	s.stats.DailyBuckets[dayKey] = bucket

	// Incremental O(1) accumulation on active horizons (0 loops, 0 heap allocations)
	if dayKey == s.activeDay {
		addToWindow(&s.stats.Windows.Today, obs, tokens, localTokens)
	} else {
		// If observation belongs to yesterday's bucket, maintain live Yesterday snapshot
		if activeTime, err := time.Parse("2006-01-02", s.activeDay); err == nil {
			if dayKey == activeTime.AddDate(0, 0, -1).Format("2006-01-02") {
				s.stats.Windows.Yesterday = bucket
			}
		}
	}
	if weekKey == s.activeWeek {
		addToWindow(&s.stats.Windows.ThisWeek, obs, tokens, localTokens)
	}
	if monthKey == s.activeMonth {
		addToWindow(&s.stats.Windows.ThisMonth, obs, tokens, localTokens)
	}

	// Incremental O(1) AllTime accumulation (0 struct allocations)
	addToWindow(&s.stats.Windows.AllTime, obs, tokens, localTokens)
	s.allTimeTokens += tokens
	s.stats.CostReductionPct = s.stats.Windows.AllTime.CostReductionPct
}

func addToWindow(w *TimeWindowMetrics, obs Observation, tokens, localTokens int64) {
	w.Requests++
	w.TokensTotal += tokens
	w.TokensLocal += localTokens
	w.CostSpentUSD += obs.CostSpent
	w.CostSavedUSD += obs.CostSaved
	w.CostReductionPct = reductionPct(w.CostSavedUSD, w.CostSpentUSD)

	if obs.CycleBreakerTriggered {
		w.CycleKiller.TotalInterventions++
		w.CycleKiller.AvoidedRunawayTokens += estimatedAvoidedTokensPerIntervention
		w.CycleKiller.AvoidedGPUSeconds += estimatedAvoidedTokensPerIntervention / estimatedLocalTokensPerSecond
		if obs.IsFallback {
			w.CycleKiller.Stage2CloudEscalations++
		} else {
			w.CycleKiller.Stage1LocalHeals++
		}
	}
	if obs.SessionKickstarted {
		w.CycleKiller.SessionKickstarts++
	}
	if obs.FairyDusted {
		w.FairyDust.TotalTriggers++
	}
}

func addBucketToWindow(w *TimeWindowMetrics, b TimeWindowMetrics) {
	w.Requests += b.Requests
	w.TokensTotal += b.TokensTotal
	w.TokensLocal += b.TokensLocal
	w.CostSpentUSD += b.CostSpentUSD
	w.CostSavedUSD += b.CostSavedUSD
	w.CostReductionPct = reductionPct(w.CostSavedUSD, w.CostSpentUSD)

	w.CycleKiller.TotalInterventions += b.CycleKiller.TotalInterventions
	w.CycleKiller.AvoidedRunawayTokens += b.CycleKiller.AvoidedRunawayTokens
	w.CycleKiller.AvoidedGPUSeconds += b.CycleKiller.AvoidedGPUSeconds
	w.CycleKiller.Stage1LocalHeals += b.CycleKiller.Stage1LocalHeals
	w.CycleKiller.Stage2CloudEscalations += b.CycleKiller.Stage2CloudEscalations
	w.CycleKiller.SessionKickstarts += b.CycleKiller.SessionKickstarts
	w.FairyDust.TotalTriggers += b.FairyDust.TotalTriggers
}

func computeHealRate(ck *CycleKillerMetrics) {
	if ck.TotalInterventions > 0 {
		ck.LocalHealSuccessRatePct = float64(ck.Stage1LocalHeals) / float64(ck.TotalInterventions) * 100
	} else {
		ck.LocalHealSuccessRatePct = 0
	}
}

func reductionPct(saved, spent float64) float64 {
	denom := saved + spent
	if denom > 0 {
		return (saved / denom) * 100.0
	}
	return 0.0
}

func (s *StatsTracker) worker() {
	defer close(s.doneChan)

	for obs := range s.obsChan {
		observedAt := obs.ObservedAt
		if observedAt.IsZero() {
			observedAt = time.Now().UTC()
		}

		s.mu.Lock()
		s.stats.TotalRequests++

		if obs.IsFallback {
			s.stats.TierBreakdown.Fallbacks++
		}

		if obs.IsExplicitOverride {
			s.stats.TierBreakdown.ExplicitOverride++
		} else if obs.IsLocal {
			s.stats.TierBreakdown.Tier1LocalFree++
		} else {
			switch obs.Tier {
			case 1:
				s.stats.TierBreakdown.Tier1LocalFree++
			case 2:
				s.stats.TierBreakdown.Tier2CloudCoder++
			case 3:
				s.stats.TierBreakdown.Tier3CloudReasoning++
			case 4:
				s.stats.TierBreakdown.Tier4CloudVision++
			default:
				s.stats.TierBreakdown.Tier2CloudCoder++
			}
		}

		if obs.IsLocal {
			s.stats.TotalTokensRoutedLocally += int64(obs.Tokens)
		}
		if obs.CostSaved > 0 {
			s.stats.EstimatedCostSavedUSD += obs.CostSaved
		}
		if obs.CostSpent > 0 {
			s.stats.TotalCostSpentUSD += obs.CostSpent
		}

		// Cycle Killer defense telemetry accumulation
		if obs.CycleBreakerTriggered {
			s.stats.CycleKiller.TotalInterventions++
			s.stats.CycleKiller.AvoidedRunawayTokens += estimatedAvoidedTokensPerIntervention
			s.stats.CycleKiller.AvoidedGPUSeconds += estimatedAvoidedTokensPerIntervention / estimatedLocalTokensPerSecond
			if obs.IsFallback {
				s.stats.CycleKiller.Stage2CloudEscalations++
			} else {
				s.stats.CycleKiller.Stage1LocalHeals++
			}
		}

		// Kickstart telemetry accumulation
		if obs.SessionKickstarted {
			s.stats.CycleKiller.SessionKickstarts++
		}

		// Fairy Dust telemetry accumulation
		if obs.FairyDusted {
			s.stats.FairyDust.TotalTriggers++
		}

		s.updateWindowsLocked(obs, observedAt)
		s.mu.Unlock()

		// Lock-free atomic read of registered sinks (0 heap allocation)
		sinksPtr := s.sinks.Load()
		if sinksPtr != nil && len(*sinksPtr) > 0 {
			record := TurnRecord{
				Timestamp:                 observedAt,
				Tokens:                    obs.Tokens,
				HasImages:                 obs.HasImages,
				HasTools:                  obs.HasTools,
				Keywords:                  obs.Keywords,
				SelectedTier:              obs.TierName,
				TargetModel:               obs.Model,
				Provider:                  obs.Provider,
				IsLocal:                   obs.IsLocal,
				IsFallback:                obs.IsFallback,
				LatencyMs:                 obs.LatencyMs,
				StatusCode:                obs.StatusCode,
				IsRetry:                   obs.IsRetry,
				CostSavedUSD:              obs.CostSaved,
				CostSpentUSD:              obs.CostSpent,
				ForcedTier:                obs.ForcedTier,
				ForcedModel:               obs.ForcedModel,
				DirectiveUsed:             obs.DirectiveUsed,
				CycleBreakerTriggered:     obs.CycleBreakerTriggered,
				CycleBreakerReason:        obs.CycleBreakerReason,
				CycleProseTokens:          obs.CycleProseTokens,
				CycleMaxNgramFreq:         obs.CycleMaxNgramFreq,
				CycleThinkingTokens:       obs.CycleThinkingTokens,
				CycleMaxThinkingNgramFreq: obs.CycleMaxThinkingNgramFreq,
				SessionKickstarted:        obs.SessionKickstarted,
				CachedTokens:              obs.CachedTokens,
				UpstreamCost:              obs.UpstreamCost,
				FairyDusted:               obs.FairyDusted,
				FairyDustEntry:            obs.FairyDustEntry,
			}
			for _, sink := range *sinksPtr {
				sink.Emit(record)
			}
		}
	}
}

// Record queues an observation for background processing without blocking the caller.
func (s *StatsTracker) Record(obs Observation) {
	select {
	case s.obsChan <- obs:
	default:
		// Queue full under extreme burst; drop gracefully to avoid blocking the proxy hot path
	}
}

// Reset clears all in-memory cumulative counters, window metrics, and daily buckets to zero.
func (s *StatsTracker) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	s.stats = StatsSnapshot{
		StartedAt:    now.Format(time.RFC3339),
		DailyBuckets: make(map[string]TimeWindowMetrics),
	}
	s.allTimeTokens = 0
	s.activeDay = now.Format("2006-01-02")
	year, week := now.ISOWeek()
	s.activeWeek = fmt.Sprintf("%d-W%02d", year, week)
	s.activeMonth = now.Format("2006-01")
}

// RecalculateFromRecords rebuilds all cumulative and windowed stats from an array of TurnRecord entries.
func (s *StatsTracker) RecalculateFromRecords(records []TurnRecord, oracle *PricingOracle, benchmarkCost float64) {
	s.RecalculateFromRecordsAt(records, oracle, benchmarkCost, time.Now().UTC())
}

// RecalculateFromRecordsAt rebuilds all cumulative and windowed stats from an array of TurnRecord entries anchored at referenceTime.
func (s *StatsTracker) RecalculateFromRecordsAt(records []TurnRecord, oracle *PricingOracle, benchmarkCost float64, referenceTime time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := referenceTime.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.stats = StatsSnapshot{
		StartedAt:    now.Format(time.RFC3339),
		DailyBuckets: make(map[string]TimeWindowMetrics),
	}
	s.allTimeTokens = 0
	s.activeDay = now.Format("2006-01-02")
	year, week := now.ISOWeek()
	s.activeWeek = fmt.Sprintf("%d-W%02d", year, week)
	s.activeMonth = now.Format("2006-01")

	for _, rec := range records {
		observedAt := rec.Timestamp
		if observedAt.IsZero() {
			observedAt = now
		}

		s.stats.TotalRequests++

		if rec.IsFallback {
			s.stats.TierBreakdown.Fallbacks++
		}

		if rec.IsLocal {
			s.stats.TierBreakdown.Tier1LocalFree++
			s.stats.TotalTokensRoutedLocally += int64(rec.Tokens)
		} else {
			tierLower := strings.ToLower(rec.SelectedTier)
			switch {
			case strings.Contains(tierLower, "tier_1") || strings.Contains(tierLower, "local"):
				s.stats.TierBreakdown.Tier1LocalFree++
			case strings.Contains(tierLower, "tier_3") || strings.Contains(tierLower, "reason"):
				s.stats.TierBreakdown.Tier3CloudReasoning++
			case strings.Contains(tierLower, "tier_4") || strings.Contains(tierLower, "vision"):
				s.stats.TierBreakdown.Tier4CloudVision++
			default:
				s.stats.TierBreakdown.Tier2CloudCoder++
			}
		}

		costSpent := rec.CostSpentUSD
		costSaved := rec.CostSavedUSD
		if oracle != nil && rec.Tokens > 0 {
			promptToks := int(float64(rec.Tokens) * 0.8)
			compToks := rec.Tokens - promptToks
			if promptToks <= 0 {
				promptToks = rec.Tokens
				compToks = 0
			}
			spent, saved := oracle.CalculateFinancials(rec.Provider, rec.TargetModel, rec.IsLocal, promptToks, compToks, rec.CachedTokens, rec.UpstreamCost, benchmarkCost)
			costSpent = spent
			costSaved = saved
		}

		if costSaved > 0 {
			s.stats.EstimatedCostSavedUSD += costSaved
		}
		if costSpent > 0 {
			s.stats.TotalCostSpentUSD += costSpent
		}

		// Cycle Killer recalculation from historical records
		if rec.CycleBreakerTriggered {
			s.stats.CycleKiller.TotalInterventions++
			s.stats.CycleKiller.AvoidedRunawayTokens += estimatedAvoidedTokensPerIntervention
			s.stats.CycleKiller.AvoidedGPUSeconds += estimatedAvoidedTokensPerIntervention / estimatedLocalTokensPerSecond
			if rec.IsFallback {
				s.stats.CycleKiller.Stage2CloudEscalations++
			} else {
				s.stats.CycleKiller.Stage1LocalHeals++
			}
		}
		if rec.SessionKickstarted {
			s.stats.CycleKiller.SessionKickstarts++
		}

		// FairyDust recalculation from historical records
		if rec.FairyDusted {
			s.stats.FairyDust.TotalTriggers++
		}

		obs := Observation{
			TierName:              rec.SelectedTier,
			Model:                 rec.TargetModel,
			Provider:              rec.Provider,
			Tokens:                rec.Tokens,
			CostSpent:             costSpent,
			CostSaved:             costSaved,
			IsLocal:               rec.IsLocal,
			IsFallback:            rec.IsFallback,
			CycleBreakerTriggered: rec.CycleBreakerTriggered,
			SessionKickstarted:    rec.SessionKickstarted,
			FairyDusted:           rec.FairyDusted,
			FairyDustEntry:        rec.FairyDustEntry,
		}
		s.updateWindowsLocked(obs, observedAt)
	}

	s.restoreWindowsFromBuckets(now)
	s.stats.CostReductionPct = reductionPct(s.stats.EstimatedCostSavedUSD, s.stats.TotalCostSpentUSD)
	s.stats.Windows.AllTime.CostReductionPct = s.stats.CostReductionPct
}

// GetStats returns a point-in-time snapshot of cumulative proxy stats in O(1) time.
func (s *StatsTracker) GetStats() StatsSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap := s.stats
	// Compute derived Cycle Killer heal rate on read per window (never stored to disk)
	computeHealRate(&snap.CycleKiller)
	computeHealRate(&snap.Windows.Today.CycleKiller)
	computeHealRate(&snap.Windows.Yesterday.CycleKiller)
	computeHealRate(&snap.Windows.ThisWeek.CycleKiller)
	computeHealRate(&snap.Windows.ThisMonth.CycleKiller)
	computeHealRate(&snap.Windows.AllTime.CycleKiller)
	if s.stats.DailyBuckets != nil {
		snap.DailyBuckets = make(map[string]TimeWindowMetrics, len(s.stats.DailyBuckets))
		for k, bucket := range s.stats.DailyBuckets {
			computeHealRate(&bucket.CycleKiller)
			snap.DailyBuckets[k] = bucket
		}
	}
	return snap
}

// Flush drains pending observations before shutdown or snapshot export.
func (s *StatsTracker) Flush() {
	deadline := time.Now().Add(1 * time.Second)
	for len(s.obsChan) > 0 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	time.Sleep(10 * time.Millisecond)
}

// Close gracefully closes the observation channel and waits for worker completion.
func (s *StatsTracker) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.obsChan)
	s.mu.Unlock()
	<-s.doneChan
}

// Handler returns an http.HandlerFunc providing a /v1/stats JSON endpoint.
func (s *StatsTracker) Handler() http.HandlerFunc {
	return s.ServeHTTP
}

// ServeHTTP implements http.Handler for the /v1/stats JSON endpoint.
func (s *StatsTracker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := s.GetStats()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stats)
}
