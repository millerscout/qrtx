package tracker

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// NewNonce returns random hex for one create; retries reuse it.
func NewNonce() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// NewClientRef is the idempotency key of one create.
func NewClientRef(repo, change, title, nonce string) string {
	sum := sha256.Sum256([]byte(repo + "\x00" + change + "\x00" + title + "\x00" + nonce))
	return hex.EncodeToString(sum[:])[:12]
}
