package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"

	"github.com/limaflucas/heuristic_checkers/algorithms/gameai"
	"github.com/limaflucas/heuristic_checkers/internal/engine"
)

type BotFunc func(pos engine.Position, color engine.Color, stats *gameai.SearchStats) engine.Move

func main() {
	rand.Seed(42) // for reproducibility
	gameai.SetMCTSSeed(42)

	log.Println("Starting Search Efficiency Benchmark...")
	benchmarkSearchEfficiency()

	log.Println("Starting The Grand Tournament...")
	runGrandTournament()
	
	log.Println("All tasks completed.")
}

func benchmarkSearchEfficiency() {
	pos := engine.StartPosition()
	color := engine.Red

	f, err := os.Create("search_efficiency.csv")
	if err != nil {
		log.Fatalf("failed to create search_efficiency.csv: %v", err)
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()
	_ = writer.Write([]string{"Algorithm", "MaxDepth", "NodesExpanded", "ExecutionTimeMs"})

	// Negamax
	var statsNegamax gameai.SearchStats
	log.Println("Benchmarking Negamax...")
	gameai.NegamaxChooseMove(pos, color, &statsNegamax)
	_ = writer.Write([]string{
		"Negamax",
		strconv.Itoa(statsNegamax.MaxDepthReached),
		strconv.FormatInt(statsNegamax.NodesExpanded, 10),
		fmt.Sprintf("%.2f", statsNegamax.ExecutionTimeMs),
	})

	// PVS
	var statsPVS gameai.SearchStats
	log.Println("Benchmarking PVS...")
	gameai.PVSChooseMove(pos, color, &statsPVS)
	_ = writer.Write([]string{
		"PVS",
		strconv.Itoa(statsPVS.MaxDepthReached),
		strconv.FormatInt(statsPVS.NodesExpanded, 10),
		fmt.Sprintf("%.2f", statsPVS.ExecutionTimeMs),
	})
}

func runGrandTournament() {
	botNames := []string{"Negamax", "PVS", "MCTS", "Random"}
	bots := map[string]BotFunc{
		"Negamax": gameai.NegamaxChooseMove,
		"PVS":     gameai.PVSChooseMove,
		"MCTS":    gameai.MCTSChooseMove,
		"Random":  gameai.RandomChooseMove,
	}

	f, err := os.Create("tournament_results.csv")
	if err != nil {
		log.Fatalf("failed to create tournament_results.csv: %v", err)
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()
	_ = writer.Write([]string{"Player1", "Player2", "P1_Wins", "P2_Wins", "Draws"})

	gamesPerMatch := 10

	for i := 0; i < len(botNames); i++ {
		for j := 0; j < len(botNames); j++ {
			p1Name := botNames[i]
			p2Name := botNames[j]

			p1Wins := 0
			p2Wins := 0
			draws := 0

			log.Printf("Matchup: %s vs %s", p1Name, p2Name)

			for g := 0; g < gamesPerMatch; g++ {
				// Alternate colors
				var redBot, blackBot BotFunc
				var isP1Red bool

				if g%2 == 0 {
					redBot = bots[p1Name]
					blackBot = bots[p2Name]
					isP1Red = true
				} else {
					redBot = bots[p2Name]
					blackBot = bots[p1Name]
					isP1Red = false
				}

				pos := engine.StartPosition()
				turn := engine.Red
				ply := 0

				for {
					moves := engine.LegalMoves(pos, turn)
					if len(moves) == 0 {
						// Current player has no moves and loses
						if (turn == engine.Red && isP1Red) || (turn == engine.Black && !isP1Red) {
							p2Wins++
						} else {
							p1Wins++
						}
						break
					}

					if ply >= 200 {
						draws++
						break
					}

					var move engine.Move
					if turn == engine.Red {
						move = redBot(pos, turn, nil)
					} else {
						move = blackBot(pos, turn, nil)
					}

					pos = engine.ApplyMove(pos, turn, move)
					turn = turn.Opponent()
					ply++
				}
			}

			_ = writer.Write([]string{
				p1Name,
				p2Name,
				strconv.Itoa(p1Wins),
				strconv.Itoa(p2Wins),
				strconv.Itoa(draws),
			})
		}
	}
}
