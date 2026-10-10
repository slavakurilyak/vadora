package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gagliardetto/solana-go"

	"github.com/vadora/vadora/api/types/domain"
	"github.com/vadora/vadora/api/types/rest"
	client "github.com/vadora/vadora/sdk"
)

// recordingSigner counts every request to sign. A test purchase that refuses a live network must never reach it.
type recordingSigner struct {
	address solana.PublicKey
	calls   atomic.Int64
}

func (r *recordingSigner) Address() solana.PublicKey { return r.address }

func (r *recordingSigner) SignTransaction(context.Context, *solana.Transaction) error {
	r.calls.Add(1)
	return nil
}

// buyerAPI is a fake of the buyer API that answers the quote stage with a quote on one network class and counts every
// attempt to pay for it.
func buyerAPI(t *testing.T, class string, payments *atomic.Int64) *httptest.Server {
	t.Helper()
	answer := func(w http.ResponseWriter, status int, body any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Error(err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/products/"):
			answer(w, http.StatusOK, rest.Product{ID: "lst_live", Title: "TEST kettle", Currency: "USD", Price: "5.00"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/checkout-sessions":
			answer(w, http.StatusCreated, rest.CheckoutSession{ID: "chk_1", Token: "checkout-token-0123456789", NetworkClass: class})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/shipping-options"):
			answer(w, http.StatusOK, rest.ShippingOptions{Options: []rest.ShippingOption{{Handle: "standard", Title: "Standard", Currency: "USD",
				SubtotalMinor: 500, ShippingMinor: 50, TotalMinor: 550}}})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/quotes":
			answer(w, http.StatusCreated, rest.Quote{ID: "quo_1", NetworkClass: class, TotalUSDMinor: 550, Offers: []rest.Offer{{ChainID: "solana", ProtocolID: "x402",
				Network: "solana:test", Asset: "TESTMINT", Recipient: "TESTMERCHANT", AmountBaseUnits: 5_500_000}}})
		default:
			payments.Add(1)
			http.Error(w, "a test purchase that refuses the quote never goes on", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func testDelivery() domain.DeliveryDetails {
	return domain.DeliveryDetails{Name: "TEST recipient", StreetLines: []string{"1 TEST street"}, City: "Test city", Region: "CA", PostalCode: "94000", Country: "US"}
}

// walk runs the test purchase stages against the fake API with a signer that records its calls.
func walk(t *testing.T, class string) (*testReport, error, *recordingSigner, int64) {
	t.Helper()
	var payments atomic.Int64
	server := buyerAPI(t, class, &payments)
	api, err := client.New(server.URL, testCredential)
	if err != nil {
		t.Fatal(err)
	}
	signer := &recordingSigner{address: newKey(t).PublicKey()}
	env := &Env{ConfigDir: t.TempDir(), HTTP: server.Client(), APIBase: server.URL}
	report := newReport("lst_live")
	policy := SpendingPolicy{MaxPerPurchaseUSDCents: 10_000, AllowedNetworks: []string{"solana:test"}, AllowedAssets: []string{"TESTMINT"}}
	failure := walkTestPurchase(env, api, signer, policy, report, "lst_live", 1, "", testDelivery())
	return report, failure, signer, payments.Load()
}

// A test purchase never pays on a live network. A quote on the main network, which is what a live store gives
// every shopper, stops the command at the quote stage before the wallet is asked to sign and before any payment is
// sent (001:516).
func TestATestPurchaseRefusesAMainnetQuoteBeforeItSigns(t *testing.T) {
	report, failure, signer, payments := walk(t, "mainnet")
	if failure == nil {
		t.Fatal("a test purchase of a live network quote fails")
	}
	if !strings.Contains(failure.Error(), "a test purchase never pays on a live network") {
		t.Fatalf("the refusal says why, got %v", failure)
	}
	if signer.calls.Load() != 0 {
		t.Fatalf("the wallet was asked to sign %d times", signer.calls.Load())
	}
	if payments != 0 {
		t.Fatalf("a payment was sent %d times", payments)
	}
	if !strings.Contains(report.human(), "quote") {
		t.Fatalf("the report names the stage that failed: %s", report.human())
	}
	for _, stage := range report.Stages {
		if stage.Name != "quote" {
			if stage.Status != "not reached" {
				t.Fatalf("no later stage was reached, got %+v", report.Stages)
			}
		}
	}
}

func TestATestPurchaseGoesOnToThePaymentStageOnlyForAQuoteOnTheTestNetwork(t *testing.T) {
	report, failure, _, payments := walk(t, "testnet")
	if failure != nil {
		if strings.Contains(failure.Error(), "never pays on a live network") {
			t.Fatalf("a quote on the test network is not refused as a live network quote: %v", failure)
		}
	}
	if payments == 0 {
		t.Fatalf("a quote on the test network goes on to ask for the payment, got %s", report.human())
	}
}
