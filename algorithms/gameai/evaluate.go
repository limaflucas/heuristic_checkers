// Package gameai provides game-tree search algorithms for English Checkers.
// The position evaluation heuristic is shared between BFS and DFS.
package gameai

import "github.com/limaflucas/heuristic_checkers/internal/engine"

// Evaluate scores pos from color's perspective.
//   positive → good for color
//   negative → bad  for color
//
// Scoring:
//   Man  = 100 pts
//   King = 300 pts
//   Man advancement bonus: +3 pts per row advanced from home
func Evaluate(pos engine.Position, color engine.Color) int {
	bm, rm, bk, rk := pos.RemainingCounts()
	score := (rm*100 + rk*300) - (bm*100 + bk*300)

	// Advancement bonus for men (encourages pushing toward king-row)
	// Internal layout: row = sq/4, 0=Red home (bottom), 7=Black home (top)
	// Red men advance toward row 7 → higher sq/4 → higher sq
	// Black men advance toward row 0 → lower sq/4 → lower sq
	for _, sq := range pos.PieceSquares(engine.Red) {
		if !pos.IsKing(sq) {
			score += (sq / 4) * 3
		}
	}
	for _, sq := range pos.PieceSquares(engine.Black) {
		if !pos.IsKing(sq) {
			score -= (7 - sq/4) * 3
		}
	}

	if color == engine.Red {
		return score
	}
	return -score
}
