package gameai

import (
	"math"
	"sort"
	"time"

	"github.com/limaflucas/heuristic_checkers/internal/engine"
)

const (
	ttFlagExact      uint8 = 0
	ttFlagLowerBound uint8 = 1
	ttFlagUpperBound uint8 = 2

	mateScore = 32000
	negInf    = -1 << 29
	posInf    = 1 << 29
	timeLimit = 1800 * time.Millisecond
)

// ttEntry is one slot in the transposition table.
type ttEntry struct {
	depth    int
	score    int
	flag     uint8
	bestMove engine.Move // for move ordering at deeper iterations
}

// NegamaxChooseMove satisfies the bot.Algorithm signature.
// It runs iterative-deepening Negamax with alpha-beta pruning and a Zobrist TT.
// Pass a non-nil *SearchStats to receive telemetry.
func NegamaxChooseMove(pos engine.Position, color engine.Color, stats *SearchStats) engine.Move {
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
		move, _, ok := negamaxSearch(pos, color, depth, 0, negInf, posInf, w, tt, deadline, stats)
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

// negamaxSearch is the recursive alpha-beta Negamax implementation.
func negamaxSearch(
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
			// Use PST-based evaluation (converted to int for alpha-beta comparisons)
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

	for _, m := range ordered {
		child := engine.ApplyMove(pos, color, m)
		_, childScore, ok := negamaxSearch(child, color.Opponent(), depth-1, rootDist+1, -β, -α, w, tt, deadline, stats)
		if !ok {
			return engine.Move{}, 0, false
		}
		score := -childScore

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

// orderMoves sorts moves for best alpha-beta efficiency:
// TT move first → captures (most first) → promotions → rest.
func orderMoves(moves []engine.Move, ttMove engine.Move, hasTTMove bool) []engine.Move {
	ordered := make([]engine.Move, 0, len(moves))
	if hasTTMove {
		for _, m := range moves {
			if engine.MovesEqual(m, ttMove) {
				ordered = append(ordered, m)
				break
			}
		}
	}
	rest := make([]engine.Move, 0, len(moves))
	for _, m := range moves {
		if hasTTMove && engine.MovesEqual(m, ttMove) {
			continue
		}
		rest = append(rest, m)
	}
	sort.SliceStable(rest, func(i, j int) bool {
		a, b := rest[i], rest[j]
		ac, bc := len(a.Captures), len(b.Captures)
		if ac != bc {
			return ac > bc
		}
		if a.Promoted != b.Promoted {
			return a.Promoted
		}
		return false
	})
	return append(ordered, rest...)
}

// PVLeaf runs a shallow Negamax search and returns the leaf position
// at the end of the Principal Variation — used by the TD-Leaf trainer.
func PVLeaf(pos engine.Position, color engine.Color, depth int, w *PSTWeights) engine.Position {
	if depth == 0 {
		return pos
	}
	moves := engine.LegalMoves(pos, color)
	if len(moves) == 0 {
		return pos
	}
	ordered := orderMoves(moves, engine.Move{}, false)
	bestScore := negInf
	bestChild := pos
	for _, m := range ordered {
		child := engine.ApplyMove(pos, color, m)
		score := int(PSTEvalColor(child, color, w))
		if score > bestScore {
			bestScore = score
			bestChild = child
		}
	}
	return PVLeaf(bestChild, color.Opponent(), depth-1, w)
}
