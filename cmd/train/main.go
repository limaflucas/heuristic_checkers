// cmd/train/main.go — TD-Leaf(λ) offline trainer for the Checkers PST weights.
//
// Self-plays Negamax-vs-Negamax games and applies the TD-Leaf update rule:
//   at each step, the gradient comes from the PV leaf (not the root position),
//   and scores are mapped through a sigmoid before computing temporal differences.
//
// Usage:
//
//	go run ./cmd/train [flags]
//	go run ./cmd/train --games 500 --alpha 0.001 --lambda 0.7 --depth 2 --out weights/pst_weights.json
package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/limaflucas/heuristic_checkers/algorithms/gameai"
	"github.com/limaflucas/heuristic_checkers/internal/engine"
)

func main() {
	games  := flag.Int("games", 1000, "number of self-play games")
	alpha  := flag.Float64("alpha", 0.001, "learning rate")
	lambda := flag.Float64("lambda", 0.7, "TD(λ) trace decay")
	depth  := flag.Int("depth", 2, "PV search depth for leaf extraction")
	out    := flag.String("out", "weights/pst_weights.json", "output weights file")
	flag.Parse()

	log.Printf("TD-Leaf trainer: games=%d α=%.4f λ=%.4f PV-depth=%d", *games, *alpha, *lambda, *depth)

	w := gameai.LoadPSTWeights(*out)
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	totalWins, totalDraws, totalLosses := 0, 0, 0

	for g := 1; g <= *games; g++ {
		// Self-play one game and collect positions + outcome.
		positions, outcome := selfPlayGame(w, rng, *depth)

		switch outcome {
		case 1.0:
			totalWins++
		case 0.5:
			totalDraws++
		default:
			totalLosses++
		}

		// TD-Leaf(λ) online eligibility-trace update.
		tdLeafUpdate(w, positions, outcome, *alpha, *lambda, *depth)

		if g%100 == 0 {
			log.Printf("Game %d/%d — Red W:%d D:%d L:%d", g, *games, totalWins, totalDraws, totalLosses)
			if err := gameai.SavePSTWeights(*out, w); err != nil {
				log.Printf("WARN: could not save weights: %v", err)
			}
		}
	}

	if err := gameai.SavePSTWeights(*out, w); err != nil {
		log.Fatalf("failed to save weights: %v", err)
	}
	fmt.Printf("Training complete. Weights saved to %s\n", *out)
	fmt.Printf("Final record: W=%d D=%d L=%d\n", totalWins, totalDraws, totalLosses)
}

// selfPlayGame plays one Negamax self-play game and returns all root positions
// (from Red's POV) plus the outcome (1=Red wins, 0.5=draw, 0=Black wins).
func selfPlayGame(w *gameai.PSTWeights, rng *rand.Rand, pvDepth int) ([]engine.Position, float64) {
	pos := engine.StartPosition()
	color := engine.Red
	positions := []engine.Position{pos}

	for i := 0; i < 300; i++ { // cap at 300 plies to avoid infinite games
		moves := engine.LegalMoves(pos, color)
		if len(moves) == 0 {
			// Current player loses
			if color == engine.Red {
				return positions, 0.0
			}
			return positions, 1.0
		}
		// Occasionally inject random moves for state-space coverage
		var chosen engine.Move
		if rng.Float64() < 0.05 {
			chosen = moves[rng.Intn(len(moves))]
		} else {
			// Simple greedy PST move selection for fast self-play
			chosen = greedyMove(pos, color, moves, w)
		}
		pos = engine.ApplyMove(pos, color, chosen)
		positions = append(positions, pos)
		color = color.Opponent()
	}
	return positions, 0.5 // game too long → draw
}

// greedyMove picks the move maximising PST eval for the current player.
func greedyMove(pos engine.Position, color engine.Color, moves []engine.Move, w *gameai.PSTWeights) engine.Move {
	best := moves[0]
	bestScore := -1e18
	for _, m := range moves {
		child := engine.ApplyMove(pos, color, m)
		s := gameai.PSTEvalColor(child, color, w)
		if s > bestScore {
			bestScore = s
			best = m
		}
	}
	return best
}

// tdLeafUpdate applies the TD-Leaf(λ) weight update for one game.
// The gradient at each step comes from the PV leaf position (true TD-Leaf),
// and scores are normalised through sigmoid before computing δ.
func tdLeafUpdate(
	w *gameai.PSTWeights,
	positions []engine.Position,
	outcome, alpha, lambda float64,
	pvDepth int,
) {
	if len(positions) < 2 {
		return
	}

	var e [128]float64     // eligibility trace
	color := engine.Red    // positions stored from game start; Red moves first
	prevSigV := gameai.Sigmoid(gameai.PSTEval(pv(positions[0], color, pvDepth, w), w))

	for t := 1; t < len(positions); t++ {
		// True TD-Leaf: find the PV leaf from the current root
		leaf := pv(positions[t], color, pvDepth, w)
		sigV := gameai.Sigmoid(gameai.PSTEval(leaf, w))

		// Gradient from the PV leaf (not the root)
		grad := gameai.FeatureVector(leaf)

		// Update eligibility trace: e = λ*e + grad
		for i := range e {
			e[i] = lambda*e[i] + grad[i]
		}

		// Temporal difference in sigmoid space
		delta := sigV - prevSigV
		gameai.ApplyGradient(w, e, alpha*delta)

		prevSigV = sigV
		color = color.Opponent() // alternate turns
	}

	// Terminal correction: δ_T = outcome − sigmoid(V(leaf_T))
	lastLeaf := pv(positions[len(positions)-1], color, pvDepth, w)
	sigVT := gameai.Sigmoid(gameai.PSTEval(lastLeaf, w))
	grad := gameai.FeatureVector(lastLeaf)

	// Update trace one final time
	for i := range e {
		e[i] = lambda*e[i] + grad[i]
	}
	deltaT := outcome - sigVT
	gameai.ApplyGradient(w, e, alpha*deltaT)
}

// pv returns the PV leaf position from a shallow greedy search (depth d).
// This makes the gradient extraction a true TD-Leaf gradient.
func pv(pos engine.Position, color engine.Color, depth int, w *gameai.PSTWeights) engine.Position {
	if depth == 0 {
		return pos
	}
	moves := engine.LegalMoves(pos, color)
	if len(moves) == 0 {
		return pos
	}
	best := moves[0]
	bestScore := -1e18
	for _, m := range moves {
		child := engine.ApplyMove(pos, color, m)
		s := gameai.PSTEvalColor(child, color, w)
		if s > bestScore {
			bestScore = s
			best = m
		}
	}
	return pv(engine.ApplyMove(pos, color, best), color.Opponent(), depth-1, w)
}
