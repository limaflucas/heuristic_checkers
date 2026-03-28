package gameai

import (
	"testing"
	"time"

	"github.com/limaflucas/heuristic_checkers/internal/engine"
)

func TestNegamaxReturnsMove(t *testing.T) {
	pos := engine.StartPosition()
	m := NegamaxChooseMove(pos, engine.Red, nil)
	if m.From == 0 && m.To == 0 {
		t.Error("NegamaxChooseMove returned zero move from start position")
	}
}

func TestNegamaxTimeBound(t *testing.T) {
	pos := engine.StartPosition()
	start := time.Now()
	NegamaxChooseMove(pos, engine.Red, nil)
	if elapsed := time.Since(start); elapsed > 2500*time.Millisecond {
		t.Errorf("NegamaxChooseMove took %v, want < 2.5s", elapsed)
	}
}

func TestNegamaxWithStats(t *testing.T) {
	pos := engine.StartPosition()
	var stats SearchStats
	NegamaxChooseMove(pos, engine.Red, &stats)
	if stats.NodesEvaluated == 0 {
		t.Error("expected NodesEvaluated > 0 with stats enabled")
	}
	if stats.MaxDepthReached == 0 {
		t.Error("expected MaxDepthReached > 0")
	}
	if stats.ExecutionTimeMs <= 0 {
		t.Error("expected ExecutionTimeMs > 0")
	}
}

func TestZobristDiffPositions(t *testing.T) {
	pos := engine.StartPosition()
	other := engine.Position{Black: pos.Black, Red: pos.Red ^ 1, Kings: pos.Kings}
	h1 := ZobristHash(pos, engine.Red)
	h2 := ZobristHash(other, engine.Red)
	if h1 == h2 {
		t.Error("different positions produced the same Zobrist hash")
	}
}

func TestZobristSideToMoveDiffers(t *testing.T) {
	pos := engine.StartPosition()
	hRed := ZobristHash(pos, engine.Red)
	hBlack := ZobristHash(pos, engine.Black)
	if hRed == hBlack {
		t.Error("same position with different side-to-move produced identical Zobrist hash")
	}
}

func TestNegamaxForcedCapture(t *testing.T) {
	pos := engine.StartPosition()
	moves := engine.LegalMoves(pos, engine.Red)
	if len(moves) == 0 {
		t.Fatal("no initial moves")
	}
	pos = engine.ApplyMove(pos, engine.Red, moves[0])

	m := NegamaxChooseMove(pos, engine.Black, nil)
	legal := engine.LegalMoves(pos, engine.Black)
	found := false
	for _, l := range legal {
		if engine.MovesEqual(l, m) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("NegamaxChooseMove returned an illegal move: from=%d to=%d", m.From, m.To)
	}
}

func TestPSTMirror(t *testing.T) {
	// 180° rotation: mirror(sq) = 31 - sq
	for sq := 0; sq < 32; sq++ {
		m := mirrorSq(sq)
		if m != 31-sq {
			t.Errorf("mirrorSq(%d) = %d, want %d", sq, m, 31-sq)
		}
	}
}

func TestSigmoidBounds(t *testing.T) {
	cases := []float64{-1000, -100, 0, 100, 1000}
	for _, v := range cases {
		s := Sigmoid(v)
		if s < 0 || s > 1 {
			t.Errorf("Sigmoid(%v) = %v, want in [0,1]", v, s)
		}
	}
	// Zero input → 0.5
	if got := Sigmoid(0); got != 0.5 {
		t.Errorf("Sigmoid(0) = %v, want 0.5", got)
	}
}

func TestFeatureVectorLength(t *testing.T) {
	pos := engine.StartPosition()
	grad := FeatureVector(pos)
	if len(grad) != 128 {
		t.Errorf("FeatureVector length = %d, want 128", len(grad))
	}
}

func TestMCTSReturnsMove(t *testing.T) {
	pos := engine.StartPosition()
	m := MCTSChooseMove(pos, engine.Red, nil)
	if m.From == 0 && m.To == 0 {
		t.Error("MCTSChooseMove returned zero move from start position")
	}
}

func TestMCTSLegal(t *testing.T) {
	pos := engine.StartPosition()
	m := MCTSChooseMove(pos, engine.Red, nil)
	legal := engine.LegalMoves(pos, engine.Red)
	found := false
	for _, l := range legal {
		if engine.MovesEqual(l, m) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("MCTSChooseMove returned illegal move: from=%d to=%d", m.From, m.To)
	}
}
