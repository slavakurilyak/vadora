package client

import (
	"crypto/rand"
	"encoding/hex"
	"net/url"
)

// NewKey returns a fresh high entropy idempotency key for one mutating call. A
// caller that must survive a restart stores the key before it sends the call, so
// a retry repeats the call and never applies it twice.
func NewKey() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		// Without randomness there is no safe key, and a mutating call with a
		// guessable key could collide with another caller's.
		panic("client: an idempotency key could not be generated: " + err.Error())
	}
	return hex.EncodeToString(buffer)
}

// newKey is the package's own name for NewKey.
func newKey() string { return NewKey() }

// pathEscape escapes one path segment, so an identifier can never escape its
// position and reach another resource.
func pathEscape(value string) string { return url.PathEscape(value) }
