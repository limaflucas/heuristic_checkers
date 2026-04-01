// Package bot provides an in-process bot runner that plays a game
// by subscribing to its broadcast hub and calling a search algorithm
// whenever it is the bot's turn.
package bot

import (
	"time"

	"github.com/limaflucas/heuristic_checkers/algorithms/gameai"
	"github.com/limaflucas/heuristic_checkers/internal/engine"
)

// Algorithm is the canonical bot function signature.
// The *SearchStats parameter is nil-safe: bots populate it when non-nil.
type Algorithm func(pos engine.Position, color engine.Color, stats *gameai.SearchStats) engine.Move

// ByName returns the algorithm function for a given name.
// Returns nil for unknown names.
func ByName(name string) Algorithm {
	switch name {
	case "bfs":
		return gameai.BFSChooseMove
	case "dfs":
		return gameai.DFSChooseMove
	case "negamax":
		return gameai.NegamaxChooseMove
	case "mcts":
		return gameai.MCTSChooseMove
	case "pvs":
		return gameai.PVSChooseMove
	case "random":
		return gameai.RandomChooseMove
	}
	return nil
}

// Run subscribes to the game's broadcast hub and makes moves whenever it is
// the bot's turn. It blocks until the game ends or the subscription closes.
// Call as a goroutine: go bot.Run(g, color, algo, delay).
func Run(g *engine.Game, color engine.Color, algo Algorithm, thinkDelay time.Duration) {
	ch := g.Subscribe()
	defer func() {
		g.Unsubscribe(ch)
		ClearMoveStats(g.ID) // g.ID is a plain string field
	}()

	// Play immediately if it is already our turn.
	snap := g.Snapshot()
	if snap.Turn == color && snap.Status == engine.StatusInProgress {
		playTurn(g, color, algo, thinkDelay)
	}

	for snap := range ch {
		if snap.Status != engine.StatusInProgress {
			return
		}
		if snap.Turn == color {
			playTurn(g, color, algo, thinkDelay)
		}
	}
}

func playTurn(g *engine.Game, color engine.Color, algo Algorithm, delay time.Duration) {
	time.Sleep(delay)

	// Re-read state after delay — another goroutine may have moved first.
	snap := g.Snapshot()
	if snap.Turn != color || snap.Status != engine.StatusInProgress {
		return
	}

	var stats gameai.SearchStats
	move := algo(snap.Position, color, &stats)
	if move.From == 0 && move.To == 0 {
		return
	}
	SetMoveStats(g.ID, color, &stats) // g.ID is a string field
	_ = g.MakeMove(color, move, &stats)
}
