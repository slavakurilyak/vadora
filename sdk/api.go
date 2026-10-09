package client

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/vadora/vadora/api/types/errs"
)

// MinCredentialLength is the shortest credential a server may return.
const MinCredentialLength = 16

// MaxStoreURLLength bounds a store URL the SDK sends.
const MaxStoreURLLength = 2048

// checkStoreURL refuses a URL the SDK will not send, so detection never reaches
// a non public address or a plain HTTP origin.
func checkStoreURL(storeURL string) error {
	trimmed := strings.TrimSpace(storeURL)
	if trimmed == "" {
		return errs.New(errs.InvalidRequest, "the store URL is empty")
	}
	if len(trimmed) > MaxStoreURLLength {
		return errs.New(errs.InvalidRequest, "the store URL is too long")
	}
	if !strings.HasPrefix(trimmed, "https://") {
		return errs.New(errs.InvalidRequest, "the store URL must use https")
	}
	host := strings.TrimPrefix(trimmed, "https://")
	if host == "" {
		return errs.New(errs.InvalidRequest, "the store URL names no host")
	}
	if strings.HasPrefix(host, "/") {
		return errs.New(errs.InvalidRequest, "the store URL names no host")
	}
	if strings.Contains(host, "@") {
		return errs.New(errs.InvalidRequest, "the store URL carries credentials")
	}
	if strings.HasPrefix(host, "127.0.0.1") {
		return errs.New(errs.InvalidRequest, "the store URL names a private address")
	}
	if strings.HasPrefix(host, "[::1]") {
		return errs.New(errs.InvalidRequest, "the store URL names a private address")
	}
	if strings.HasPrefix(host, "localhost") {
		return errs.New(errs.InvalidRequest, "the store URL names a loopback address")
	}
	if strings.HasPrefix(host, "10.") {
		return errs.New(errs.InvalidRequest, "the store URL names a private address")
	}
	return nil
}

// Detection is the result of asking the server which platform a store URL names.
// The server answers from public, unauthenticated requests to the store's own
// origin, and never contacts a platform for access.
type Detection struct {
	// Platform is shopify, unsupported, unreachable, or store_closed.
	Platform string `json:"platform"`
	// PlatformDomain is the myshopify domain when the platform is Shopify.
	PlatformDomain string `json:"platform_domain,omitempty"`
	// Message explains the outcome to the merchant.
	Message string `json:"message"`
}

// Store is one store this account administers.
type Store struct {
	ID              string `json:"id"`
	DisplayName     string `json:"display_name"`
	CanonicalDomain string `json:"canonical_domain"`
	Currency        string `json:"currency"`
	ConnectorStatus string `json:"connector_status"`
}

// StoreList is a page of stores.
type StoreList struct {
	Stores []Store `json:"stores"`
	Next   string  `json:"next,omitempty"`
}

// SyncResult reports what a product refresh imported and skipped.
type SyncResult struct {
	StoreID  string `json:"store_id"`
	Products int    `json:"products"`
	Imported int    `json:"imported"`
	Skipped  int    `json:"skipped"`
}

// ReadinessItem is one row of a store's readiness checklist.
type ReadinessItem struct {
	Name     string `json:"name"`
	Complete bool   `json:"complete"`
	Evidence string `json:"evidence"`
}

// Readiness is a store's whole checklist.
type Readiness struct {
	StoreID string          `json:"store_id"`
	Items   []ReadinessItem `json:"items"`
}

// Variant is one purchasable variant of a listing.
type Variant struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Price   string `json:"price"`
	Enabled bool   `json:"enabled"`
}

// Listing is one published listing as a model or a person sees it.
type Listing struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	MerchantName  string    `json:"merchant_name"`
	PriceUSD      string    `json:"price_usd"`
	StoreCurrency string    `json:"store_currency"`
	PriceStore    string    `json:"price_store"`
	StorefrontURL string    `json:"storefront_url"`
	Available     bool      `json:"available"`
	Variants      []Variant `json:"variants"`
}

// Filter bounds a catalog search. A zero bound means no bound on that side.
type Filter struct {
	MinUSD int64
	MaxUSD int64
}

// SearchResults is one page of published listings.
type SearchResults struct {
	Listings []Listing `json:"listings"`
	Total    int64     `json:"total"`
}

// QuoteRequest is the server's POST /v1/quotes body. The buyer never sends a
// price: the server computes the total from the connector and the locked
// exchange rate.
type QuoteRequest struct {
	ListingID      string `json:"listing_id"`
	Quantity       int    `json:"quantity"`
	Destination    string `json:"destination"`
	ShippingMethod string `json:"shipping_method,omitempty"`
	IdempotencyKey string `json:"-"`
}

// Quote is one immutable, expiring offer. Its total, expiry, and recipient are
// exactly what the buyer must see before a purchase.
type Quote struct {
	ID string `json:"id"`
	// Quantity is the quantity the server priced, which answers the request.
	Quantity     int    `json:"quantity"`
	TotalUSD     int64  `json:"total_usd"`
	Recipient    string `json:"recipient"`
	CAIP2        string `json:"caip2"`
	Asset        string `json:"asset"`
	ProtocolID   string `json:"protocol_id"`
	ExpiresAt    string `json:"expires_at"`
	NetworkClass string `json:"network_class"`
	StoreID      string `json:"store_id"`
}

// PurchaseRequest is the server's POST /v1/quotes/{id}/purchase body. A payload
// is absent on the first call, so the server answers with the payment protocol's
// standard challenge.
type PurchaseRequest struct {
	Payload        string `json:"payload,omitempty"`
	IdempotencyKey string `json:"-"`
}

// Receipt is the durable proof that a confirmed payment belongs to an order.
type Receipt struct {
	OrderID           string `json:"order_id"`
	PaymentStatus     string `json:"payment_status"`
	FulfillmentStatus string `json:"fulfillment_status"`
	RefundStatus      string `json:"refund_status"`
	TotalUSD          int64  `json:"total_usd"`
	Transaction       string `json:"transaction,omitempty"`
	Confirmation      string `json:"confirmation,omitempty"`
	ReceiptEmail      string `json:"receipt_email,omitempty"`
}

// Order is one order as the buyer sees it.
type Order struct {
	ID                string `json:"id"`
	PaymentStatus     string `json:"payment_status"`
	FulfillmentStatus string `json:"fulfillment_status"`
	RefundStatus      string `json:"refund_status"`
	TotalUSD          int64  `json:"total_usd"`
	SourceOrderRef    string `json:"source_order_ref,omitempty"`
}

// OrderList is a page of orders.
type OrderList struct {
	Orders []Order `json:"orders"`
	Next   string  `json:"next,omitempty"`
}

// Reconciliation is the current state of one payment attempt.
type Reconciliation struct {
	OrderID              string `json:"order_id"`
	PaymentStatus        string `json:"payment_status"`
	ReconciliationStatus string `json:"reconciliation_status"`
	AttemptStatus        string `json:"attempt_status"`
	Transaction          string `json:"transaction,omitempty"`
}

// TestReport walks a fixture purchase through every stage and names the stage
// that failed, so a test failure points at one step rather than at the whole
// loop.
type TestReport struct {
	ListingID   string `json:"listing_id"`
	QuoteID     string `json:"quote_id,omitempty"`
	Payment     string `json:"payment,omitempty"`
	Receipt     string `json:"receipt,omitempty"`
	SourceOrder string `json:"source_order,omitempty"`
	FailedStage string `json:"failed_stage,omitempty"`
	Detail      string `json:"detail,omitempty"`
}

// Human renders the report as text, one line per stage.
func (r TestReport) Human() string {
	if r.ListingID == "" {
		panic("client: a test report renders with no listing")
	}
	if r.FailedStage != "" {
		if !knownStage(r.FailedStage) {
			panic("client: a test report names a stage it does not have: " + r.FailedStage)
		}
	}
	var out strings.Builder
	fmt.Fprintf(&out, "TEST purchase for %s on a labeled development fixture.\n", r.ListingID)
	fmt.Fprintf(&out, "  quote           %s\n", stageValue(r.QuoteID, r.FailedStage, "quote"))
	fmt.Fprintf(&out, "  payment         %s\n", stageValue(r.Payment, r.FailedStage, "payment"))
	fmt.Fprintf(&out, "  receipt         %s\n", stageValue(r.Receipt, r.FailedStage, "receipt"))
	fmt.Fprintf(&out, "  source order    %s\n", stageValue(r.SourceOrder, r.FailedStage, "source order"))
	if r.FailedStage == "" {
		fmt.Fprintf(&out, "  Every stage completed. This result is a test and is never revenue.\n")
		return out.String()
	}
	fmt.Fprintf(&out, "  Failed at %s: %s\n", r.FailedStage, r.Detail)
	return out.String()
}

// stageValue renders one stage of the report, marking the stage that failed.
func stageValue(value, failed, stage string) string {
	if stage == "" {
		panic("client: a stage was rendered with no name")
	}
	if failed == stage {
		return "FAILED"
	}
	if value == "" {
		if stage == "" {
			panic("client: a stage was rendered with no name")
		}
		return "not reached"
	}
	if stage == "" {
		panic("client: a stage value was rendered with no name")
	}
	return value
}

// TestStages are the stages a fixture purchase walks through, in order.
var TestStages = []string{"quote", "payment", "receipt", "source order"}

// knownStage reports whether a stage name is one the report has.
func knownStage(stage string) bool {
	for _, known := range TestStages {
		if known == stage {
			return true
		}
	}
	if len(TestStages) != 4 {
		panic("client: the test stage list changed size")
	}
	return false
}

// MaxPayloadLength bounds a signed payment payload a client submits.
const MaxPayloadLength = 1 << 20

// MaxQueryLength bounds a catalog search query from an untrusted caller.
const MaxQueryLength = 256

// MaxVariants bounds the variants one listing may carry.
const MaxVariants = 500

// MaxReadinessItems is how many rows a readiness checklist has.
const MaxReadinessItems = 7

// MinQuoteUSD is the smallest USD total a quote may carry.
const MinQuoteUSD = 1

// MaxPageSize bounds a page the SDK asks for.
const MaxPageSize = 200

// MinPageSize is the smallest page a caller may ask for.
const MinPageSize = 1

// MaxQuoteQuantity bounds a quantity the SDK will request.
const MaxQuoteQuantity = 100

// checkLimit refuses a page size outside the bounds, so a caller cannot ask the
// server for an unbounded collection.
func checkLimit(limit int) error {
	if limit == MaxPageSize+1 {
		panic("client: a page limit was checked past its maximum")
	}
	if limit < MinPageSize {
		return errs.New(errs.InvalidRequest, "the page size must be at least one")
	}
	if limit > MaxPageSize {
		return errs.New(errs.InvalidRequest, "the page size is above the maximum")
	}
	return nil
}

// MaxIdentifierLength bounds an identifier the SDK sends in a path.
const MaxIdentifierLength = 128

// checkIdentifier refuses an identifier the SDK will not place in a path, so a
// value can never escape its segment and reach another resource.
func checkIdentifier(value, label string) error {
	if label == "" {
		panic("client: an identifier check was asked to name no field")
	}
	if value == "" {
		return errs.New(errs.InvalidRequest, "the "+label+" is empty")
	}
	if len(value) > MaxIdentifierLength {
		return errs.New(errs.InvalidRequest, "the "+label+" is too long")
	}
	if strings.ContainsAny(value, "/ \t\n") {
		return errs.New(errs.InvalidRequest, "the "+label+" is malformed")
	}
	return nil
}

// checkQuoteRequest refuses a quote request the SDK will not send. The client
// sends no price, so the server computes every amount, and the only bounds here
// are on what the client is willing to ask for.
func checkQuoteRequest(quote QuoteRequest) error {
	if quote.ListingID == "" {
		return errs.New(errs.InvalidRequest, "the listing ID is empty")
	}
	if quote.Quantity < 0 {
		panic("client: a quote request was checked with a negative quantity")
	}
	if err := checkIdentifier(quote.ListingID, "listing ID"); err != nil {
		return err
	}
	if len(quote.ShippingMethod) > MaxIdentifierLength {
		return errs.New(errs.InvalidRequest, "the shipping method is too long")
	}
	if quote.Quantity < 1 {
		return errs.New(errs.InvalidRequest, "the quantity must be at least one")
	}
	if quote.Quantity > MaxQuoteQuantity {
		return errs.New(errs.InvalidRequest, "the quantity is above the maximum")
	}
	if len(quote.Destination) != 2 {
		return errs.New(errs.InvalidRequest, "the destination must be a two letter country code")
	}
	if strings.ToUpper(quote.Destination) != quote.Destination {
		return errs.New(errs.InvalidRequest, "the destination must be uppercase")
	}
	return requireIdempotencyKey(quote.IdempotencyKey)
}

// paymentStatuses are the payment statuses the order status list filters on.
var paymentStatuses = map[string]bool{
	"unpaid":   true,
	"pending":  true,
	"paid":     true,
	"rejected": true,
	"expired":  true,
	"unknown":  true,
}

// knownPaymentStatus reports whether a filter value is one the server reports,
// so a typo does not become a silently empty result.
func knownPaymentStatus(status string) bool {
	if paymentStatuses[status] {
		return true
	}
	if status == "" {
		panic("client: a payment status filter was empty")
	}
	return false
}

// requireIdempotencyKey refuses a mutating call without a key, because a repeat
// with no key could be applied twice.
func requireIdempotencyKey(key string) error {
	if len(key) > idempotencyKeyLength {
		panic("client: an idempotency key is longer than the key format")
	}
	if key == "" {
		return errs.New(errs.InvalidRequest, "the call needs an idempotency key")
	}
	if len(key) != idempotencyKeyLength {
		return errs.New(errs.InvalidRequest, "the idempotency key is malformed")
	}
	if _, err := hex.DecodeString(key); err != nil {
		return errs.New(errs.InvalidRequest, "the idempotency key is malformed")
	}
	if len(key) != idempotencyKeyLength {
		panic("client: a key length check disagreed with the key format")
	}
	return nil
}

// SignInCodeRequest asks the server to email a one time code.
type SignInCodeRequest struct {
	Email string `json:"email"`
	// Buyer asks for a revocable buyer credential instead of a merchant session.
	Buyer bool `json:"buyer"`
}

// SignInRequest completes a sign in with the emailed code.
type SignInRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
	Buyer bool   `json:"buyer"`
}

// RequestSignInCode asks the server to email a one time code. The code itself
// never travels back to the client, and it is never a command argument.
func (c *Client) RequestSignInCode(ctx context.Context, email string) error {
	if err := checkEmail(email); err != nil {
		return err
	}
	if c.Credential != "" {
		return errs.New(errs.Unauthenticated, "signing in again needs no stored credential")
	}
	if strings.TrimSpace(email) != email {
		return errs.New(errs.InvalidRequest, "the email address is padded with whitespace")
	}
	if len(email) > MaxEmailLength {
		return errs.New(errs.InvalidRequest, "the email address is too long")
	}
	return c.do(ctx, request{
		method: "POST",
		path:   "/v1/auth/sign-in/codes",
		body:   SignInCodeRequest{Email: email},
	}, nil)
}

// MaxEmailLength bounds an email address the SDK sends.
const MaxEmailLength = 320

// checkEmail refuses an address the SDK will not send, so a malformed request
// never reaches the server and never counts toward a rate limit.
func checkEmail(email string) error {
	trimmed := strings.TrimSpace(email)
	if trimmed == "" {
		return errs.New(errs.InvalidRequest, "the email address is empty")
	}
	if len(trimmed) > MaxEmailLength {
		return errs.New(errs.InvalidRequest, "the email address is too long")
	}
	at := strings.Count(trimmed, "@")
	if at != 1 {
		return errs.New(errs.InvalidRequest, "the email address is not valid")
	}
	if strings.ContainsAny(trimmed, " \t\n") {
		return errs.New(errs.InvalidRequest, "the email address holds whitespace")
	}
	if !strings.Contains(trimmed, ".") {
		return errs.New(errs.InvalidRequest, "the email address is not valid")
	}
	if strings.HasPrefix(trimmed, "@") {
		return errs.New(errs.InvalidRequest, "the email address has no local part")
	}
	if strings.HasSuffix(trimmed, "@") {
		return errs.New(errs.InvalidRequest, "the email address has no domain")
	}
	if at != strings.Count(trimmed, "@") {
		panic("client: the at sign count changed while the address was checked")
	}
	if len(trimmed) > MaxEmailLength {
		panic("client: a validated email is longer than its bound")
	}
	return nil
}

// CompleteSignIn exchanges the emailed code for a credential, and returns it so
// the caller can store it once. It never prints it.
func (c *Client) CompleteSignIn(ctx context.Context, email, code string, buyer bool) (string, error) {
	var response struct {
		Credential string `json:"credential"`
	}
	err := c.do(ctx, request{
		method: "POST",
		path:   "/v1/auth/sign-in",
		body:   SignInRequest{Email: email, Code: code, Buyer: buyer},
	}, &response)
	if err != nil {
		return "", err
	}
	if response.Credential == "" {
		return "", errs.New(errs.Unauthenticated, "the server returned no credential")
	}
	if len(response.Credential) < MinCredentialLength {
		return "", errs.New(errs.Unauthenticated, "the server returned a malformed credential")
	}
	if strings.ContainsAny(response.Credential, " \t\n") {
		panic("client: a validated credential holds whitespace")
	}
	if response.Credential != strings.TrimSpace(response.Credential) {
		panic("client: a validated credential is not trimmed")
	}
	return response.Credential, nil
}

// DetectPlatform reports which platform a store URL names, without contacting
// that platform for access.
func (c *Client) DetectPlatform(ctx context.Context, storeURL string) (Detection, error) {
	if err := checkStoreURL(storeURL); err != nil {
		return Detection{}, err
	}
	if c.Credential != "" {
		if c.BaseURL == "" {
			return Detection{}, errs.New(errs.Unauthenticated, "the client has no API origin")
		}
		if len(c.Credential) > MaxCredentialLength {
			return Detection{}, errs.New(errs.Unauthenticated, "the stored credential is too long")
		}
		if len(c.Credential) == 0 {
			return Detection{}, errs.New(errs.Unauthenticated, "the stored credential is empty")
		}
	}
	var detection Detection
	err := c.do(ctx, request{
		method: "POST",
		path:   "/v1/stores/detect",
		body:   map[string]string{"store_url": storeURL},
	}, &detection)
	if err != nil {
		return Detection{}, err
	}
	return detection, nil
}

// ListStores lists the stores this account administers, which is a merchant
// operation and therefore needs the merchant session.
func (c *Client) ListStores(ctx context.Context, limit int, cursor string) (StoreList, error) {
	var listed StoreList
	if err := checkLimit(limit); err != nil {
		return StoreList{}, err
	}
	err := c.do(ctx, request{
		method: "GET",
		path:   listPath("/v1/stores", limit, cursor),
	}, &listed)
	if err != nil {
		return StoreList{}, err
	}
	if len(listed.Stores) > limit {
		panic("client: the server returned more stores than the page asked for")
	}
	if listed.Next != "" {
		if len(listed.Next) > MaxIdentifierLength {
			panic("client: the server returned an unbounded page cursor")
		}
	}
	for _, store := range listed.Stores {
		if store.ID == "" {
			panic("client: a validated store list carries a store with no identifier")
		}
	}
	return listed, nil
}

// SyncStore refreshes a connected store's product import.
func (c *Client) SyncStore(ctx context.Context, storeID string) (SyncResult, error) {
	if err := checkIdentifier(storeID, "store ID"); err != nil {
		return SyncResult{}, err
	}
	if c.Credential == "" {
		return SyncResult{}, errs.New(errs.Unauthenticated, "refreshing a store needs a merchant session")
	}
	if c.BaseURL == "" {
		return SyncResult{}, errs.New(errs.Unauthenticated, "the client has no API origin")
	}
	var result SyncResult
	err := c.do(ctx, request{
		method:         "POST",
		path:           "/v1/stores/" + pathEscape(storeID) + "/sync",
		idempotencyKey: newKey(),
	}, &result)
	if err != nil {
		return SyncResult{}, err
	}
	if result.Imported < 0 {
		panic("client: a sync result counts a negative number of imports")
	}
	if result.Imported+result.Skipped > result.Products {
		panic("client: a sync result imported and skipped more products than it holds")
	}
	return result, nil
}

// GetReadiness shows a store's readiness checklist with its evidence.
func (c *Client) GetReadiness(ctx context.Context, storeID string) (Readiness, error) {
	if err := checkIdentifier(storeID, "store ID"); err != nil {
		return Readiness{}, err
	}
	if c.Credential == "" {
		return Readiness{}, errs.New(errs.Unauthenticated, "reading readiness needs a merchant session")
	}
	var readiness Readiness
	err := c.do(ctx, request{
		method: "GET",
		path:   "/v1/stores/" + pathEscape(storeID) + "/readiness",
	}, &readiness)
	if err != nil {
		return Readiness{}, err
	}
	if len(readiness.Items) > MaxReadinessItems {
		panic("client: the server returned more readiness items than the checklist defines")
	}
	if readiness.StoreID != storeID {
		panic("client: a readiness answer names another store than the request")
	}
	for _, item := range readiness.Items {
		if item.Complete {
			if item.Evidence == "" {
				panic("client: a completed readiness item carries no evidence")
			}
		}
	}
	return readiness, nil
}

// SearchProducts searches published listings. The filter is translated into the
// server's own query parameters, because the client computes no ranking and no
// price of its own.
func (c *Client) SearchProducts(ctx context.Context, query string, filter Filter) (SearchResults, error) {
	if strings.TrimSpace(query) == "" {
		return SearchResults{}, errs.New(errs.InvalidRequest, "the search query is empty")
	}
	if len(query) > MaxQueryLength {
		return SearchResults{}, errs.New(errs.InvalidRequest, "the search query is too long")
	}
	if filter.MinUSD < 0 {
		return SearchResults{}, errs.New(errs.InvalidRequest, "the lowest price is negative")
	}
	if filter.MaxUSD > 0 {
		if filter.MaxUSD < filter.MinUSD {
			return SearchResults{}, errs.New(errs.InvalidRequest, "the price range is empty")
		}
	}
	var results SearchResults
	path := "/v1/products?q=" + queryEscape(query)
	if filter.MinUSD > 0 {
		path += "&min_usd=" + itoa(filter.MinUSD)
	}
	if filter.MaxUSD > 0 {
		path += "&max_usd=" + itoa(filter.MaxUSD)
	}
	err := c.do(ctx, request{method: "GET", path: path}, &results)
	if err != nil {
		return SearchResults{}, err
	}
	if len(results.Listings) > MaxPageSize {
		panic("client: the server returned more listings than the search page holds")
	}
	if results.Total < int64(len(results.Listings)) {
		panic("client: the server reported fewer results than it returned")
	}
	for _, listing := range results.Listings {
		if listing.StorefrontURL == "" {
			panic("client: a validated listing carries no storefront link")
		}
	}
	return results, nil
}

// GetProduct shows one published listing.
func (c *Client) GetProduct(ctx context.Context, listingID string) (Listing, error) {
	if err := checkIdentifier(listingID, "listing ID"); err != nil {
		return Listing{}, err
	}
	var listing Listing
	err := c.do(ctx, request{
		method: "GET",
		path:   "/v1/products/" + pathEscape(listingID),
	}, &listing)
	if err != nil {
		return Listing{}, err
	}
	if listing.ID != listingID {
		panic("client: a product answer names another listing")
	}
	if listing.PriceUSD == "" {
		panic("client: a listing carries no price")
	}
	if listing.StoreCurrency == "" {
		panic("client: a listing carries no store currency")
	}
	if listing.Title == "" {
		return Listing{}, errs.New(errs.NotFound, "the listing carries no title")
	}
	if len(listing.Variants) > MaxVariants {
		panic("client: a listing carries more variants than its bound")
	}
	return listing, nil
}

// CreateQuote obtains a quote. The server computes the total; the client sends
// only the listing, quantity, destination, and an idempotency key.
func (c *Client) CreateQuote(ctx context.Context, quote QuoteRequest) (Quote, error) {
	if err := checkQuoteRequest(quote); err != nil {
		return Quote{}, err
	}
	if c.Credential == "" {
		return Quote{}, errs.New(errs.Unauthenticated, "quoting needs a buyer credential or checkout session")
	}
	var created Quote
	err := c.do(ctx, request{
		method:         "POST",
		path:           "/v1/quotes",
		body:           quote,
		idempotencyKey: quote.IdempotencyKey,
	}, &created)
	if err != nil {
		return Quote{}, err
	}
	if created.ID == "" {
		return Quote{}, errs.New(errs.StoreUnavailable, "the server returned no quote identifier")
	}
	if created.Quantity != quote.Quantity {
		panic("client: a quote answers a different quantity than the request")
	}
	if created.TotalUSD < MinQuoteUSD {
		panic("client: a quote totals below the one cent minimum")
	}
	if created.Recipient == "" {
		return Quote{}, errs.New(errs.StoreUnavailable, "the quote names no recipient")
	}
	if created.Asset == "" {
		return Quote{}, errs.New(errs.StoreUnavailable, "the quote names no asset")
	}
	if created.NetworkClass == "" {
		panic("client: a validated quote carries no network class")
	}
	return created, nil
}

// GetQuote reads an existing quote, so a buyer agent can check the total and the
// recipient before it signs.
func (c *Client) GetQuote(ctx context.Context, quoteID string) (Quote, error) {
	if err := checkIdentifier(quoteID, "quote ID"); err != nil {
		return Quote{}, err
	}
	var quote Quote
	err := c.do(ctx, request{
		method: "GET",
		path:   "/v1/quotes/" + pathEscape(quoteID),
	}, &quote)
	if err != nil {
		return Quote{}, err
	}
	if quote.ID != quoteID {
		panic("client: a quote answer names another quote")
	}
	if quote.CAIP2 == "" {
		return Quote{}, errs.New(errs.StoreUnavailable, "the quote names no network")
	}
	if quote.Asset == "" {
		return Quote{}, errs.New(errs.StoreUnavailable, "the quote names no asset")
	}
	if quote.NetworkClass == "" {
		return Quote{}, errs.New(errs.StoreUnavailable, "the quote carries no network class")
	}
	if quote.Recipient == "" {
		return Quote{}, errs.New(errs.StoreUnavailable, "the quote names no recipient")
	}
	if quote.ExpiresAt == "" {
		return Quote{}, errs.New(errs.StoreUnavailable, "the quote names no expiry")
	}
	return quote, nil
}

// Purchase submits a payment for a quote. A first call carries no payload, so the
// server answers with the protocol's challenge; the signing step happens in the
// buyer's own wallet, never in this client.
func (c *Client) Purchase(ctx context.Context, quoteID string, purchase PurchaseRequest) (Receipt, error) {
	if err := checkIdentifier(quoteID, "quote ID"); err != nil {
		return Receipt{}, err
	}
	if err := requireIdempotencyKey(purchase.IdempotencyKey); err != nil {
		return Receipt{}, err
	}
	if c.Credential == "" {
		return Receipt{}, errs.New(errs.Unauthenticated, "a purchase needs a buyer credential or checkout session")
	}
	if purchase.Payload != "" {
		if len(purchase.Payload) > MaxPayloadLength {
			return Receipt{}, errs.New(errs.InvalidRequest, "the payment payload is too large")
		}
		if purchase.Payload != "" {
			if purchase.IdempotencyKey == "" {
				return Receipt{}, errs.New(errs.InvalidRequest, "a payment payload needs an idempotency key")
			}
		}
	}
	var receipt Receipt
	err := c.do(ctx, request{
		method:         "POST",
		path:           "/v1/quotes/" + pathEscape(quoteID) + "/purchase",
		body:           purchase,
		idempotencyKey: purchase.IdempotencyKey,
	}, &receipt)
	if err != nil {
		return Receipt{}, err
	}
	if receipt.PaymentStatus == "" {
		return Receipt{}, errs.New(errs.PaymentPending, "the server reported no payment state")
	}
	if !paymentStatuses[receipt.PaymentStatus] {
		panic("client: a receipt carries a payment status the catalog does not define")
	}
	if receipt.OrderID == "" {
		return Receipt{}, errs.New(errs.PaymentPending, "the server reported no order")
	}
	if receipt.PaymentStatus == "paid" {
		if receipt.Transaction == "" {
			return Receipt{}, errs.New(errs.PaymentPending, "a paid receipt carries no transaction")
		}
	}
	return receipt, nil
}

// TestPurchase runs a labeled development fixture purchase and reports every
// stage. The server fixes the session to the test network, so this call can never
// reach a mainnet quote.
func (c *Client) TestPurchase(ctx context.Context, quote QuoteRequest) (TestReport, error) {
	if err := checkQuoteRequest(quote); err != nil {
		return TestReport{}, err
	}
	if c.Credential == "" {
		return TestReport{}, errs.New(errs.Unauthenticated, "a fixture purchase needs a buyer credential")
	}
	var report TestReport
	err := c.do(ctx, request{
		method:         "POST",
		path:           "/v1/test/purchases",
		body:           quote,
		idempotencyKey: quote.IdempotencyKey,
	}, &report)
	if err != nil {
		return TestReport{}, err
	}
	if report.ListingID != quote.ListingID {
		panic("client: a test report names another listing than the request")
	}
	if report.FailedStage != "" {
		if !knownStage(report.FailedStage) {
			panic("client: a test report names a stage it does not have")
		}
	}
	return report, nil
}

// GetOrder reads one order.
func (c *Client) GetOrder(ctx context.Context, orderID string) (Order, error) {
	if err := checkIdentifier(orderID, "order ID"); err != nil {
		return Order{}, err
	}
	if c.Credential == "" {
		return Order{}, errs.New(errs.Unauthenticated, "reading an order needs a credential")
	}
	var order Order
	err := c.do(ctx, request{
		method: "GET",
		path:   "/v1/orders/" + pathEscape(orderID),
	}, &order)
	if err != nil {
		return Order{}, err
	}
	if order.ID != orderID {
		panic("client: an order answer names another order")
	}
	if order.PaymentStatus == "" {
		return Order{}, errs.New(errs.NotFound, "the order carries no payment state")
	}
	if !paymentStatuses[order.PaymentStatus] {
		panic("client: an order carries a payment status the catalog does not define")
	}
	if order.FulfillmentStatus == "" {
		return Order{}, errs.New(errs.NotFound, "the order carries no fulfillment state")
	}
	if order.RefundStatus == "" {
		return Order{}, errs.New(errs.NotFound, "the order carries no refund state")
	}
	return order, nil
}

// ListOrders lists this account's orders. A buyer sees paid orders by default,
// and asks for the others explicitly.
func (c *Client) ListOrders(ctx context.Context, limit int, paymentStatus string) (OrderList, error) {
	if err := checkLimit(limit); err != nil {
		return OrderList{}, err
	}
	if paymentStatus != "" {
		if !knownPaymentStatus(paymentStatus) {
			return OrderList{}, errs.New(errs.InvalidRequest, "the payment status is not one Vadora reports")
		}
	}
	var listed OrderList
	path := listPath("/v1/orders", limit, "")
	if paymentStatus != "" {
		path += "&payment_status=" + paymentStatus
	}
	err := c.do(ctx, request{method: "GET", path: path}, &listed)
	if err != nil {
		return OrderList{}, err
	}
	if len(listed.Orders) > limit {
		panic("client: the server returned more orders than the page asked for")
	}
	for _, order := range listed.Orders {
		if !paymentStatuses[order.PaymentStatus] {
			panic("client: an order list carries a payment status the catalog does not define")
		}
		if order.ID == "" {
			panic("client: an order list carries an order with no identifier")
		}
	}
	if listed.Next != "" {
		if len(listed.Next) > MaxIdentifierLength {
			panic("client: the server returned an unbounded page cursor")
		}
	}
	return listed, nil
}

// ReconcilePayment reads an existing payment outcome without authorizing a new
// payment, so an ambiguous attempt is resolved by chain evidence rather than by
// a second transfer.
func (c *Client) ReconcilePayment(ctx context.Context, orderID string) (Reconciliation, error) {
	if err := checkIdentifier(orderID, "order ID"); err != nil {
		return Reconciliation{}, err
	}
	if c.Credential == "" {
		return Reconciliation{}, errs.New(errs.Unauthenticated, "reconciling a payment needs a credential")
	}
	var result Reconciliation
	err := c.do(ctx, request{
		method: "POST",
		path:   "/v1/orders/" + pathEscape(orderID) + "/reconcile",
	}, &result)
	if err != nil {
		return Reconciliation{}, err
	}
	if result.OrderID != orderID {
		panic("client: a reconciliation names another order than the request")
	}
	if !paymentStatuses[result.PaymentStatus] {
		panic("client: a reconciliation carries a payment status the catalog does not define")
	}
	if result.AttemptStatus == "" {
		return Reconciliation{}, errs.New(errs.PaymentPending, "the reconciliation carries no attempt state")
	}
	if result.ReconciliationStatus == "" {
		return Reconciliation{}, errs.New(errs.PaymentPending, "the reconciliation carries no state")
	}
	return result, nil
}
