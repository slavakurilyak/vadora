package cli

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gagliardetto/solana-go"
)

func newKey(t *testing.T) solana.PrivateKey {
	t.Helper()
	key, err := solana.NewRandomPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// paymentLikeTransaction builds a transaction whose fee payer is another account, as
// an x402 payment is, so the buyer signs second.
func paymentLikeTransaction(t *testing.T, payer solana.PublicKey) *solana.Transaction {
	t.Helper()
	feePayer := newKey(t).PublicKey()
	instruction := solana.NewInstruction(solana.MemoProgramID, solana.AccountMetaSlice{solana.Meta(payer).SIGNER()}, []byte("quo_1"))
	tx, err := solana.NewTransactionBuilder().AddInstruction(instruction).
		SetRecentBlockHash(solana.MustHashFromBase58("EtWTRABZaYq6iMfeYKouRu166VU2xqa1EtWTRABZaYq6")).SetFeePayer(feePayer).Build()
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func writeKeypair(t *testing.T, key solana.PrivateKey, mode os.FileMode, array bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wallet")
	content := []byte(key.String())
	if array {
		numbers := make([]int, len(key))
		for i, value := range key {
			numbers[i] = int(value)
		}
		content, _ = json.Marshal(numbers)
	}
	if err := os.WriteFile(path, content, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAKeypairFileSignsOnlyTheBuyersSlot(t *testing.T) {
	key := newKey(t)
	for _, array := range []bool{true, false} {
		signer, err := newKeypairSigner(writeKeypair(t, key, 0o600, array))
		if err != nil {
			t.Fatalf("a keypair file (array=%v) loads: %v", array, err)
		}
		if signer.Address() != key.PublicKey() {
			t.Fatal("the signer reports the wallet's address")
		}
		tx := paymentLikeTransaction(t, key.PublicKey())
		if err := signer.SignTransaction(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
		message, _ := tx.Message.MarshalBinary()
		if !ed25519.Verify(ed25519.PublicKey(key.PublicKey().Bytes()), message, tx.Signatures[1][:]) {
			t.Fatal("the buyer's signature is valid over the message")
		}
		if !tx.Signatures[0].IsZero() {
			t.Fatal("the fee payer's slot stays empty: the facilitator signs it")
		}
	}
}

func TestAKeypairFileThatAnotherUserCanReadIsRefused(t *testing.T) {
	for _, mode := range []os.FileMode{0o640, 0o644, 0o604} {
		if _, err := newKeypairSigner(writeKeypair(t, newKey(t), mode, true)); err == nil {
			t.Fatalf("a keypair file with mode %v was accepted", mode)
		}
	}
	notAKey := filepath.Join(t.TempDir(), "wallet")
	if err := os.WriteFile(notAKey, []byte("not a key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newKeypairSigner(notAKey); err == nil {
		t.Fatal("a file that is not a keypair was accepted")
	}
}

func TestTheSettingsChooseExactlyOneSigningInterface(t *testing.T) {
	env := func(settings map[string]string) *Env {
		return &Env{Getenv: func(name string) string { return settings[name] }}
	}
	if _, err := loadSigner(env(nil)); err == nil {
		t.Fatal("no signing interface is configured")
	}
	if _, err := loadSigner(env(map[string]string{settingKeypair: "a", settingCommand: "b"})); err == nil {
		t.Fatal("two signing interfaces are ambiguous")
	}
}

func TestARemoteWalletBridgeSignsAndTheCLIChecksTheSignature(t *testing.T) {
	key := newKey(t)
	t.Setenv("VADORA_TEST_BRIDGE_KEY", key.String())
	signer, err := newCommandSigner(os.Args[0])
	if err != nil {
		t.Fatalf("the bridge reports its wallet: %v", err)
	}
	if signer.Address() != key.PublicKey() {
		t.Fatal("the bridge's address is the wallet's address")
	}
	tx := paymentLikeTransaction(t, key.PublicKey())
	if err := signer.SignTransaction(context.Background(), tx); err != nil {
		t.Fatalf("the bridge signs: %v", err)
	}
	message, _ := tx.Message.MarshalBinary()
	if !ed25519.Verify(ed25519.PublicKey(key.PublicKey().Bytes()), message, tx.Signatures[1][:]) {
		t.Fatal("the bridge's signature is valid over the message")
	}

	// A bridge that signs with another key is refused before the signature is used.
	t.Setenv("VADORA_TEST_BRIDGE_WRONG_KEY", "1")
	other := paymentLikeTransaction(t, key.PublicKey())
	if err := signer.SignTransaction(context.Background(), other); err == nil {
		t.Fatal("a signature by another key was accepted")
	}
	if len(other.Signatures) > 1 {
		if !other.Signatures[1].IsZero() {
			t.Fatal("a refused signature must not be written into the transaction")
		}
	}
}

func TestABridgeThatIsNotAWalletIsRefused(t *testing.T) {
	if _, err := newCommandSigner("/nonexistent/vadora-wallet"); err == nil {
		t.Fatal("a missing bridge program was accepted")
	}
}
