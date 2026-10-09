package cli

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/gagliardetto/solana-go"
)

// TestMain lets the test binary stand in for a remote wallet bridge: when the
// environment names a bridge key, the binary answers the two bridge verbs and exits.
func TestMain(m *testing.M) {
	if encoded := os.Getenv("VADORA_TEST_BRIDGE_KEY"); encoded != "" {
		os.Exit(bridge(encoded, os.Args[1:]))
	}
	os.Exit(m.Run())
}

// bridge answers "address" and "sign" with an in memory key. The verb "bad" is
// handled by signing with another key, to prove that the CLI checks the bridge.
func bridge(encoded string, args []string) int {
	key, err := solana.PrivateKeyFromBase58(encoded)
	if err != nil {
		return 2
	}
	if len(args) != 1 {
		return 2
	}
	switch args[0] {
	case "address":
		fmt.Println(key.PublicKey())
		return 0
	case "sign":
		input, _ := io.ReadAll(os.Stdin)
		message, err := base64.StdEncoding.DecodeString(string(input))
		if err != nil {
			return 2
		}
		signing := key
		if os.Getenv("VADORA_TEST_BRIDGE_WRONG_KEY") != "" {
			_, other, _ := ed25519.GenerateKey(nil)
			signing = solana.PrivateKey(other)
		}
		signature, err := signing.Sign(message)
		if err != nil {
			return 2
		}
		fmt.Println(signature)
		return 0
	}
	return 2
}
