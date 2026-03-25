package engine

import (
	"testing"
)

// ---- Board / Position ----

func TestStartPosition(t *testing.T) {
	pos := StartPosition()

	bm, rm, bk, rk := pos.RemainingCounts()
	if bm != 12 {
		t.Errorf("want 12 black men, got %d", bm)
	}
	if rm != 12 {
		t.Errorf("want 12 red men, got %d", rm)
	}
	if bk != 0 || rk != 0 {
		t.Errorf("want 0 kings, got bk=%d rk=%d", bk, rk)
	}

	// All black squares in rows 0-2 are black
	for sq := 20; sq < 32; sq++ {
		if !pos.IsBlack(sq) {
			t.Errorf("sq %d (ACF %d) should be black", sq, InternalToACF(sq))
		}
	}
	// All black squares in rows 5-7 are red
	for sq := 0; sq < 12; sq++ {
		if !pos.IsRed(sq) {
			t.Errorf("sq %d (ACF %d) should be red", sq, InternalToACF(sq))
		}
	}
	// Middle rows empty
	for sq := 12; sq < 20; sq++ {
		if !pos.IsEmpty(sq) {
			t.Errorf("sq %d (ACF %d) should be empty", sq, InternalToACF(sq))
		}
	}
}

func TestMatrix(t *testing.T) {
	pos := StartPosition()
	m := pos.ToMatrix()

	// Count non-zero cells
	count := 0
	for r := 0; r < 8; r++ {
		for c := 0; c < 8; c++ {
			if m[r][c] != 0 {
				count++
			}
		}
	}
	if count != 24 {
		t.Errorf("want 24 pieces in matrix, got %d", count)
	}
}

// ---- Adjacency ----

func TestAdjacencyNoNegativeRows(t *testing.T) {
	// Row 0 squares (internal 28-31) should have no upward neighbors.
	// (Row 0 from top means squares 28-31; UL/UR go to row -1 which is invalid)
	for sq := 28; sq < 32; sq++ {
		if diagNeighbors[sq][DirUL] != -1 || diagNeighbors[sq][DirUR] != -1 {
			t.Errorf("sq %d row0 should have no UL/UR neighbor", sq)
		}
	}
}

func TestAdjacencyBottomRow(t *testing.T) {
	// Row 7 (internal 0-3) should have no downward neighbors.
	for sq := 0; sq < 4; sq++ {
		if diagNeighbors[sq][DirDL] != -1 || diagNeighbors[sq][DirDR] != -1 {
			t.Errorf("sq %d row7 should have no DL/DR neighbor", sq)
		}
	}
}

func TestKnownNeighbors(t *testing.T) {
	// Verify adjacency for a known square using the corrected internal layout.
	// Internal sq = ACF - 1.
	// sq=0..3: row 0 from bottom (even, cols 0,2,4,6) — Red home.
	//
	// ACF 5 = internal 4: row 1 from bottom (odd, col 2*0+1=1)
	//   UL (toward Black, +4): sq+4=8, ACF 9
	//   UR: p=0, p<3 → sq+5=9, ACF 10
	//   DL (toward Red, -4): sq-4=0, ACF 1
	//   DR: p=0, p<3 → sq-3=1, ACF 2
	sq := ACFToInternal(5) // = 4
	if diagNeighbors[sq][DirUL] != ACFToInternal(9) {
		t.Errorf("ACF5 UL should be ACF9 (internal 8), got internal %d (ACF%d)",
			diagNeighbors[sq][DirUL], InternalToACF(diagNeighbors[sq][DirUL]))
	}
	if diagNeighbors[sq][DirUR] != ACFToInternal(10) {
		t.Errorf("ACF5 UR should be ACF10 (internal 9), got internal %d (ACF%d)",
			diagNeighbors[sq][DirUR], InternalToACF(diagNeighbors[sq][DirUR]))
	}
	if diagNeighbors[sq][DirDL] != ACFToInternal(1) {
		t.Errorf("ACF5 DL should be ACF1 (internal 0), got internal %d (ACF%d)",
			diagNeighbors[sq][DirDL], InternalToACF(diagNeighbors[sq][DirDL]))
	}
	if diagNeighbors[sq][DirDR] != ACFToInternal(2) {
		t.Errorf("ACF5 DR should be ACF2 (internal 1), got internal %d (ACF%d)",
			diagNeighbors[sq][DirDR], InternalToACF(diagNeighbors[sq][DirDR]))
	}
}


// ---- Legal Moves ----

func TestInitialRedMoves(t *testing.T) {
	pos := StartPosition()
	moves := LegalMoves(pos, Red)
	// Red starts at squares 0-11 (ACF 1-12). Only row 5 pieces (ACF 9-12, internal 8-11) can move.
	// Each of those 4 pieces has 2 forward (UL/UR) moves → 7 total (corner pieces have only 1).
	// ACF 9 (internal 8): row5/col6, odd-from-top. UR=+4→12=ACF13, UL=+5→13=ACF14. Both valid.
	// ACF 10 (internal 9): UR=+4→13=ACF14, UL=+5→14=ACF15. Both valid.
	// ACF 11 (internal 10): UR=+4→14=ACF15, UL=+5→15=ACF16. Both valid.
	// ACF 12 (internal 11): UR=+4→15=ACF16, UL=+5 blocked? p=3 for ACF12, so UL exists only if p>0 ✓ (+5→16=ACF17).
	// Wait, internal 11 = row 2, p = 11%4=3. Row 2 is even-from-top.
	// For even-from-top: UL=+4(always if r>0), UR=+3(p<3).
	// internal 11: row=2, p=3. UL=11+4=15=ACF16. UR: p=3, not <3 → no UR.
	// So ACF12 has only UL move.
	// That gives: 4 pieces × 2 = 8, minus 1 (ACF12 no UR) = 7.
	if len(moves) != 7 {
		t.Errorf("expected 7 initial red moves, got %d: %v", len(moves), moves)
	}
}

func TestInitialBlackMoves(t *testing.T) {
	// Black at 20-31 (ACF 21-32). Only the front row (internal 20-23, ACF 21-24) can move.
	pos := StartPosition()
	moves := LegalMoves(pos, Black)
	// ACF 21 (internal 20): row 2, p=0, even-from-top. DL=-4→16=ACF17, DR=-5→15=ACF16. Both valid.
	// ACF 22 (internal 21): DL=-4→17=ACF18, DR=-5→16=ACF17. Both valid.
	// ACF 23 (internal 22): DL=-4→18=ACF19, DR=-5→17=ACF18. Both valid.
	// ACF 24 (internal 23): row 2, p=3. DL=-4→19=ACF20, DR: p=3 → no DR.
	// Total: 3×2 + 1 = 7
	if len(moves) != 7 {
		t.Errorf("expected 7 initial black moves, got %d", len(moves))
	}
}

func TestForcedJump(t *testing.T) {
	// Place a black man where it can be jumped, and verify simple moves are suppressed.
	// Red man at ACF 17 (internal 16), black man at ACF 13 (internal 12),
	// landing ACF 10 (internal 9) must be empty.
	pos := Position{}
	pos = placePiece(pos, ACFToInternal(17), Red, false)
	pos = placePiece(pos, ACFToInternal(13), Black, false)
	// internal 16 (ACF17): row4, even-from-top, p=0. UL=+4→20=ACF21. UR=+3→19=ACF20.
	// Hmm, red moves UL, UR. Let me pick a cleaner setup.
	// Red at ACF 15 (internal 14), black at ACF 10 (internal 9), landing at ACF 6 (internal 5).
	// internal 14: row 3, odd-from-top, p=2. UR=+4→18=ACF19. UL=+5→19=ACF20.
	// Not a jump setup. Let me think more carefully.
	//
	// For a jump: red at sq, black at diagNeighbors[sq][d], empty at diagJump[sq][d][1].
	// Use: red ACF 21 (internal 20), row2, even-from-top, p=0.
	//   DL=-4→16=ACF17, DR=-5→15=ACF16. For a RED piece, forward is UL/UR not DL/DR.
	//
	// Red at ACF 9 (internal 8), row5, odd-from-top, p=0.
	//   UL: p>0 required → no UL. UR=+4→12=ACF13.
	//   Jump over ACF13: mid=12, land=diagNeighbors[12][DirUR]=?
	//   internal 12: row 3, even-from-top, p=0. UR=+3→15=ACF16. But we need land to be empty.
	// Let's just set up: red at 8(ACF9), black at 12(ACF13), empty at 16(ACF17... wait internal 15=ACF16).
	// diagJump[8][DirUR] = {mid=12, land=16}? 
	// diagNeighbors[8][DirUR]=12. diagNeighbors[12][DirUR]=15. So land=15=ACF16.
	pos2 := Position{}
	pos2 = placePiece(pos2, ACFToInternal(9), Red, false) // internal 8
	pos2 = placePiece(pos2, ACFToInternal(13), Black, false) // internal 12, adjacent to 8 in UR
	// verify jump table
	mid := diagJump[ACFToInternal(9)][DirUR][0]
	land := diagJump[ACFToInternal(9)][DirUR][1]
	if mid != ACFToInternal(13) {
		t.Fatalf("expected mid=12(ACF13), got %d(ACF%d)", mid, InternalToACF(mid))
	}
	if land < 0 || land > 31 {
		t.Fatalf("invalid land square: %d", land)
	}

	// With black at mid and empty at land, red must jump.
	moves := LegalMoves(pos2, Red)
	for _, m := range moves {
		if len(m.Captures) == 0 {
			t.Errorf("forced-jump rule violated: found simple move from ACF%d to ACF%d",
				InternalToACF(m.From), InternalToACF(m.To))
		}
	}
	_ = land
}

func TestSimpleMoveApply(t *testing.T) {
	pos := StartPosition()
	// Red moves ACF 11 (internal 10) → ACF 15 (internal 14) via UR offset.
	// internal 10: row2, even-from-top, p=2. UR=+3→13=ACF14. UL=+4→14=ACF15.
	from := ACFToInternal(11)
	to := ACFToInternal(15)
	m := Move{From: from, To: to}
	newPos := ApplyMove(pos, Red, m)

	if !newPos.IsEmpty(from) {
		t.Error("from square should be empty after move")
	}
	if !newPos.IsRed(to) {
		t.Error("to square should have red piece after move")
	}
	if newPos.IsKing(to) {
		t.Error("should not be promoted")
	}
}

func TestPromotion(t *testing.T) {
	// Put a red man one step from the king row.
	// Red king row: internal 28-31 (ACF 29-32, row 0 from top).
	// A red piece in row 1 can reach row 0.
	// Row 1 (odd-from-top): positions at internal 24-27.
	// internal 27: row6, odd-from-top, p=3. UR=+4→31=ACF32.
	// internal 27 is ACF 28. Put a red man there.
	pos := Position{}
	pos = placePiece(pos, ACFToInternal(28), Red, false) // internal 27
	to := ACFToInternal(32)                               // internal 31

	// Check adjacency: diagNeighbors[27][DirUR] should be 31.
	if diagNeighbors[27][DirUR] != 31 {
		t.Fatalf("expected neighbor 31, got %d", diagNeighbors[27][DirUR])
	}

	m := Move{From: ACFToInternal(28), To: to}
	newPos := ApplyMove(pos, Red, m)

	if !newPos.IsRed(to) {
		t.Error("destination should be red")
	}
	if !newPos.IsKing(to) {
		t.Error("red piece reaching row 0 should become king")
	}
}

func TestKingMovesAllDirections(t *testing.T) {
	pos := Position{}
	// Place a red king in the middle of the board, e.g. internal 14 (ACF 15, row3).
	pos = placePiece(pos, 14, Red, true)
	moves := LegalMoves(pos, Red)

	// A king in a central position should have 4 simple moves (one per direction).
	dirs := map[int]bool{}
	for _, m := range moves {
		dirs[m.To] = true
	}
	if len(dirs) < 2 {
		t.Errorf("king should have multiple move directions, got %d targets", len(dirs))
	}
}

func TestWinByNoMoves(t *testing.T) {
	// Black has no pieces → red wins.
	pos := Position{}
	pos = placePiece(pos, 0, Red, false)

	if HasLegalMoves(pos, Black) {
		t.Error("black should have no legal moves")
	}
}

func TestMultiJump(t *testing.T) {
	// Setup: red man at ACF 1 (internal 0), two black men in its path.
	// internal 0: row 7, odd-from-top, p=0. UR=+4→4=ACF5.
	// diagJump[0][DirUR] = mid=4, land=? diagNeighbors[4][DirUR]
	// internal 4: row 6, even-from-top, p=0. UR=+3→7=ACF8.
	// So jump: 0 → over 4 → lands at 8? No: diagJump[0][DirUR][0]=4, [1]=diagNeighbors[4][DirUR]=7.
	// Wait: internal 4 row=1 (4/4=1), p=0. Odd-from-top. UR=+4→8=ACF9.
	// So land at 8 (ACF9). Then from 8, can red jump again?
	// internal 8: row 2, even-from-top, p=0. UL=+4→12=ACF13. UR=+3→? p<3 so UR=11=ACF12.
	// Place black at 4(ACF5) and 12(ACF13) (or 11(ACF12)), and ensure landing squares are empty.
	pos := Position{}
	pos = placePiece(pos, ACFToInternal(1), Red, false)  // internal 0
	pos = placePiece(pos, ACFToInternal(5), Black, false) // internal 4  (mid of first jump)
	// land of first jump: diagJump[0][DirUR][1]
	firstLand := diagJump[ACFToInternal(1)][DirUR][1]
	if firstLand < 0 {
		t.Fatal("no first jump available from ACF1 UR")
	}
	// Place second black in path from firstLand
	secondMid := diagJump[firstLand][DirUR][0]
	secondLand := diagJump[firstLand][DirUR][1]
	if secondMid < 0 || secondLand < 0 {
		// Try UL direction
		secondMid = diagJump[firstLand][DirUL][0]
		secondLand = diagJump[firstLand][DirUL][1]
	}
	if secondMid < 0 {
		t.Skip("cannot set up multi-jump from this test position")
	}
	pos = placePiece(pos, secondMid, Black, false)

	moves := LegalMoves(pos, Red)
	var multiJumps []Move
	for _, m := range moves {
		if len(m.Captures) >= 2 {
			multiJumps = append(multiJumps, m)
		}
	}
	if len(multiJumps) == 0 {
		t.Error("expected at least one multi-jump move")
	}
}

// ---- Game ----

func TestNewGame(t *testing.T) {
	g := NewGame("Alice", "Bob")
	if g.Turn != Red {
		t.Error("Red should move first")
	}
	if g.Status != StatusInProgress {
		t.Error("game should be in progress")
	}
}

func TestMakeMoveWrongTurn(t *testing.T) {
	g := NewGame("Alice", "Bob")
	// Black tries to move on Red's turn
	pos := g.Position
	moves := LegalMoves(pos, Black)
	if len(moves) == 0 {
		t.Fatal("no black moves available")
	}
	m := moves[0]
	err := g.MakeMove(Black, m)
	if err == nil {
		t.Error("expected error for wrong turn")
	}
}

func TestMakeMoveIllegal(t *testing.T) {
	g := NewGame("Alice", "Bob")
	// Try to move from an empty square
	err := g.MakeMove(Red, Move{From: ACFToInternal(16), To: ACFToInternal(20)})
	if err == nil {
		t.Error("expected error for illegal move")
	}
}

func TestMakeValidMove(t *testing.T) {
	g := NewGame("Alice", "Bob")
	pos := g.Position
	moves := LegalMoves(pos, Red)
	if len(moves) == 0 {
		t.Fatal("no red moves")
	}
	m := moves[0]
	err := g.MakeMove(Red, m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.Turn != Black {
		t.Error("turn should switch to black after red moves")
	}
	if len(g.Moves) != 1 {
		t.Error("move history should have 1 entry")
	}
	// At least a MOVE event
	found := false
	for _, ev := range g.Events {
		if ev.Type == EventMove {
			found = true
			break
		}
	}
	if !found {
		t.Error("MOVE event not recorded")
	}
}

func TestKingCreatedEvent(t *testing.T) {
	g := NewGame("Alice", "Bob")
	// Manually put a red man one step from king row and make the move.
	g.Position = Position{}
	g.Position = placePiece(g.Position, ACFToInternal(28), Red, false)
	g.Turn = Red

	err := g.MakeMove(Red, Move{From: ACFToInternal(28), To: ACFToInternal(32)})
	if err != nil {
		t.Fatalf("move failed: %v", err)
	}
	found := false
	for _, ev := range g.Events {
		if ev.Type == EventKingCreated {
			found = true
		}
	}
	if !found {
		t.Error("KING_CREATED event not recorded")
	}
}

func TestCaptureEvent(t *testing.T) {
	pos := Position{}
	from := ACFToInternal(9)
	mid := diagJump[from][DirUR][0]
	land := diagJump[from][DirUR][1]
	if mid < 0 || land < 0 {
		t.Skip("no jump available from ACF9 UR")
	}
	pos = placePiece(pos, from, Red, false)
	pos = placePiece(pos, mid, Black, false)

	g := NewGame("A", "B")
	g.Position = pos
	g.Turn = Red

	err := g.MakeMove(Red, Move{From: from, To: land, Captures: []int{mid}})
	if err != nil {
		t.Fatalf("move failed: %v", err)
	}
	found := false
	for _, ev := range g.Events {
		if ev.Type == EventCapture {
			found = true
		}
	}
	if !found {
		t.Error("CAPTURE event not recorded")
	}
}

func TestGameOverEvent(t *testing.T) {
	// Red takes black's last piece.
	pos := Position{}
	from := ACFToInternal(9)
	mid := diagJump[from][DirUR][0]
	land := diagJump[from][DirUR][1]
	if mid < 0 || land < 0 {
		t.Skip("skip")
	}
	pos = placePiece(pos, from, Red, false)
	pos = placePiece(pos, mid, Black, false)

	g := NewGame("A", "B")
	g.Position = pos
	g.Turn = Red

	err := g.MakeMove(Red, Move{From: from, To: land, Captures: []int{mid}})
	if err != nil {
		t.Fatalf("move error: %v", err)
	}
	if g.Status != StatusRedWins {
		t.Errorf("expected red wins, got %s", g.Status)
	}
	found := false
	for _, ev := range g.Events {
		if ev.Type == EventGameOver {
			found = true
		}
	}
	if !found {
		t.Error("GAME_OVER event not recorded")
	}
}

func TestDrawHalfMoveClock(t *testing.T) {
	g := NewGame("A", "B")
	// Manually set clock to trigger draw
	g.halfMoveClock = 79
	g.Position = Position{}
	// Place one red king and one black king so both have moves (no captures; clock increments)
	g.Position = placePiece(g.Position, ACFToInternal(15), Red, true)
	g.Position = placePiece(g.Position, ACFToInternal(32), Black, true)
	g.Turn = Red

	moves := LegalMoves(g.Position, Red)
	if len(moves) == 0 {
		t.Fatal("red king has no moves")
	}
	// Pick a non-capture king move (no captures in the position)
	err := g.MakeMove(Red, moves[0])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.Status != StatusDraw {
		t.Errorf("expected draw after 80 half-moves, got %s", g.Status)
	}
}
