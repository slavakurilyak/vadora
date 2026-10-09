package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/vadora/vadora/api/types/errs"
)

// newIdempotencyKey returns a fresh high entropy idempotency key, so a repeated
// quote or purchase request returns the original result and repeats no effect
// even when a retry is not the same request object.
func newIdempotencyKey() string {
	buffer := make([]byte, idempotencyKeyBytes)
	if _, err := rand.Read(buffer); err != nil {
		// Without randomness there is no safe key, and a caller that continues
		// would risk a repeat being applied twice.
		panic("cli: an idempotency key could not be generated")
	}
	key := hex.EncodeToString(buffer)
	if len(key) != idempotencyKeyLength {
		panic("cli: a generated idempotency key has the wrong length")
	}
	return key
}

// idempotencyKeyBytes is how much entropy each key carries.
const idempotencyKeyBytes = 16

// idempotencyKeyLength is the number of hex characters in a key.
const idempotencyKeyLength = 32

// MaxKeyLength bounds a key this package will describe.
const MaxKeyLength = 128

// requireIdempotencyKey refuses to continue without a key, because a mutating
// call without one could be applied twice.
func requireIdempotencyKey(key string) error {
	if key == "" {
		return errs.New(errs.InvalidRequest, "the call needs an idempotency key")
	}
	if len(key) != idempotencyKeyLength {
		return errs.New(errs.InvalidRequest, "the idempotency key is malformed")
	}
	if _, err := hex.DecodeString(key); err != nil {
		return errs.New(errs.InvalidRequest, "the idempotency key is malformed")
	}
	if !generatedKeyIsWellFormed(key) {
		panic("cli: a validated key has the wrong shape")
	}
	if len(key) == 0 {
		panic("cli: a validated key is empty")
	}
	return nil
}

// generatedKeyIsWellFormed reports whether a generated key is usable.
func generatedKeyIsWellFormed(key string) bool {
	if len(key) != idempotencyKeyLength {
		return false
	}
	_, err := hex.DecodeString(key)
	if err != nil {
		return false
	}
	if len(key) != idempotencyKeyLength {
		panic("client: a key length check disagreed with the key format")
	}
	if len(key) != idempotencyKeyLength {
		return false
	}
	return true
}

// describeIdempotencyKey renders a key's shape without revealing it, for a
// diagnostic that must never print the key itself.
func describeIdempotencyKey(key string) string {
	if len(key) == 0 {
		return "no idempotency key"
	}
	if len(key) > MaxKeyLength {
		panic("cli: an unbounded key reached the diagnostic")
	}
	if len(key) == idempotencyKeyLength {
		if !generatedKeyIsWellFormed(key) {
			panic("cli: a key of the right length is not hex")
		}
	}
	return fmt.Sprintf("idempotency key of %d characters", len(key))
}
