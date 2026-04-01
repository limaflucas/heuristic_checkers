package gameai

import (
	"math"
	"time"

	"github.com/limaflucas/heuristic_checkers/internal/engine"
)

// PVSChooseMove satisfies the bot.Algorithm signature.
// It runs iterative-deepening Principal Variation Search (PVS) with a Zobrist TT.
func PVSChooseMove(pos engine.Position, color engine.Color, stats *SearchStats) engine.Move {
	t := newTimer()
	moves := engine.LegalMoves(pos, color)
	if stats != nil {
		stats.NodesExpanded++
	}
	if len(moves) == 0 {
		return engine.Move{}
	}
	if len(moves) == 1 {
		return moves[0]
	}

	w := GlobalPST()
	tt := make(map[uint64]ttEntry, 1<<16)
	deadline := time.Now().Add(timeLimit)
	bestMove := moves[0]

	for depth := 1; ; depth++ {
		move, _, ok := pvsSearch(pos, color, depth, 0, negInf, posInf, w, tt, deadline, stats)
		if !ok {
			break
		}
		bestMove = move
		if stats != nil && depth > stats.MaxDepthReached {
			stats.MaxDepthReached = depth
		}
		if time.Now().After(deadline) {
			break
		}
	}

	if stats != nil {
		stats.ExecutionTimeMs = t.elapsedMs()
		if stats.NodesExpanded > 0 {
			stats.AverageBranchingFactor = float64(stats.NodesExpanded) / math.Max(1, float64(stats.NodesExpanded-stats.NodesEvaluated))
		}
	}
	return bestMove
}

// pvsSearch is the recursive PVS implementation.
func pvsSearch(
	pos engine.Position,
	color engine.Color,
	depth, rootDist int,
	α, β int,
	w *PSTWeights,
	tt map[uint64]ttEntry,
	deadline time.Time,
	stats *SearchStats,
) (engine.Move, int, bool) {

	if time.Now().After(deadline) {
		return engine.Move{}, 0, false
	}

	hash := ZobristHash(pos, color)
	var ttBestMove engine.Move
	var hasTTMove bool

	if entry, ok := tt[hash]; ok {
		if stats != nil {
			stats.TTHits++
		}
		if entry.depth >= depth {
			switch entry.flag {
			case ttFlagExact:
				return entry.bestMove, entry.score, true
			case ttFlagLowerBound:
				if entry.score > α {
					α = entry.score
				}
			case ttFlagUpperBound:
				if entry.score < β {
					β = entry.score
				}
			}
			if α >= β {
				if stats != nil {
					stats.TTCutoffs++
				}
				return entry.bestMove, entry.score, true
			}
		}
		ttBestMove = entry.bestMove
		hasTTMove = true
	}

	moves := engine.LegalMoves(pos, color)
	if stats != nil {
		stats.NodesExpanded++
	}

	if depth == 0 || len(moves) == 0 {
		var score int
		if len(moves) == 0 {
			score = -(mateScore - rootDist)
		} else {
			score = int(PSTEvalColor(pos, color, w))
		}
		if stats != nil {
			stats.NodesEvaluated++
		}
		return engine.Move{}, score, true
	}

	ordered := orderMoves(moves, ttBestMove, hasTTMove)
	origα := α
	bestScore := negInf
	bestMove := ordered[0]

	for i, m := range ordered {
		child := engine.ApplyMove(pos, color, m)
		var score int
		var ok bool

		if i == 0 {
			// First move (Principal Variation) — full window
			_, score, ok = pvsSearch(child, color.Opponent(), depth-1, rootDist+1, -β, -α, w, tt, deadline, stats)
			score = -score
		} else {
			// Subsequent moves — null window search
			_, score, ok = pvsSearch(child, color.Opponent(), depth-1, rootDist+1, -α-1, -α, w, tt, deadline, stats)
			score = -score

			if ok && score > α && score < β {
				// Re-search with full window if the null window search fails high
				_, score, ok = pvsSearch(child, color.Opponent(), depth-1, rootDist+1, -β, -α, w, tt, deadline, stats)
				score = -score
			}
		}

		if !ok {
			return engine.Move{}, 0, false
		}

		if score > bestScore {
			bestScore = score
			bestMove = m
		}
		if score > α {
			α = score
		}
		if α >= β {
			if stats != nil {
				stats.TTCutoffs++
			}
			break
		}
	}

	var flag uint8
	switch {
	case bestScore <= origα:
		flag = ttFlagUpperBound
	case bestScore >= β:
		flag = ttFlagLowerBound
	default:
		flag = ttFlagExact
	}
	tt[hash] = ttEntry{depth: depth, score: bestScore, flag: flag, bestMove: bestMove}

	return bestMove, bestScore, true
}
