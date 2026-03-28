package engine

// Move represents a single complete move (possibly multi-jump).
type Move struct {
	From       int   // internal square (0-31)
	To         int   // internal square (0-31)
	Captures   []int // internal squares captured, in order
	IsKingMove bool  // the moving piece was a king before the move
	Promoted   bool  // the move resulted in a promotion
}

// forwardDirs returns the forward diagonal directions for color c.
// Kings use all four; men use only the two "toward opponent" directions.
func forwardDirs(c Color, king bool) []int {
	if king {
		return []int{DirUL, DirUR, DirDL, DirDR}
	}
	// Red moves toward row 0 (upward) → UL, UR
	if c == Red {
		return []int{DirUL, DirUR}
	}
	// Black moves toward row 7 (downward) → DL, DR
	return []int{DirDL, DirDR}
}

// LegalMoves returns all legal moves for color c in position pos.
// If any jump exists, only jump moves are returned (mandatory capture).
func LegalMoves(pos Position, c Color) []Move {
	squares := pos.PieceSquares(c)

	// Collect jumps first.
	var jumps []Move
	for _, sq := range squares {
		isKing := pos.IsKing(sq)
		jumps = append(jumps, genJumpsFrom(pos, sq, sq, c, isKing, 0, nil)...)
	}
	if len(jumps) > 0 {
		return jumps
	}

	// No jumps → collect simple moves.
	var moves []Move
	for _, sq := range squares {
		isKing := pos.IsKing(sq)
		dirs := forwardDirs(c, isKing)
		for _, d := range dirs {
			nb := diagNeighbors[sq][d]
			if nb == -1 {
				continue
			}
			if pos.IsEmpty(nb) {
				promoted := !isKing && IsKingRow(nb, c)
				moves = append(moves, Move{
					From:       sq,
					To:         nb,
					Captures:   nil,
					IsKingMove: isKing,
					Promoted:   promoted,
				})
			}
		}
	}
	return moves
}

// genJumpsFrom recursively finds all multi-jump chains starting from sq.
// originalFrom is the very first square of the full move (constant through recursion).
// visitedMask tracks already-captured squares to prevent re-capturing.
// capturedSoFar accumulates the capture list for the current chain.
// Returns complete moves (moves where no further jump is possible).
func genJumpsFrom(pos Position, sq, originalFrom int, c Color, isKing bool, visitedMask uint32, capturedSoFar []int) []Move {
	dirs := forwardDirs(c, isKing)
	opp := c.Opponent()

	var result []Move

	for _, d := range dirs {
		mid := diagJump[sq][d][0]
		land := diagJump[sq][d][1]
		if mid == -1 || land == -1 {
			continue
		}
		// mid must have opponent piece and not already captured.
		if pos.ColorAt(mid) != opp {
			continue
		}
		if visitedMask>>mid&1 == 1 {
			continue
		}
		if !pos.IsEmpty(land) {
			continue
		}

		// Apply this jump temporarily.
		newPos := applyJump(pos, c, sq, mid, land)
		newVisited := visitedMask | (1 << mid)
		newCaptures := append(append([]int(nil), capturedSoFar...), mid)

		promoted := !isKing && IsKingRow(land, c)
		if promoted {
			// Promotion ends the turn immediately — no further jumps allowed.
			result = append(result, Move{
				From:       originalFrom,
				To:         land,
				Captures:   newCaptures,
				IsKingMove: isKing,
				Promoted:   true,
			})
			continue
		}

		// Try to continue jumping from the landing square.
		// A man that reached a non-king-row square continues as a man.
		sub := genJumpsFrom(newPos, land, originalFrom, c, isKing, newVisited, newCaptures)
		if len(sub) > 0 {
			result = append(result, sub...)
		} else {
			// No further jump possible — terminate chain here.
			result = append(result, Move{
				From:       originalFrom,
				To:         land,
				Captures:   newCaptures,
				IsKingMove: isKing,
				Promoted:   false,
			})
		}
	}

	return result
}

// applyJump returns a new position after a single jump step.
// It does NOT promote: promotion is handled by ApplyMove.
func applyJump(pos Position, c Color, from, mid, land int) Position {
	// Remove piece from `from`, place on `land`.
	isKing := pos.IsKing(from)
	p := pos
	p = removePiece(p, from)
	p = placePiece(p, land, c, isKing)
	p = removePiece(p, mid) // capture
	return p
}

// ApplyMove returns the new position after fully applying a complete move.
// It handles promotion and all captures.
func ApplyMove(pos Position, c Color, m Move) Position {
	isKing := pos.IsKing(m.From)
	p := removePiece(pos, m.From)

	// Apply in sequence: if multi-jump we re-apply intermediate state isn't needed
	// because the Move already stores From/To/Captures.
	for _, cap := range m.Captures {
		p = removePiece(p, cap)
	}

	promoteNow := !isKing && IsKingRow(m.To, c)
	p = placePiece(p, m.To, c, isKing || promoteNow)
	return p
}

// IsLegal checks whether m is among the legal moves for color c in pos.
func IsLegal(pos Position, c Color, m Move) bool {
	for _, legal := range LegalMoves(pos, c) {
		if movesEqual(legal, m) {
			return true
		}
	}
	return false
}

func movesEqual(a, b Move) bool {
	return MovesEqual(a, b)
}

// MovesEqual reports whether two moves are identical (same from/to/captures sequence).
func MovesEqual(a, b Move) bool {
	if a.From != b.From || a.To != b.To || len(a.Captures) != len(b.Captures) {
		return false
	}
	for i := range a.Captures {
		if a.Captures[i] != b.Captures[i] {
			return false
		}
	}
	return true
}

// removePiece clears a square from all bitboards.
func removePiece(p Position, sq int) Position {
	bit := ^(uint32(1) << sq)
	p.Black &= bit
	p.Red &= bit
	p.Kings &= bit
	return p
}

// placePiece sets a piece at sq.
func placePiece(p Position, sq int, c Color, king bool) Position {
	bit := uint32(1) << sq
	if c == Black {
		p.Black |= bit
	} else {
		p.Red |= bit
	}
	if king {
		p.Kings |= bit
	}
	return p
}

// LegalMovesPerPiece returns a map from square → legal moves for that square.
// Only includes pieces that have at least one legal move.
func LegalMovesPerPiece(pos Position, c Color) map[int][]Move {
	all := LegalMoves(pos, c)
	out := make(map[int][]Move)
	for _, m := range all {
		out[m.From] = append(out[m.From], m)
	}
	return out
}

// HasLegalMoves reports whether color c has any legal move.
func HasLegalMoves(pos Position, c Color) bool {
	return len(LegalMoves(pos, c)) > 0
}
