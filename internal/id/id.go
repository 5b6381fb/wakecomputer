package id

import (
	"crypto/rand"
	"encoding/binary"
	"math/big"
	"time"
)

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// New returns a ULID-compatible 26-character identifier:
// 48-bit millisecond timestamp + 80 bits of randomness, Crockford base32.
func New() string {
	var b [16]byte
	ms := uint64(time.Now().UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	if _, err := rand.Read(b[6:]); err != nil {
		// crypto/rand failure is not recoverable in practice; keep uniqueness
		binary.BigEndian.PutUint64(b[8:], uint64(time.Now().UnixNano()))
	}
	return encode(b)
}

func encode(b [16]byte) string {
	n := new(big.Int).SetBytes(b[:])
	mask := big.NewInt(31)
	var out [26]byte
	for i := 25; i >= 0; i-- {
		out[i] = crockford[new(big.Int).And(n, mask).Uint64()]
		n.Rsh(n, 5)
	}
	return string(out[:])
}
