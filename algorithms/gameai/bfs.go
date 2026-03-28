package gameai

import (
	"math"

	"github.com/limaflucas/heuristic_checkers/internal/engine"
)

// BFSChooseMove uses a FIFO queue to explore the game tree breadth-first.
//
// Algorithm:
//  1. Enqueue all legal moves at depth 1, recording which first-ply move led there.
//  2. Dequeue each state; if depth == maxDepth (or terminal) evaluate and record the
//     score against the originating first-ply move using a min-score strategy
//     (assume opponent plays to minimise our score at even depths).
//  3. Enqueue children for shallower states.
//  4. Return the first-ply move whose associated min-score is highest.
//
// Depth is kept at 2 to balance quality and queue memory.

const bfsMaxDepth = 2

type bfsItem struct {
	pos      engine.Position
	moveIdx  int          // index into the initial moves slice
	depth    int
	player   engine.Color // whose turn it is at this state
}

// BFSChooseMove returns the best move for color in pos using breadth-first search.
func BFSChooseMove(pos engine.Position, color engine.Color, stats *SearchStats) engine.Move {
	t := newTimer()
	initial := engine.LegalMoves(pos, color)
	if stats != nil {
		stats.NodesExpanded++
	}
	if len(initial) == 0 {
		return engine.Move{}
	}
	if len(initial) == 1 {
		return initial[0]
	}

	// scores[i] = worst-case score from opponent ply for initial move i
	scores := make([]int, len(initial))
	for i := range scores {
		scores[i] = math.MaxInt32
	}

	queue := make([]bfsItem, 0, len(initial)*8)
	for i, m := range initial {
		newPos := engine.ApplyMove(pos, color, m)
		queue = append(queue, bfsItem{newPos, i, 1, color.Opponent()})
	}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:] // dequeue (FIFO)

		moves := engine.LegalMoves(item.pos, item.player)
		if stats != nil {
			stats.NodesExpanded++
		}

		if item.depth >= bfsMaxDepth || len(moves) == 0 {
			// Leaf: evaluate from our (color's) perspective
			sc := Evaluate(item.pos, color)
			if stats != nil {
				stats.NodesEvaluated++
			}
			if sc < scores[item.moveIdx] {
				scores[item.moveIdx] = sc // take the worst opponent response
			}
			continue
		}

		// Expand children
		for _, m := range moves {
			newPos := engine.ApplyMove(item.pos, item.player, m)
			queue = append(queue, bfsItem{newPos, item.moveIdx, item.depth + 1, item.player.Opponent()})
		}
	}

	// Choose first-ply move with the highest guaranteed (maximin) score
	best := 0
	for i := 1; i < len(scores); i++ {
		if scores[i] > scores[best] {
			best = i
		}
	}
	if stats != nil {
		stats.MaxDepthReached = bfsMaxDepth
		stats.ExecutionTimeMs = t.elapsedMs()
	}
	return initial[best]
}
