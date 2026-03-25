package api

import "github.com/limaflucas/heuristic_checkers/internal/engine"

// ---- Requests ----

// NewGameRequest creates a new game. RedBot/BlackBot are optional bot names ("bfs" or "dfs").
type NewGameRequest struct {
	RedPlayer   string `json:"red_player"`
	BlackPlayer string `json:"black_player"`
	RedBot      string `json:"red_bot,omitempty"`
	BlackBot    string `json:"black_bot,omitempty"`
}

// MoveRequest uses ACF square numbers (1-32).
type MoveRequest struct {
	Player   string `json:"player"`    // "red" or "black"
	From     int    `json:"from"`      // ACF 1-32
	To       int    `json:"to"`        // ACF 1-32
	Captures []int  `json:"captures"`  // ACF squares captured (for multi-jump validation aid; optional)
}

// ---- Responses ----

type PieceInfo struct {
	SquareACF int    `json:"square_acf"`
	Row       int    `json:"row"`
	Col       int    `json:"col"`
	Color     string `json:"color"`
	King      bool   `json:"king"`
}

type BoardResponse struct {
	GameID      string      `json:"game_id"`
	RedPlayer   string      `json:"red_player"`
	BlackPlayer string      `json:"black_player"`
	Turn        string      `json:"turn"`        // "red" or "black"
	Status      string      `json:"status"`
	Matrix      [8][8]int8  `json:"matrix"`      // 8×8: 0=empty,1=red man,2=red king,-1=black man,-2=black king
	Pieces      []PieceInfo `json:"pieces"`
	BlackMen    int         `json:"black_men"`
	RedMen      int         `json:"red_men"`
	BlackKings  int         `json:"black_kings"`
	RedKings    int         `json:"red_kings"`
	ElapsedSec  float64     `json:"elapsed_seconds"`
	StartTime   string      `json:"start_time"`
}

type LegalMoveEntry struct {
	FromACF     int   `json:"from_acf"`
	ToACF       int   `json:"to_acf"`
	CapturesACF []int `json:"captures_acf,omitempty"`
	IsKingMove  bool  `json:"is_king_move"`
	Promoted    bool  `json:"promoted"`
}

type LegalMovesResponse struct {
	GameID  string           `json:"game_id"`
	Turn    string           `json:"turn"`
	ByPiece []PieceMoveGroup `json:"by_piece"`
	Total   int              `json:"total_moves"`
}

type PieceMoveGroup struct {
	FromACF int              `json:"from_acf"`
	IsKing  bool             `json:"is_king"`
	Moves   []LegalMoveEntry `json:"moves"`
}

type EventsResponse struct {
	GameID string         `json:"game_id"`
	Events []engine.Event `json:"events"`
	Total  int            `json:"total"`
}

type MovesResponse struct {
	GameID string            `json:"game_id"`
	Total  int               `json:"total"`
	Moves  []engine.GameMove `json:"moves"`
}

type MoveResponse struct {
	OK      bool          `json:"ok"`
	Message string        `json:"message,omitempty"`
	Board   *BoardResponse `json:"board,omitempty"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type NewGameResponse struct {
	GameID      string         `json:"game_id"`
	RedPlayer   string         `json:"red_player"`
	BlackPlayer string         `json:"black_player"`
	Turn        string         `json:"turn"`
	Board       *BoardResponse `json:"board"`
}

// GamesListResponse wraps the list of all game summaries.
type GamesListResponse struct {
	Total int                   `json:"total"`
	Games []engine.GameSummary  `json:"games"`
}
