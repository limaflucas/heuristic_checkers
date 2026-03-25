// Package bot provides an in-process bot runner that plays a game
// by subscribing to its broadcast hub and calling a search algorithm
// whenever it is the bot's turn.
package bot

import (
	"time"

	"github.com/limaflucas/heuristic_checkers/algorithms/gameai"
	"github.com/limaflucas/heuristic_checkers/internal/engine"
)

// Algorithm is the signature shared by BFSChooseMove and DFSChooseMove.
type Algorithm func(pos engine.Position, color engine.Color) engine.Move

// ByName returns the algorithm function for a given name ("bfs" | "dfs").
// Returns nil for unknown names.
func ByName(name string) Algorithm {
	switch name {
	case "bfs":
		return gameai.BFSChooseMove
	case "dfs":
		return gameai.DFSChooseMove
	}
	return nil
}

// Run subscribes to the game's broadcast hub and makes moves whenever it is
// the bot's turn. It blocks until the game ends or the subscription closes.
// Call as a goroutine: go bot.Run(g, color, algo, delay).
func Run(g *engine.Game, color engine.Color, algo Algorithm, thinkDelay time.Duration) {
	ch := g.Subscribe()
	defer g.Unsubscribe(ch)

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

	move := algo(snap.Position, color)
	if move.From == 0 && move.To == 0 {
		return
	}
	_ = g.MakeMove(color, move)
}
