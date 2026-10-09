package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/vadora/vadora/api/types/errs"
)

// merchantServer records each call and answers it from a table.
type merchantServer struct {
	mu      sync.Mutex
	seen    []string
	headers []string
	bodies  []string
	answers map[string]string
}

func newMerchantClient(t *testing.T, answers map[string]string) (*merchantServer, *Client) {
	t.Helper()
	fake := &merchantServer{answers: answers}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		fake.mu.Lock()
		fake.seen = append(fake.seen, r.Method+" "+r.URL.Path)
		fake.headers = append(fake.headers, r.Header.Get("Authorization"))
		fake.bodies = append(fake.bodies, string(raw))
		answer, found := fake.answers[r.Method+" "+r.URL.Path]
		fake.mu.Unlock()
		if !found {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, answer)
	}))
	t.Cleanup(server.Close)
	client, err := New(server.URL, "session-access-token-0123456789")
	if err != nil {
		t.Fatal(err)
	}
	return fake, client
}

func (f *merchantServer) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.seen...)
}

const readinessAnswer = `{"store_id":"s1","items":[{"name":"connected","complete":true,"evidence":"connected on Oct 9"},{"name":"wallet_linked","complete":false}],"limitations":[],"agent_surface":[]}`

// Each merchant call carries the session's access token, uses the contract's path and
// method, and sends only what the contract names.
func TestMerchantCallsCarryTheSessionTokenToTheContractRoutes(t *testing.T) {
	fake, client := newMerchantClient(t, map[string]string{
		"GET /v1/merchant/stores":     `{"stores":[{"id":"s1","domain":"shop.example","name":"Shop","platform":"shopify","status":"connected","listings":4,"published_listings":1}]}`,
		"POST /v1/stores/detect":      `{"platform":"shopify","platform_domain":"shop.myshopify.com","canonical_domain":"shop.example","status":"shopify","limitations":[],"evidence":["public_ucp_mcp_domain"]}`,
		"POST /v1/stores/connect":     `{"store_id":"s1","canonical_domain":"shop.example","platform":"shopify","platform_domain":"shop.myshopify.com","install_url":"https://admin.shopify.com/install","authorization_url":"https://shop.myshopify.com/admin/oauth/authorize?x=1","requested_scopes":["read_products"],"workspace_url":"https://app.vadora.test/stores/s1"}`,
		"POST /v1/stores/s1/sync":     `{"store_id":"s1","created":1,"updated":2,"skipped":[{"title":"Ebook","reason":"digital"}]}`,
		"GET /v1/stores/s1/readiness": readinessAnswer,
	})
	ctx := t.Context()
	page, err := client.MyStores(ctx)
	if err != nil {
		t.Fatalf("got %+v (%v)", page, err)
	}
	if len(page.Stores) != 1 {
		t.Fatalf("got %+v (%v)", page, err)
	}
	if page.Stores[0].Published != 1 {
		t.Fatalf("got %+v (%v)", page, err)
	}
	found, err := client.DetectPlatform(ctx, " https://shop.example ")
	if err != nil {
		t.Fatalf("got %+v (%v)", found, err)
	}
	if found.Platform != "shopify" {
		t.Fatalf("got %+v (%v)", found, err)
	}
	connection, err := client.ConnectStore(ctx, "https://shop.example")
	if err != nil {
		t.Fatalf("got %+v (%v)", connection, err)
	}
	if connection.StoreID != "s1" {
		t.Fatalf("got %+v (%v)", connection, err)
	}
	synced, err := client.SyncStore(ctx, "s1")
	if err != nil {
		t.Fatalf("got %+v (%v)", synced, err)
	}
	if synced.Created != 1 {
		t.Fatalf("got %+v (%v)", synced, err)
	}
	if len(synced.Skipped) != 1 {
		t.Fatalf("got %+v (%v)", synced, err)
	}
	readiness, err := client.GetReadiness(ctx, "s1")
	if err != nil {
		t.Fatalf("got %+v (%v)", readiness, err)
	}
	if len(readiness.Items) != 2 {
		t.Fatalf("got %+v (%v)", readiness, err)
	}
	want := "GET /v1/merchant/stores,POST /v1/stores/detect,POST /v1/stores/connect,POST /v1/stores/s1/sync,GET /v1/stores/s1/readiness"
	if strings.Join(fake.calls(), ",") != want {
		t.Fatalf("the calls are %v", fake.calls())
	}
	for _, header := range fake.headers {
		if header != "Bearer session-access-token-0123456789" {
			t.Fatalf("every call carries the session token, got %q", header)
		}
	}
	if fake.bodies[1] != `{"store_url":"https://shop.example"}` {
		t.Fatalf("detection sends the trimmed store URL only, got %s", fake.bodies[1])
	}
	if fake.bodies[3] != "" {
		t.Fatalf("a refresh sends no body, got %s", fake.bodies[3])
	}
}

// A call without a session, and a store URL that the SDK will not send, never reach the server.
func TestAMerchantCallThatCannotSucceedNeverReachesTheServer(t *testing.T) {
	fake, signedIn := newMerchantClient(t, map[string]string{})
	anonymous := &Client{BaseURL: signedIn.BaseURL, HTTP: signedIn.HTTP}
	ctx := t.Context()
	var refused []error
	_, err := anonymous.MyStores(ctx)
	refused = append(refused, err)
	_, err = anonymous.DetectPlatform(ctx, "https://shop.example")
	refused = append(refused, err)
	_, err = anonymous.ConnectStore(ctx, "https://shop.example")
	refused = append(refused, err)
	_, err = anonymous.SyncStore(ctx, "s1")
	refused = append(refused, err)
	_, err = anonymous.GetReadiness(ctx, "s1")
	refused = append(refused, err)
	for _, err := range refused {
		failure, ok := errs.As(err)
		if !ok {
			t.Fatalf("got %v", err)
		}
		if failure.Code != errs.Unauthenticated {
			t.Fatalf("a call without a session is UNAUTHENTICATED, got %v", err)
		}
		if !strings.Contains(failure.Reason, "vadora login") {
			t.Fatalf("the refusal names the way forward, got %q", failure.Reason)
		}
	}
	for _, address := range []string{"", "http://shop.example", "ftp://shop.example", "https://", "https://user:pass@shop.example", "shop.example", "https://" + strings.Repeat("a", MaxStoreURLLength)} {
		if _, err := signedIn.DetectPlatform(ctx, address); err == nil {
			t.Errorf("%q was sent", address)
		}
		if _, err := signedIn.ConnectStore(ctx, address); err == nil {
			t.Errorf("%q was sent", address)
		}
	}
	for _, identifier := range []string{"", "a/b", "a b", "../x", strings.Repeat("a", 200)} {
		if _, err := signedIn.SyncStore(ctx, identifier); err == nil {
			t.Errorf("store %q was sent", identifier)
		}
		if _, err := signedIn.GetReadiness(ctx, identifier); err == nil {
			t.Errorf("store %q was sent", identifier)
		}
	}
	if len(fake.calls()) != 0 {
		t.Fatalf("the refused calls reached the server: %v", fake.calls())
	}
}

// The SDK trusts no server decision it did not read back: an answer that leaves the
// contract is refused whole, and a link that is not HTTPS is never handed on.
func TestAnAnswerOutsideTheContractIsRefused(t *testing.T) {
	connect := func(authorization, install string) string {
		encoded, _ := json.Marshal(map[string]any{"store_id": "s1", "canonical_domain": "shop.example", "platform": "shopify",
			"platform_domain": "shop.myshopify.com", "install_url": install, "authorization_url": authorization, "requested_scopes": []string{}, "workspace_url": "https://app.vadora.test/stores/s1"})
		return string(encoded)
	}
	cases := map[string]struct {
		route, answer string
		call          func(c *Client) error
	}{
		"an approval link on another scheme": {"POST /v1/stores/connect", connect("javascript:alert(1)", ""), func(c *Client) error {
			_, err := c.ConnectStore(t.Context(), "https://shop.example")
			return err
		}},
		"a plain http approval link": {"POST /v1/stores/connect", connect("http://shop.myshopify.com/admin", ""), func(c *Client) error {
			_, err := c.ConnectStore(t.Context(), "https://shop.example")
			return err
		}},
		"an install link with credentials": {"POST /v1/stores/connect", connect("https://shop.myshopify.com/admin", "https://user:pass@admin.shopify.com/x"), func(c *Client) error {
			_, err := c.ConnectStore(t.Context(), "https://shop.example")
			return err
		}},
		"a connection that names no store": {"POST /v1/stores/connect", `{"store_id":"","authorization_url":"https://x.example/a"}`, func(c *Client) error {
			_, err := c.ConnectStore(t.Context(), "https://shop.example")
			return err
		}},
		"a readiness of another store": {"GET /v1/stores/s2/readiness", readinessAnswer, func(c *Client) error {
			_, err := c.GetReadiness(t.Context(), "s2")
			return err
		}},
		"a completed item with no evidence": {"GET /v1/stores/s1/readiness", `{"store_id":"s1","items":[{"name":"connected","complete":true}]}`, func(c *Client) error {
			_, err := c.GetReadiness(t.Context(), "s1")
			return err
		}},
		"a checklist that is too long": {"GET /v1/stores/s1/readiness", `{"store_id":"s1","items":[{},{},{},{},{},{},{},{}]}`, func(c *Client) error {
			_, err := c.GetReadiness(t.Context(), "s1")
			return err
		}},
		"a refresh that names another store": {"POST /v1/stores/s1/sync", `{"store_id":"s2","created":0,"updated":0,"skipped":[]}`, func(c *Client) error {
			_, err := c.SyncStore(t.Context(), "s1")
			return err
		}},
		"a refresh with a negative count": {"POST /v1/stores/s1/sync", `{"store_id":"s1","created":-1,"updated":0,"skipped":[]}`, func(c *Client) error {
			_, err := c.SyncStore(t.Context(), "s1")
			return err
		}},
		"a detection that names no platform": {"POST /v1/stores/detect", `{"platform":"","status":""}`, func(c *Client) error {
			_, err := c.DetectPlatform(t.Context(), "https://shop.example")
			return err
		}},
		"a store list with a nameless store": {"GET /v1/merchant/stores", `{"stores":[{"id":""}]}`, func(c *Client) error {
			_, err := c.MyStores(t.Context())
			return err
		}},
	}
	for name, test := range cases {
		_, client := newMerchantClient(t, map[string]string{test.route: test.answer})
		failure, ok := errs.As(test.call(client))
		if !ok {
			t.Errorf("%s: the answer was accepted", name)
			continue
		}
		if failure.Code != errs.StoreUnavailable {
			t.Errorf("%s: got %v", name, failure)
		}
	}
}
