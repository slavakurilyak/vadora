package client

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vadora/vadora/api/types/errs"
)

// signInServer is the part of Vadora's OAuth server that the terminal sign in talks to.
type signInServer struct {
	t         *testing.T
	mu        sync.Mutex
	terminal  []byte
	form      []url.Values
	echoState func(sent string) string
	token     map[string]any
	calls     []string
}

func newSignInServer(t *testing.T) (*signInServer, *Client) {
	t.Helper()
	fake := &signInServer{t: t, echoState: func(sent string) string { return sent }}
	server := httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(server.Close)
	client, err := New(server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	moment := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	client.Now = func() time.Time { return moment }
	fake.token = map[string]any{"access_token": "access-token-0123456789", "token_type": "Bearer", "expires_in": 3600,
		"refresh_token": "refresh-token-0123456789", "scope": "merchant"}
	return fake, client
}

func (f *signInServer) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/.well-known/oauth-protected-resource":
		_, _ = io.WriteString(w, `{"resource":"https://api.vadora.test/mcp","authorization_servers":["https://api.vadora.test"]}`)
	case "/oauth/terminal/authorize":
		raw, _ := io.ReadAll(r.Body)
		f.terminal = raw
		var sent struct {
			State string `json:"state"`
		}
		_ = json.Unmarshal(raw, &sent)
		encoded, _ := json.Marshal(map[string]string{"code": "grant-code-0123456789", "state": f.echoState(sent.State)})
		_, _ = w.Write(encoded)
	case "/oauth/token", "/oauth/revoke":
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			f.t.Errorf("%s: the token endpoints read a form, got %q", r.URL.Path, r.Header.Get("Content-Type"))
		}
		if err := r.ParseForm(); err != nil {
			f.t.Error(err)
		}
		f.form = append(f.form, r.PostForm)
		encoded, _ := json.Marshal(f.token)
		_, _ = w.Write(encoded)
	default:
		http.NotFound(w, r)
	}
}

// The terminal sign in sends a PKCE challenge with the emailed code and the verifier only
// with the token request, so the code alone is useless to anyone who saw it.
func TestTheTerminalSignInTradesTheEmailedCodeForASessionWithPKCE(t *testing.T) {
	fake, client := newSignInServer(t)
	session, err := client.TerminalSignIn(t.Context(), "request-1", "owner@shop.example", "123456")
	if err != nil {
		t.Fatal(err)
	}
	var sent terminalRequest
	if err := json.Unmarshal(fake.terminal, &sent); err != nil {
		t.Fatal(err)
	}
	assertTheAuthorizationRequest(t, sent)
	exchange := fake.form[0]
	verifier := exchange.Get("code_verifier")
	digest := sha256.Sum256([]byte(verifier))
	if base64.RawURLEncoding.EncodeToString(digest[:]) != sent.CodeChallenge {
		t.Fatalf("the verifier must hash to the challenge that the sign in sent, got %q", verifier)
	}
	if strings.Contains(string(fake.terminal), verifier) {
		t.Fatal("the verifier travelled with the authorization request")
	}
	for name, want := range map[string]string{"grant_type": "authorization_code", "client_id": CLIClientID, "code": "grant-code-0123456789",
		"redirect_uri": CLIRedirectURI, "resource": "https://api.vadora.test/mcp"} {
		if exchange.Get(name) != want {
			t.Fatalf("the token request carries %s=%q, got %q", name, want, exchange.Get(name))
		}
	}
	assertTheSession(t, session)
}

// assertTheAuthorizationRequest checks what the terminal authorization carries.
func assertTheAuthorizationRequest(t *testing.T, sent terminalRequest) {
	t.Helper()
	if sent.ClientID != CLIClientID {
		t.Fatalf("the sign in names the CLI client, its loopback redirect, and only the merchant scope, got %+v", sent)
	}
	if sent.RedirectURI != CLIRedirectURI {
		t.Fatalf("the sign in names the CLI client, its loopback redirect, and only the merchant scope, got %+v", sent)
	}
	if sent.Scope != MerchantScope {
		t.Fatalf("the sign in names the CLI client, its loopback redirect, and only the merchant scope, got %+v", sent)
	}
	if sent.ResponseType != "code" {
		t.Fatalf("the sign in is an authorization code flow with S256, got %+v", sent)
	}
	if sent.CodeChallengeMethod != "S256" {
		t.Fatalf("the sign in is an authorization code flow with S256, got %+v", sent)
	}
	if sent.RequestID != "request-1" {
		t.Fatalf("the sign in carries the request, the email, and the emailed code, got %+v", sent)
	}
	if sent.Email != "owner@shop.example" {
		t.Fatalf("the sign in carries the request, the email, and the emailed code, got %+v", sent)
	}
	if sent.Code != "123456" {
		t.Fatalf("the sign in carries the request, the email, and the emailed code, got %+v", sent)
	}
	if sent.Resource != "https://api.vadora.test/mcp" {
		t.Fatalf("the sign in names the resource that the server's discovery document names, got %q", sent.Resource)
	}
	if len(sent.CodeChallenge) != 43 {
		t.Fatalf("the challenge is 43 characters and the state is long enough, got %+v", sent)
	}
	if len(sent.State) < 16 {
		t.Fatalf("the challenge is 43 characters and the state is long enough, got %+v", sent)
	}
}

// assertTheSession checks the session that the sign in returns.
func assertTheSession(t *testing.T, session Session) {
	t.Helper()
	if session.AccessToken != "access-token-0123456789" {
		t.Fatalf("got %+v", session)
	}
	if session.RefreshToken != "refresh-token-0123456789" {
		t.Fatalf("got %+v", session)
	}
	if session.ExpiresAt != time.Date(2026, 10, 10, 13, 0, 0, 0, time.UTC) {
		t.Fatalf("the session ends an hour after the client's clock says it started, got %v", session.ExpiresAt)
	}
	if session.Scope != "merchant" {
		t.Fatalf("got %+v", session)
	}
	if session.Resource != "https://api.vadora.test/mcp" {
		t.Fatalf("got %+v", session)
	}
}

// Every sign in has its own verifier and state, and a state that the server does not
// echo ends the sign in before any token is asked for.
func TestASignInAnswerForAnotherRequestIsRefusedBeforeAnyTokenIsAsked(t *testing.T) {
	fake, client := newSignInServer(t)
	fake.echoState = func(string) string { return "another-request-state-0123456789" }
	_, err := client.TerminalSignIn(t.Context(), "request-1", "owner@shop.example", "123456")
	if failure, ok := errs.As(err); !ok {
		t.Fatalf("got %v", err)
	} else if failure.Code != errs.Unauthenticated {
		t.Fatalf("got %v", err)
	}
	for _, call := range fake.calls {
		if call == "POST /oauth/token" {
			t.Fatal("a token was asked for after the state did not match")
		}
	}
	first, second := newProofOrFail(t), newProofOrFail(t)
	if first.verifier == second.verifier {
		t.Fatal("two sign ins share a secret")
	}
	if first.state == second.state {
		t.Fatal("two sign ins share a secret")
	}
	if first.verifier == first.state {
		t.Fatal("two sign ins share a secret")
	}
}

func newProofOrFail(t *testing.T) proof {
	t.Helper()
	made, err := newProof()
	if err != nil {
		t.Fatal(err)
	}
	return made
}

// A token answer that the SDK cannot place in the contract is refused whole, so the CLI
// never stores half a session.
func TestATokenAnswerOutsideTheContractIsRefused(t *testing.T) {
	cases := map[string]map[string]any{
		"not a bearer token": {"access_token": "access-token-0123456789", "token_type": "mac", "expires_in": 3600, "refresh_token": "refresh-token-0123456789"},
		"no refresh token":   {"access_token": "access-token-0123456789", "token_type": "Bearer", "expires_in": 3600, "refresh_token": ""},
		"no access token":    {"access_token": "", "token_type": "Bearer", "expires_in": 3600, "refresh_token": "refresh-token-0123456789"},
		"no lifetime":        {"access_token": "access-token-0123456789", "token_type": "Bearer", "expires_in": 0, "refresh_token": "refresh-token-0123456789"},
		"a token with space": {"access_token": "access token", "token_type": "Bearer", "expires_in": 3600, "refresh_token": "refresh-token-0123456789"},
	}
	for name, answer := range cases {
		fake, client := newSignInServer(t)
		fake.token = answer
		if _, err := client.TerminalSignIn(t.Context(), "request-1", "owner@shop.example", "123456"); err == nil {
			t.Errorf("%s: the session was accepted", name)
		}
	}
}

// A refresh sends the refresh token and the session's resource, and returns the rotated
// session. A revoke sends the refresh token to the revocation endpoint.
func TestASessionIsRefreshedAndRevokedWithItsRefreshToken(t *testing.T) {
	fake, client := newSignInServer(t)
	fake.token["access_token"] = "access-token-rotated-0123"
	fake.token["refresh_token"] = "refresh-token-rotated-01"
	old := Session{AccessToken: "access-token-0123456789", RefreshToken: "refresh-token-0123456789", Scope: "merchant",
		Resource: "https://api.vadora.test/mcp", ExpiresAt: client.now().Add(-time.Hour)}
	renewed, err := client.RefreshSession(t.Context(), old)
	if err != nil {
		t.Fatal(err)
	}
	form := fake.form[0]
	if form.Get("grant_type") != "refresh_token" {
		t.Fatalf("the refresh sends the refresh token, got %v", form)
	}
	if form.Get("refresh_token") != old.RefreshToken {
		t.Fatalf("the refresh sends the refresh token, got %v", form)
	}
	if form.Get("client_id") != CLIClientID {
		t.Fatalf("the refresh names the CLI client and the session's resource, got %v", form)
	}
	if form.Get("resource") != old.Resource {
		t.Fatalf("the refresh names the CLI client and the session's resource, got %v", form)
	}
	if renewed.AccessToken != "access-token-rotated-0123" {
		t.Fatalf("the caller stores the rotated tokens, got %+v", renewed)
	}
	if renewed.RefreshToken != "refresh-token-rotated-01" {
		t.Fatalf("the caller stores the rotated tokens, got %+v", renewed)
	}
	if !renewed.ExpiresAt.After(client.now()) {
		t.Fatalf("a renewed session is live, got %v", renewed.ExpiresAt)
	}
	if err := client.RevokeSession(t.Context(), renewed); err != nil {
		t.Fatal(err)
	}
	revoke := fake.form[1]
	if revoke.Get("token") != renewed.RefreshToken {
		t.Fatalf("the revoke names the client and the refresh token, got %v", revoke)
	}
	if revoke.Get("client_id") != CLIClientID {
		t.Fatalf("the revoke names the client and the refresh token, got %v", revoke)
	}
	if _, err := client.RefreshSession(t.Context(), Session{}); err == nil {
		t.Fatal("a session without a refresh token cannot refresh")
	}
	if err := client.RevokeSession(t.Context(), Session{}); err == nil {
		t.Fatal("a session without a refresh token cannot be revoked")
	}
}

// A token counts as ended a minute early, so a call never starts with a token that
// expires on the way.
func TestASessionCountsAsEndedWithinTheRefreshMargin(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	session := Session{ExpiresAt: now.Add(2 * time.Minute)}
	if session.Expired(now) {
		t.Fatal("a token with two minutes left is live")
	}
	if !session.Expired(now.Add(time.Minute + time.Second)) {
		t.Fatal("a token with less than a minute left counts as ended")
	}
	if !session.Expired(now.Add(3 * time.Minute)) {
		t.Fatal("a token past its end is ended")
	}
}
