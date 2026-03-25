package gameai

import (
	"math"

	"github.com/limaflucas/heuristic_checkers/internal/engine"
)

// DFSChooseMove uses an explicit LIFO stack to explore the game tree depth-first.
//
// Algorithm:
//  1. Push all legal moves at depth 1, recording which first-ply move led there.
//  2. Pop each state (LIFO); if depth == maxDepth (or terminal) evaluate and record
//     the score against the originating first-ply move. We keep the BEST score seen
//     for each first-ply move (optimistic depth-first — take best leaf encountered).
//  3. Push children for shallower states.
//  4. Return the first-ply move with the highest best-leaf score.
//
// The stack causes child states to be explored in LIFO order (deep before wide),
// which is the fundamental distinction from BFS.

const dfsMaxDepth = 5

type dfsItem struct {
	pos     engine.Position
	moveIdx int
	depth   int
	player  engine.Color
}

// DFSChooseMove returns the best move for color in pos using depth-first search.
func DFSChooseMove(pos engine.Position, color engine.Color) engine.Move {
	initial := engine.LegalMoves(pos, color)
	if len(initial) == 0 {
		return engine.Move{}
	}
	if len(initial) == 1 {
		return initial[0]
	}

	// scores[i] = best leaf score seen for initial move i (optimistic DFS)
	scores := make([]int, len(initial))
	for i := range scores {
		scores[i] = math.MinInt32
	}

	// Use a slice as a LIFO stack (append = push, pop from end)
	stack := make([]dfsItem, 0, 64)
	for i, m := range initial {
		newPos := engine.ApplyMove(pos, color, m)
		stack = append(stack, dfsItem{newPos, i, 1, color.Opponent()})
	}

	for len(stack) > 0 {
		// Pop from the end (LIFO)
		n := len(stack) - 1
		item := stack[n]
		stack = stack[:n]

		moves := engine.LegalMoves(item.pos, item.player)

		if item.depth >= dfsMaxDepth || len(moves) == 0 {
			// Leaf: evaluate from our (color's) perspective
			sc := Evaluate(item.pos, color)
			if sc > scores[item.moveIdx] {
				scores[item.moveIdx] = sc // keep best leaf score for this first move
			}
			continue
		}

		// Push children onto the stack (LIFO — last pushed = first explored next)
		for _, m := range moves {
			newPos := engine.ApplyMove(item.pos, item.player, m)
			stack = append(stack, dfsItem{newPos, item.moveIdx, item.depth + 1, item.player.Opponent()})
		}
	}

	best := 0
	for i := 1; i < len(scores); i++ {
		if scores[i] > scores[best] {
			best = i
		}
	}
	return initial[best]
}
