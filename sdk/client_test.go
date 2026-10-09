package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vadora/vadora/api/types/errs"
)

// newTestClient points a client at a test server and returns it with the server.
func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := New(server.URL, "buyer-credential-for-tests")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if client.BaseURL != strings.TrimRight(server.URL, "/") {
		t.Fatalf("base = %q", client.BaseURL)
	}
	return client, server
}

// TestNewRefusesAnOriginThatIsNotHTTPS proves the SDK cannot be pointed at a
// plain HTTP or host less origin, so a credential never leaves over an
// unencrypted connection.
func TestNewRefusesAnOriginThatIsNotHTTPS(t *testing.T) {
	for _, origin := range []string{"", "   ", "http://api.example", "not a url", "/relative"} {
		if _, err := New(origin, "credential"); err == nil {
			t.Fatalf("New accepted the origin %q", origin)
		}
	}
	if _, err := New("https://api.vadora.xyz/", ""); err != nil {
		t.Fatalf("New rejected a valid origin: %v", err)
	}
}

// TestAnErrorResponseBecomesTheSharedCatalogError proves a REST failure reaches
// the CLI as the same code and client action the catalog defines, so the SDK
// never reports an error of its own.
func TestAnErrorResponseBecomesTheSharedCatalogError(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGone)
		_, _ = w.Write([]byte(`{"error":{"code":"QUOTE_EXPIRED","reason":"quote expired","action":"request_new_quote","details":{"cause":"store_paused"}}}`))
	})
	err := client.do(context.Background(), request{method: http.MethodGet, path: "/v1/quotes/q1"}, nil)
	catalogErr, known := errs.As(err)
	if !known {
		t.Fatalf("the SDK returned a non catalog error: %v", err)
	}
	if catalogErr.Code != errs.QuoteExpired {
		t.Fatalf("code = %q", catalogErr.Code)
	}
	if catalogErr.Action != errs.ActionNewQuote {
		t.Fatalf("action = %q", catalogErr.Action)
	}
	if catalogErr.Details["cause"] != "store_paused" {
		t.Fatalf("details = %v", catalogErr.Details)
	}
}

// TestACodeOutsideTheCatalogIsRefused proves the SDK trusts only the catalog, so
// a hostile or broken server cannot invent an error code the CLI would obey.
func TestACodeOutsideTheCatalogIsRefused(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte(`{"error":{"code":"PAY_NOW","reason":"invented","action":"pay"}}`))
	})
	err := client.do(context.Background(), request{method: http.MethodGet, path: "/v1/stores"}, nil)
	catalogErr, known := errs.As(err)
	if !known {
		t.Fatalf("the SDK returned a non catalog error: %v", err)
	}
	if catalogErr.Code == errs.Code("PAY_NOW") {
		t.Fatal("the SDK accepted an invented code")
	}
}

// TestTheCredentialIsSentAsABearerValue proves buyer calls carry the credential
// and no other authentication header.
func TestTheCredentialIsSentAsABearerValue(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer buyer-credential-for-tests" {
			t.Errorf("Authorization = %q", got)
		}
		if r.Header.Get("Cookie") != "" {
			t.Error("the SDK sent a cookie")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})
	if err := client.do(context.Background(), request{method: http.MethodGet, path: "/v1/orders"}, nil); err != nil {
		t.Fatalf("the call failed: %v", err)
	}
}

// TestIdempotencyKeyIsSentForAMutatingCall proves a repeated quote or purchase
// request carries the key, so the server can return the original result and
// repeat no effect.
func TestIdempotencyKeyIsSentForAMutatingCall(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Idempotency-Key"); got != "key-1" {
			t.Errorf("Idempotency-Key = %q", got)
		}
		_, _ = w.Write([]byte(`{}`))
	})
	err := client.do(context.Background(), request{
		method: http.MethodPost, path: "/v1/quotes",
		idempotencyKey: "key-1",
		body:           map[string]any{"listing_id": "l1"},
	}, nil)
	if err != nil {
		t.Fatalf("the call failed: %v", err)
	}
}

// TestANilContextIsRefused proves the SDK never sends a request without a
// deadline the caller controls.
func TestANilContextIsRefused(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {})
	// A nil context is exactly what this check refuses.
	err := client.do(nil, request{method: http.MethodGet, path: "/v1/stores"}, nil)
	if err == nil {
		t.Fatal("the SDK sent a call with no context")
	}
	if _, known := errs.As(err); !known {
		t.Fatalf("the error is not a catalog error: %v", err)
	}
}

// TestAnUnreachableServerIsReportedAsUnavailable proves a transport failure is
// a catalog error rather than a leaked transport type.
func TestAnUnreachableServerIsReportedAsUnavailable(t *testing.T) {
	client, server := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {})
	server.Close()
	err := client.do(context.Background(), request{method: http.MethodGet, path: "/v1/stores"}, nil)
	catalogErr, known := errs.As(err)
	if !known {
		t.Fatalf("the SDK returned a non catalog error: %v", err)
	}
	if catalogErr.Code != errs.StoreUnavailable {
		t.Fatalf("code = %q", catalogErr.Code)
	}
}

// TestAnAnonymousClientSendsNoCredential proves public discovery needs no
// credential, so the CLI can browse the catalog before it signs in.
func TestAnAnonymousClientSendsNoCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, present := r.Header["Authorization"]; present {
			t.Error("an anonymous client sent an Authorization header")
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	client, err := New(server.URL, "")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := client.do(context.Background(), request{method: http.MethodGet, path: "/v1/stores"}, nil); err != nil {
		t.Fatalf("the call failed: %v", err)
	}
}
