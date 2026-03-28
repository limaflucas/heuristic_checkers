package manager

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/limaflucas/heuristic_checkers/internal/bot"
	"github.com/limaflucas/heuristic_checkers/internal/engine"
)

type Config struct {
	RedPlayer       string `json:"red_player"`
	BlackPlayer     string `json:"black_player"`
	RedBot          string `json:"red_bot,omitempty"`
	BlackBot        string `json:"black_bot,omitempty"`
	Epochs          int    `json:"epochs"`
	MatchesPerEpoch int    `json:"matches_per_epoch"`
	HumanSpeed      bool   `json:"human_speed"`
}

type Status struct {
	ID              string    `json:"id"`
	Config          Config    `json:"config"`
	CurrentEpoch    int       `json:"current_epoch"`
	CurrentMatch    int       `json:"current_match"`
	CurrentGameID   string    `json:"current_game_id"`
	IsFinished      bool      `json:"is_finished"`
	BaseDir         string    `json:"-"`
	BaseDirPub      string    `json:"base_dir"` // exported for UI links
	RedWins         int       `json:"red_wins"`
	BlackWins       int       `json:"black_wins"`
	Draws           int       `json:"draws"`
	StartTime       time.Time `json:"start_time"`
	EndTime         time.Time `json:"end_time,omitempty"`
}

type Manager struct {
	store *engine.GameStore
	
	mu       sync.RWMutex
	sessions map[string]*Status
}

func NewManager(store *engine.GameStore) *Manager {
	return &Manager{
		store:    store,
		sessions: make(map[string]*Status),
	}
}

// List active or finished sessions.
func (m *Manager) List() []*Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*Status, 0, len(m.sessions))
	for _, s := range m.sessions {
		// make a shallow copy for safety
		copyS := *s
		res = append(res, &copyS)
	}
	return res
}

func (m *Manager) Get(id string) (*Status, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, false
	}
	copyS := *s
	return &copyS, true
}

func (m *Manager) Start(id string, cfg Config) *Status {
	if cfg.Epochs < 1 {
		cfg.Epochs = 1
	}
	if cfg.MatchesPerEpoch < 1 {
		cfg.MatchesPerEpoch = 1
	}

	nowStr := time.Now().Format("20060102T150405")
	var baseDir string
	var baseDirPub string
	if cfg.Epochs == 1 {
		baseDir = filepath.Join("results", fmt.Sprintf("%s_best_of_%d", nowStr, cfg.MatchesPerEpoch))
	} else {
		baseDir = filepath.Join("results", fmt.Sprintf("%s_%d_epochs", nowStr, cfg.Epochs))
	}
	
	// Convert baseDir to an absolute path for the UI link
	absPath, err := filepath.Abs(baseDir)
	if err == nil {
		baseDirPub = absPath
	} else {
		baseDirPub = baseDir
	}

	// Note: We create this directory inside the current working directory.
	os.MkdirAll(baseDir, 0755)

	s := &Status{
		ID:           id,
		Config:       cfg,
		CurrentEpoch: 1,
		CurrentMatch: 0,
		IsFinished:   false,
		BaseDir:      baseDir,
		BaseDirPub:   baseDirPub,
		StartTime:    time.Now(),
	}

	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()

	go m.runLoop(s)
	return s
}

func (m *Manager) updateStatus(s *Status, epoch, match int, gameID string, finished bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s.CurrentEpoch = epoch
	s.CurrentMatch = match
	s.CurrentGameID = gameID
	s.IsFinished = finished
}

func (m *Manager) runLoop(s *Status) {
	for e := 1; e <= s.Config.Epochs; e++ {
		var epochDir string
		if s.Config.Epochs > 1 {
			epochDir = filepath.Join(s.BaseDir, fmt.Sprintf("epoch_%02d", e))
			os.MkdirAll(epochDir, 0755)
		} else {
			epochDir = s.BaseDir
		}

		for rm := 1; rm <= s.Config.MatchesPerEpoch; rm++ {
			g := m.store.Create(s.Config.RedPlayer, s.Config.BlackPlayer)
			m.updateStatus(s, e, rm, g.ID, false)

			// Spawn bots
			var delay time.Duration
			if s.Config.HumanSpeed {
				delay = 250 * time.Millisecond
			}

			if algo := bot.ByName(s.Config.RedBot); algo != nil {
				go bot.Run(g, engine.Red, algo, delay)
			}
			if algo := bot.ByName(s.Config.BlackBot); algo != nil {
				go bot.Run(g, engine.Black, algo, delay)
			}

			// Wait for game to finish synchronously without blocking on channel bottlenecks
			for {
				snap := g.Snapshot()
				if snap.Status != engine.StatusInProgress {
					// Game is done!
					
					// Aggregate stats
					m.mu.Lock()
					if snap.Status == engine.StatusRedWins {
						s.RedWins++
					} else if snap.Status == engine.StatusBlackWins {
						s.BlackWins++
					} else if snap.Status == engine.StatusDraw {
						s.Draws++
					}
					m.mu.Unlock()

					// Write results to file
					timestamp := time.Now().Format("20060102T150405")
					fileName := fmt.Sprintf("%s_match_%02d.json", timestamp, rm)
					filePath := filepath.Join(epochDir, fileName)
					
					fileData, _ := json.MarshalIndent(snap, "", "  ")
					os.WriteFile(filePath, fileData, 0644)
					
					// Clean the game engine memory!
					m.store.Delete(g.ID)
					break
				}
				time.Sleep(2 * time.Millisecond)
			}
		}
	}

	m.mu.Lock()
	s.EndTime = time.Now()
	m.mu.Unlock()
	
	m.updateStatus(s, s.Config.Epochs, s.Config.MatchesPerEpoch, "", true)
}
