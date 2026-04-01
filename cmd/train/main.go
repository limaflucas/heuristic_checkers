// cmd/train/main.go — TD-Leaf(λ) offline trainer for the Checkers PST weights.
//
// Self-plays Negamax-vs-Negamax games and applies the TD-Leaf update rule:
//
//	at each step, the gradient comes from the PV leaf (not the root position),
//	and scores are mapped through a sigmoid before computing temporal differences.
//
// Usage:
//
//	go run ./cmd/train [flags]
//	go run ./cmd/train --games 500 --alpha 0.001 --lambda 0.7 --depth 2 --out weights/pst_weights.json
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/limaflucas/heuristic_checkers/algorithms/gameai"
	"github.com/limaflucas/heuristic_checkers/internal/engine"
)

func main() {
	games := flag.Int("games", 1000, "number of self-play games")
	alpha := flag.Float64("alpha", 0.001, "learning rate")
	lambda := flag.Float64("lambda", 0.7, "TD(λ) trace decay")
	depth := flag.Int("depth", 2, "PV search depth for leaf extraction")
	out := flag.String("out", "weights/pst_weights.json", "output weights file")
	csvOut := flag.String("csv", "training_log.csv", "CSV log path (game, wins, draws, losses)")
	seed := flag.Int64("seed", 42, "random seed for reproducibility (0 for random)")
	flag.Parse()

	if *seed == 0 {
		*seed = time.Now().UnixNano()
	}
	log.Printf("TD-Leaf trainer: games=%d α=%.4f λ=%.4f PV-depth=%d seed=%d", *games, *alpha, *lambda, *depth, *seed)

	w := gameai.LoadPSTWeights(*out)
	rng := rand.New(rand.NewSource(*seed))

	// Initialise CSV log (append mode — creates file with header if new).
	csvFile, csvWriter := initCSVLog(*csvOut)

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

		if g%50 == 0 {
			appendCSVRow(csvWriter, g, totalWins, totalDraws, totalLosses)
		}
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
	csvFile.Close()
	fmt.Printf("Training complete. Weights saved to %s\n", *out)
	fmt.Printf("Training log saved to %s\n", *csvOut)
	fmt.Printf("Final record: W=%d D=%d L=%d\n", totalWins, totalDraws, totalLosses)
}

// ── CSV logging helpers ───────────────────────────────────────────────────────

// initCSVLog opens (or creates) the CSV file, writes a header if the file is
// new, and returns the file handle and a *csv.Writer for use by appendCSVRow.
func initCSVLog(path string) (*os.File, *csv.Writer) {
	newFile := false
	if _, err := os.Stat(path); os.IsNotExist(err) {
		newFile = true
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("cannot open CSV log %s: %v", path, err)
	}
	w := csv.NewWriter(f)
	if newFile {
		_ = w.Write([]string{"game", "wins", "draws", "losses"})
		w.Flush()
	}
	return f, w
}

// appendCSVRow appends one row to the training CSV (flushed immediately).
func appendCSVRow(w *csv.Writer, game, wins, draws, losses int) {
	_ = w.Write([]string{
		strconv.Itoa(game),
		strconv.Itoa(wins),
		strconv.Itoa(draws),
		strconv.Itoa(losses),
	})
	w.Flush()
}

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
			// REPLACED: Use bounded Alpha-Beta search instead of 1-ply greedyMove
			chosen = trainingSearchMove(pos, color, pvDepth, w)
		}
		pos = engine.ApplyMove(pos, color, chosen)
		positions = append(positions, pos)
		color = color.Opponent()
	}
	return positions, 0.5 // game too long → draw
}

// trainingSearchMove uses a fast, fixed-depth Alpha-Beta search to generate
// much higher quality training games than a simple 1-ply greedy move.
func trainingSearchMove(pos engine.Position, color engine.Color, depth int, w *gameai.PSTWeights) engine.Move {
	moves := engine.LegalMoves(pos, color)
	if len(moves) == 0 {
		return engine.Move{}
	}
	if len(moves) == 1 {
		return moves[0] // Auto-play forced moves
	}

	bestMove := moves[0]
	bestScore := -1 << 29
	alpha := -1 << 29
	beta := 1 << 29

	for _, m := range moves {
		child := engine.ApplyMove(pos, color, m)
		score := -alphaBeta(child, color.Opponent(), depth-1, -beta, -alpha, w)

		if score > bestScore {
			bestScore = score
			bestMove = m
		}
		if score > alpha {
			alpha = score
		}
	}
	return bestMove
}

// alphaBeta is a lightweight minimax helper for generating training games.
func alphaBeta(pos engine.Position, color engine.Color, depth, alpha, beta int, w *gameai.PSTWeights) int {
	if depth == 0 {
		return int(gameai.PSTEvalColor(pos, color, w))
	}

	moves := engine.LegalMoves(pos, color)
	if len(moves) == 0 {
		return -32000 // Mate score (loss for current player)
	}

	bestScore := -1 << 29
	for _, m := range moves {
		child := engine.ApplyMove(pos, color, m)
		score := -alphaBeta(child, color.Opponent(), depth-1, -beta, -alpha, w)

		if score > bestScore {
			bestScore = score
		}
		if score > alpha {
			alpha = score
		}
		if alpha >= beta {
			break // Alpha-Beta Cutoff
		}
	}
	return bestScore
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

	var e [128]float64  // eligibility trace
	color := engine.Red // positions stored from game start; Red moves first
	prevSigV := gameai.Sigmoid(gameai.PSTEval(gameai.PVLeaf(positions[0], color, pvDepth, w), w))

	for t := 1; t < len(positions); t++ {
		// True TD-Leaf: find the PV leaf from the current root
		leaf := gameai.PVLeaf(positions[t], color, pvDepth, w)
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
	lastLeaf := gameai.PVLeaf(positions[len(positions)-1], color, pvDepth, w)
	sigVT := gameai.Sigmoid(gameai.PSTEval(lastLeaf, w))
	grad := gameai.FeatureVector(lastLeaf)

	// Update trace one final time
	for i := range e {
		e[i] = lambda*e[i] + grad[i]
	}
	deltaT := outcome - sigVT
	gameai.ApplyGradient(w, e, alpha*deltaT)
}
