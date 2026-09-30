package digest

import (
	"crypto/sha256"
	"encoding/hex"
)

func Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func String(s string) string {
	return Hex([]byte(s))
}
