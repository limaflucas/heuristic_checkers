package gameai

import "github.com/limaflucas/heuristic_checkers/internal/engine"

// negamaxEval scores pos from color's perspective for the Negamax bot.
// It is intentionally separate from Evaluate (used by BFS/DFS) so both
// can be tuned independently.
//
// Material:
//   Man  = +100
//   King = +150  (Kings dominate Checkers; higher than man, but not excessively)
//
// Positional bonuses (per piece):
//   Center control (internal rows 2-5, even cols 2-5): +10
//   Back-rank defense (Red on row 0, Black on row 7):  +15
func negamaxEval(pos engine.Position, color engine.Color) int {
	bm, rm, bk, rk := pos.RemainingCounts()

	// Material differential from Red's absolute perspective.
	score := (rm*100 + rk*150) - (bm*100 + bk*150)

	// Positional bonuses — iterate over all pieces.
	for sq := 0; sq < 32; sq++ {
		row := sq / 4 // 0 = Red home (bottom), 7 = Black home (top)

		isCenter := row >= 2 && row <= 5 && centerCol(sq)

		switch {
		case pos.IsRed(sq):
			if isCenter {
				score += 10
			}
			if row == 0 { // back rank for Red
				score += 15
			}
		case pos.IsBlack(sq):
			if isCenter {
				score -= 10
			}
			if row == 7 { // back rank for Black
				score -= 15
			}
		}
	}

	if color == engine.Red {
		return score
	}
	return -score
}

// centerCol returns true if the square is in the middle two board columns (cols 2-5).
// Internal layout: even rows use even board cols (0,2,4,6), odd rows use odd cols (1,3,5,7).
// Center columns 2-5 correspond to piece indices p=1 and p=2 in every row.
func centerCol(sq int) bool {
	p := sq % 4
	return p == 1 || p == 2
}
