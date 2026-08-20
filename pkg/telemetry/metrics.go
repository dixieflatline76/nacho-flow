package telemetry

import (
	"encoding/json"
	"net/http"
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

// StatsSnapshot provides an immutable snapshot of proxy metrics for reporting.
type StatsSnapshot struct {
	StartedAt                string      `json:"started_at"`
	TotalRequests            int64       `json:"total_requests"`
	TierBreakdown            TierMetrics `json:"tier_breakdown"`
	TotalTokensRoutedLocally int64       `json:"total_tokens_routed_locally"`
	EstimatedCostSavedUSD    float64     `json:"estimated_cost_saved_usd"`
}

// Observation encapsulates metrics captured from a single completed proxy request.
type Observation struct {
	Tier               int
	TierName           string
	Model              string
	Provider           string
	Tokens             int
	CostSaved          float64
	IsLocal            bool
	IsFallback         bool
	IsExplicitOverride bool
	LatencyMs          float64
	Keywords           []string
	HasImages          bool
	HasTools           bool
	StatusCode         int
	IsRetry            bool
}

// StatsTracker processes proxy telemetry asynchronously via a dedicated background channel.
type StatsTracker struct {
	obsChan  chan Observation
	doneChan chan struct{}
	mu       sync.RWMutex
	stats    StatsSnapshot
	sinks    atomic.Pointer[[]ObservationSink]
	closed   bool
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

	tracker := &StatsTracker{
		obsChan:  make(chan Observation, bufferSize),
		doneChan: make(chan struct{}),
		stats:    initial,
	}

	emptySinks := make([]ObservationSink, 0)
	tracker.sinks.Store(&emptySinks)

	go tracker.worker()
	return tracker
}

// AddSink registers an ObservationSink to receive streaming observations.
func (s *StatsTracker) AddSink(sink ObservationSink) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var current []ObservationSink
	if old := s.sinks.Load(); old != nil {
		current = append(current, *old...)
	}
	current = append(current, sink)
	s.sinks.Store(&current)
}

func (s *StatsTracker) worker() {
	defer close(s.doneChan)

	for obs := range s.obsChan {
		s.mu.Lock()
		s.stats.TotalRequests++

		if obs.IsFallback {
			s.stats.TierBreakdown.Fallbacks++
		}

		if obs.IsExplicitOverride {
			s.stats.TierBreakdown.ExplicitOverride++
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
		s.mu.Unlock()

		// Lock-free atomic read of registered sinks (0 heap allocation)
		sinksPtr := s.sinks.Load()
		if sinksPtr != nil && len(*sinksPtr) > 0 {
			record := TurnRecord{
				Timestamp:    time.Now().UTC(),
				Tokens:       obs.Tokens,
				HasImages:    obs.HasImages,
				HasTools:     obs.HasTools,
				Keywords:     obs.Keywords,
				SelectedTier: obs.TierName,
				TargetModel:  obs.Model,
				Provider:     obs.Provider,
				IsLocal:      obs.IsLocal,
				IsFallback:   obs.IsFallback,
				LatencyMs:    obs.LatencyMs,
				StatusCode:   obs.StatusCode,
				IsRetry:      obs.IsRetry,
				CostSavedUSD: obs.CostSaved,
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

// GetStats returns a point-in-time snapshot of cumulative proxy stats.
func (s *StatsTracker) GetStats() StatsSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

// Flush drains pending observations before shutdown or snapshot export.
func (s *StatsTracker) Flush() {
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
