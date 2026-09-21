package tuner

import (
	"sort"

	"github.com/dixieflatline76/nacho-flow/pkg/telemetry"
)

// SessionTrajectory represents a complete ordered sequence of turns within one agentic session.
type SessionTrajectory struct {
	SessionID    string                 `json:"session_id"`
	Turns        []telemetry.TurnRecord `json:"turns"` // sorted by Timestamp ascending
	TotalTurns   int                    `json:"total_turns"`
	TotalCost    float64                `json:"total_cost"`
	TotalRetries int                    `json:"total_retries"`
	Resolved     bool                   `json:"resolved"` // Did the session end with successful tool progress?
}

// GroupBySession takes flat TurnRecord slices and returns ordered session trajectories.
// Records with empty SessionID are dropped from v2 tuning.
func GroupBySession(records []telemetry.TurnRecord) []SessionTrajectory {
	if len(records) == 0 {
		return []SessionTrajectory{}
	}

	grouped := make(map[string][]telemetry.TurnRecord)
	for _, r := range records {
		if r.SessionID == "" {
			continue
		}
		grouped[r.SessionID] = append(grouped[r.SessionID], r)
	}

	if len(grouped) == 0 {
		return []SessionTrajectory{}
	}

	trajectories := make([]SessionTrajectory, 0, len(grouped))

	for sessionID, turns := range grouped {
		sort.SliceStable(turns, func(i, j int) bool {
			return turns[i].Timestamp.Before(turns[j].Timestamp)
		})

		var totalCost float64
		var totalRetries int

		for _, turn := range turns {
			totalCost += turn.CostSpentUSD
			if turn.IsRetry {
				totalRetries++
			}
		}

		resolved := false
		if len(turns) > 0 {
			lastTurn := turns[len(turns)-1]
			// Forward progress: write progress, test pass, or finished turn without being a retry
			if lastTurn.HasWriteProgress || lastTurn.HasTestPass || (!lastTurn.IsRetry && (lastTurn.StatusCode == 0 || lastTurn.StatusCode == 200)) {
				resolved = true
			}
		}

		trajectories = append(trajectories, SessionTrajectory{
			SessionID:    sessionID,
			Turns:        turns,
			TotalTurns:   len(turns),
			TotalCost:    totalCost,
			TotalRetries: totalRetries,
			Resolved:     resolved,
		})
	}

	// Sort trajectories by SessionID for deterministic ordering
	sort.Slice(trajectories, func(i, j int) bool {
		return trajectories[i].SessionID < trajectories[j].SessionID
	})

	return trajectories
}
