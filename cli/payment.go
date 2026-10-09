package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	x402 "github.com/x402-foundation/x402/go/v2"
	"github.com/x402-foundation/x402/go/v2/extensions/paymentidentifier"
	"github.com/x402-foundation/x402/go/v2/mechanisms/svm"
	svmclient "github.com/x402-foundation/x402/go/v2/mechanisms/svm/exact/client"
	"github.com/x402-foundation/x402/go/v2/types"

	"github.com/vadora/vadora/api/types/errs"
	"github.com/vadora/vadora/sdk"
)

// createPayment builds the PAYMENT-SIGNATURE header for one x402 challenge with the
// reference x402 client for Solana, signed by the buyer's own signing interface. It
// signs only after the caller checked the quote against the spending policy, and
// it signs the one option that the challenge lists, for exactly the amount, the
// recipient, and the memo that the challenge names (005:56).
//
// Verified on October 9, 2026 against github.com/x402-foundation/x402/go/v2
// v2.28.0: the exact scheme for Solana builds a compute limit, a compute price, one
// checked transfer, and the memo, and the buyer signs second to the fee payer.
func createPayment(ctx context.Context, signer Signer, rpcURL string, challenge *client.Challenge, identifier string) (string, error) {
	var required types.PaymentRequired
	if err := json.Unmarshal(challenge.Raw, &required); err != nil {
		return "", errs.New(errs.StoreUnavailable, "the payment challenge could not be read")
	}
	if len(required.Accepts) != 1 {
		return "", errs.New(errs.PaymentMethodUnsupported, "the challenge must offer exactly one payment option")
	}
	var config *svm.ClientConfig
	if rpcURL != "" {
		config = &svm.ClientConfig{RPCURL: rpcURL}
	}
	payer := x402.Newx402Client(x402.WithSpendControlsDisabled()).
		Register("solana:*", svmclient.NewExactSvmScheme(signer, config))
	payload, err := payer.CreatePaymentPayload(ctx, required.Accepts[0], required.Resource, required.Extensions)
	if err != nil {
		return "", errs.New(errs.StoreUnavailable, "the payment could not be prepared: "+firstLine(err.Error()))
	}
	if err := paymentidentifier.AppendPaymentIdentifierToExtensions(payload.Extensions, identifier); err != nil {
		return "", errs.New(errs.StoreUnavailable, "the payment identifier could not be added")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", errs.New(errs.StoreUnavailable, "the payment could not be encoded")
	}
	return base64.StdEncoding.EncodeToString(encoded), nil
}

// matchOffer checks that the challenge's one option is exactly the offer of the
// quote that the buyer approved. A challenge that differs in the network, the
// asset, the recipient, the amount, or the memo is refused before anything is
// signed, so the approval and the payment cannot diverge.
func matchOffer(challenge *client.Challenge, quoteID, network, asset, recipient string, amountBaseUnits int64) error {
	if len(challenge.Accepts) != 1 {
		return errs.New(errs.PaymentMethodUnsupported, "the challenge must offer exactly one payment option")
	}
	option := challenge.Accepts[0]
	memo, _ := option.Extra["memo"].(string)
	mismatches := []struct {
		field string
		same  bool
	}{
		{"scheme", option.Scheme == "exact"},
		{"network", option.Network == network},
		{"asset", option.Asset == asset},
		{"recipient", option.PayTo == recipient},
		{"amount", option.Amount == fmt.Sprint(amountBaseUnits)},
		{"quote binding", memo == quoteID},
	}
	for _, check := range mismatches {
		if !check.same {
			return errs.New(errs.PaymentRejected, "the payment challenge differs from the quote in its "+check.field+": nothing was signed")
		}
	}
	return nil
}
