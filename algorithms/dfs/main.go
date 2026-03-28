// DFS bot — standalone checkers player using DFS game-tree search.
//
// Usage:
//   go run ./algorithms/dfs --api http://localhost:8080 --game <game-id> --player black
//
// Identical SSE-based turn detection as the BFS bot; differs only in
// the search algorithm (LIFO stack, depth 5 vs BFS FIFO queue, depth 2).
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/limaflucas/heuristic_checkers/algorithms/gameai"
	"github.com/limaflucas/heuristic_checkers/internal/engine"
)

func main() {
	api := flag.String("api", "http://localhost:8080", "API base URL")
	gameID := flag.String("game", "", "Game ID (required)")
	playerStr := flag.String("player", "black", "Player color: red or black")
	flag.Parse()

	if *gameID == "" {
		log.Fatal("--game is required")
	}

	var color engine.Color
	switch strings.ToLower(*playerStr) {
	case "red":
		color = engine.Red
	case "black":
		color = engine.Black
	default:
		log.Fatalf("unknown player color: %s", *playerStr)
	}

	log.Printf("DFS bot starting — game=%s player=%s", *gameID, *playerStr)
	watchSSE(*api, *gameID, color)
}

func watchSSE(api, gameID string, color engine.Color) {
	url := fmt.Sprintf("%s/api/v1/games/%s/watch", api, gameID)
	for {
		if err := connect(url, api, gameID, color); err != nil {
			log.Printf("SSE error: %v — reconnecting in 2s", err)
		}
		time.Sleep(2 * time.Second)
	}
}

type boardResponse struct {
	Turn   string      `json:"turn"`
	Status string      `json:"status"`
	Pieces []pieceInfo `json:"pieces"`
}
type pieceInfo struct {
	SquareACF int    `json:"square_acf"`
	Color     string `json:"color"`
	King      bool   `json:"king"`
}

func connect(url, api, gameID string, color engine.Color) error {
	resp, err := http.Get(url) //nolint
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	var evtType, data string

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			evtType = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		case line == "" && evtType == "board" && data != "":
			handleBoard(api, gameID, color, data)
			evtType, data = "", ""
		case line == "":
			evtType, data = "", ""
		}
	}
	return scanner.Err()
}

func handleBoard(api, gameID string, color engine.Color, raw string) {
	var board boardResponse
	if err := json.Unmarshal([]byte(raw), &board); err != nil {
		return
	}
	if board.Status != "in_progress" {
		log.Printf("Game over: %s", board.Status)
		return
	}
	if board.Turn != color.String() {
		return // not our turn
	}

	pos := positionFromPieces(board.Pieces)

	time.Sleep(600 * time.Millisecond) // think time

	move := gameai.DFSChooseMove(pos, color, nil)
	if move.From == 0 && move.To == 0 {
		return
	}

	capsACF := make([]int, len(move.Captures))
	for i, sq := range move.Captures {
		capsACF[i] = engine.InternalToACF(sq)
	}

	body, _ := json.Marshal(map[string]any{
		"player":   color.String(),
		"from":     engine.InternalToACF(move.From),
		"to":       engine.InternalToACF(move.To),
		"captures": capsACF,
	})

	url := fmt.Sprintf("%s/api/v1/games/%s/moves", api, gameID)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body)) //nolint
	if err != nil {
		log.Printf("move POST failed: %v", err)
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	log.Printf("DFS moved: %d→%d", engine.InternalToACF(move.From), engine.InternalToACF(move.To))
}

func positionFromPieces(pieces []pieceInfo) engine.Position {
	var pos engine.Position
	for _, p := range pieces {
		sq := engine.ACFToInternal(p.SquareACF)
		if p.Color == "red" {
			pos.Red |= 1 << sq
		} else {
			pos.Black |= 1 << sq
		}
		if p.King {
			pos.Kings |= 1 << sq
		}
	}
	return pos
}
