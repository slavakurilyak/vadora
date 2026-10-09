package cli_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vadora/vadora/api/types/errs"
	"github.com/vadora/vadora/cli"
)

// accountServer plays the part of Vadora's API for the CLI's account commands: the
// sign in code, the terminal OAuth flow with PKCE, the token endpoint that rotates the
// refresh token, the revocation, and the store routes that need the current access token.
type accountServer struct {
	mu       sync.Mutex
	issued   int
	access   string
	refresh  string
	calls    []string
	forms    []url.Values
	refusals map[string]bool
	skipped  string
}

func newAccountServer(t *testing.T) (*accountServer, *httptest.Server) {
	t.Helper()
	fake := &accountServer{refusals: map[string]bool{}, skipped: "Ebook"}
	server := httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(server.Close)
	return fake, server
}

func (a *accountServer) mint() map[string]any {
	a.issued++
	a.access = "access-token-" + strings.Repeat("a", a.issued) + "-0123456789"
	a.refresh = "refresh-token-" + strings.Repeat("r", a.issued) + "-0123456789"
	return map[string]any{"access_token": a.access, "token_type": "Bearer", "expires_in": 3600, "refresh_token": a.refresh, "scope": "merchant"}
}

func (a *accountServer) fail(w http.ResponseWriter, status int, code, reason string, details map[string]string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "reason": reason, "action": "authenticate", "details": details}})
}

func (a *accountServer) serve(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, r.Method+" "+r.URL.Path)
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/v1/auth/sign-in/codes":
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"request_id":"request-1","expires_at":"2026-10-10T12:10:00Z"}`)
	case "/v1/auth/sign-in":
		_, _ = io.WriteString(w, `{"credential":"buyer-credential-0123456789","id":"cred-1","expires_at":"2027-01-01T00:00:00Z"}`)
	case "/.well-known/oauth-protected-resource":
		_, _ = io.WriteString(w, `{"resource":"https://api.vadora.test/mcp"}`)
	case "/oauth/terminal/authorize":
		var sent struct {
			State string `json:"state"`
		}
		_ = json.NewDecoder(r.Body).Decode(&sent)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "grant-code-0123456789", "state": sent.State})
	case "/oauth/token":
		_ = r.ParseForm()
		a.forms = append(a.forms, r.PostForm)
		if r.PostForm.Get("grant_type") == "refresh_token" {
			if a.refusals["refresh"] {
				a.fail(w, http.StatusUnauthorized, "UNAUTHENTICATED", "the OAuth grant is invalid or expired", nil)
				return
			}
		}
		_ = json.NewEncoder(w).Encode(a.mint())
	case "/oauth/revoke":
		_ = r.ParseForm()
		a.forms = append(a.forms, r.PostForm)
	default:
		a.serveStores(w, r)
	}
}

func (a *accountServer) serveStores(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+a.access {
		a.fail(w, http.StatusUnauthorized, "UNAUTHENTICATED", "sign in on Vadora to continue", nil)
		return
	}
	switch r.Method + " " + r.URL.Path {
	case "GET /v1/merchant/stores":
		_, _ = io.WriteString(w, `{"stores":[{"id":"s1","domain":"shop.example","name":"Shop","platform":"shopify","status":"connected","test":true,"listings":4,"published_listings":1}]}`)
	case "POST /v1/stores/connect":
		if a.refusals["agreement"] {
			w.WriteHeader(http.StatusPreconditionRequired)
			_, _ = io.WriteString(w, `{"error":{"code":"CONFIRMATION_REQUIRED","reason":"accept the current agreement in the Vadora workspace","action":"confirm_in_workspace","details":{"workspace_url":"https://app.vadora.test/account/agreements","agreement":"merchant"}}}`)
			return
		}
		_, _ = io.WriteString(w, `{"store_id":"s1","canonical_domain":"shop.example","platform":"shopify","platform_domain":"shop.myshopify.com","authorization_url":"https://shop.myshopify.com/admin/oauth/authorize?state=x","requested_scopes":["read_products","write_orders"],"workspace_url":"https://app.vadora.test/stores/s1"}`)
	case "POST /v1/stores/s1/sync":
		encoded, _ := json.Marshal(map[string]any{"store_id": "s1", "created": 1, "updated": 2, "skipped": []map[string]string{{"title": a.skipped, "reason": "digital products are not sold"}}})
		_, _ = w.Write(encoded)
	case "GET /v1/stores/s1/readiness":
		_, _ = io.WriteString(w, `{"store_id":"s1","items":[{"name":"connected","complete":true,"evidence":"connected"},{"name":"wallet_linked","complete":false}],"limitations":["missing_stock_reservation"],"agent_surface":[]}`)
	default:
		http.NotFound(w, r)
	}
}

func (a *accountServer) saw(call string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	count := 0
	for _, seen := range a.calls {
		if seen == call {
			count++
		}
	}
	return count
}

// laptop is one machine: a configuration directory, a clock, and the CLI's streams.
type laptop struct {
	t      *testing.T
	dir    string
	server *httptest.Server
	moment time.Time
}

func newLaptop(t *testing.T, server *httptest.Server) *laptop {
	t.Helper()
	return &laptop{t: t, dir: t.TempDir(), server: server, moment: time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)}
}

func (l *laptop) run(input string, args ...string) (string, string, int) {
	l.t.Helper()
	var out, errOut bytes.Buffer
	env := &cli.Env{Out: &out, Err: &errOut, In: strings.NewReader(input), Version: "test", ConfigDir: l.dir, Now: func() time.Time { return l.moment },
		HTTP: l.server.Client()}
	code := cli.Execute(cli.NewClientTree(), append([]string{"--api", l.server.URL}, args...), env)
	return out.String(), errOut.String(), code
}

func (l *laptop) ok(input string, args ...string) string {
	l.t.Helper()
	out, errOut, code := l.run(input, args...)
	if code != 0 {
		l.t.Fatalf("vadora %s exited %d\nstdout: %s\nstderr: %s", strings.Join(args, " "), code, out, errOut)
	}
	return out
}

// credentials reads the private credential file.
func (l *laptop) credentials() map[string]string {
	l.t.Helper()
	path := filepath.Join(l.dir, "credentials.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		l.t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		l.t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		l.t.Fatalf("the credential file is readable by another user: %v", info.Mode())
	}
	stored := map[string]string{}
	if err := json.Unmarshal(raw, &stored); err != nil {
		l.t.Fatal(err)
	}
	return stored
}

// session decodes the stored account session.
func (l *laptop) session() map[string]any {
	l.t.Helper()
	encoded := l.credentials()[l.server.URL+"#account-session"]
	if encoded == "" {
		l.t.Fatal("no account session is stored")
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		l.t.Fatal(err)
	}
	var session map[string]any
	if err := json.Unmarshal(raw, &session); err != nil {
		l.t.Fatal(err)
	}
	return session
}

const signInInput = "owner@shop.example\n123456\n"

// vadora login signs in with the emailed code over OAuth with PKCE, stores the account
// session in the private credential file under its own key, and prints no token.
func TestLoginKeepsTheAccountSessionPrivatelyAndPrintsNoToken(t *testing.T) {
	fake, server := newAccountServer(t)
	l := newLaptop(t, server)
	out := l.ok(signInInput, "login")
	if !strings.Contains(out, "Signed in") {
		t.Fatalf("login printed %q", out)
	}
	if strings.Contains(out, fake.access) {
		t.Fatalf("login printed a token: %q", out)
	}
	if strings.Contains(out, fake.refresh) {
		t.Fatalf("login printed a token: %q", out)
	}
	session := l.session()
	if session["access_token"] != fake.access {
		t.Fatalf("the stored session holds the issued tokens, got %v", session)
	}
	if session["refresh_token"] != fake.refresh {
		t.Fatalf("the stored session holds the issued tokens, got %v", session)
	}
	if _, stored := l.credentials()[server.URL]; stored {
		t.Fatal("an account sign in stores no buyer credential")
	}
	if fake.saw("POST /oauth/terminal/authorize") != 1 {
		t.Fatalf("the sign in ran the terminal OAuth flow, calls %v", fake.calls)
	}
	if fake.saw("POST /oauth/token") != 1 {
		t.Fatalf("the sign in ran the terminal OAuth flow, calls %v", fake.calls)
	}
	asJSON := l.ok("owner@shop.example\n123456\n", "login", "--json")
	if strings.Contains(asJSON, fake.access) {
		t.Fatalf("login printed a token as JSON: %q", asJSON)
	}
	if strings.Contains(asJSON, fake.refresh) {
		t.Fatalf("login printed a token as JSON: %q", asJSON)
	}
}

// The store commands send the access token, renew it with the refresh token once it has
// ended, and store the rotated tokens before they run.
func TestTheStoreCommandsRenewTheSessionWhenItsAccessTokenEnds(t *testing.T) {
	fake, server := newAccountServer(t)
	l := newLaptop(t, server)
	l.ok(signInInput, "login")
	first := l.session()["refresh_token"]
	l.moment = l.moment.Add(30 * time.Minute)
	out := l.ok("", "stores", "list", "--mine")
	if !strings.Contains(out, "s1  Shop  shop.example  shopify  connected  4 listings, 1 published") {
		t.Fatalf("got %q", out)
	}
	if !strings.HasSuffix(strings.TrimSpace(out), "[TEST]") {
		t.Fatalf("a test store carries the test label, got %q", out)
	}
	if fake.saw("POST /oauth/token") != 1 {
		t.Fatal("a live access token needs no refresh")
	}
	l.moment = l.moment.Add(2 * time.Hour)
	l.ok("", "stores", "list", "--mine")
	if fake.saw("POST /oauth/token") != 2 {
		t.Fatal("an ended access token is renewed once")
	}
	renewed := l.session()
	if renewed["refresh_token"] == first {
		t.Fatal("the rotated refresh token is stored")
	}
	if renewed["refresh_token"] != fake.refresh {
		t.Fatalf("the stored session holds the newest tokens, got %v", renewed)
	}
	if renewed["access_token"] != fake.access {
		t.Fatalf("the stored session holds the newest tokens, got %v", renewed)
	}
	last := fake.forms[len(fake.forms)-1]
	if last.Get("grant_type") != "refresh_token" {
		t.Fatalf("the renewal sent the old refresh token, got %v", last)
	}
	if last.Get("refresh_token") != first {
		t.Fatalf("the renewal sent the old refresh token, got %v", last)
	}
}

// A refresh token that the server refuses ends the session on this machine, and the
// person is told to sign in again.
func TestARefusedRefreshTokenEndsTheSessionOnThisMachine(t *testing.T) {
	fake, server := newAccountServer(t)
	l := newLaptop(t, server)
	l.ok(signInInput, "login")
	fake.refusals["refresh"] = true
	l.moment = l.moment.Add(3 * time.Hour)
	_, errOut, code := l.run("", "stores", "list", "--mine")
	if code == 0 {
		t.Fatal("a command with an ended session succeeded")
	}
	if !strings.Contains(errOut, string(errs.Unauthenticated)) {
		t.Fatalf("the person is told to sign in again, got %q", errOut)
	}
	if !strings.Contains(errOut, "vadora login") {
		t.Fatalf("the person is told to sign in again, got %q", errOut)
	}
	if _, kept := l.credentials()[server.URL+"#account-session"]; kept {
		t.Fatal("the refused session is still stored")
	}
}

// A store command without a session tells the person to sign in and reaches no server.
func TestAStoreCommandWithoutASessionReachesNoServer(t *testing.T) {
	fake, server := newAccountServer(t)
	l := newLaptop(t, server)
	for _, args := range [][]string{{"stores", "list", "--mine"}, {"stores", "connect", "https://shop.example"}, {"stores", "sync", "s1"}, {"stores", "readiness", "s1"}} {
		_, errOut, code := l.run("", args...)
		if code == 0 {
			t.Fatalf("%v succeeded without a session", args)
		}
		if !strings.Contains(errOut, "vadora login") {
			t.Fatalf("%v: got %q", args, errOut)
		}
	}
	if len(fake.calls) != 0 {
		t.Fatalf("a command without a session reached the server: %v", fake.calls)
	}
}

// A buyer credential never works as an account session, and an account session never
// works as a buyer credential: they sit under separate keys.
func TestTheBuyerCredentialAndTheAccountSessionStaySeparate(t *testing.T) {
	fake, server := newAccountServer(t)
	l := newLaptop(t, server)
	l.ok(signInInput, "login", "--buyer")
	if l.credentials()[server.URL] != "buyer-credential-0123456789" {
		t.Fatalf("the buyer credential is stored for the origin, got %v", l.credentials())
	}
	_, errOut, code := l.run("", "stores", "list", "--mine")
	if code == 0 {
		t.Fatalf("a buyer credential is no account session, got %d %q", code, errOut)
	}
	if !strings.Contains(errOut, "vadora login") {
		t.Fatalf("a buyer credential is no account session, got %d %q", code, errOut)
	}
	l.ok(signInInput, "login")
	stored := l.credentials()
	if stored[server.URL] != "buyer-credential-0123456789" {
		t.Fatal("an account sign in left the buyer credential alone")
	}
	if stored[server.URL+"#account-session"] == "" {
		t.Fatal("the account session is stored")
	}
	if fake.saw("POST /v1/stores/connect") != 0 {
		t.Fatal("nothing was connected")
	}
}

// Logout ends the account session on the server with its refresh token, forgets it and
// the buyer credential on this machine, and says what is left to do.
func TestLogoutEndsTheSessionOnTheServerAndForgetsBothCredentials(t *testing.T) {
	fake, server := newAccountServer(t)
	l := newLaptop(t, server)
	l.ok(signInInput, "login", "--buyer")
	l.ok(signInInput, "login")
	refresh := fake.refresh
	out := l.ok("", "logout")
	if !strings.Contains(out, "ended on Vadora too") {
		t.Fatalf("got %q", out)
	}
	if !strings.Contains(out, "Revoke the buyer credential") {
		t.Fatalf("got %q", out)
	}
	revoke := fake.forms[len(fake.forms)-1]
	if revoke.Get("token") != refresh {
		t.Fatalf("the session ended on the server with its refresh token, got %v", revoke)
	}
	if revoke.Get("client_id") != "vadora-cli" {
		t.Fatalf("the session ended on the server with its refresh token, got %v", revoke)
	}
	if len(l.credentials()) != 0 {
		t.Fatalf("logout left %v", l.credentials())
	}
	_, errOut, code := l.run("", "logout")
	if code == 0 {
		t.Fatalf("a second logout has nothing to forget, got %d %q", code, errOut)
	}
	if !strings.Contains(errOut, "no credential is stored") {
		t.Fatalf("a second logout has nothing to forget, got %d %q", code, errOut)
	}
}

// vadora stores connect prints the platform, the access that Vadora asks for, and the
// approval link for the person's own browser. A missing merchant agreement names the page
// where it is accepted.
func TestStoresConnectPrintsTheApprovalLinkOrNamesTheAgreementPage(t *testing.T) {
	fake, server := newAccountServer(t)
	l := newLaptop(t, server)
	l.ok(signInInput, "login")
	out := l.ok("", "stores", "connect", "https://shop.example")
	for _, want := range []string{"Store shop.example is on shopify (shop.myshopify.com).", "read_products, write_orders", "approve the connection",
		"https://shop.myshopify.com/admin/oauth/authorize?state=x", "vadora stores readiness s1", "https://app.vadora.test/stores/s1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("connect must print %q, got %q", want, out)
		}
	}
	fake.refusals["agreement"] = true
	_, errOut, code := l.run("", "stores", "connect", "https://shop.example")
	if code == 0 {
		t.Fatal("a connection without the merchant agreement succeeded")
	}
	for _, want := range []string{"CONFIRMATION_REQUIRED", "confirm_in_workspace", "workspace_url: https://app.vadora.test/account/agreements"} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("the refusal must carry %q, got %q", want, errOut)
		}
	}
}

// A refresh says that nothing was published, lists what it left out, and keeps text that a
// store supplied from steering the terminal.
func TestStoresSyncAndReadinessWordTheResultAndKeepStoreTextInert(t *testing.T) {
	fake, server := newAccountServer(t)
	l := newLaptop(t, server)
	l.ok(signInInput, "login")
	fake.skipped = "Ebook\x1b[31m red \x1b]0;pwned\x07"
	out := l.ok("", "stores", "sync", "s1")
	if !strings.Contains(out, "Refreshed store s1: 1 new, 2 updated, 1 skipped.") {
		t.Fatalf("got %q", out)
	}
	if !strings.Contains(out, "New listings start unpublished. Publish them in your Vadora workspace.") {
		t.Fatalf("got %q", out)
	}
	if strings.ContainsAny(out, "\x1b\x07") {
		t.Fatalf("a store's text steered the terminal: %q", out)
	}
	readiness := l.ok("", "stores", "readiness", "s1")
	for _, want := range []string{"connected", "complete", "wallet_linked", "incomplete", "Limitations: missing_stock_reservation."} {
		if !strings.Contains(readiness, want) {
			t.Fatalf("readiness must print %q, got %q", want, readiness)
		}
	}
	asJSON := l.ok("", "stores", "readiness", "s1", "--json")
	var decoded map[string]any
	if err := json.Unmarshal([]byte(asJSON), &decoded); err != nil {
		t.Fatalf("--json prints the document, got %q", asJSON)
	}
	if decoded["store_id"] != "s1" {
		t.Fatalf("got %v", decoded)
	}
	_, errOut, code := l.run("", "stores", "list", "--mine", "--limit", "5")
	if code == 0 {
		t.Fatalf("--mine takes no other flag, got %d %q", code, errOut)
	}
	if !strings.Contains(errOut, "--mine takes no other flag") {
		t.Fatalf("--mine takes no other flag, got %d %q", code, errOut)
	}
}
