package gameai

import "time"

// SearchStats collects performance metrics from a single bot move-choice call.
// Pass a non-nil pointer to any Algorithm to receive telemetry; pass nil to skip.
type SearchStats struct {
	// Time
	ExecutionTimeMs float64

	// Node counts
	NodesEvaluated int64 // calls to the static leaf evaluator
	NodesExpanded  int64 // calls to engine.LegalMoves (child generation)

	// Search tree shape
	MaxDepthReached        int
	AverageBranchingFactor float64 // NodesExpanded / non-leaf expansions

	// Negamax / alpha-beta specific
	TTHits    int64 // TT entries that produced a usable score or move ordering hint
	TTCutoffs int64 // beta cut-offs triggered

	// MCTS specific
	TotalRollouts     int64
	SimulationsPerSec float64
}

// Timer helps algorithms record wall-clock ExecutionTimeMs.
// Call start := NewTimer() at the top of the bot, then stats.Finish(start) at the end.
type statTimer struct{ t time.Time }

func newTimer() statTimer              { return statTimer{time.Now()} }
func (s statTimer) elapsedMs() float64 { return float64(time.Since(s.t).Nanoseconds()) / 1e6 }
