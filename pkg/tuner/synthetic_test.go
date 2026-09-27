package tuner

import (
	"testing"
)

func TestGenerateSyntheticTrajectories_ModelsRealWorldFailures(t *testing.T) {
	const totalTurns = 2000
	trajectories := GenerateSyntheticTrajectories(totalTurns, 42)

	var hasTestFailCount int
	var hasTestPassCount int
	var hasChurnLoopCount int

	for _, traj := range trajectories {
		for _, turn := range traj.Turns {
			if turn.HasTestFail {
				hasTestFailCount++
			}
			if turn.HasTestPass {
				hasTestPassCount++
			}
			if turn.IsRetry && turn.HasWriteProgress && turn.HasTestFail {
				hasChurnLoopCount++
			}
		}
	}

	if hasTestFailCount == 0 {
		t.Errorf("Synthetic generator produced 0 turns with HasTestFail across %d turns; must model real-world test failures", totalTurns)
	}
	if hasTestPassCount == 0 {
		t.Errorf("Synthetic generator produced 0 turns with HasTestPass across %d turns; must model test resolutions", totalTurns)
	}
	if hasChurnLoopCount == 0 {
		t.Errorf("Synthetic generator produced 0 churn loop turns (IsRetry && HasWriteProgress && HasTestFail) across %d turns; must model agent diff-mangling churn", totalTurns)
	}
}
