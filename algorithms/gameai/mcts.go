package gameai

import (
	"math"
	"math/rand"
	"time"

	"github.com/limaflucas/heuristic_checkers/internal/engine"
)

const (
	mctsTimeLimit    = 1800 * time.Millisecond
	mctsExploreC     = 1.41421356 // sqrt(2)
	mctsEpsilon      = 0.20       // random rollout probability
	mctsMaxRollout   = 80         // max moves per simulation
)

// mctsNode is a single node in the MCTS search tree.
type mctsNode struct {
	pos           engine.Position
	color         engine.Color   // who is to move at this node
	move          engine.Move    // move that produced this position (zero for root)
	parent        *mctsNode
	children      []*mctsNode
	unexpanded    []engine.Move  // legal moves not yet expanded
	visits        float64
	redWins       float64        // total accumulated value for Red in this subtree
}

// newMCTSNode creates a node, pre-populating the unexpanded move list.
func newMCTSNode(pos engine.Position, color engine.Color, move engine.Move, parent *mctsNode) *mctsNode {
	return &mctsNode{
		pos:        pos,
		color:      color,
		move:       move,
		parent:     parent,
		unexpanded: engine.LegalMoves(pos, color),
	}
}

// ── UCT selection ─────────────────────────────────────────────────────────────

// uct returns the UCT score of child from the parent's perspective.
// redWins is stored absolutely; we flip based on which player benefits.
func uct(child *mctsNode, parentVisits float64) float64 {
	if child.visits == 0 {
		return math.MaxFloat64
	}
	// Exploitation: wins from parent's color perspective.
	var exploit float64
	if child.parent != nil && child.parent.color == engine.Black {
		// Parent is Black → parent benefits from Red wins in this subtree.
		exploit = child.redWins / child.visits
	} else {
		// Parent is Red → parent benefits from Black wins (low redWins).
		exploit = (child.visits - child.redWins) / child.visits
	}
	explore := mctsExploreC * math.Sqrt(math.Log(parentVisits)/child.visits)
	return exploit + explore
}

// selectChild picks the child with the highest UCT score.
func (n *mctsNode) selectChild() *mctsNode {
	best := n.children[0]
	bestScore := uct(best, n.visits)
	for _, c := range n.children[1:] {
		if s := uct(c, n.visits); s > bestScore {
			bestScore = s
			best = c
		}
	}
	return best
}

// ── Expansion ─────────────────────────────────────────────────────────────────

// expand picks one unexpanded move, creates its child node, and returns it.
func (n *mctsNode) expand() *mctsNode {
	idx := len(n.unexpanded) - 1
	m := n.unexpanded[idx]
	n.unexpanded = n.unexpanded[:idx]
	child := newMCTSNode(engine.ApplyMove(n.pos, n.color, m), n.color.Opponent(), m, n)
	n.children = append(n.children, child)
	return child
}

// ── Rollout ───────────────────────────────────────────────────────────────────

var mctsRNG = rand.New(rand.NewSource(time.Now().UnixNano()))

// rollout simulates a game from pos using an ε-greedy policy guided by PST eval.
// Returns the rollout outcome from Red's perspective: 1.0 win, 0.5 draw, 0.0 loss.
func rollout(pos engine.Position, color engine.Color, w *PSTWeights, depth int) float64 {
	for i := 0; i < depth; i++ {
		moves := engine.LegalMoves(pos, color)
		if len(moves) == 0 {
			// Current player has no moves → they lose.
			if color == engine.Red {
				return 0.0
			}
			return 1.0
		}

		var chosen engine.Move
		if mctsRNG.Float64() < mctsEpsilon {
			// Random exploration
			chosen = moves[mctsRNG.Intn(len(moves))]
		} else {
			// PST-guided: pick best move for the current player (perspective-aware).
			bestScore := math.Inf(-1)
			chosen = moves[0]
			for _, m := range moves {
				child := engine.ApplyMove(pos, color, m)
				raw := PSTEval(child, w)
				// Score from current player's perspective
				var s float64
				if color == engine.Red {
					s = raw // Red wants high raw score
				} else {
					s = -raw // Black wants low raw score (high negative)
				}
				if s > bestScore {
					bestScore = s
					chosen = m
				}
			}
		}
		pos = engine.ApplyMove(pos, color, chosen)
		color = color.Opponent()
	}
	// Depth limit reached: use PST eval as static outcome estimate.
	raw := PSTEval(pos, w)
	return Sigmoid(raw) // maps to [0,1] from Red's perspective
}

// ── Backpropagation ───────────────────────────────────────────────────────────

func backpropagate(n *mctsNode, redWinValue float64) {
	for n != nil {
		n.visits++
		n.redWins += redWinValue
		n = n.parent
	}
}

// ── Main entry point ──────────────────────────────────────────────────────────

// MCTSChooseMove satisfies the bot.Algorithm signature.
// It runs MCTS with UCT selection, PST-guided ε-greedy rollout, and standard
// backpropagation until the 1.8 s deadline is reached.
func MCTSChooseMove(pos engine.Position, color engine.Color, stats *SearchStats) engine.Move {
	t := newTimer()
	moves := engine.LegalMoves(pos, color)
	if len(moves) == 0 {
		return engine.Move{}
	}
	if len(moves) == 1 {
		return moves[0]
	}

	w := GlobalPST()
	root := newMCTSNode(pos, color, engine.Move{}, nil)
	deadline := time.Now().Add(mctsTimeLimit)
	var rollouts int64

	for time.Now().Before(deadline) {
		// ── Selection
		node := root
		for len(node.unexpanded) == 0 && len(node.children) > 0 {
			node = node.selectChild()
		}

		// ── Expansion
		if len(node.unexpanded) > 0 {
			node = node.expand()
		}

		// ── Simulation
		result := rollout(node.pos, node.color, w, mctsMaxRollout)
		rollouts++

		// ── Backpropagation
		backpropagate(node, result)
	}

	// Pick the child of the root with the most visits (most robust selection).
	best := root.children[0]
	for _, c := range root.children[1:] {
		if c.visits > best.visits {
			best = c
		}
	}

	elapsedMs := t.elapsedMs()
	if stats != nil {
		stats.TotalRollouts = rollouts
		stats.ExecutionTimeMs = elapsedMs
		if elapsedMs > 0 {
			stats.SimulationsPerSec = float64(rollouts) / (elapsedMs / 1000.0)
		}
		stats.NodesExpanded = int64(countNodes(root))
	}

	return best.move
}

// countNodes returns the total node count in the MCTS tree (for stats).
func countNodes(n *mctsNode) int {
	count := 1
	for _, c := range n.children {
		count += countNodes(c)
	}
	return count
}
