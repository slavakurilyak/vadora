package rest

import (
	"encoding/json"
	"slices"
	"testing"
)

// keysOf returns the JSON field names that a value serializes with.
func keysOf(t *testing.T, value any) []string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// A client sends facts it knows and never a price, a total, a network class, or a
// status, because the server computes and decides all of them (012:92).
func TestNoRequestCarriesAPriceATotalANetworkClassOrAStatus(t *testing.T) {
	forbidden := []string{"price", "total", "total_usd_minor", "amount", "amount_base_units", "network_class", "status", "payment_status", "recipient", "test"}
	requests := map[string]any{
		"checkout session": CreateCheckoutSessionRequest{ListingID: "listing"},
		"shipping options": ShippingOptionsRequest{ListingID: "listing", Quantity: 1},
		"quote":            QuoteRequest{SessionID: "session", ListingID: "listing", Quantity: 1},
	}
	for name, request := range requests {
		for _, key := range keysOf(t, request) {
			if slices.Contains(forbidden, key) {
				t.Errorf("the %s request carries the server decided field %q", name, key)
			}
		}
	}
}

// The checkout session token reaches only the caller that authenticated with a
// buyer credential, so an empty token never appears on the wire.
func TestAnEmptySessionTokenIsOmitted(t *testing.T) {
	if keys := keysOf(t, CheckoutSession{ID: "session", StoreID: "store"}); slices.Contains(keys, "token") {
		t.Fatalf("an empty token must be omitted, got %v", keys)
	}
	if keys := keysOf(t, CheckoutSession{ID: "session", Token: "secret"}); !slices.Contains(keys, "token") {
		t.Fatalf("a token that the server issues must be present, got %v", keys)
	}
}

// A buyer's order carries no delivery detail and no email (001:472), and a quote
// carries no fee payer or quote binding.
func TestNoResponseLeaksDeliveryDetailsOrInternalBindings(t *testing.T) {
	leaks := []string{"delivery", "email", "buyer_email", "fee_payer", "memo", "credential"}
	responses := map[string]any{"order": Order{}, "quote": Quote{}, "receipt": Receipt{}, "offer": Offer{}}
	for name, response := range responses {
		for _, key := range keysOf(t, response) {
			if slices.Contains(leaks, key) {
				t.Errorf("the %s response carries the internal field %q", name, key)
			}
		}
	}
}

// The three version 2 headers and the idempotency header are fixed names that
// every client and the server share.
func TestTheHeaderNamesAreTheVersionTwoNames(t *testing.T) {
	names := map[string]string{
		HeaderPaymentRequired:  "PAYMENT-REQUIRED",
		HeaderPaymentSignature: "PAYMENT-SIGNATURE",
		HeaderPaymentResponse:  "PAYMENT-RESPONSE",
		HeaderIdempotencyKey:   "Idempotency-Key",
	}
	for got, want := range names {
		if got != want {
			t.Errorf("the header is %q, want %q", got, want)
		}
	}
}
