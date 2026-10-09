package client

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/vadora/vadora/api/types/errs"
)

// The CLI signs an account in with OAuth and PKCE against the API origin (001:46). The
// server registers one client for it, opens no browser, and issues no third credential
// type: the answer is an access token and a refresh token of the same OAuth server that
// the agent plugins use. A buyer API call keeps its own revocable buyer credential.

// CLIClientID is the OAuth client that the Vadora CLI signs in with.
const CLIClientID = "vadora-cli"

// CLIRedirectURI is the loopback redirect that the server registers for the CLI client.
// The terminal sign in never opens it, because the code comes back in the answer.
const CLIRedirectURI = "http://127.0.0.1:8765/callback"

// MerchantScope lets a token administer the signed in account's own stores. It grants no
// spending authority and cannot sign or submit a payment.
const MerchantScope = "merchant"

// refreshMargin is how long before its end an access token counts as ended, so a call
// never starts with a token that expires on the way.
const refreshMargin = time.Minute

// maxSessionTokenLength bounds a token that the SDK holds or sends.
const maxSessionTokenLength = MaxCredentialLength

// Session is the CLI's account session: the access token that the merchant calls carry
// and the refresh token that renews it. Both are secrets. The server stores only hashes.
type Session struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Scope        string    `json:"scope"`
	Resource     string    `json:"resource"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// Expired reports whether the access token has ended, or ends within the margin.
func (s Session) Expired(now time.Time) bool {
	return !now.Add(refreshMargin).Before(s.ExpiresAt)
}

// now reads the client's clock.
func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// proof is the one time secret of a PKCE sign in, with the state that binds the answer
// to the request (RFC 7636, S256).
type proof struct {
	verifier, challenge, state string
}

// randomText returns 43 URL safe characters of 32 random bytes.
func randomText() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", errs.New(errs.InvalidRequest, "the system could not make a random value")
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// newProof makes the verifier, its S256 challenge, and the state of one sign in.
func newProof() (proof, error) {
	verifier, err := randomText()
	if err != nil {
		return proof{}, err
	}
	state, err := randomText()
	if err != nil {
		return proof{}, err
	}
	digest := sha256.Sum256([]byte(verifier))
	return proof{verifier: verifier, challenge: base64.RawURLEncoding.EncodeToString(digest[:]), state: state}, nil
}

// protectedResource is the part of the server's discovery document that the sign in reads.
type protectedResource struct {
	Resource string `json:"resource"`
}

// resource reads the resource that the server's tokens authorize from its discovery
// document, so the CLI names the resource the server names (RFC 9728).
func (c *Client) resource(ctx context.Context) (string, error) {
	var document protectedResource
	if err := c.do(ctx, request{method: http.MethodGet, path: "/.well-known/oauth-protected-resource"}, &document); err != nil {
		return "", err
	}
	if document.Resource == "" {
		return "", errs.New(errs.StoreUnavailable, "the server names no resource for its tokens")
	}
	if len(document.Resource) > MaxOriginLength {
		return "", errs.New(errs.StoreUnavailable, "the server names a resource that is too long")
	}
	return document.Resource, nil
}

// terminalRequest is the terminal authorization: the emailed code of the sign in
// request, and the authorization request with its PKCE challenge.
type terminalRequest struct {
	RequestID           string `json:"request_id"`
	Email               string `json:"email"`
	Code                string `json:"code"`
	ClientID            string `json:"client_id"`
	RedirectURI         string `json:"redirect_uri"`
	ResponseType        string `json:"response_type"`
	Scope               string `json:"scope"`
	State               string `json:"state"`
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
	Resource            string `json:"resource"`
}

// terminalGrant answers the terminal authorization with the one time authorization code.
type terminalGrant struct {
	Code  string `json:"code"`
	State string `json:"state"`
}

// tokenAnswer is the OAuth token response.
type tokenAnswer struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

// TerminalSignIn trades the code that Vadora emailed for an account session. The sign in
// request was made with RequestSignInCode as an account request, and the code works once.
// The PKCE verifier never leaves this call except in the token request, so a code that
// someone else saw is useless to them. The session carries the merchant scope only.
func (c *Client) TerminalSignIn(ctx context.Context, requestID, email, code string) (Session, error) {
	if requestID == "" {
		return Session{}, errs.New(errs.InvalidRequest, "the sign in request is missing")
	}
	resource, err := c.resource(ctx)
	if err != nil {
		return Session{}, err
	}
	secret, err := newProof()
	if err != nil {
		return Session{}, err
	}
	var grant terminalGrant
	err = c.do(ctx, request{method: http.MethodPost, path: "/oauth/terminal/authorize", body: terminalRequest{RequestID: requestID,
		Email: email, Code: code, ClientID: CLIClientID, RedirectURI: CLIRedirectURI, ResponseType: "code", Scope: MerchantScope,
		State: secret.state, CodeChallenge: secret.challenge, CodeChallengeMethod: "S256", Resource: resource}}, &grant)
	if err != nil {
		return Session{}, err
	}
	if grant.Code == "" {
		return Session{}, errs.New(errs.Unauthenticated, "the server returned no authorization code")
	}
	if grant.State != secret.state {
		return Session{}, errs.New(errs.Unauthenticated, "the server answered another sign in request")
	}
	return c.token(ctx, resource, url.Values{"grant_type": {"authorization_code"}, "client_id": {CLIClientID}, "code": {grant.Code},
		"redirect_uri": {CLIRedirectURI}, "code_verifier": {secret.verifier}, "resource": {resource}})
}

// RefreshSession renews the session with its refresh token. The server issues a new
// refresh token each time and ends the old one, so the caller stores the answer.
func (c *Client) RefreshSession(ctx context.Context, session Session) (Session, error) {
	if session.RefreshToken == "" {
		return Session{}, errs.New(errs.Unauthenticated, "the session has no refresh token: sign in again")
	}
	if session.Resource == "" {
		return Session{}, errs.New(errs.Unauthenticated, "the session names no resource: sign in again")
	}
	return c.token(ctx, session.Resource, url.Values{"grant_type": {"refresh_token"}, "client_id": {CLIClientID},
		"refresh_token": {session.RefreshToken}, "resource": {session.Resource}})
}

// RevokeSession ends the session on the server, so a copy of its tokens stops working.
func (c *Client) RevokeSession(ctx context.Context, session Session) error {
	if session.RefreshToken == "" {
		return errs.New(errs.Unauthenticated, "the session has no refresh token to revoke")
	}
	return c.do(ctx, request{method: http.MethodPost, path: "/oauth/revoke",
		form: url.Values{"client_id": {CLIClientID}, "token": {session.RefreshToken}}}, nil)
}

// token calls the token endpoint and checks the answer before the caller keeps it.
func (c *Client) token(ctx context.Context, resource string, form url.Values) (Session, error) {
	var answer tokenAnswer
	if err := c.do(ctx, request{method: http.MethodPost, path: "/oauth/token", form: form}, &answer); err != nil {
		return Session{}, err
	}
	if !strings.EqualFold(answer.TokenType, "bearer") {
		return Session{}, errs.New(errs.StoreUnavailable, "the server issued a token that is not a bearer token")
	}
	for _, value := range []string{answer.AccessToken, answer.RefreshToken} {
		if value == "" {
			return Session{}, errs.New(errs.StoreUnavailable, "the server issued an incomplete session")
		}
		if len(value) > maxSessionTokenLength {
			return Session{}, errs.New(errs.StoreUnavailable, "the server issued a token that is too long")
		}
		if strings.ContainsAny(value, " \t\r\n") {
			return Session{}, errs.New(errs.StoreUnavailable, "the server issued a malformed token")
		}
	}
	if answer.ExpiresIn <= 0 {
		return Session{}, errs.New(errs.StoreUnavailable, "the server issued a token with no lifetime")
	}
	return Session{AccessToken: answer.AccessToken, RefreshToken: answer.RefreshToken, Scope: answer.Scope, Resource: resource,
		ExpiresAt: c.now().Add(time.Duration(answer.ExpiresIn) * time.Second).UTC()}, nil
}
