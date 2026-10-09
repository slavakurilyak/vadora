package cli

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gagliardetto/solana-go"

	"github.com/vadora/vadora/api/types/errs"
)

// Signer is the buyer's signing interface. A buyer agent's spending key lives
// behind it, in a local keypair file or in a remote wallet, and it never reaches
// the hosted backend, an agent host, a command argument, or any output
// (001:543, shared:118). The interface has the shape of the x402 client signer.
type Signer interface {
	// Address is the wallet that pays.
	Address() solana.PublicKey
	// SignTransaction adds the wallet's signature to a payment transaction and
	// leaves the fee payer's signature to the facilitator.
	SignTransaction(ctx context.Context, tx *solana.Transaction) error
}

// The settings that choose a signing interface. A command line argument never
// names a key.
const (
	settingKeypair = "VADORA_SIGNER_KEYPAIR"
	settingCommand = "VADORA_SIGNER_COMMAND"
	settingRPC     = "VADORA_SOLANA_RPC_URL"
)

// loadSigner builds the configured signing interface, or refuses with the one
// action that fixes it.
func loadSigner(env *Env) (Signer, error) {
	keypair := env.getenv(settingKeypair)
	command := env.getenv(settingCommand)
	if keypair != "" {
		if command != "" {
			return nil, errs.New(errs.InvalidRequest, "set one signing interface: "+settingKeypair+" or "+settingCommand+", not both")
		}
	}
	if keypair != "" {
		return newKeypairSigner(keypair)
	}
	if command != "" {
		return newCommandSigner(command)
	}
	return nil, errs.New(errs.InvalidRequest, "no signing interface is configured: set "+settingKeypair+" to a keypair file that only you can read, or "+settingCommand+" to a remote wallet bridge")
}

// keypairSigner signs with a locally controlled keypair file.
type keypairSigner struct{ key solana.PrivateKey }

// newKeypairSigner reads a Solana keypair file, either the JSON array that the
// Solana tools write or a base58 string. A file that another user can read is
// refused.
func newKeypairSigner(path string) (Signer, error) {
	if err := requireOwnerOnly(path); err != nil {
		return nil, errs.New(errs.InvalidRequest, err.Error())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, errs.New(errs.InvalidRequest, "the keypair file could not be read")
	}
	text := strings.TrimSpace(string(raw))
	var key solana.PrivateKey
	if strings.HasPrefix(text, "[") {
		key, err = solana.PrivateKeyFromSolanaKeygenFile(path)
	} else {
		key, err = solana.PrivateKeyFromBase58(text)
	}
	if err != nil {
		return nil, errs.New(errs.InvalidRequest, "the keypair file is not a Solana keypair")
	}
	if !key.IsValid() {
		return nil, errs.New(errs.InvalidRequest, "the keypair file is not a Solana keypair")
	}
	return keypairSigner{key: key}, nil
}

func (s keypairSigner) Address() solana.PublicKey { return s.key.PublicKey() }

func (s keypairSigner) SignTransaction(_ context.Context, tx *solana.Transaction) error {
	_, err := tx.PartialSign(func(key solana.PublicKey) *solana.PrivateKey {
		if key == s.key.PublicKey() {
			return &s.key
		}
		return nil
	})
	return err
}

// commandSigner bridges a remote or hardware wallet. The bridge is one program with
// two verbs: "address" prints the wallet address in base58, and "sign" reads the
// base64 transaction message on standard input and prints the base58 signature of
// the wallet over it. The CLI runs it without a shell and checks every signature.
type commandSigner struct {
	program string
	address solana.PublicKey
}

// signerTimeout bounds one call of the bridge program.
const signerTimeout = 2 * time.Minute

func newCommandSigner(program string) (Signer, error) {
	out, err := runBridge(context.Background(), program, "address", nil)
	if err != nil {
		return nil, err
	}
	address, err := solana.PublicKeyFromBase58(strings.TrimSpace(out))
	if err != nil {
		return nil, errs.New(errs.InvalidRequest, "the signer bridge printed no valid wallet address")
	}
	return commandSigner{program: program, address: address}, nil
}

func (s commandSigner) Address() solana.PublicKey { return s.address }

func (s commandSigner) SignTransaction(ctx context.Context, tx *solana.Transaction) error {
	message, err := tx.Message.MarshalBinary()
	if err != nil {
		return err
	}
	out, err := runBridge(ctx, s.program, "sign", []byte(base64.StdEncoding.EncodeToString(message)))
	if err != nil {
		return err
	}
	signature, err := solana.SignatureFromBase58(strings.TrimSpace(out))
	if err != nil {
		return errs.New(errs.InvalidRequest, "the signer bridge printed no valid signature")
	}
	if !ed25519.Verify(ed25519.PublicKey(s.address.Bytes()), message, signature[:]) {
		return errs.New(errs.InvalidRequest, "the signer bridge signed with another key or another message")
	}
	required := int(tx.Message.Header.NumRequiredSignatures)
	index := -1
	for i, key := range tx.Message.AccountKeys {
		if i >= required {
			break
		}
		if key == s.address {
			index = i
		}
	}
	if index < 0 {
		return errs.New(errs.InvalidRequest, "the transaction does not name the signer's wallet as a signer")
	}
	if len(tx.Signatures) != required {
		tx.Signatures = make([]solana.Signature, required)
	}
	tx.Signatures[index] = signature
	return nil
}

// runBridge runs the bridge program once. The program path is an executable, not a
// shell line, so nothing in the settings can inject a command.
func runBridge(ctx context.Context, program, verb string, stdin []byte) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, signerTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, program, verb)
	command.Stdin = bytes.NewReader(stdin)
	var stdout bytes.Buffer
	command.Stdout = &stdout
	if err := command.Run(); err != nil {
		return "", errs.New(errs.InvalidRequest, fmt.Sprintf("the signer bridge failed on %s", verb))
	}
	return stdout.String(), nil
}
