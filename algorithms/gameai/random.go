package gameai

import (
	"math/rand"

	"github.com/limaflucas/heuristic_checkers/internal/engine"
)

// RandomChooseMove is a baseline bot that picks a move uniformly at random.
func RandomChooseMove(pos engine.Position, color engine.Color, stats *SearchStats) engine.Move {
	moves := engine.LegalMoves(pos, color)
	if len(moves) == 0 {
		return engine.Move{}
	}
	
	if stats != nil {
		stats.NodesExpanded++
		stats.NodesEvaluated++
	}

	return moves[rand.Intn(len(moves))]
}
