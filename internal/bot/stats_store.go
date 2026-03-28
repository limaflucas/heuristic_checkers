package bot

import (
	"sync"

	"github.com/limaflucas/heuristic_checkers/algorithms/gameai"
	"github.com/limaflucas/heuristic_checkers/internal/engine"
)

// MoveStats holds the last SearchStats produced by each bot in a game.
type MoveStats struct {
	Red   *gameai.SearchStats `json:"red,omitempty"`
	Black *gameai.SearchStats `json:"black,omitempty"`
}

var (
	statsMu  sync.RWMutex
	statsMap = make(map[string]*MoveStats)
)

// SetMoveStats stores the latest stats for color in game gameID.
func SetMoveStats(gameID string, color engine.Color, s *gameai.SearchStats) {
	statsMu.Lock()
	defer statsMu.Unlock()
	ms := statsMap[gameID]
	if ms == nil {
		ms = &MoveStats{}
		statsMap[gameID] = ms
	}
	if color == engine.Red {
		ms.Red = s
	} else {
		ms.Black = s
	}
}

// GetMoveStats returns the latest stats for a game (nil if no bot has moved yet).
func GetMoveStats(gameID string) *MoveStats {
	statsMu.RLock()
	defer statsMu.RUnlock()
	return statsMap[gameID]
}

// ClearMoveStats removes stored stats for a finished or deleted game.
func ClearMoveStats(gameID string) {
	statsMu.Lock()
	defer statsMu.Unlock()
	delete(statsMap, gameID)
}
