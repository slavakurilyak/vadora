package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vadora/vadora/api/types/errs"
	"github.com/vadora/vadora/api/types/rest"
)

const challengeDocument = `{"x402Version":2,"accepts":[{"scheme":"exact","network":"solana:devnet","asset":"MINT","amount":"5500000","payTo":"RECIPIENT","maxTimeoutSeconds":300,"extra":{"memo":"quo_1","feePayer":"FEE"}}],"extensions":{}}`

func serverOf(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	api, err := New(server.URL, "buyer-credential-for-tests")
	if err != nil {
		t.Fatal(err)
	}
	return api
}

// A purchase without a payment answers HTTP 402 with the challenge in
// PAYMENT-REQUIRED, and the SDK hands it over as a challenge, never as an error.
func TestAPurchaseWithoutAPaymentReturnsTheDecodedChallenge(t *testing.T) {
	api := serverOf(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PAYMENT-SIGNATURE") != "" {
			t.Error("a challenge request carries no payment")
		}
		if r.Header.Get("Idempotency-Key") != "key-1" {
			t.Errorf("Idempotency-Key = %q", r.Header.Get("Idempotency-Key"))
		}
		w.Header().Set("PAYMENT-REQUIRED", base64.StdEncoding.EncodeToString([]byte(challengeDocument)))
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(challengeDocument))
	})
	result, err := api.Purchase(context.Background(), "quo_1", "key-1", "")
	if err != nil {
		t.Fatalf("the answer is a challenge, got %+v (%v)", result, err)
	}
	if result.Challenge == nil {
		t.Fatalf("the answer is a challenge, got %+v (%v)", result, err)
	}
	if result.Order != nil {
		t.Fatalf("the answer is a challenge, got %+v (%v)", result, err)
	}
	option := result.Challenge.Accepts[0]
	if option.PayTo != "RECIPIENT" {
		t.Fatalf("the option is decoded, got %+v", option)
	}
	if option.Amount != "5500000" {
		t.Fatalf("the option is decoded, got %+v", option)
	}
	if option.Extra["memo"] != "quo_1" {
		t.Fatalf("the option is decoded, got %+v", option)
	}
	if len(result.Challenge.Raw) == 0 {
		t.Fatal("the raw document stays available for the x402 client")
	}
}

func TestASettledPurchaseReturnsTheOrderAndTheOutcomeHeader(t *testing.T) {
	api := serverOf(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PAYMENT-SIGNATURE") != "SIGNED" {
			t.Errorf("PAYMENT-SIGNATURE = %q", r.Header.Get("PAYMENT-SIGNATURE"))
		}
		w.Header().Set("PAYMENT-RESPONSE", "OUTCOME")
		_ = json.NewEncoder(w).Encode(rest.Order{ID: "ord_1", PaymentStatus: "paid"})
	})
	result, err := api.Purchase(context.Background(), "quo_1", "key-1", "SIGNED")
	if err != nil {
		t.Fatalf("the answer is the order with the outcome header, got %+v (%v)", result, err)
	}
	if result.Order == nil {
		t.Fatalf("the answer is the order with the outcome header, got %+v (%v)", result, err)
	}
	if result.Order.ID != "ord_1" {
		t.Fatalf("the answer is the order with the outcome header, got %+v (%v)", result, err)
	}
	if result.Response != "OUTCOME" {
		t.Fatalf("the answer is the order with the outcome header, got %+v (%v)", result, err)
	}
}

// Every answer other than the challenge and the order is a catalog error, and a
// pending payment tells the client to wait, never to pay again.
func TestAPendingPaymentIsACatalogErrorThatNeverInvitesASecondPayment(t *testing.T) {
	api := serverOf(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":"PAYMENT_PENDING","reason":"the original payment is being confirmed; do not pay again","action":"wait","retries":false}}`))
	})
	_, err := api.Purchase(context.Background(), "quo_1", "key-1", "SIGNED")
	catalog, ok := errs.As(err)
	if !ok {
		t.Fatalf("got %v", err)
	}
	if catalog.Code != errs.PaymentPending {
		t.Fatalf("got %v", err)
	}
	if catalog.Action != errs.ActionWait {
		t.Fatalf("got %v", err)
	}
}

func TestAChallengeThatIsNotX402VersionTwoIsRefused(t *testing.T) {
	for name, header := range map[string]string{
		"not base64":     "!!!",
		"not a document": base64.StdEncoding.EncodeToString([]byte("nope")),
		"version one":    base64.StdEncoding.EncodeToString([]byte(`{"x402Version":1,"accepts":[{}]}`)),
		"no option":      base64.StdEncoding.EncodeToString([]byte(`{"x402Version":2,"accepts":[]}`)),
		"missing header": "",
	} {
		api := serverOf(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("PAYMENT-REQUIRED", header)
			w.WriteHeader(http.StatusPaymentRequired)
		})
		if result, err := api.Purchase(context.Background(), "quo_1", "key-1", ""); err == nil {
			t.Errorf("%s: a malformed challenge was accepted: %+v", name, result)
		}
	}
}

// The SDK sends only the facts that the buyer knows, and it checks what the server
// would refuse before the call leaves the machine.
func TestTheSDKRefusesWhatTheServerWouldRefuse(t *testing.T) {
	api := serverOf(t, func(w http.ResponseWriter, r *http.Request) { t.Error("no request may leave the machine") })
	ctx := context.Background()
	if _, err := api.Purchase(ctx, "quo/1", "key", ""); err == nil {
		t.Error("a quote ID that escapes its path segment was sent")
	}
	if _, err := api.Purchase(ctx, "quo_1", "", ""); err == nil {
		t.Error("a purchase without an idempotency key was sent")
	}
	if _, err := api.Purchase(ctx, "quo_1", "key with space", ""); err == nil {
		t.Error("an idempotency key outside printable ASCII was sent")
	}
	if _, err := api.CreateQuote(ctx, "", rest.QuoteRequest{}); err == nil {
		t.Error("a quote without an idempotency key was sent")
	}
	if _, err := api.GetOrder(ctx, "../orders"); err == nil {
		t.Error("an order ID that traverses upwards was sent")
	}
	if _, err := api.SearchProducts(ctx, "kettle", Filter{MinUSDCents: -1}); err == nil {
		t.Error("a negative price bound was sent")
	}
	if _, err := api.ListOrders(ctx, "", "", MaxPageSize+1); err == nil {
		t.Error("a page size above the bound was sent")
	}
}

func TestSearchSendsTheFiltersAsQueryParameters(t *testing.T) {
	api := serverOf(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		want := map[string]string{"q": "wool scarf", "store_id": "sto_1", "merchant": "shop.example", "category": "Apparel",
			"destination": "US", "min_usd_cents": "1000", "max_usd_cents": "5000", "limit": "20", "cursor": "next"}
		for name, value := range want {
			if query.Get(name) != value {
				t.Errorf("%s = %q, want %q", name, query.Get(name), value)
			}
		}
		_ = json.NewEncoder(w).Encode(rest.ProductPage{Products: []rest.Product{{ID: "lst_1"}}})
	})
	page, err := api.SearchProducts(context.Background(), "wool scarf", Filter{StoreID: "sto_1", Merchant: "shop.example",
		Category: "Apparel", Destination: "US", MinUSDCents: 1000, MaxUSDCents: 5000, Limit: 20, Cursor: "next"})
	if err != nil {
		t.Fatalf("got %+v (%v)", page, err)
	}
	if len(page.Products) != 1 {
		t.Fatalf("got %+v (%v)", page, err)
	}
}

func TestTheBuyerSignInRequestsACodeThenReturnsTheCredential(t *testing.T) {
	api := serverOf(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/sign-in/codes":
			var body rest.SignInCodeRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Email != "buyer@example.test" {
				t.Errorf("the code request is %+v", body)
			} else if !body.Buyer {
				t.Errorf("the code request is %+v", body)
			}
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(rest.SignInCode{RequestID: "sic_1", ExpiresAt: "later"})
		case "/v1/auth/sign-in":
			var body rest.SignInRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.RequestID != "sic_1" {
				t.Errorf("the sign in is %+v", body)
			} else if body.Code != "123456" {
				t.Errorf("the sign in is %+v", body)
			} else if !body.Buyer {
				t.Errorf("the sign in is %+v", body)
			}
			_ = json.NewEncoder(w).Encode(rest.BuyerCredential{Credential: "buyer-credential-secret", ID: "bcr_1"})
		}
	})
	code, err := api.RequestSignInCode(context.Background(), "buyer@example.test", true)
	if err != nil {
		t.Fatalf("got %+v (%v)", code, err)
	}
	if code.RequestID != "sic_1" {
		t.Fatalf("got %+v (%v)", code, err)
	}
	issued, err := api.CompleteBuyerSignIn(context.Background(), code.RequestID, "buyer@example.test", "123456")
	if err != nil {
		t.Fatalf("got %+v (%v)", issued, err)
	}
	if issued.Credential == "" {
		t.Fatalf("got %+v (%v)", issued, err)
	}
}
