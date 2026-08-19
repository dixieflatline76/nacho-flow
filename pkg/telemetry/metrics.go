package telemetry

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// TierBreakdown tracks the number of requests routed to each tier.
type TierBreakdown struct {
	Tier1LocalFree      int64 `json:"tier1_local_free"`
	Tier2CloudCoder     int64 `json:"tier2_cloud_coder"`
	Tier3CloudReasoning int64 `json:"tier3_cloud_reasoning"`
	Tier4CloudVision    int64 `json:"tier4_cloud_vision"`
	ExplicitOverride    int64 `json:"explicit_override"`
	Fallbacks           int64 `json:"fallbacks"`
}

// StatsSnapshot represents the JSON schema returned by /v1/stats.
type StatsSnapshot struct {
	StartedAt                string        `json:"started_at"`
	TotalRequests            int64         `json:"total_requests"`
	TierBreakdown            TierBreakdown `json:"tier_breakdown"`
	TotalTokensRoutedLocally int64         `json:"total_tokens_routed_locally"`
	EstimatedCostSavedUSD    float64       `json:"estimated_cost_saved_usd"`
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
	sinks    []ObservationSink
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
		bufferSize = 1000
	}
	if initial.StartedAt == "" {
		initial.StartedAt = time.Now().UTC().Format(time.RFC3339)
	}

	tracker := &StatsTracker{
		obsChan:  make(chan Observation, bufferSize),
		doneChan: make(chan struct{}),
		stats:    initial,
	}

	go tracker.worker()
	return tracker
}

// AddSink registers an ObservationSink to receive streaming observations.
func (s *StatsTracker) AddSink(sink ObservationSink) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sinks = append(s.sinks, sink)
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
			s.stats.EstimatedCostSavedUSD += obs.CostSaved
		}

		// Snapshot sinks for fanout
		sinks := make([]ObservationSink, len(s.sinks))
		copy(sinks, s.sinks)
		s.mu.Unlock()

		// Fan out asynchronously to sinks
		if len(sinks) > 0 {
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
			for _, sink := range sinks {
				sink.Emit(record)
			}
		}
	}
}

// Record dispatches an observation to the background worker channel. Non-blocking.
func (s *StatsTracker) Record(obs Observation) {
	s.mu.RLock()
	isClosed := s.closed
	s.mu.RUnlock()

	if isClosed {
		return
	}

	select {
	case s.obsChan <- obs:
	default:
		// Queue full: drop to avoid slowing proxy hot-path
	}
}

// Flush waits until all queued observations have been processed by the worker.
func (s *StatsTracker) Flush() {
	for len(s.obsChan) > 0 {
		time.Sleep(1 * time.Millisecond)
	}
	// Extra brief tick to ensure current lock release
	time.Sleep(2 * time.Millisecond)
}

// GetStats returns a thread-safe snapshot of the current stats.
func (s *StatsTracker) GetStats() StatsSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

// ServeHTTP exposes /v1/stats matching OpenAI/Python prototype contract.
func (s *StatsTracker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	stats := s.GetStats()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(stats)
}

// Close gracefully closes the observation channel and waits for worker termination.
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
