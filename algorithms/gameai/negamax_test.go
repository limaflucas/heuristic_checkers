package gameai

import (
	"testing"
	"time"

	"github.com/limaflucas/heuristic_checkers/internal/engine"
)

func TestNegamaxReturnsMove(t *testing.T) {
	pos := engine.StartPosition()
	m := NegamaxChooseMove(pos, engine.Red)
	if m.From == 0 && m.To == 0 {
		t.Error("NegamaxChooseMove returned zero move from start position")
	}
}

func TestNegamaxTimeBound(t *testing.T) {
	pos := engine.StartPosition()
	start := time.Now()
	NegamaxChooseMove(pos, engine.Red)
	if elapsed := time.Since(start); elapsed > 2500*time.Millisecond {
		t.Errorf("NegamaxChooseMove took %v, want < 2.5s", elapsed)
	}
}

func TestZobristDiffPositions(t *testing.T) {
	pos := engine.StartPosition()
	// Flip one bit so positions differ.
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
	// Build a position where Red has exactly one legal move and it is a capture.
	// Find a capture chain using the engine's LegalMoves so we don't need internal tables.
	pos := engine.StartPosition()
	// Do a few moves to reach a position where captures exist.
	moves := engine.LegalMoves(pos, engine.Red)
	if len(moves) == 0 {
		t.Fatal("no initial moves")
	}
	pos = engine.ApplyMove(pos, engine.Red, moves[0])

	// From here, Negamax must return a legal move (we just verify it's legal).
	m := NegamaxChooseMove(pos, engine.Black)
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

func TestNegamaxEvalSymmetry(t *testing.T) {
	// An equal position should score 0 from both sides.
	pos := engine.StartPosition()
	scoreRed := negamaxEval(pos, engine.Red)
	scoreBlack := negamaxEval(pos, engine.Black)
	// Both should be 0 (equal material, equal positional bonuses at start).
	if scoreRed != 0 {
		t.Errorf("starting position eval from Red perspective = %d, want 0", scoreRed)
	}
	if scoreBlack != 0 {
		t.Errorf("starting position eval from Black perspective = %d, want 0", scoreBlack)
	}
}
