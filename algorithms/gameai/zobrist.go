package gameai

import (
	"math/rand"
	"sync"

	"github.com/limaflucas/heuristic_checkers/internal/engine"
)

// Zobrist hashing for checkers positions.
// 32 squares × 4 piece types + 1 side-to-move key = 129 random uint64 values.

const (
	ztRedMan   = 0
	ztRedKing  = 1
	ztBlackMan = 2
	ztBlackKing = 3
)

var (
	zobristOnce  sync.Once
	zobristTable [32][4]uint64
	zobristBlack uint64 // XOR in when it is Black's turn
)

func initZobrist() {
	zobristOnce.Do(func() {
		r := rand.New(rand.NewSource(0xDEADBEEFCAFE))
		for sq := 0; sq < 32; sq++ {
			for t := 0; t < 4; t++ {
				zobristTable[sq][t] = r.Uint64()
			}
		}
		zobristBlack = r.Uint64()
	})
}

// ZobristHash computes the Zobrist hash of pos with color to move.
func ZobristHash(pos engine.Position, color engine.Color) uint64 {
	initZobrist()
	var h uint64
	for sq := 0; sq < 32; sq++ {
		switch {
		case pos.IsRed(sq) && pos.IsKing(sq):
			h ^= zobristTable[sq][ztRedKing]
		case pos.IsRed(sq):
			h ^= zobristTable[sq][ztRedMan]
		case pos.IsBlack(sq) && pos.IsKing(sq):
			h ^= zobristTable[sq][ztBlackKing]
		case pos.IsBlack(sq):
			h ^= zobristTable[sq][ztBlackMan]
		}
	}
	if color == engine.Black {
		h ^= zobristBlack
	}
	return h
}
