package gameai

import (
	"encoding/json"
	"math"
	"os"
	"sync"

	"github.com/limaflucas/heuristic_checkers/internal/engine"
)

// PSTWeights holds four 32-square tables (indexed by internal square, 0-31).
// Values are additive positional bonuses on top of base material scores.
// All tables are from Red's perspective; Black pieces use the 180° rotated square.
type PSTWeights struct {
	MenOpening   [32]float64 `json:"men_opening"`
	MenEndgame   [32]float64 `json:"men_endgame"`
	KingsOpening [32]float64 `json:"kings_opening"`
	KingsEndgame [32]float64 `json:"kings_endgame"`
}

// DefaultPSTWeights returns a zero-initialised weight set.
// The engine stays fully functional with zeroed PSTs (pure material balance).
func DefaultPSTWeights() *PSTWeights { return &PSTWeights{} }

// LoadPSTWeights loads weights from a JSON file.
// Falls back to DefaultPSTWeights on any error so the bot always starts cleanly.
func LoadPSTWeights(path string) *PSTWeights {
	f, err := os.Open(path)
	if err != nil {
		return DefaultPSTWeights()
	}
	defer f.Close()
	var w PSTWeights
	if err := json.NewDecoder(f).Decode(&w); err != nil {
		return DefaultPSTWeights()
	}
	return &w
}

// SavePSTWeights serialises weights to a JSON file (pretty-printed).
func SavePSTWeights(path string, w *PSTWeights) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(w)
}

// ── Package-level singleton ───────────────────────────────────────────────────

var (
	globalPSTOnce    sync.Once
	globalPSTWeights *PSTWeights
)

const defaultWeightsPath = "weights/pst_weights.json"

// GlobalPST returns the package-level PST weights, loaded once from disk.
func GlobalPST() *PSTWeights {
	globalPSTOnce.Do(func() {
		globalPSTWeights = LoadPSTWeights(defaultWeightsPath)
	})
	return globalPSTWeights
}

// ReloadGlobalPST forces a fresh load from disk (useful after training).
func ReloadGlobalPST() {
	globalPSTOnce = sync.Once{}
	globalPSTOnce.Do(func() {
		globalPSTWeights = LoadPSTWeights(defaultWeightsPath)
	})
}

// ── Evaluation ────────────────────────────────────────────────────────────────

const (
	pstManMaterial  = 100.0
	pstKingMaterial = 150.0
	// SigmoidK maps ±200 material balance to ≈ 0.12–0.88 sigmoid range.
	SigmoidK = 0.01
)

// mirrorSq performs a 180° board rotation for Checkers symmetry.
// Checkers is rotationally (not vertically) symmetric: mirror = 31 - sq.
func mirrorSq(sq int) int { return 31 - sq }

// taperingPhase returns a value in [0,1] representing game phase.
// 1.0 = full opening (24 pieces), 0.0 = deep endgame (≤4 pieces).
func taperingPhase(bm, rm, bk, rk int) float64 {
	total := float64(bm + rm + bk + rk)
	phase := (total - 4.0) / 20.0
	if phase < 0 {
		return 0
	}
	if phase > 1 {
		return 1
	}
	return phase
}

// taperedValue interpolates between opening and endgame values.
func taperedValue(opening, endgame, phase float64) float64 {
	return opening*phase + endgame*(1-phase)
}

// PSTEval scores pos from Red's absolute perspective using the given weights.
// Returns a raw score (not normalised — use Sigmoid for probability space).
func PSTEval(pos engine.Position, w *PSTWeights) float64 {
	bm, rm, bk, rk := pos.RemainingCounts()
	phase := taperingPhase(bm, rm, bk, rk)
	score := 0.0

	// Red pieces — use square directly
	for sq := 0; sq < 32; sq++ {
		if !pos.IsRed(sq) {
			continue
		}
		if pos.IsKing(sq) {
			score += pstKingMaterial + taperedValue(w.KingsOpening[sq], w.KingsEndgame[sq], phase)
		} else {
			score += pstManMaterial + taperedValue(w.MenOpening[sq], w.MenEndgame[sq], phase)
		}
	}

	// Black pieces — 180° rotation
	for sq := 0; sq < 32; sq++ {
		if !pos.IsBlack(sq) {
			continue
		}
		msq := mirrorSq(sq)
		if pos.IsKing(sq) {
			score -= pstKingMaterial + taperedValue(w.KingsOpening[msq], w.KingsEndgame[msq], phase)
		} else {
			score -= pstManMaterial + taperedValue(w.MenOpening[msq], w.MenEndgame[msq], phase)
		}
	}

	return score
}

// PSTEvalColor returns the PST score from color's perspective.
func PSTEvalColor(pos engine.Position, color engine.Color, w *PSTWeights) float64 {
	raw := PSTEval(pos, w)
	if color == engine.Red {
		return raw
	}
	return -raw
}

// Sigmoid maps a raw PST score to [0,1] probability space.
// outcome=1.0 means Red wins; 0.0 means Black wins.
func Sigmoid(v float64) float64 {
	return 1.0 / (1.0 + math.Exp(-SigmoidK*v))
}

// ── Feature vector for TD-Leaf gradient ──────────────────────────────────────

// FeatureVector returns the gradient ∂V/∂w for a position (length 128).
// Layout: [0-31]=MenOpening, [32-63]=MenEndgame, [64-95]=KingsOpening, [96-127]=KingsEndgame.
// Sign is +1 for Red pieces (increase weight → higher Red score → Red wins),
// -1 for Black pieces (using mirrored square indexing).
func FeatureVector(pos engine.Position) [128]float64 {
	bm, rm, bk, rk := pos.RemainingCounts()
	phase := taperingPhase(bm, rm, bk, rk)
	var grad [128]float64

	for sq := 0; sq < 32; sq++ {
		switch {
		case pos.IsRed(sq) && !pos.IsKing(sq):
			grad[sq] += phase       // MenOpening
			grad[32+sq] += 1 - phase // MenEndgame
		case pos.IsRed(sq) && pos.IsKing(sq):
			grad[64+sq] += phase        // KingsOpening
			grad[96+sq] += 1 - phase // KingsEndgame
		case pos.IsBlack(sq) && !pos.IsKing(sq):
			msq := mirrorSq(sq)
			grad[msq] -= phase
			grad[32+msq] -= 1 - phase
		case pos.IsBlack(sq) && pos.IsKing(sq):
			msq := mirrorSq(sq)
			grad[64+msq] -= phase
			grad[96+msq] -= 1 - phase
		}
	}
	return grad
}

// ApplyGradient adds alpha * delta * grad to weights (in-place).
func ApplyGradient(w *PSTWeights, grad [128]float64, scale float64) {
	for i := 0; i < 32; i++ {
		w.MenOpening[i] += scale * grad[i]
		w.MenEndgame[i] += scale * grad[32+i]
		w.KingsOpening[i] += scale * grad[64+i]
		w.KingsEndgame[i] += scale * grad[96+i]
	}
}
