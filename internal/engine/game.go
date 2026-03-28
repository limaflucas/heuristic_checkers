package engine

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// GameStatus represents the current state of the game.
type GameStatus string

const (
	StatusInProgress GameStatus = "in_progress"
	StatusRedWins    GameStatus = "red_wins"
	StatusBlackWins  GameStatus = "black_wins"
	StatusDraw       GameStatus = "draw"
)

// EventType categorises game events.
type EventType string

const (
	EventMove        EventType = "MOVE"
	EventCapture     EventType = "CAPTURE"
	EventKingCreated EventType = "KING_CREATED"
	EventGameOver    EventType = "GAME_OVER"
	EventDraw        EventType = "DRAW"
)

// Event is a discrete occurrence recorded during the game.
type Event struct {
	Type      EventType `json:"type"`
	MoveNum   int       `json:"move_number"`
	Player    string    `json:"player"`
	Square    int       `json:"square,omitempty"`   // ACF (1-32), for CAPTURE and KING_CREATED
	FromACF   int       `json:"from_acf,omitempty"` // for MOVE
	ToACF     int       `json:"to_acf,omitempty"`   // for MOVE
	Timestamp time.Time `json:"timestamp"`
}

// GameMove records everything about a single player turn.
type GameMove struct {
	MoveNumber  int       `json:"move_number"`
	Player      string    `json:"player"`
	FromACF     int       `json:"from_acf"`
	ToACF       int       `json:"to_acf"`
	CapturesACF []int     `json:"captures_acf,omitempty"`
	IsKingMove  bool      `json:"is_king_move"`
	Promoted    bool      `json:"promoted"`
	Timestamp   time.Time `json:"timestamp"`
	DurationMs  int64     `json:"duration_ms"`
}

// Game holds the complete state of one game session.
type Game struct {
	ID          string     `json:"id"`
	RedPlayer   string     `json:"red_player"`
	BlackPlayer string     `json:"black_player"`
	Position    Position   `json:"-"`
	Turn        Color      `json:"turn_color"`
	Status      GameStatus `json:"status"`
	Moves       []GameMove `json:"moves"`
	Events      []Event    `json:"events"`
	StartTime   time.Time  `json:"start_time"`
	EndTime     *time.Time `json:"end_time,omitempty"`
	LastMoveAt  time.Time  `json:"-"`

	halfMoveClock int

	mu sync.RWMutex

	// Real-time broadcast (SSE/WebSocket subscribers).
	subsMu sync.Mutex
	subs   map[chan Snapshot]struct{}
}

// NewGame initialises a game with standard starting position.
func NewGame(redPlayer, blackPlayer string) *Game {
	now := time.Now()
	return &Game{
		ID:          newID(),
		RedPlayer:   redPlayer,
		BlackPlayer: blackPlayer,
		Position:    StartPosition(),
		Turn:        Red,
		Status:      StatusInProgress,
		StartTime:   now,
		LastMoveAt:  now,
		subs:        make(map[chan Snapshot]struct{}),
	}
}

// Subscribe registers a new real-time listener and returns its channel.
// The caller must call Unsubscribe when done.
func (g *Game) Subscribe() chan Snapshot {
	ch := make(chan Snapshot, 8) // buffered to avoid blocking MakeMove
	g.subsMu.Lock()
	g.subs[ch] = struct{}{}
	g.subsMu.Unlock()
	return ch
}

// Unsubscribe removes a listener and closes its channel.
func (g *Game) Unsubscribe(ch chan Snapshot) {
	g.subsMu.Lock()
	delete(g.subs, ch)
	g.subsMu.Unlock()
	// drain any buffered items so the channel can be GC'd
	for len(ch) > 0 {
		<-ch
	}
}

// broadcast delivers a snapshot to all current subscribers.
// Non-blocking: slow consumers drop the frame and receive the next one.
func (g *Game) broadcast(snap Snapshot) {
	g.subsMu.Lock()
	defer g.subsMu.Unlock()
	for ch := range g.subs {
		select {
		case ch <- snap:
		default:
		}
	}
}

// ElapsedSeconds returns the total game duration in seconds.
func (g *Game) ElapsedSeconds() float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	end := time.Now()
	if g.EndTime != nil {
		end = *g.EndTime
	}
	return end.Sub(g.StartTime).Seconds()
}

func (g *Game) playerName(c Color) string {
	if c == Red {
		return g.RedPlayer
	}
	return g.BlackPlayer
}

// MakeMove validates and applies a move. Returns an error if invalid.
func (g *Game) MakeMove(c Color, m Move) error {
	g.mu.Lock()

	if g.Status != StatusInProgress {
		g.mu.Unlock()
		return errors.New("game is not in progress")
	}
	if c != g.Turn {
		g.mu.Unlock()
		return errors.New("it is not your turn")
	}

	m.IsKingMove = g.Position.IsKing(m.From)
	m.Promoted = !m.IsKingMove && IsKingRow(m.To, c)

	if !IsLegal(g.Position, c, m) {
		g.mu.Unlock()
		return errors.New("illegal move")
	}

	now := time.Now()
	durationMs := now.Sub(g.LastMoveAt).Milliseconds()
	moveNum := len(g.Moves) + 1
	player := g.playerName(c)

	captACF := make([]int, len(m.Captures))
	for i, sq := range m.Captures {
		captACF[i] = InternalToACF(sq)
	}

	g.Moves = append(g.Moves, GameMove{
		MoveNumber:  moveNum,
		Player:      player,
		FromACF:     InternalToACF(m.From),
		ToACF:       InternalToACF(m.To),
		CapturesACF: captACF,
		IsKingMove:  m.IsKingMove,
		Promoted:    m.Promoted,
		Timestamp:   now,
		DurationMs:  durationMs,
	})

	g.Events = append(g.Events, Event{
		Type:      EventMove,
		MoveNum:   moveNum,
		Player:    player,
		FromACF:   InternalToACF(m.From),
		ToACF:     InternalToACF(m.To),
		Timestamp: now,
	})

	for _, sq := range m.Captures {
		g.Events = append(g.Events, Event{
			Type:      EventCapture,
			MoveNum:   moveNum,
			Player:    player,
			Square:    InternalToACF(sq),
			Timestamp: now,
		})
	}

	g.Position = ApplyMove(g.Position, c, m)

	if m.Promoted {
		g.Events = append(g.Events, Event{
			Type:      EventKingCreated,
			MoveNum:   moveNum,
			Player:    player,
			Square:    InternalToACF(m.To),
			Timestamp: now,
		})
	}

	wasManAdvance := !m.IsKingMove
	if len(m.Captures) > 0 || wasManAdvance {
		g.halfMoveClock = 0
	} else {
		g.halfMoveClock++
	}

	g.LastMoveAt = now
	g.Turn = g.Turn.Opponent()

	g.checkTermination(now)

	g.mu.Unlock()

	// Broadcast updated state to all SSE/WebSocket subscribers.
	snap := g.Snapshot()
	g.broadcast(snap)

	return nil
}

func (g *Game) checkTermination(now time.Time) {
	opp := g.Turn

	if !HasLegalMoves(g.Position, opp) {
		winner := opp.Opponent()
		if winner == Red {
			g.Status = StatusRedWins
		} else {
			g.Status = StatusBlackWins
		}
		t := now
		g.EndTime = &t
		g.Events = append(g.Events, Event{
			Type:      EventGameOver,
			Player:    g.playerName(winner),
			Timestamp: now,
		})
		return
	}

	if g.halfMoveClock >= 80 {
		g.Status = StatusDraw
		t := now
		g.EndTime = &t
		g.Events = append(g.Events, Event{
			Type:      EventDraw,
			Timestamp: now,
		})
	}
}

// Snapshot returns a thread-safe copy for API responses.
type Snapshot struct {
	ID          string
	RedPlayer   string
	BlackPlayer string
	Position    Position
	Turn        Color
	TurnName    string
	Status      GameStatus
	StartTime   time.Time
	EndTime     *time.Time
	ElapsedSec  float64
	Moves       []GameMove
	Events      []Event
	BlackMen    int
	RedMen      int
	BlackKings  int
	RedKings    int
}

func (g *Game) Snapshot() Snapshot {
	g.mu.RLock()
	defer g.mu.RUnlock()
	end := time.Now()
	if g.EndTime != nil {
		end = *g.EndTime
	}
	bm, rm, bk, rk := g.Position.RemainingCounts()
	moves := make([]GameMove, len(g.Moves))
	copy(moves, g.Moves)
	events := make([]Event, len(g.Events))
	copy(events, g.Events)
	return Snapshot{
		ID:          g.ID,
		RedPlayer:   g.RedPlayer,
		BlackPlayer: g.BlackPlayer,
		Position:    g.Position,
		Turn:        g.Turn,
		TurnName:    g.playerName(g.Turn),
		Status:      g.Status,
		StartTime:   g.StartTime,
		EndTime:     g.EndTime,
		ElapsedSec:  end.Sub(g.StartTime).Seconds(),
		Moves:       moves,
		Events:      events,
		BlackMen:    bm,
		RedMen:      rm,
		BlackKings:  bk,
		RedKings:    rk,
	}
}

// GameSummary is a lightweight summary used in the games list endpoint.
type GameSummary struct {
	ID          string     `json:"id"`
	RedPlayer   string     `json:"red_player"`
	BlackPlayer string     `json:"black_player"`
	Status      GameStatus `json:"status"`
	Turn        string     `json:"turn"`
	MoveCount   int        `json:"move_count"`
	ElapsedSec  float64    `json:"elapsed_sec"`
	StartTime   time.Time  `json:"start_time"`
	RedMen      int        `json:"red_men"`
	BlackMen    int        `json:"black_men"`
	RedKings    int        `json:"red_kings"`
	BlackKings  int        `json:"black_kings"`
}

// Summary returns a lightweight snapshot for the games list.
func (g *Game) Summary() GameSummary {
	g.mu.RLock()
	defer g.mu.RUnlock()
	end := time.Now()
	if g.EndTime != nil {
		end = *g.EndTime
	}
	bm, rm, bk, rk := g.Position.RemainingCounts()
	return GameSummary{
		ID:          g.ID,
		RedPlayer:   g.RedPlayer,
		BlackPlayer: g.BlackPlayer,
		Status:      g.Status,
		Turn:        g.Turn.String(),
		MoveCount:   len(g.Moves),
		ElapsedSec:  end.Sub(g.StartTime).Seconds(),
		StartTime:   g.StartTime,
		RedMen:      rm,
		BlackMen:    bm,
		RedKings:    rk,
		BlackKings:  bk,
	}
}

// ---- GameStore ----

// GameStore is a thread-safe in-memory registry of all active games.
type GameStore struct {
	mu    sync.RWMutex
	games map[string]*Game
}

func NewGameStore() *GameStore {
	return &GameStore{games: make(map[string]*Game)}
}

func (s *GameStore) Create(redPlayer, blackPlayer string) *Game {
	g := NewGame(redPlayer, blackPlayer)
	s.mu.Lock()
	s.games[g.ID] = g
	s.mu.Unlock()
	return g
}

func (s *GameStore) Get(id string) (*Game, bool) {
	s.mu.RLock()
	g, ok := s.games[id]
	s.mu.RUnlock()
	return g, ok
}

// Delete removes a game by ID (used by the manager to clear completed memory).
func (s *GameStore) Delete(id string) {
	s.mu.Lock()
	delete(s.games, id)
	s.mu.Unlock()
}

// List returns a summary of every game, ordered by start time (newest first).
func (s *GameStore) List() []GameSummary {
	s.mu.RLock()
	games := make([]*Game, 0, len(s.games))
	for _, g := range s.games {
		games = append(games, g)
	}
	s.mu.RUnlock()

	result := make([]GameSummary, len(games))
	for i, g := range games {
		result[i] = g.Summary()
	}
	// Sort newest first.
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].StartTime.After(result[i].StartTime) {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

// ---- helpers ----

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
