package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/limaflucas/heuristic_checkers/internal/bot"
	"github.com/limaflucas/heuristic_checkers/internal/engine"
)

// Handlers holds all HTTP handler dependencies.
type Handlers struct {
	store *engine.GameStore
}

func NewHandlers(store *engine.GameStore) *Handlers {
	return &Handlers{store: store}
}

// ---- GET /api/v1/games ----

func (h *Handlers) ListGames(w http.ResponseWriter, r *http.Request) {
	summaries := h.store.List()
	writeJSON(w, http.StatusOK, GamesListResponse{
		Total: len(summaries),
		Games: summaries,
	})
}

// ---- POST /api/v1/games ----

func (h *Handlers) CreateGame(w http.ResponseWriter, r *http.Request) {
	var req NewGameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RedPlayer == "" || req.BlackPlayer == "" {
		writeError(w, http.StatusBadRequest, "body must include red_player and black_player")
		return
	}
	g := h.store.Create(req.RedPlayer, req.BlackPlayer)

	// Spawn in-process bots if requested.
	if algo := bot.ByName(req.RedBot); algo != nil {
		go bot.Run(g, engine.Red, algo, 400*time.Millisecond)
	}
	if algo := bot.ByName(req.BlackBot); algo != nil {
		go bot.Run(g, engine.Black, algo, 600*time.Millisecond)
	}

	snap := g.Snapshot()
	resp := NewGameResponse{
		GameID:      snap.ID,
		RedPlayer:   snap.RedPlayer,
		BlackPlayer: snap.BlackPlayer,
		Turn:        snap.Turn.String(),
		Board:       snapshotToBoard(snap),
	}
	writeJSON(w, http.StatusCreated, resp)
}

// ---- POST /api/v1/games/{id}/moves ----

func (h *Handlers) MakeMove(w http.ResponseWriter, r *http.Request) {
	g, ok := h.gameFromPath(w, r)
	if !ok {
		return
	}

	var req MoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Resolve color.
	var c engine.Color
	switch strings.ToLower(req.Player) {
	case "red":
		c = engine.Red
	case "black":
		c = engine.Black
	default:
		writeError(w, http.StatusBadRequest, "player must be 'red' or 'black'")
		return
	}

	// Validate ACF squares.
	if req.From < 1 || req.From > 32 || req.To < 1 || req.To > 32 {
		writeError(w, http.StatusBadRequest, "from and to must be ACF squares 1-32")
		return
	}

	// Build captures list (internal indices).
	caps := make([]int, len(req.Captures))
	for i, acf := range req.Captures {
		if acf < 1 || acf > 32 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("capture square %d out of range", acf))
			return
		}
		caps[i] = engine.ACFToInternal(acf)
	}

	m := engine.Move{
		From:     engine.ACFToInternal(req.From),
		To:       engine.ACFToInternal(req.To),
		Captures: caps,
	}

	if err := g.MakeMove(c, m); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	snap := g.Snapshot()
	resp := MoveResponse{OK: true, Board: snapshotToBoard(snap)}
	writeJSON(w, http.StatusOK, resp)
}

// ---- GET /api/v1/games/{id}/board ----

func (h *Handlers) GetBoard(w http.ResponseWriter, r *http.Request) {
	g, ok := h.gameFromPath(w, r)
	if !ok {
		return
	}
	snap := g.Snapshot()
	writeJSON(w, http.StatusOK, snapshotToBoard(snap))
}

// ---- GET /api/v1/games/{id}/legal-moves ----

func (h *Handlers) GetLegalMoves(w http.ResponseWriter, r *http.Request) {
	g, ok := h.gameFromPath(w, r)
	if !ok {
		return
	}
	snap := g.Snapshot()
	perPiece := engine.LegalMovesPerPiece(snap.Position, snap.Turn)

	var groups []PieceMoveGroup
	total := 0
	for fromSq, moves := range perPiece {
		entries := make([]LegalMoveEntry, len(moves))
		for i, m := range moves {
			capsACF := make([]int, len(m.Captures))
			for j, c := range m.Captures {
				capsACF[j] = engine.InternalToACF(c)
			}
			entries[i] = LegalMoveEntry{
				FromACF:     engine.InternalToACF(m.From),
				ToACF:       engine.InternalToACF(m.To),
				CapturesACF: capsACF,
				IsKingMove:  m.IsKingMove,
				Promoted:    m.Promoted,
			}
		}
		groups = append(groups, PieceMoveGroup{
			FromACF: engine.InternalToACF(fromSq),
			IsKing:  snap.Position.IsKing(fromSq),
			Moves:   entries,
		})
		total += len(entries)
	}

	resp := LegalMovesResponse{
		GameID:  snap.ID,
		Turn:    snap.Turn.String(),
		ByPiece: groups,
		Total:   total,
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---- GET /api/v1/games/{id}/events ----

func (h *Handlers) GetEvents(w http.ResponseWriter, r *http.Request) {
	g, ok := h.gameFromPath(w, r)
	if !ok {
		return
	}
	snap := g.Snapshot()
	writeJSON(w, http.StatusOK, EventsResponse{
		GameID: snap.ID,
		Events: snap.Events,
		Total:  len(snap.Events),
	})
}

// ---- GET /api/v1/games/{id}/moves ----

func (h *Handlers) GetMoves(w http.ResponseWriter, r *http.Request) {
	g, ok := h.gameFromPath(w, r)
	if !ok {
		return
	}
	snap := g.Snapshot()
	writeJSON(w, http.StatusOK, MovesResponse{
		GameID: snap.ID,
		Total:  len(snap.Moves),
		Moves:  snap.Moves,
	})
}

// ---- GET /api/v1/games/{id}/watch  (Server-Sent Events) ----

func (h *Handlers) WatchGame(w http.ResponseWriter, r *http.Request) {
	g, ok := h.gameFromPath(w, r)
	if !ok {
		return
	}

	// Upgrade the connection to server-sent events.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Subscribe to new board states.
	ch := g.Subscribe()
	defer g.Unsubscribe(ch)

	// Send the current state immediately so the client has something to render.
	sendSSEBoard(w, flusher, g.Snapshot())

	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()

	for {
		select {
		case snap, open := <-ch:
			if !open {
				return
			}
			sendSSEBoard(w, flusher, snap)
		case <-ping.C:
			// SSE comment keeps the TCP connection alive through proxies.
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func sendSSEBoard(w http.ResponseWriter, f http.Flusher, snap engine.Snapshot) {
	// Full board event
	board := snapshotToBoard(snap)
	b, err := json.Marshal(board)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: board\ndata: %s\n\n", b)

	// Lightweight turn event — lets bots react without parsing full board JSON.
	type turnPayload struct {
		GameID     string `json:"game_id"`
		Turn       string `json:"turn"`
		PlayerName string `json:"player_name"`
		Status     string `json:"status"`
	}
	tp := turnPayload{
		GameID:     snap.ID,
		Turn:       snap.Turn.String(),
		PlayerName: snap.TurnName,
		Status:     string(snap.Status),
	}
	tb, _ := json.Marshal(tp)
	fmt.Fprintf(w, "event: turn\ndata: %s\n\n", tb)

	f.Flush()
}

// ---- helpers ----

// gameFromPath extracts the game ID from the URL path and retrieves the game.
// Path pattern: /api/v1/games/{id}/...
func (h *Handlers) gameFromPath(w http.ResponseWriter, r *http.Request) (*engine.Game, bool) {
	// Path: /api/v1/games/<id>/...
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// parts: ["api","v1","games","<id>","<action>"]
	if len(parts) < 4 {
		writeError(w, http.StatusBadRequest, "missing game id")
		return nil, false
	}
	id := parts[3]
	g, ok := h.store.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "game not found")
		return nil, false
	}
	return g, true
}

func snapshotToBoard(snap engine.Snapshot) *BoardResponse {
	matrix := snap.Position.ToMatrix()
	var pieces []PieceInfo
	for sq := 0; sq < 32; sq++ {
		c := snap.Position.ColorAt(sq)
		if c == engine.None {
			continue
		}
		row, col := squareToRowColPublic(sq)
		pieces = append(pieces, PieceInfo{
			SquareACF: engine.InternalToACF(sq),
			Row:       row,
			Col:       col,
			Color:     c.String(),
			King:      snap.Position.IsKing(sq),
		})
	}
	b := &BoardResponse{
		GameID:      snap.ID,
		RedPlayer:   snap.RedPlayer,
		BlackPlayer: snap.BlackPlayer,
		Turn:        snap.Turn.String(),
		Status:      string(snap.Status),
		Matrix:      matrix,
		Pieces:      pieces,
		BlackMen:    snap.BlackMen,
		RedMen:      snap.RedMen,
		BlackKings:  snap.BlackKings,
		RedKings:    snap.RedKings,
		ElapsedSec:  snap.ElapsedSec,
		StartTime:   snap.StartTime.Format("2006-01-02T15:04:05Z07:00"),
	}
	return b
}

func squareToRowColPublic(sq int) (int, int) {
	bottomRow := sq / 4
	p := sq % 4
	row := 7 - bottomRow
	var col int
	if bottomRow%2 == 0 {
		col = 2 * p
	} else {
		col = 2*p + 1
	}
	return row, col
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}
