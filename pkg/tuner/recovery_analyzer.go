package tuner

// RecoveryStats captures empirical self-healing dynamics for an individual model.
type RecoveryStats struct {
	Model             string  `json:"model"`
	TotalFailures     int     `json:"total_failures"`       // turns where IsRetry=true
	SelfRecoveries    int     `json:"self_recoveries"`      // failures where subsequent turn by same model showed progress
	SelfRecoveryRate  float64 `json:"self_recovery_rate"`   // SelfRecoveries / TotalFailures
	AvgTurnsToRecover float64 `json:"avg_turns_to_recover"` // average turns from failure to next progress
}

// AnalyzeRecovery computes per-model self-recovery statistics from session trajectories.
func AnalyzeRecovery(trajectories []SessionTrajectory) map[string]RecoveryStats {
	if len(trajectories) == 0 {
		return map[string]RecoveryStats{}
	}

	type modelAccumulator struct {
		totalFailures       int
		selfRecoveries      int
		totalTurnsToRecover int
	}

	accum := make(map[string]*modelAccumulator)

	for _, traj := range trajectories {
		for i, turn := range traj.Turns {
			model := turn.TargetModel
			if model == "" {
				continue
			}

			if _, ok := accum[model]; !ok {
				accum[model] = &modelAccumulator{}
			}

			if turn.IsRetry {
				accum[model].totalFailures++

				// Look ahead in the trajectory for the next turn by this model
				turnsElapsed := 0
				recovered := false

				for j := i + 1; j < len(traj.Turns); j++ {
					nextTurn := traj.Turns[j]
					turnsElapsed++

					// If a different model intervened (e.g. escalated to cloud), not a self-recovery
					if nextTurn.TargetModel != model {
						break
					}

					// Did this turn have forward progress?
					if nextTurn.HasWriteProgress || nextTurn.HasTestPass || (!nextTurn.IsRetry && (nextTurn.StatusCode == 0 || nextTurn.StatusCode == 200)) {
						recovered = true
						break
					}
				}

				if recovered {
					accum[model].selfRecoveries++
					accum[model].totalTurnsToRecover += turnsElapsed
				}
			}
		}
	}

	results := make(map[string]RecoveryStats, len(accum))
	for model, data := range accum {
		var rate float64
		var avgTurns float64

		if data.totalFailures > 0 {
			rate = float64(data.selfRecoveries) / float64(data.totalFailures)
		}
		if data.selfRecoveries > 0 {
			avgTurns = float64(data.totalTurnsToRecover) / float64(data.selfRecoveries)
		}

		results[model] = RecoveryStats{
			Model:             model,
			TotalFailures:     data.totalFailures,
			SelfRecoveries:    data.selfRecoveries,
			SelfRecoveryRate:  rate,
			AvgTurnsToRecover: avgTurns,
		}
	}

	return results
}
