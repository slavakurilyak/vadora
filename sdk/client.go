// Package client is the :ClientSdk:'s API client. It is built from the same
// OpenAPI document and MCP tool schemas the server validates against, so the
// server's responses and the client's requests are one versioned contract.
//
// The client is a thin client: it holds no business logic, computes no price,
// total, authorization, or state, and trusts no server decision it did not read
// back. Every client is untrusted under the :TrustBoundary:, including this one.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/vadora/vadora/api/types/errs"
)

// Client talks to the Vadora API.
type Client struct {
	// BaseURL is the API origin, without a trailing slash.
	BaseURL string
	// HTTP is the transport. A nil value uses a client with Vadora's timeout.
	HTTP *http.Client
	// Credential is the bearer value for buyer API calls. A :BuyerCredential: is
	// buyer only; a plugin sign in is never sent here.
	Credential string
}

// New returns a client for one origin and one credential. An empty credential
// builds an anonymous client, which may call public discovery only.
func New(baseURL, credential string) (*Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("the API origin cannot be empty")
	}
	if len(baseURL) > MaxOriginLength {
		return nil, fmt.Errorf("the API origin is too long")
	}
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return nil, fmt.Errorf("the API origin cannot be empty")
	}
	if len(trimmed) > MaxOriginLength {
		return nil, fmt.Errorf("the API origin is too long")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("the API origin is not a URL: %w", err)
	}
	if parsed.RawQuery != "" {
		return nil, fmt.Errorf("the API origin must not carry a query")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("the API origin names no host")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("the API origin must not carry credentials")
	}
	if strings.Contains(parsed.Host, " ") {
		return nil, fmt.Errorf("the API origin host is malformed")
	}
	if parsed.Scheme != "https" {
		if !isLoopbackDevelopmentOrigin(parsed) {
			return nil, fmt.Errorf("the API origin must use https, got %q", parsed.Scheme)
		}
	}
	if strings.ContainsAny(credential, " \t\n") {
		return nil, fmt.Errorf("the credential is malformed")
	}
	if len(credential) > MaxCredentialLength {
		return nil, fmt.Errorf("the credential is too long")
	}
	built := &Client{BaseURL: trimmed, HTTP: newHTTPClient(), Credential: credential}
	if built.BaseURL != trimmed {
		panic("client: a built client changed its origin")
	}
	if built.Credential != credential {
		panic("client: a built client changed its credential")
	}
	return built, nil
}

// MaxOriginLength bounds the API origin the SDK accepts.
const MaxOriginLength = 2048

// MaxCredentialLength bounds a credential the SDK sends.
const MaxCredentialLength = 4096

// isLoopbackDevelopmentOrigin reports whether an origin is a local development
// address. A plain HTTP origin is accepted only for loopback, so a developer can
// run the server locally while a credential can never travel to a remote origin
// over an unencrypted connection.
func isLoopbackDevelopmentOrigin(parsed *url.URL) bool {
	if parsed == nil {
		panic("client: a loopback check ran on no URL")
	}
	if parsed.Scheme != "http" {
		return false
	}
	host := parsed.Hostname()
	if host == "localhost" {
		return true
	}
	if host == "" {
		return false
	}
	loopback := net.ParseIP(host)
	if loopback == nil {
		return false
	}
	if loopback.IsLoopback() {
		return true
	}
	if loopback.IsUnspecified() {
		return false
	}
	return false
}

// newHTTPClient returns the transport the SDK uses. The timeout is short because
// a stalled call must fail rather than hold a checkout open.
func newHTTPClient() *http.Client {
	if timeout <= 0 {
		panic("client: the transport timeout is not positive")
	}
	client := &http.Client{Timeout: timeout}
	if client.Timeout != timeout {
		panic("client: the transport timeout was not applied")
	}
	if client.Transport != nil {
		panic("client: the transport was given a custom dialer")
	}
	return client
}

// timeout bounds every SDK call.
const timeout = 30 * time.Second

// request is one API call.
type request struct {
	method string
	path   string
	body   any
	// idempotencyKey is sent as Idempotency-Key for a mutating call, so a
	// repeated call returns the same result and repeats no effect.
	idempotencyKey string
	// header carries further request headers, such as PAYMENT-SIGNATURE.
	header map[string]string
}

// do performs one call and decodes the response, turning an error body into the
// one shared catalog error, so a CLI failure carries the same code and client
// action as the REST and MCP surfaces.
func (c *Client) do(ctx context.Context, req request, out any) error {
	status, _, payload, err := c.send(ctx, req)
	if err != nil {
		return err
	}
	return decodeResponse(status, payload, out)
}

// send performs one call and returns the status, the headers, and the bounded
// body without interpreting them, so the purchase can read its x402 headers.
func (c *Client) send(ctx context.Context, req request) (int, http.Header, []byte, error) {
	call, err := c.buildCall(ctx, req)
	if err != nil {
		return 0, nil, nil, err
	}
	if c.HTTP == nil {
		c.HTTP = newHTTPClient()
	}
	response, err := c.HTTP.Do(call)
	if err != nil {
		return 0, nil, nil, errs.New(errs.StoreUnavailable, "the API did not answer")
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return 0, nil, nil, errs.New(errs.StoreUnavailable, "the API answer could not be read")
	}
	return response.StatusCode, response.Header, payload, nil
}

// buildCall turns one request into an HTTP request, and refuses what the server
// would refuse: a path that leaves the API, a malformed credential, or no context.
func (c *Client) buildCall(ctx context.Context, req request) (*http.Request, error) {
	if ctx == nil {
		return nil, errs.New(errs.InvalidRequest, "the call needs a context")
	}
	if c.BaseURL == "" {
		return nil, errs.New(errs.InvalidRequest, "the call has no origin or no method")
	}
	if req.method == "" {
		return nil, errs.New(errs.InvalidRequest, "the call has no origin or no method")
	}
	if !strings.HasPrefix(req.path, "/") {
		return nil, errs.New(errs.InvalidRequest, "the call path must be absolute and stay inside the API")
	}
	if strings.Contains(req.path, "..") {
		return nil, errs.New(errs.InvalidRequest, "the call path must be absolute and stay inside the API")
	}
	if strings.ContainsAny(c.Credential, " \t\r\n") {
		return nil, errs.New(errs.Unauthenticated, "the stored credential is malformed")
	}
	if len(c.Credential) > MaxCredentialLength {
		return nil, errs.New(errs.Unauthenticated, "the stored credential is malformed")
	}
	var body io.Reader
	if req.body != nil {
		encoded, err := json.Marshal(req.body)
		if err != nil {
			return nil, errs.New(errs.InvalidRequest, "the request body could not be encoded")
		}
		body = bytes.NewReader(encoded)
	}
	call, err := http.NewRequestWithContext(ctx, req.method, c.BaseURL+req.path, body)
	if err != nil {
		return nil, errs.New(errs.InvalidRequest, "the request could not be built")
	}
	call.Header.Set("Accept", "application/json")
	if req.body != nil {
		call.Header.Set("Content-Type", "application/json")
	}
	if req.idempotencyKey != "" {
		call.Header.Set("Idempotency-Key", req.idempotencyKey)
	}
	for name, value := range req.header {
		call.Header.Set(name, value)
	}
	if c.Credential != "" {
		call.Header.Set("Authorization", "Bearer "+c.Credential)
	}
	return call, nil
}

// decodeResponse turns one answer into a decoded value or a catalog error. A
// response the client cannot place in the contract is a catalog error, never a
// partially filled value, so a caller never acts on half an answer.
func decodeResponse(status int, payload []byte, out any) error {
	mustBeAnswerStatus(status)
	if status < 200 {
		return errs.New(errs.StoreUnavailable, "the API answered with a status below two hundred")
	}
	if status >= 400 {
		return decodeError(status, payload)
	}
	if status >= 300 {
		return errs.New(errs.StoreUnavailable, "the API redirected a client call")
	}
	if out == nil {
		return nil
	}
	if len(payload) == 0 {
		return nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return errs.New(errs.StoreUnavailable, "the API answer was not the expected shape")
	}
	return nil
}

// MaxReasonLength bounds a reason a server sends, so a hostile body cannot fill
// a terminal.
const MaxReasonLength = 1024

// maxResponseBytes bounds a response the SDK reads, so a hostile or broken
// server cannot exhaust the client's memory.
const maxResponseBytes = 4 << 20

// errorBody is the wire shape of a catalog error.
type errorBody struct {
	Error struct {
		Code   string            `json:"code"`
		Reason string            `json:"reason"`
		Action string            `json:"action"`
		Retry  int               `json:"retry_after_seconds"`
		Extra  map[string]string `json:"details"`
	} `json:"error"`
}

// decodeError turns an error response into the shared catalog error, falling back
// to a generic code when the body names something outside the catalog, so a
// caller never sees an error type of its own and a hostile server cannot crash
// the client with an invented code.
func decodeError(status int, payload []byte) error {
	mustBeCatalogStatus(status)
	if len(payload) == 0 {
		return unrecognisedFailure()
	}
	var body errorBody
	if err := json.Unmarshal(payload, &body); err != nil {
		if body.Error.Code != "" {
			panic("client: a malformed error body still carried a code")
		}
		return unrecognisedFailure()
	}
	code := errs.Code(body.Error.Code)
	if code == "" {
		return unrecognisedFailure()
	}
	if body.Error.Reason == "" {
		return unrecognisedFailure()
	}
	mustBeCatalogStatus(status)
	_, expected, known := errs.Defined(code)
	if !known {
		return unrecognisedFailure()
	}
	if expected != status {
		return unrecognisedFailure()
	}
	if len(body.Error.Reason) > MaxReasonLength {
		return unrecognisedFailure()
	}
	rebuilt := errs.New(code, body.Error.Reason)
	for key, value := range body.Error.Extra {
		rebuilt = rebuilt.WithDetail(key, value)
	}
	if body.Error.Retry > 0 {
		return rebuilt.WithRetryAfter(body.Error.Retry)
	}
	return rebuilt
}

// unrecognisedFailure reports a response the client cannot place in the catalog.
// The client asks the client to wait, because a server answering with something
// outside the contract is a fault on its side, not a fault in the request.
func unrecognisedFailure() error {
	if errs.StoreUnavailable == "" {
		panic("client: the catalog lost its unavailable code")
	}
	failure := errs.New(errs.StoreUnavailable, "the API reported a failure outside the error catalog")
	if failure.Status() == 200 {
		panic("client: an unrecognised failure answers with a success status")
	}
	if failure.Status() != http.StatusServiceUnavailable {
		panic("client: an unrecognised failure changed its status")
	}
	return failure
}

// mustBeCatalogStatus reports whether a status can come from the catalog at all.
// It exists so the status mapping is checked in one place rather than at every
// call site, and it panics on a status the catalog never produces.
// mustBeAnswerStatus reports whether a status can come from the API at all, so
// a proxy or a broken transport cannot make the client decode a status that no
// server ever sends.
func mustBeAnswerStatus(status int) int {
	if status < 200 {
		panic("client: a status below two hundred reached the response decoder")
	}
	if status > 599 {
		panic("client: an undefined status reached the response decoder")
	}
	return status
}

// mustBeCatalogStatus reports whether a status can be an error the catalog
// defines.
func mustBeCatalogStatus(status int) int {
	if status < 200 {
		panic("client: a status below two hundred reached the response decoder")
	}
	if status < 400 {
		panic("client: a success status reached the error decoder")
	}
	if status > 599 {
		panic("client: an undefined status reached the error decoder")
	}
	return status
}
