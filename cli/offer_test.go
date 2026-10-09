package cli

import (
	"testing"

	"github.com/vadora/vadora/api/types/errs"
	"github.com/vadora/vadora/api/types/rest"
	"github.com/vadora/vadora/sdk"
)

func goodChallenge() *client.Challenge {
	return &client.Challenge{X402Version: 2, Accepts: []client.Requirement{{Scheme: "exact", Network: "solana:devnet", Asset: "MINT",
		Amount: "5500000", PayTo: "RECIPIENT", Extra: map[string]any{"memo": "quo_1", "feePayer": "FEE"}}}}
}

// The challenge must be the quote's own offer before anything is signed, so the
// buyer's approval and the payment cannot diverge.
func TestTheChallengeMustBeTheQuotesOwnOffer(t *testing.T) {
	if err := matchOffer(goodChallenge(), "quo_1", "solana:devnet", "MINT", "RECIPIENT", 5_500_000); err != nil {
		t.Fatalf("the quote's own offer matches: %v", err)
	}
	mutations := map[string]func(*client.Requirement){
		"scheme":        func(r *client.Requirement) { r.Scheme = "batch-settlement" },
		"network":       func(r *client.Requirement) { r.Network = "solana:mainnet" },
		"asset":         func(r *client.Requirement) { r.Asset = "OTHER" },
		"recipient":     func(r *client.Requirement) { r.PayTo = "ATTACKER" },
		"amount":        func(r *client.Requirement) { r.Amount = "5500001" },
		"quote binding": func(r *client.Requirement) { r.Extra = map[string]any{"memo": "quo_other"} },
	}
	for field, mutate := range mutations {
		challenge := goodChallenge()
		mutate(&challenge.Accepts[0])
		err := matchOffer(challenge, "quo_1", "solana:devnet", "MINT", "RECIPIENT", 5_500_000)
		catalog, ok := errs.As(err)
		if !ok {
			t.Fatalf("a challenge that differs in its %s must be refused, got %v", field, err)
		}
		if catalog.Code != errs.PaymentRejected {
			t.Fatalf("a challenge that differs in its %s must be refused, got %v", field, err)
		}
	}
	two := goodChallenge()
	two.Accepts = append(two.Accepts, two.Accepts[0])
	if err := matchOffer(two, "quo_1", "solana:devnet", "MINT", "RECIPIENT", 5_500_000); err == nil {
		t.Fatal("a challenge with two options must be refused")
	}
}

func TestShippingIsChosenByHandleOrIsTheOnlyOption(t *testing.T) {
	one := rest.ShippingOptions{Options: []rest.ShippingOption{{Handle: "standard", Title: "Standard", Currency: "USD", TotalMinor: 550}}}
	if handle, err := chooseShipping(one, ""); err != nil {
		t.Fatalf("the only method is chosen, got %q (%v)", handle, err)
	} else if handle != "standard" {
		t.Fatalf("the only method is chosen, got %q (%v)", handle, err)
	}
	two := rest.ShippingOptions{Options: append(one.Options, rest.ShippingOption{Handle: "express", Title: "Express", Currency: "USD", TotalMinor: 950})}
	if _, err := chooseShipping(two, ""); err == nil {
		t.Fatal("with two methods the buyer must choose one")
	}
	if handle, err := chooseShipping(two, "express"); err != nil {
		t.Fatalf("the named method is chosen, got %q (%v)", handle, err)
	} else if handle != "express" {
		t.Fatalf("the named method is chosen, got %q (%v)", handle, err)
	}
	if _, err := chooseShipping(two, "overnight"); err == nil {
		t.Fatal("a method that the store does not offer is refused")
	}
	if _, err := chooseShipping(rest.ShippingOptions{}, ""); err == nil {
		t.Fatal("no method means the destination is unsupported")
	}
}

func TestAnOrderIsFinishedOnlyWhenItWillNotChangeByItself(t *testing.T) {
	cases := []struct {
		payment, fulfillment string
		want                 bool
	}{
		{"paid", "sync_pending", false}, {"paid", "received", true}, {"paid", "fulfilled", true},
		{"paid", "needs_attention", true}, {"pending", "awaiting_payment", false}, {"unknown", "awaiting_payment", false},
		{"rejected", "cancelled", true}, {"expired", "cancelled", true},
	}
	for _, tc := range cases {
		if got := finished(rest.Order{PaymentStatus: tc.payment, FulfillmentStatus: tc.fulfillment}); got != tc.want {
			t.Errorf("payment %s and fulfillment %s: finished = %v, want %v", tc.payment, tc.fulfillment, got, tc.want)
		}
	}
}

func TestDollarsAreReadAsExactCents(t *testing.T) {
	for text, want := range map[string]int64{"12.50": 1250, "12.5": 1250, "12": 1200, "0.05": 5, "0": 0, "1000000": 100_000_000} {
		got, err := dollarsToCents(text)
		if err != nil {
			t.Errorf("%q = %d (%v), want %d", text, got, err, want)
		} else if got != want {
			t.Errorf("%q = %d (%v), want %d", text, got, err, want)
		}
	}
	for _, text := range []string{"", "-1", "1.234", "1.", ".5", "abc", "1,5", "99999999999999"} {
		if _, err := dollarsToCents(text); err == nil {
			t.Errorf("%q must be refused", text)
		}
	}
}
