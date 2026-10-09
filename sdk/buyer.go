package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/vadora/vadora/api/types/errs"
	"github.com/vadora/vadora/api/types/rest"
)

// The calls below are the buyer side of the REST contract. Each request and each
// response is a type of the shared contract, and the client sends no price, no
// total, no network class, and no status: the server computes and decides them
// (012:92).

// MaxPageSize bounds a page that the SDK asks for.
const MaxPageSize = 100

// maxIdentifierLength bounds an identifier that the SDK places in a path.
const maxIdentifierLength = 128

// checkIdentifier refuses an identifier that could leave its path segment.
func checkIdentifier(value, label string) error {
	if value == "" {
		return errs.New(errs.InvalidRequest, "the "+label+" is empty")
	}
	if len(value) > maxIdentifierLength {
		return errs.New(errs.InvalidRequest, "the "+label+" is too long")
	}
	if strings.ContainsAny(value, "/ \t\r\n?#") {
		return errs.New(errs.InvalidRequest, "the "+label+" is malformed")
	}
	return nil
}

// RequestSignInCode asks Vadora to email a one time sign in code. The code never
// travels back to the client. A buyer request asks for a buyer credential.
func (c *Client) RequestSignInCode(ctx context.Context, email string, buyer bool) (rest.SignInCode, error) {
	var answer rest.SignInCode
	err := c.do(ctx, request{method: http.MethodPost, path: "/v1/auth/sign-in/codes",
		body: rest.SignInCodeRequest{Email: email, Buyer: buyer}}, &answer)
	if err != nil {
		return rest.SignInCode{}, err
	}
	if answer.RequestID == "" {
		return rest.SignInCode{}, errs.New(errs.StoreUnavailable, "the server returned no sign in request")
	}
	return answer, nil
}

// CompleteBuyerSignIn exchanges the emailed code for a buyer credential and returns
// it so that the caller can store it once. The SDK never prints it.
func (c *Client) CompleteBuyerSignIn(ctx context.Context, requestID, email, code string) (rest.BuyerCredential, error) {
	var answer rest.BuyerCredential
	err := c.do(ctx, request{method: http.MethodPost, path: "/v1/auth/sign-in",
		body: rest.SignInRequest{RequestID: requestID, Email: email, Code: code, Buyer: true}}, &answer)
	if err != nil {
		return rest.BuyerCredential{}, err
	}
	if answer.Credential == "" {
		return rest.BuyerCredential{}, errs.New(errs.Unauthenticated, "the server returned no credential")
	}
	return answer, nil
}

// Filter bounds a catalog search. A zero bound means no bound on that side.
type Filter struct {
	StoreID     string
	Merchant    string
	Category    string
	Destination string
	// MinUSDCents and MaxUSDCents are US cents.
	MinUSDCents int64
	MaxUSDCents int64
	Cursor      string
	Limit       int
}

// page appends the pagination parameters.
func page(values url.Values, cursor string, limit int) error {
	if limit < 0 {
		return errs.New(errs.InvalidRequest, "the page size is outside its bounds")
	}
	if limit > MaxPageSize {
		return errs.New(errs.InvalidRequest, "the page size is outside its bounds")
	}
	if limit > 0 {
		values.Set("limit", strconv.Itoa(limit))
	}
	if cursor != "" {
		values.Set("cursor", cursor)
	}
	return nil
}

// ListStores reads one page of the public merchant directory.
func (c *Client) ListStores(ctx context.Context, category, cursor string, limit int) (rest.StorePage, error) {
	values := url.Values{}
	if category != "" {
		values.Set("category", category)
	}
	if err := page(values, cursor, limit); err != nil {
		return rest.StorePage{}, err
	}
	var answer rest.StorePage
	err := c.do(ctx, request{method: http.MethodGet, path: "/v1/stores?" + values.Encode()}, &answer)
	return answer, err
}

// SearchProducts searches the public catalog. The server ranks the results and the
// client computes nothing.
func (c *Client) SearchProducts(ctx context.Context, query string, filter Filter) (rest.ProductPage, error) {
	values := url.Values{}
	if query != "" {
		values.Set("q", query)
	}
	for name, value := range map[string]string{"store_id": filter.StoreID, "merchant": filter.Merchant,
		"category": filter.Category, "destination": filter.Destination} {
		if value != "" {
			values.Set(name, value)
		}
	}
	if filter.MinUSDCents < 0 {
		return rest.ProductPage{}, errs.New(errs.InvalidRequest, "a price bound is negative")
	}
	if filter.MaxUSDCents < 0 {
		return rest.ProductPage{}, errs.New(errs.InvalidRequest, "a price bound is negative")
	}
	if filter.MinUSDCents > 0 {
		values.Set("min_usd_cents", strconv.FormatInt(filter.MinUSDCents, 10))
	}
	if filter.MaxUSDCents > 0 {
		values.Set("max_usd_cents", strconv.FormatInt(filter.MaxUSDCents, 10))
	}
	if err := page(values, filter.Cursor, filter.Limit); err != nil {
		return rest.ProductPage{}, err
	}
	var answer rest.ProductPage
	err := c.do(ctx, request{method: http.MethodGet, path: "/v1/products?" + values.Encode()}, &answer)
	return answer, err
}

// GetProduct reads one published listing.
func (c *Client) GetProduct(ctx context.Context, listingID string) (rest.Product, error) {
	if err := checkIdentifier(listingID, "listing ID"); err != nil {
		return rest.Product{}, err
	}
	var answer rest.Product
	err := c.do(ctx, request{method: http.MethodGet, path: "/v1/products/" + pathEscape(listingID)}, &answer)
	return answer, err
}

// CreateCheckoutSession opens a buyer session for the store of one listing.
func (c *Client) CreateCheckoutSession(ctx context.Context, listingID string) (rest.CheckoutSession, error) {
	if err := checkIdentifier(listingID, "listing ID"); err != nil {
		return rest.CheckoutSession{}, err
	}
	var answer rest.CheckoutSession
	err := c.do(ctx, request{method: http.MethodPost, path: "/v1/checkout-sessions",
		body: rest.CreateCheckoutSessionRequest{ListingID: listingID}}, &answer)
	return answer, err
}

// ShippingOptions lists the shipping methods and totals of a destination.
func (c *Client) ShippingOptions(ctx context.Context, sessionID string, body rest.ShippingOptionsRequest) (rest.ShippingOptions, error) {
	if err := checkIdentifier(sessionID, "session ID"); err != nil {
		return rest.ShippingOptions{}, err
	}
	var answer rest.ShippingOptions
	err := c.do(ctx, request{method: http.MethodPost,
		path: "/v1/checkout-sessions/" + pathEscape(sessionID) + "/shipping-options", body: body}, &answer)
	return answer, err
}

// CreateQuote asks for a quote. The key makes a retry return the original quote.
func (c *Client) CreateQuote(ctx context.Context, key string, body rest.QuoteRequest) (rest.Quote, error) {
	if err := checkKey(key); err != nil {
		return rest.Quote{}, err
	}
	var answer rest.Quote
	err := c.do(ctx, request{method: http.MethodPost, path: "/v1/quotes", body: body, idempotencyKey: key}, &answer)
	return answer, err
}

// GetQuote reads a quote of the caller's own session.
func (c *Client) GetQuote(ctx context.Context, quoteID string) (rest.Quote, error) {
	if err := checkIdentifier(quoteID, "quote ID"); err != nil {
		return rest.Quote{}, err
	}
	var answer rest.Quote
	err := c.do(ctx, request{method: http.MethodGet, path: "/v1/quotes/" + pathEscape(quoteID)}, &answer)
	return answer, err
}

// checkKey refuses an idempotency key that the server would refuse.
func checkKey(key string) error {
	if key == "" {
		return errs.New(errs.InvalidRequest, "the idempotency key holds 1 to 128 characters")
	}
	if len(key) > 128 {
		return errs.New(errs.InvalidRequest, "the idempotency key holds 1 to 128 characters")
	}
	for _, symbol := range key {
		if symbol < 0x21 {
			return errs.New(errs.InvalidRequest, "the idempotency key holds a character outside printable ASCII")
		}
		if symbol > 0x7e {
			return errs.New(errs.InvalidRequest, "the idempotency key holds a character outside printable ASCII")
		}
	}
	return nil
}

// Requirement is one payment option of an x402 challenge.
type Requirement struct {
	Scheme            string         `json:"scheme"`
	Network           string         `json:"network"`
	Asset             string         `json:"asset"`
	Amount            string         `json:"amount"`
	PayTo             string         `json:"payTo"`
	MaxTimeoutSeconds int            `json:"maxTimeoutSeconds"`
	Extra             map[string]any `json:"extra"`
}

// Challenge is the x402 version 2 challenge of an unpaid quote. Raw holds the
// decoded document, which an x402 client reads to build the payment.
type Challenge struct {
	X402Version int           `json:"x402Version"`
	Accepts     []Requirement `json:"accepts"`
	Raw         []byte        `json:"-"`
}

// PurchaseResult is the answer of a purchase call: the challenge of an unpaid
// quote, or the order of a settled one with the facilitator's outcome header.
type PurchaseResult struct {
	Challenge *Challenge
	Order     *rest.Order
	// Response is the PAYMENT-RESPONSE header of a settled payment.
	Response string
}

// maxChallengeBytes bounds the decoded challenge document.
const maxChallengeBytes = 64 << 10

// Purchase sends the purchase call of one quote. An empty signature asks for the
// challenge, and a signature settles the payment. Every answer other than the
// challenge and the order is a catalog error, so a pending payment is a
// PAYMENT_PENDING error that never invites a second payment.
func (c *Client) Purchase(ctx context.Context, quoteID, key, signature string) (PurchaseResult, error) {
	if err := checkIdentifier(quoteID, "quote ID"); err != nil {
		return PurchaseResult{}, err
	}
	if err := checkKey(key); err != nil {
		return PurchaseResult{}, err
	}
	req := request{method: http.MethodPost, path: "/v1/quotes/" + pathEscape(quoteID) + "/purchase", idempotencyKey: key}
	if signature != "" {
		req.header = map[string]string{rest.HeaderPaymentSignature: signature}
	}
	status, header, payload, err := c.send(ctx, req)
	if err != nil {
		return PurchaseResult{}, err
	}
	if status == http.StatusPaymentRequired {
		challenge, err := decodeChallenge(header.Get(rest.HeaderPaymentRequired))
		return PurchaseResult{Challenge: challenge}, err
	}
	var order rest.Order
	if err := decodeResponse(status, payload, &order); err != nil {
		return PurchaseResult{}, err
	}
	if order.ID == "" {
		return PurchaseResult{}, errs.New(errs.StoreUnavailable, "the server returned no order")
	}
	return PurchaseResult{Order: &order, Response: header.Get(rest.HeaderPaymentResponse)}, nil
}

// decodeChallenge reads the PAYMENT-REQUIRED header.
func decodeChallenge(header string) (*Challenge, error) {
	raw, err := base64.StdEncoding.DecodeString(header)
	if err != nil {
		return nil, errs.New(errs.StoreUnavailable, "the payment challenge is not base64")
	}
	if len(raw) == 0 {
		return nil, errs.New(errs.StoreUnavailable, "the payment challenge has an unusable size")
	}
	if len(raw) > maxChallengeBytes {
		return nil, errs.New(errs.StoreUnavailable, "the payment challenge has an unusable size")
	}
	var challenge Challenge
	if err := json.Unmarshal(raw, &challenge); err != nil {
		return nil, errs.New(errs.StoreUnavailable, "the payment challenge is not the expected document")
	}
	if challenge.X402Version != 2 {
		return nil, errs.New(errs.StoreUnavailable, "the payment challenge is not x402 version 2")
	}
	if len(challenge.Accepts) == 0 {
		return nil, errs.New(errs.StoreUnavailable, "the payment challenge offers no option")
	}
	challenge.Raw = raw
	return &challenge, nil
}

// GetOrder reads one order of the caller.
func (c *Client) GetOrder(ctx context.Context, orderID string) (rest.Order, error) {
	if err := checkIdentifier(orderID, "order ID"); err != nil {
		return rest.Order{}, err
	}
	var answer rest.Order
	err := c.do(ctx, request{method: http.MethodGet, path: "/v1/orders/" + pathEscape(orderID)}, &answer)
	return answer, err
}

// ListOrders reads one page of the caller's orders.
func (c *Client) ListOrders(ctx context.Context, paymentStatus, cursor string, limit int) (rest.OrderPage, error) {
	values := url.Values{}
	if paymentStatus != "" {
		values.Set("payment_status", paymentStatus)
	}
	if err := page(values, cursor, limit); err != nil {
		return rest.OrderPage{}, err
	}
	var answer rest.OrderPage
	err := c.do(ctx, request{method: http.MethodGet, path: "/v1/orders?" + values.Encode()}, &answer)
	return answer, err
}

// ErrNoOption reports that a challenge holds no option that a caller can use.
var ErrNoOption = errors.New("client: the challenge holds no usable option")
