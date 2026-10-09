package client

import (
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"strconv"
	"strings"
)

// newKey returns a fresh high entropy idempotency key for one mutating call.
func newKey() string {
	buffer := make([]byte, idempotencyKeyBytes)
	if _, err := rand.Read(buffer); err != nil {
		// Without randomness there is no safe key, and a mutating call with an
		// empty key could be applied twice.
		panic("client: an idempotency key could not be generated")
	}
	key := hex.EncodeToString(buffer)
	if len(key) != idempotencyKeyLength {
		panic("client: a generated idempotency key has the wrong length")
	}
	return key
}

// idempotencyKeyBytes is how much entropy each key carries.
const idempotencyKeyBytes = 16

// MaxIntegerText bounds the digits an integer renders to.
const MaxIntegerText = 24

// idempotencyKeyLength is the number of hex characters in a key.
const idempotencyKeyLength = 32

// listPath builds a paginated collection path.
func listPath(base string, limit int, cursor string) string {
	if limit < 1 {
		panic("client: a collection path was built with no page size")
	}
	path := base + "?limit=" + itoa(int64(limit))
	if cursor != "" {
		path += "&cursor=" + queryEscape(cursor)
	}
	if !strings.Contains(path, "limit=") {
		panic("client: a collection path lost its page size")
	}
	return path
}

// itoa renders an integer as its decimal text, which is how every amount and
// identifier travels on the wire.
func itoa(value int64) string {
	text := strconv.FormatInt(value, 10)
	if text == "" {
		panic("client: an integer rendered as no text")
	}
	if len(text) > MaxIntegerText {
		panic("client: an integer rendered more digits than its bound")
	}
	return text
}

// queryEscape escapes a query parameter value.
func queryEscape(value string) string {
	escaped := url.QueryEscape(value)
	if strings.ContainsAny(escaped, " \t\n") {
		panic("client: an escaped query value holds whitespace")
	}
	if len(escaped) < len(value) {
		panic("client: escaping shortened a query value")
	}
	return escaped
}

// pathEscape escapes one path segment, so an identifier can never escape its
// position and reach another resource.
func pathEscape(value string) string {
	escaped := url.PathEscape(value)
	if strings.Contains(escaped, "/") {
		panic("client: an escaped identifier still carries a path separator")
	}
	if strings.Contains(escaped, "..") {
		panic("client: an escaped identifier still traverses upwards")
	}
	return escaped
}
