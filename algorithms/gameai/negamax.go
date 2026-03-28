package gameai

import (
	"sort"
	"time"

	"github.com/limaflucas/heuristic_checkers/internal/engine"
)

const (
	ttFlagExact      uint8 = 0
	ttFlagLowerBound uint8 = 1
	ttFlagUpperBound uint8 = 2

	mateScore    = 32000
	negInf       = -1 << 29
	posInf       = 1 << 29
	timeLimit    = 1800 * time.Millisecond
)

// ttEntry is one slot in the transposition table.
type ttEntry struct {
	depth    int
	score    int
	flag     uint8
	bestMove engine.Move // best move found at this node; used for move ordering at deeper iterations
}

// NegamaxChooseMove is the public entry point that satisfies the bot.Algorithm type.
// It runs iterative-deepening Negamax with alpha-beta pruning, Zobrist hashing TT,
// and heuristic move ordering. The search runs until 1.8 s have elapsed.
func NegamaxChooseMove(pos engine.Position, color engine.Color) engine.Move {
	moves := engine.LegalMoves(pos, color)
	if len(moves) == 0 {
		return engine.Move{}
	}
	if len(moves) == 1 {
		return moves[0]
	}

	// Transposition table: created once, persists across ALL depth iterations.
	tt := make(map[uint64]ttEntry, 1<<16)

	deadline := time.Now().Add(timeLimit)
	bestMove := moves[0] // fallback: always have something to return

	for depth := 1; ; depth++ {
		move, _, ok := negamax(pos, color, depth, 0, negInf, posInf, tt, deadline)
		if !ok {
			// Time ran out mid-search; keep the best move from the previous completed depth.
			break
		}
		bestMove = move
		if time.Now().After(deadline) {
			break
		}
	}

	return bestMove
}

// negamax performs alpha-beta Negamax from pos with color to move.
//   depth    – remaining plies to search (0 = leaf)
//   rootDist – distance from the root (used for depth-adjusted mate scores)
//   α, β     – window
//   tt       – transposition table (shared across ID iterations)
//   deadline – wall-clock deadline; returns ok=false if exceeded
//
// Returns (bestMove, score, ok).
func negamax(
	pos engine.Position,
	color engine.Color,
	depth, rootDist int,
	α, β int,
	tt map[uint64]ttEntry,
	deadline time.Time,
) (engine.Move, int, bool) {

	// ── Time check ────────────────────────────────────────────
	if time.Now().After(deadline) {
		return engine.Move{}, 0, false
	}

	// ── Transposition table lookup ─────────────────────────────
	hash := ZobristHash(pos, color)
	var ttBestMove engine.Move
	var hasTTMove bool
	if entry, ok := tt[hash]; ok {
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
				return entry.bestMove, entry.score, true
			}
		}
		// Even if we can't use the score, use the stored best move for ordering.
		ttBestMove = entry.bestMove
		hasTTMove = true
	}

	// ── Leaf / terminal ───────────────────────────────────────
	moves := engine.LegalMoves(pos, color)
	if depth == 0 || len(moves) == 0 {
		var score int
		if len(moves) == 0 {
			// No legal moves → current player loses.
			score = -(mateScore - rootDist)
		} else {
			score = negamaxEval(pos, color)
		}
		return engine.Move{}, score, true
	}

	// ── Move ordering ─────────────────────────────────────────
	ordered := orderMoves(moves, ttBestMove, hasTTMove)

	// ── Recursive search ──────────────────────────────────────
	origα := α
	bestScore := negInf
	bestMove := ordered[0]

	for _, m := range ordered {
		child := engine.ApplyMove(pos, color, m)
		_, childScore, ok := negamax(child, color.Opponent(), depth-1, rootDist+1, -β, -α, tt, deadline)
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
			break // beta cut-off
		}
	}

	// ── Transposition table store ──────────────────────────────
	var flag uint8
	switch {
	case bestScore <= origα:
		flag = ttFlagUpperBound
	case bestScore >= β:
		flag = ttFlagLowerBound
	default:
		flag = ttFlagExact
	}
	tt[hash] = ttEntry{
		depth:    depth,
		score:    bestScore,
		flag:     flag,
		bestMove: bestMove,
	}

	return bestMove, bestScore, true
}

// orderMoves sorts moves for best alpha-beta cut-off efficiency:
//  1. TT best move from previous depth iteration (if available)
//  2. Captures (most captures first)
//  3. Promotions
//  4. All other moves
func orderMoves(moves []engine.Move, ttMove engine.Move, hasTTMove bool) []engine.Move {
	ordered := make([]engine.Move, 0, len(moves))

	// Pull TT move to the front if it exists in the list.
	if hasTTMove {
		for _, m := range moves {
			if engine.MovesEqual(m, ttMove) {
				ordered = append(ordered, m)
				break
			}
		}
	}

	// Sort the remaining moves: captures (desc by count) > promotions > rest.
	rest := make([]engine.Move, 0, len(moves))
	for _, m := range moves {
		if hasTTMove && engine.MovesEqual(m, ttMove) {
			continue // already added
		}
		rest = append(rest, m)
	}

	sort.SliceStable(rest, func(i, j int) bool {
		a, b := rest[i], rest[j]
		// Captures first (more captures = higher priority)
		ac, bc := len(a.Captures), len(b.Captures)
		if ac != bc {
			return ac > bc
		}
		// Promotions before quiet moves
		if a.Promoted != b.Promoted {
			return a.Promoted
		}
		return false
	})

	return append(ordered, rest...)
}
