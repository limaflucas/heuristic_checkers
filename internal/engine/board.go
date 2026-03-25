// Package engine implements the English Checkers (Draughts) rules engine.
// Board squares use the standard ACF (American Checkers Federation) numbering: 1–32,
// where 1 is at the bottom-right (Red's home side) and 32 is at the top-left (Black's side).
//
// Internal representation uses 0-indexed squares (acf-1) packed into three uint32 bitboards:
//   - Black:   bit n set  → black piece at square n
//   - Red:     bit n set  → red piece at square n
//   - Kings:   bit n set  → that piece is a king
//
// Internal square layout (row = sq/4, 0=bottom=Red home, 7=top=Black home):
//
//	Row 7 (top, Black home, ACF row 0): sq 28–31  odd board  cols 1,3,5,7
//	Row 6:                              sq 24–27  even board cols 0,2,4,6
//	Row 5:                              sq 20–23  odd board  cols 1,3,5,7
//	Row 4:                              sq 16–19  even board cols 0,2,4,6
//	Row 3:                              sq 12–15  odd board  cols 1,3,5,7
//	Row 2:                              sq  8–11  even board cols 0,2,4,6
//	Row 1:                              sq  4–7   odd board  cols 1,3,5,7
//	Row 0 (bottom, Red home, ACF row 7):sq  0–3   even board cols 0,2,4,6
//
// Diagonal directions: DirUL, DirUR go toward higher sq (toward Black);
//					  DirDL, DirDR go toward lower sq (toward Red).
package engine

// Color represents which side a piece belongs to.
type Color int8

const (
	Red   Color = 1
	Black Color = -1
	None  Color = 0
)

func (c Color) String() string {
	switch c {
	case Red:
		return "red"
	case Black:
		return "black"
	}
	return "none"
}

func (c Color) Opponent() Color { return -c }

// Position is the complete board state encoded as three bitboards.
// Squares are 0-indexed (internal) = ACF square - 1.
type Position struct {
	Black uint32
	Red   uint32
	Kings uint32
}

// Direction constants for the adjacency table.
const (
	DirUL = 0
	DirUR = 1
	DirDL = 2
	DirDR = 3
)

// diagNeighbors[sq][dir] is the adjacent square in that direction (-1 if none).
var diagNeighbors [32][4]int

// diagJump[sq][dir] = {midSquare, landSquare} for a jump in direction dir (-1 if unavailable).
var diagJump [32][4][2]int

// ACF king-row masks (internal 0-indexed).
// Red pieces reaching squares 28-31 (ACF 29-32) are promoted.
// Black pieces reaching squares 0-3  (ACF 1-4)  are promoted.
const (
	redKingRowMask   uint32 = 0xF0000000 // bits 28-31
	blackKingRowMask uint32 = 0x0000000F // bits 0-3
)

// StartPosition returns the standard opening position.
// Black: ACF 21-32 (internal 20-31), Red: ACF 1-12 (internal 0-11).
func StartPosition() Position {
	return Position{
		Black: 0xFFF00000, // bits 20-31
		Red:   0x00000FFF, // bits 0-11
		Kings: 0,
	}
}

func init() {
	// Initialise every entry to -1.
	for sq := 0; sq < 32; sq++ {
		for d := 0; d < 4; d++ {
			diagNeighbors[sq][d] = -1
			diagJump[sq][d][0] = -1
			diagJump[sq][d][1] = -1
		}
	}

	// Internal square layout:
	//   sq = 0..3   → row 0 from bottom (Red home, board row 7, even board cols 0,2,4,6)
	//   sq = 4..7   → row 1 from bottom (board row 6, odd  board cols 1,3,5,7)
	//   sq = 8..11  → row 2 from bottom (board row 5, even board cols)
	//   ...continuing alternating...
	//   sq = 28..31 → row 7 from bottom (Black home, board row 0, odd board cols)
	//
	//   row = sq/4  (0=bottom=Red side, 7=top=Black side)
	//   p   = sq%4  (0=leftmost playable square in that row)
	//
	//   Even bottom-row (row%2==0): physical cols 0,2,4,6  → col(p) = 2*p
	//   Odd  bottom-row (row%2==1): physical cols 1,3,5,7  → col(p) = 2*p+1
	//
	//   UL/UR go toward higher row (toward Black, +offset).
	//   DL/DR go toward lower  row (toward Red,   -offset).
	//
	// Offsets for even-bottom-row (col = 2p):
	//   UL: row+1 is odd, diagonal to col 2p-1 = col 2(p-1)+1 → pos (p-1) in odd row
	//       internal = (row+1)*4+(p-1) = sq+3.  Requires p>0.
	//   UR: row+1 is odd, diagonal to col 2p+1 = col 2p+1     → pos p in odd row
	//       internal = (row+1)*4+p = sq+4.
	//   DL: row-1 is odd, diagonal to col 2p-1 → pos (p-1) in odd row
	//       internal = (row-1)*4+(p-1) = sq-5.  Requires p>0.
	//   DR: row-1 is odd, diagonal to col 2p+1 → pos p in odd row
	//       internal = (row-1)*4+p = sq-4.
	//
	// Offsets for odd-bottom-row (col = 2p+1):
	//   UL: row+1 is even, diagonal to col 2p → pos p in even row
	//       internal = (row+1)*4+p = sq+4.
	//   UR: row+1 is even, diagonal to col 2p+2 → pos (p+1) in even row
	//       internal = (row+1)*4+(p+1) = sq+5.  Requires p<3.
	//   DL: row-1 is even, diagonal to col 2p → pos p in even row
	//       internal = (row-1)*4+p = sq-4.
	//   DR: row-1 is even, diagonal to col 2p+2 → pos (p+1) in even row
	//       internal = (row-1)*4+(p+1) = sq-3.  Requires p<3.

	for sq := 0; sq < 32; sq++ {
		row := sq / 4
		p := sq % 4

		if row%2 == 0 { // even bottom-row: even board columns
			if row < 7 { // can move upward
				if p > 0 {
					diagNeighbors[sq][DirUL] = sq + 3
				}
				diagNeighbors[sq][DirUR] = sq + 4
			}
			if row > 0 { // can move downward
				if p > 0 {
					diagNeighbors[sq][DirDL] = sq - 5
				}
				diagNeighbors[sq][DirDR] = sq - 4
			}
		} else { // odd bottom-row: odd board columns
			if row < 7 { // can move upward
				diagNeighbors[sq][DirUL] = sq + 4
				if p < 3 {
					diagNeighbors[sq][DirUR] = sq + 5
				}
			}
			if row > 0 { // can move downward
				diagNeighbors[sq][DirDL] = sq - 4
				if p < 3 {
					diagNeighbors[sq][DirDR] = sq - 3
				}
			}
		}
	}

	// Build jump table: a jump in direction d from sq goes OVER mid and lands at land.
	for sq := 0; sq < 32; sq++ {
		for d := 0; d < 4; d++ {
			mid := diagNeighbors[sq][d]
			if mid < 0 {
				continue
			}
			land := diagNeighbors[mid][d]
			if land < 0 {
				continue
			}
			diagJump[sq][d][0] = mid
			diagJump[sq][d][1] = land
		}
	}
}


// --- Bitboard helpers ---

func (p Position) Occupied() uint32 { return p.Black | p.Red }
func (p Position) Empty() uint32    { return ^(p.Black | p.Red) }

func (p Position) IsBlack(sq int) bool { return p.Black>>sq&1 == 1 }
func (p Position) IsRed(sq int) bool   { return p.Red>>sq&1 == 1 }
func (p Position) IsKing(sq int) bool  { return p.Kings>>sq&1 == 1 }
func (p Position) IsEmpty(sq int) bool { return (p.Occupied())>>sq&1 == 0 }

func (p Position) ColorAt(sq int) Color {
	switch {
	case p.IsBlack(sq):
		return Black
	case p.IsRed(sq):
		return Red
	}
	return None
}

// IsKingRow returns true if reaching sq promotes the given color.
func IsKingRow(sq int, c Color) bool {
	bit := uint32(1) << sq
	if c == Red {
		return bit&redKingRowMask != 0
	}
	return bit&blackKingRowMask != 0
}

// PieceSquares returns the list of squares (0-based internal) occupied by color c.
func (p Position) PieceSquares(c Color) []int {
	var board uint32
	if c == Black {
		board = p.Black
	} else {
		board = p.Red
	}
	out := make([]int, 0, 12)
	for board != 0 {
		sq := bits32TrailingZeros(board)
		out = append(out, sq)
		board &^= 1 << sq
	}
	return out
}

// RemainingCounts returns (blacks, redMen, blackKings, redKings).
func (p Position) RemainingCounts() (int, int, int, int) {
	bm := popcount(p.Black &^ p.Kings)
	rm := popcount(p.Red &^ p.Kings)
	bk := popcount(p.Black & p.Kings)
	rk := popcount(p.Red & p.Kings)
	return bm, rm, bk, rk
}

// ToMatrix returns an 8×8 grid.
// Values: 0=empty, 1=red man, 2=red king, -1=black man, -2=black king.
// Non-playable (light) squares are 0.
func (p Position) ToMatrix() [8][8]int8 {
	var m [8][8]int8
	for sq := 0; sq < 32; sq++ {
		row, col := squareToRowCol(sq)
		switch {
		case p.IsRed(sq) && p.IsKing(sq):
			m[row][col] = 2
		case p.IsRed(sq):
			m[row][col] = 1
		case p.IsBlack(sq) && p.IsKing(sq):
			m[row][col] = -2
		case p.IsBlack(sq):
			m[row][col] = -1
		}
	}
	return m
}

// squareToRowCol converts an internal square index to 8×8 board coordinates.
// The returned row is visual (0=top=Black home, 7=bottom=Red home).
func squareToRowCol(sq int) (row, col int) {
	bottomRow := sq / 4 // 0=Red home, 7=Black home
	p := sq % 4
	row = 7 - bottomRow // flip so row 0 = top = Black home
	if bottomRow%2 == 0 { // even bottom-row: even board columns
		col = 2 * p
	} else { // odd bottom-row: odd board columns
		col = 2*p + 1
	}
	return
}

// ACFToInternal converts ACF square number (1-32) to internal index (0-31).
func ACFToInternal(acf int) int { return acf - 1 }

// InternalToACF converts internal index (0-31) to ACF number (1-32).
func InternalToACF(sq int) int { return sq + 1 }

// --- bit utilities ---

func bits32TrailingZeros(x uint32) int {
	if x == 0 {
		return 32
	}
	n := 0
	for x&1 == 0 {
		x >>= 1
		n++
	}
	return n
}

func popcount(x uint32) int {
	n := 0
	for x != 0 {
		n += int(x & 1)
		x >>= 1
	}
	return n
}
