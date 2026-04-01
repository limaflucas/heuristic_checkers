package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/limaflucas/heuristic_checkers/algorithms/gameai"
	"github.com/limaflucas/heuristic_checkers/internal/engine"
)

func main() {
	iterations := flag.Int("iterations", 100, "number of SA iterations")
	initialTemp := flag.Float64("temp", 100.0, "initial temperature")
	coolingRate := flag.Float64("cool", 0.95, "cooling rate")
	seed := flag.Int64("seed", 42, "random seed (0 for random)")
	out := flag.String("out", "weights/pst_weights_sa.json", "output weights file")
	csvOut := flag.String("csv", "training_sa_log.csv", "CSV log path")
	flag.Parse()

	if *seed == 0 {
		*seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(*seed))

	log.Printf("SA Trainer: iterations=%d temp=%.2f cool=%.2f seed=%d", *iterations, *initialTemp, *coolingRate, *seed)

	currentWeights := gameai.LoadPSTWeights("weights/pst_weights.json")
	currentEnergy := calculateEnergy(currentWeights, rng)
	bestWeights := *currentWeights
	bestEnergy := currentEnergy

	csvFile, csvWriter := initCSVLog(*csvOut)
	defer csvFile.Close()

	temp := *initialTemp
	for i := 1; i <= *iterations; i++ {
		candidate := perturb(currentWeights, rng)
		candidateEnergy := calculateEnergy(candidate, rng)

		deltaE := candidateEnergy - currentEnergy
		accept := false
		if deltaE < 0 {
			accept = true
		} else {
			prob := math.Exp(-deltaE / temp)
			if rng.Float64() < prob {
				accept = true
			}
		}

		if accept {
			currentWeights = candidate
			currentEnergy = candidateEnergy
			if currentEnergy < bestEnergy {
				bestWeights = *currentWeights
				bestEnergy = currentEnergy
				log.Printf("Iteration %d: New BEST energy %.4f", i, bestEnergy)
				_ = gameai.SavePSTWeights(*out, &bestWeights)
			}
		}

		appendCSVRow(csvWriter, i, temp, currentEnergy, bestEnergy)
		temp *= *coolingRate

		if i%10 == 0 {
			log.Printf("Iteration %d/%d — Temp: %.2f, Energy: %.4f, Best: %.4f", i, *iterations, temp, currentEnergy, bestEnergy)
		}
	}

	_ = gameai.SavePSTWeights(*out, &bestWeights)
	fmt.Printf("Training complete. Best weights saved to %s\n", *out)
}

func calculateEnergy(w *gameai.PSTWeights, rng *rand.Rand) float64 {
	wins := 0.0
	games := 20
	baselineDepth := 2
	
	for g := 0; g < games; g++ {
		// Alternate colors
		botColor := engine.Red
		if g%2 == 1 {
			botColor = engine.Black
		}

		pos := engine.StartPosition()
		turn := engine.Red
		
		for ply := 0; ply < 200; ply++ {
			moves := engine.LegalMoves(pos, turn)
			if len(moves) == 0 {
				if turn != botColor {
					wins += 1.0
				}
				break
			}
			
			var move engine.Move
			if turn == botColor {
				move = gameai.TrainingChooseMove(pos, turn, 2, w)
			} else {
				move = gameai.TrainingChooseMove(pos, turn, baselineDepth, gameai.DefaultPSTWeights())
			}
			pos = engine.ApplyMove(pos, turn, move)
			turn = turn.Opponent()
			
			if ply == 199 {
				wins += 0.5 // Draw
			}
		}
	}
	
	winRate := wins / float64(games)
	return 1.0 - winRate
}

func perturb(w *gameai.PSTWeights, rng *rand.Rand) *gameai.PSTWeights {
	newW := *w
	// Perturb 1–5 weights
	num := rng.Intn(5) + 1
	for i := 0; i < num; i++ {
		tableIdx := rng.Intn(4)
		sqIdx := rng.Intn(32)
		jitter := (rng.NormFloat64() * 2.0)
		
		switch tableIdx {
		case 0:
			newW.MenOpening[sqIdx] += jitter
		case 1:
			newW.MenEndgame[sqIdx] += jitter
		case 2:
			newW.KingsOpening[sqIdx] += jitter
		case 3:
			newW.KingsEndgame[sqIdx] += jitter
		}
	}
	return &newW
}

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
		_ = w.Write([]string{"iteration", "temp", "energy", "best_energy"})
		w.Flush()
	}
	return f, w
}

func appendCSVRow(w *csv.Writer, iter int, temp, energy, best float64) {
	_ = w.Write([]string{
		strconv.Itoa(iter),
		fmt.Sprintf("%.4f", temp),
		fmt.Sprintf("%.4f", energy),
		fmt.Sprintf("%.4f", best),
	})
	w.Flush()
}
