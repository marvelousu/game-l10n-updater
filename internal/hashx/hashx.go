// Package hashx provides the deterministic content hashes used in state.
package hashx

import (
	"crypto/sha256"
	"encoding/hex"
)

// Text returns the lowercase hex SHA-256 of the UTF-8 text.
func Text(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// Bytes returns the lowercase hex SHA-256 of data.
func Bytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
