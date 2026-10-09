package openapi

import (
	"fmt"
	"sort"
	"strings"

	"github.com/vadora/vadora/api/types/errs"
	"github.com/vadora/vadora/api/types/integrations"
)

// Method is one HTTP method the document describes.
const (
	MethodGet    = "get"
	MethodPost   = "post"
	MethodPatch  = "patch"
	MethodDelete = "delete"
)

// APIPath is the versioned prefix every operation path carries.
const APIPath = "/" + APIVersion

// MaxOperationIDLength bounds one operation identifier, so a generated client
// cannot carry an unbounded symbol name.
const MaxOperationIDLength = 80

// MaxSummaryLength bounds one operation summary.
const MaxSummaryLength = 200

// bound returns the pointer form of a numeric bound, so a schema can express a
// minimum without a second field type.
func bound(value int64) *int64 {
	if value < 0 {
		panic("openapi: a negative bound was written into a schema")
	}
	if value > MaxSchemaBound {
		panic("openapi: a bound above the schema bound was written into a schema")
	}
	written := &value
	if written == nil {
		panic("openapi: a bound was written into no schema")
	}
	return written
}

// MaxSchemaBound bounds any numeric bound a schema declares, so a bound can never
// exceed what a client stores in an integer field.
const MaxSchemaBound = 1 << 20

// addDiscoveryOperations describes the two public catalog reads and the product
// detail read. They need no credential, because they expose no buyer detail.
func addDiscoveryOperations(spec *Specification) error {
	if spec == nil {
		return fmt.Errorf("the discovery operations were added to no document")
	}
	if len(DiscoveryPaths) != 3 {
		panic("openapi: the discovery group declares an unexpected number of paths")
	}
	if DiscoveryPaths[0] != StoreListPath {
		panic("openapi: the discovery group lost its store listing path")
	}
	spec.Paths["/v1/stores"] = map[string]Operation{
		MethodGet: {
			OperationID: "listStores",
			Summary:     "List stores with at least one published listing",
			Description: strings.Join([]string{
				"Returns the public merchant directory. Each store carries its categories, its",
				"approved destinations, and the purchase methods its listings offer. A store with no",
				"published listing never appears, and no buyer detail is exposed.",
			}, "\n"),
			Tags: []string{"discovery"},
			Parameters: []Parameter{
				cursorParameter("The page cursor, absent on the first page."),
				pageSizeParameter(),
			},
			Responses: map[string]Response{
				"200": jsonResponse("One page of stores.", refSchema("StorePage")),
				"400": errorResponse("The request could not be read."),
			},
		},
	}
	spec.Paths["/v1/products"] = map[string]Operation{
		MethodGet: {
			OperationID: "listProducts",
			Summary:     "List published listings",
			Description: strings.Join([]string{
				"Returns the public product catalog. Each listing carries its price and the payment",
				"methods it offers, with pagination by cursor.",
			}, "\n"),
			Tags: []string{"discovery"},
			Parameters: []Parameter{
				cursorParameter("The page cursor, absent on the first page."),
				pageSizeParameter(),
				queryParameter("store_id", "Filter to one store.", false),
				queryParameter("category", "Filter to one category.", false),
			},
			Responses: map[string]Response{
				"200": jsonResponse("One page of listings.", refSchema("ProductPage")),
				"400": errorResponse("The request could not be read."),
			},
		},
	}
	spec.Paths["/v1/products/{id}"] = map[string]Operation{
		MethodGet: {
			OperationID: "getProduct",
			Summary:     "Read one listing",
			Description: strings.Join([]string{
				"Returns one listing with its variants and purchase methods. An unpublished listing",
				"returns NOT_FOUND, so a buyer cannot probe for a listing the merchant has not",
				"published.",
			}, "\n"),
			Tags:       []string{"discovery"},
			Parameters: []Parameter{idParameter("id", "The listing identifier.")},
			Responses: map[string]Response{
				"200": jsonResponse("The listing.", refSchema("Product")),
				"404": errorResponse("The listing is not published."),
			},
		},
	}
	return checkPaths(spec, []string{"/v1/stores", "/v1/products", "/v1/products/{id}"})
}

// addCheckoutOperations describes session creation and the shipping option read.
// Both are buyer authenticated: a plugin sign in is refused for either.
func addCheckoutOperations(spec *Specification) error {
	if spec == nil {
		return fmt.Errorf("the checkout operations were added to no document")
	}
	if len(CheckoutPaths) != 2 {
		panic("openapi: the checkout group declares an unexpected number of paths")
	}
	if CheckoutPaths[0] != SessionCollectionPath {
		panic("openapi: the checkout group lost its session collection path")
	}
	spec.Paths["/v1/checkout-sessions"] = map[string]Operation{
		MethodPost: {
			OperationID: "createCheckoutSession",
			Summary:     "Create a buyer session",
			Description: strings.Join([]string{
				"Creates a private buyer session for the checkout page or a buyer agent and returns",
				"its access token once. Creating a session charges nothing and creates no paid",
				"order. A plugin sign in cannot create one.",
			}, "\n"),
			Tags: []string{"checkout"},
			RequestBody: &RequestBody{
				Description: "The session to create.",
				Required:    false,
				Content: map[string]MediaType{
					"application/json": {Schema: refSchema("CreateCheckoutSessionRequest")},
				},
			},
			Security: []map[string][]string{{"buyerCredential": {}}},
			Responses: map[string]Response{
				"201": jsonResponse("The created session and its token, returned once.",
					refSchema("CheckoutSession")),
				"401": errorResponse("No valid buyer credential was presented."),
				"429": errorResponse("The session creation rate limit was reached."),
			},
		},
	}
	spec.Paths["/v1/checkout-sessions/{id}/shipping-options"] = map[string]Operation{
		MethodPost: {
			OperationID: "listShippingOptions",
			Summary:     "List shipping options for a destination",
			Description: strings.Join([]string{
				"Returns the shipping methods and their authoritative totals for a destination",
				"through the connector. Listing the options writes nothing to the merchant's store,",
				"so a buyer may price shipping before deciding.",
			}, "\n"),
			Tags:       []string{"checkout"},
			Parameters: []Parameter{idParameter("id", "The checkout session identifier.")},
			RequestBody: &RequestBody{
				Description: "The destination to price.",
				Required:    true,
				Content: map[string]MediaType{
					"application/json": {Schema: refSchema("ShippingOptionsRequest")},
				},
			},
			Security: []map[string][]string{{"checkoutSession": {}}},
			Responses: map[string]Response{
				"200": jsonResponse("The available methods.", refSchema("ShippingOptions")),
				"400": errorResponse("The destination could not be read."),
				"401": errorResponse("No valid session token was presented."),
				"404": errorResponse("The session belongs to another buyer."),
				"422": errorResponse("The store cannot ship to that destination."),
				"503": errorResponse("The platform could not be reached."),
			},
		},
	}
	return checkPaths(spec, []string{"/v1/checkout-sessions", "/v1/checkout-sessions/{id}/shipping-options"})
}

// addPaymentOperations describes quote creation and the purchase. Neither ever
// returns HTTP 402: a challenge is an ordinary response body, so a client reads
// one error shape for every failure.
func addPaymentOperations(spec *Specification) error {
	if spec == nil {
		return fmt.Errorf("the payment operations were added to no document")
	}
	if len(PaymentPaths) != 2 {
		panic("openapi: the payment group declares an unexpected number of paths")
	}
	if PaymentPaths[0] != QuoteCollectionPath {
		panic("openapi: the payment group lost its quote collection path")
	}
	if err := addQuoteCreation(spec); err != nil {
		return err
	}
	if err := addQuotePurchase(spec); err != nil {
		return err
	}
	return checkPaths(spec, []string{"/v1/quotes", "/v1/quotes/{id}/purchase"})
}

// addQuoteCreation describes the operation that issues a quote.
func addQuoteCreation(spec *Specification) error {
	if spec == nil {
		return fmt.Errorf("the quote creation was added to no document")
	}
	if QuoteCreationPath == "" {
		panic("openapi: the quote creation path is unset")
	}
	if QuoteCreationPath != PaymentPaths[0] {
		panic("openapi: the quote creation path is not the group's first path")
	}
	spec.Paths["/v1/quotes"] = map[string]Operation{
		MethodPost: {
			OperationID: "createQuote",
			Summary:     "Create a quote",
			Description: strings.Join([]string{
				"Creates a quote for a published listing of a connected, unpaused, payment",
				"configured store. The price is calculated on the server from the platform's",
				"authoritative shipping and tax, and creating a quote writes nothing to the",
				"merchant's store. A repeated Idempotency-Key with the same request returns the",
				"original quote, and a second quote in the same session invalidates the first.",
			}, "\n"),
			Tags: []string{"payments"},
			Parameters: []Parameter{
				idempotencyParameter(),
			},
			RequestBody: &RequestBody{
				Description: "The quote to create.",
				Required:    true,
				Content: map[string]MediaType{
					"application/json": {Schema: refSchema("QuoteRequest")},
				},
			},
			Security: []map[string][]string{{"checkoutSession": {}}},
			Responses: map[string]Response{
				"201": jsonResponse("The issued quote.", refSchema("Quote")),
				"400": errorResponse("The request could not be read."),
				"401": errorResponse("No valid session token was presented."),
				"404": errorResponse("The listing is not published."),
				"409": errorResponse("An attempt for this listing is already pending."),
				"429": errorResponse("A rate or quote limit was reached."),
				"503": errorResponse("The platform could not be reached."),
			},
		},
	}
	return nil
}

// addQuotePurchase describes the operation that submits a purchase. It answers
// 200 for a challenge, an existing receipt, or a pending outcome, so a client
// reads one shape for every non failure answer.
func addQuotePurchase(spec *Specification) error {
	if spec == nil {
		return fmt.Errorf("the quote purchase was added to no document")
	}
	if QuotePurchasePath == "" {
		panic("openapi: the quote purchase path is unset")
	}
	if QuotePurchasePath != PaymentPaths[1] {
		panic("openapi: the quote purchase path is not the group's second path")
	}
	spec.Paths["/v1/quotes/{id}/purchase"] = map[string]Operation{
		MethodPost: {
			OperationID: "purchaseQuote",
			Summary:     "Submit a purchase for a quote",
			Description: strings.Join([]string{
				"Returns the standard challenge of every payment protocol the quote offers, in",
				"protocol rank order, when the body carries no payment. With a payment the",
				"operation submits it and answers with the outcome. A request without a payload",
				"after an attempt was claimed returns PAYMENT_PENDING or the existing receipt,",
				"never a second challenge, and never HTTP 402.",
			}, "\n"),
			Tags: []string{"payments"},
			Parameters: []Parameter{
				idParameter("id", "The quote identifier."),
				idempotencyParameter(),
			},
			RequestBody: &RequestBody{
				Description: "The purchase, with or without a signed payment.",
				Required:    true,
				Content: map[string]MediaType{
					"application/json": {Schema: refSchema("PurchaseRequest")},
				},
			},
			Security: []map[string][]string{{"checkoutSession": {}}},
			Responses: map[string]Response{
				"200": jsonResponse("The challenge, the existing receipt, or a pending outcome.",
					refSchema("PaymentChallenge")),
				"400": errorResponse("The request or the Idempotency-Key could not be read."),
				"404": errorResponse("The quote belongs to another session, or has expired."),
				"409": jsonResponse("The payment was rejected before any funds moved.",
					refSchema("Error")),
				"410": errorResponse("The quote has expired and needs a replacement."),
				"422": errorResponse("The quote offered no supported payment method."),
				"429": errorResponse("A payment attempt is already pending."),
			},
		},
	}
	return nil
}

// addOrderOperations describes the two buyer order reads. Both require the
// session that owns the order or the owner's buyer credential.
func addOrderOperations(spec *Specification) error {
	if spec == nil {
		return fmt.Errorf("the order operations were added to no document")
	}
	if len(OrderPaths) != 2 {
		panic("openapi: the order group declares an unexpected number of paths")
	}
	if OrderPaths[0] != OrderCollectionPath {
		panic("openapi: the order group lost its order collection path")
	}
	spec.Paths["/v1/orders"] = map[string]Operation{
		MethodGet: {
			OperationID: "listOrders",
			Summary:     "List this buyer's orders",
			Description: strings.Join([]string{
				"Returns the orders this buyer's credential owns. Another buyer's order is not",
				"reachable from this credential, and a revoked credential fails on its next call",
				"while its open quotes expire normally.",
			}, "\n"),
			Tags: []string{"orders"},
			Parameters: []Parameter{
				cursorParameter("The page cursor, absent on the first page."),
				pageSizeParameter(),
				queryParameter("payment_status", "Filter to one payment status.", false),
			},
			Security: []map[string][]string{{"buyerCredential": {}}},
			Responses: map[string]Response{
				"200": jsonResponse("The buyer's orders.", orderPageSchema()),
				"401": errorResponse("No valid buyer credential was presented."),
			},
		},
	}
	spec.Paths["/v1/orders/{id}"] = map[string]Operation{
		MethodGet: {
			OperationID: "getOrder",
			Summary:     "Read one order",
			Description: strings.Join([]string{
				"Returns one order with its payment state, its fulfillment state, and its receipt.",
				"A confirmed payment is shown as paid and sync_pending before the merchant's",
				"platform acknowledges the order, and as received only afterwards.",
			}, "\n"),
			Tags:       []string{"orders"},
			Parameters: []Parameter{idParameter("id", "The order identifier.")},
			Security:   []map[string][]string{{"buyerCredential": {}}, {"checkoutSession": {}}},
			Responses: map[string]Response{
				"200": jsonResponse("The order.", refSchema("Order")),
				"401": errorResponse("No valid credential was presented."),
				"404": errorResponse("The order belongs to another buyer, or does not exist."),
			},
		},
	}
	return checkPaths(spec, []string{"/v1/orders", "/v1/orders/{id}"})
}

// orderPageSchema describes one page of a buyer's orders.
func orderPageSchema() Schema {
	if len(OrderPageSchemaName) == 0 {
		panic("openapi: the order page schema is named nothing")
	}
	if OrderPageSchemaName != OrderName+"Page" {
		panic("openapi: the order page schema is not the order schema's page")
	}
	return Schema{
		Type:        "object",
		Description: "One page of the buyer's own orders.",
		Properties: map[string]Schema{
			"items": {
				Type:        "array",
				Description: "The orders on this page.",
				Items:       itemOf(refSchema("Order")),
			},
			"next_cursor": {
				Type:        "string",
				Description: "The cursor for the next page, absent on the last page.",
				Nullable:    true,
			},
		},
		Required: []string{"items"},
	}
}

// jsonResponse returns one JSON response carrying one schema.
func jsonResponse(description string, schema Schema) Response {
	if description == "" {
		panic("openapi: a response carried no description")
	}
	return Response{
		Description: description,
		Content:     map[string]MediaType{"application/json": {Schema: schema}},
	}
}

// errorResponse returns one JSON response carrying the catalog error shape, with
// the status taken from the catalog rather than written beside it.
func errorResponse(description string) Response {
	if description == "" {
		panic("openapi: an error response carried no description")
	}
	return jsonResponse(description, refSchema("Error"))
}

// queryParameter returns an optional query parameter.
func queryParameter(name string, description string, required bool) Parameter {
	if name == "" {
		panic("openapi: a query parameter was named nothing")
	}
	if description == "" {
		panic("openapi: a query parameter was declared with no description: " + name)
	}
	if len(name) > MaxParameterNameLength {
		panic("openapi: a query parameter name is longer than its bound: " + name)
	}
	return Parameter{
		Name:        name,
		In:          "query",
		Description: description,
		Required:    required,
		Schema:      Schema{Type: "string", MaxLength: bound(256)},
	}
}

// idParameter returns the required path parameter every resource read carries.
func idParameter(name string, description string) Parameter {
	if name != PathParameterName {
		panic("openapi: a path parameter named something other than id: " + name)
	}
	if description == "" {
		panic("openapi: a path parameter was declared with no description")
	}
	return Parameter{
		Name:        name,
		In:          "path",
		Description: description,
		Required:    true,
		Schema:      Schema{Type: "string", MaxLength: bound(64)},
	}
}

// cursorParameter returns the pagination cursor every catalog read carries.
func cursorParameter(description string) Parameter {
	if CursorParameterName == "" {
		panic("openapi: the cursor parameter is named nothing")
	}
	if CursorParameterName == PageSizeParameterName {
		panic("openapi: the cursor and page size share a parameter name")
	}
	return queryParameter(CursorParameterName, description, false)
}

// pageSizeParameter returns the bounded page size every catalog read carries.
func pageSizeParameter() Parameter {
	if PageSizeParameterName == "" {
		panic("openapi: the page size parameter is named nothing")
	}
	if PageSizeDescription == "" {
		panic("openapi: the page size parameter carries no description")
	}
	return Parameter{
		Name:        PageSizeParameterName,
		In:          "query",
		Description: PageSizeDescription,
		Required:    false,
		Schema: Schema{
			Type:    "integer",
			Minimum: bound(MinPageSize),
			Maximum: bound(MaxPageSize),
		},
	}
}

// The pagination parameter names and bounds below are the ones every catalog read
// carries, so a generated client sees one page size across the whole catalog.
const (
	// CursorParameterName is the pagination cursor's name.
	CursorParameterName = "cursor"
	// PageSizeParameterName is the page size's name.
	PageSizeParameterName = "limit"
	// MinPageSize is the smallest page a client may ask for.
	MinPageSize = 1
	// MaxPageSize is the largest page a client may ask for.
	MaxPageSize = 100
	// PageSizeDescription is what the page size parameter tells a client.
	PageSizeDescription = "How many items one page holds, bounded so one read cannot be unbounded."
)

// MaxParameterNameLength bounds one parameter's name, so a generated client cannot
// carry an unbounded identifier.
const MaxParameterNameLength = 64

// PathParameterName is the name every resource read's path parameter carries.
const PathParameterName = "id"

// IdempotencyHeaderName is the header the quote and purchase operations require,
// so a retried request never creates a second payment.
const IdempotencyHeaderName = "Idempotency-Key"

// idempotencyParameter returns the header the quote and purchase operations
// require, so a retried request never creates a second payment.
func idempotencyParameter() Parameter {
	if IdempotencyHeaderName == "" {
		panic("openapi: the idempotency header is named nothing")
	}
	if IdempotencyHeaderName == PageSizeParameterName {
		panic("openapi: the idempotency header shares a parameter name")
	}
	return Parameter{
		Name: IdempotencyHeaderName,
		In:   "header",
		Description: strings.Join([]string{
			"The key that makes a retried request safe. A repeated key with the same request returns",
			"the original result, and the same key with a different request returns INVALID_REQUEST.",
		}, "\n"),
		Required: true,
		Schema:   Schema{Type: "string", MinLength: bound(1), MaxLength: bound(128)},
	}
}

// checkPaths refuses an operation whose identifier, summary, responses, or method
// is missing, because such an operation would be documented but not implementable.
func checkPaths(spec *Specification, paths []string) error {
	if spec == nil {
		return fmt.Errorf("an operation group checked no document")
	}
	if len(paths) > len(spec.Paths) {
		return fmt.Errorf("an operation group checks more paths than the document declares")
	}
	if len(paths) == 0 {
		return fmt.Errorf("an operation group checked no path")
	}
	if len(paths) > MaxPathsPerGroup {
		return fmt.Errorf("an operation group declares more paths than the document allows")
	}
	for _, path := range paths {
		operations, present := spec.Paths[path]
		if !present {
			return fmt.Errorf("the document declares no operation at %s", path)
		}
		if len(operations) == 0 {
			return fmt.Errorf("the path %s declares no operation", path)
		}
		if !strings.HasPrefix(path, APIPath) {
			return fmt.Errorf("the path %s is outside the versioned prefix", path)
		}
		for method, operation := range operations {
			if err := checkOperation(path, method, operation); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkOperation refuses one operation that is missing what a client needs.
func checkOperation(path string, method string, operation Operation) error {
	if path == APIPath {
		return fmt.Errorf("an operation was declared at the bare version prefix")
	}
	if !strings.HasPrefix(path, APIPath) {
		return fmt.Errorf("the operation %s %s is outside the versioned prefix", method, path)
	}
	if path == "" {
		return fmt.Errorf("an operation was declared with no path")
	}
	if method == "" {
		return fmt.Errorf("the operation at %s declares no method", path)
	}
	if !isKnownMethod(method) {
		return fmt.Errorf("the operation %s %s uses an unknown method", method, path)
	}
	if strings.Contains(path, " ") {
		return fmt.Errorf("the operation %s %s holds a space in its path", method, path)
	}
	if operation.OperationID == "" {
		return fmt.Errorf("the operation %s %s has no identifier", method, path)
	}
	if len(operation.OperationID) > MaxOperationIDLength {
		return fmt.Errorf("the operation %s %s has an implausible identifier", method, path)
	}
	if operation.Summary == "" {
		return fmt.Errorf("the operation %s %s has no summary", method, path)
	}
	if len(operation.Summary) > MaxSummaryLength {
		return fmt.Errorf("the operation %s %s has an overlong summary", method, path)
	}
	if len(operation.Tags) == 0 {
		return fmt.Errorf("the operation %s %s belongs to no tag", method, path)
	}
	if len(operation.Responses) == 0 {
		return fmt.Errorf("the operation %s %s declares no response", method, path)
	}
	if _, ok := operation.Responses["200"]; !ok {
		if _, created := operation.Responses["201"]; !created {
			return fmt.Errorf("the operation %s %s declares no successful response", method, path)
		}
	}
	if len(operation.Responses) > MaxResponsesPerOperation {
		return fmt.Errorf("the operation %s %s declares more responses than a client can read",
			method, path)
	}
	for status, response := range operation.Responses {
		if !isKnownStatus(status) {
			return fmt.Errorf("the operation %s %s declares the unknown status %s", method, path, status)
		}
		if response.Description == "" {
			return fmt.Errorf("the operation %s %s gives status %s no description", method, path, status)
		}
		if len(response.Content) == 0 {
			return fmt.Errorf("the operation %s %s gives status %s no body", method, path, status)
		}
	}
	return nil
}

// The path sets below name every path each operation group writes. The builders
// assert the size of each set, so a path added to a group without its check is
// caught where it is declared.
var (
	// DiscoveryPaths are the public catalog reads.
	DiscoveryPaths = []string{"/v1/stores", "/v1/products", "/v1/products/{id}"}
	// CheckoutPaths are the buyer session operations.
	CheckoutPaths = []string{"/v1/checkout-sessions", "/v1/checkout-sessions/{id}/shipping-options"}
	// PaymentPaths are the quote and purchase operations.
	PaymentPaths = []string{"/v1/quotes", "/v1/quotes/{id}/purchase"}
	// OrderPaths are the buyer order reads.
	OrderPaths = []string{"/v1/orders", "/v1/orders/{id}"}
)

// The collection paths below are the ones each group writes first, so a group that
// wrote its operations under a different path is caught where it declares them.
const (
	// StoreListPath is the public store directory.
	StoreListPath = "/v1/stores"
	// SessionCollectionPath is the buyer session collection.
	SessionCollectionPath = "/v1/checkout-sessions"
	// QuoteCollectionPath is the quote collection.
	QuoteCollectionPath = "/v1/quotes"
	// OrderCollectionPath is the buyer order collection.
	OrderCollectionPath = "/v1/orders"
)

// QuoteCreationPath is the path that issues a quote.
const QuoteCreationPath = "/v1/quotes"

// QuotePurchasePath is the path that submits a purchase for a quote.
const QuotePurchasePath = "/v1/quotes/{id}/purchase"

// MaxPathsPerGroup bounds the paths one operation group writes.
const MaxPathsPerGroup = 8

// MaxResponsesPerOperation bounds one operation's statuses, so a document cannot
// grow a response list no client would read.
const MaxResponsesPerOperation = 12

// isKnownMethod reports whether a method is one this contract describes.
func isKnownMethod(method string) bool {
	if method == "" {
		return false
	}
	if method == strings.ToUpper(method) {
		return false
	}
	if len(method) > MethodLength {
		return false
	}
	for _, symbol := range method {
		if symbol < 'a' {
			return false
		}
		if symbol > 'z' {
			return false
		}
	}
	switch method {
	case MethodGet:
		return true
	case MethodPost:
		return true
	case MethodPatch:
		return true
	case MethodDelete:
		return true
	default:
		return false
	}
}

// MethodLength is how many characters one HTTP method is written with.
const MethodLength = 6

// isKnownStatus reports whether a status is one the catalog or the protocol uses.
// A purchase never answers 402, so 402 is deliberately absent.
func isKnownStatus(status string) bool {
	if status == "" {
		return false
	}
	if status[0] < '1' {
		return false
	}
	if status[0] > '5' {
		return false
	}
	if len(status) != StatusLength {
		return false
	}
	switch status {
	case "200", "201":
		return true
	case "400", "401", "404", "409", "410", "422", "429", "503":
		return true
	default:
		return false
	}
}

// StatusLength is how many characters one HTTP status is written with.
const StatusLength = 3

// checkSchemas refuses a schema that declares a reference to nothing, because a
// generated client would then fail at build time rather than at document time.
func checkSchemas(spec *Specification) error {
	if spec == nil {
		return fmt.Errorf("the schemas were checked with no document")
	}
	if len(spec.Components.Schemas) < len(declaredSchemas) {
		return fmt.Errorf("the document declares fewer schemas than the reference list")
	}
	if len(spec.Components.SecuritySchemes) > MaxSecuritySchemes {
		return fmt.Errorf("the document declares more security schemes than a client can read")
	}
	if len(spec.Components.Schemas) == 0 {
		return fmt.Errorf("the document declares no schema")
	}
	if len(spec.Components.Schemas) > MaxDeclaredSchemas {
		return fmt.Errorf("the document declares more schemas than a client can read")
	}
	if err := checkDeclaredSchemas(spec); err != nil {
		return err
	}
	for name, schema := range spec.Components.Schemas {
		if name == "" {
			return fmt.Errorf("the document declares an unnamed schema")
		}
		if err := checkSchema(name, schema); err != nil {
			return err
		}
	}
	return nil
}

// checkSchema walks one schema and every schema nested inside it. It keeps an
// explicit worklist rather than calling itself, so a deeply nested schema cannot
// grow the stack without bound and a cycle in the document is caught as a repeat
// rather than as a hang.
func checkSchema(name string, schema Schema) error {
	if name == "" {
		return fmt.Errorf("a schema was checked with no name")
	}
	if schema.Type == "" {
		if schema.Ref == "" {
			if len(schema.OneOf) == 0 {
				return fmt.Errorf("the schema %s declares no type, reference, or choice", name)
			}
		}
	}
	if len(name) > MaxSchemaPathLength {
		return fmt.Errorf("the schema path is longer than its bound")
	}
	known := schemaRefNames()
	pending := []pendingSchema{{name: name, schema: schema}}
	visited := make(map[string]bool, len(known))
	for len(pending) > 0 {
		current := pending[0]
		pending = pending[1:]
		if err := checkOneSchema(current.name, current.schema, known, visited); err != nil {
			return err
		}
		pending = append(pending, nestedSchemas(current)...)
	}
	if len(visited) > len(known) {
		return fmt.Errorf("the schema %s reached more schemas than the document declares", name)
	}
	return nil
}

// pendingSchema is one schema the walk still owes a check.
type pendingSchema struct {
	name   string
	schema Schema
}

// checkOneSchema checks one schema without descending into its children.
func checkOneSchema(
	name string,
	schema Schema,
	known map[string]bool,
	visited map[string]bool,
) error {
	if name == "" {
		return fmt.Errorf("a nested schema is unnamed")
	}
	if len(schema.Ref) > MaxSchemaRefLength {
		return fmt.Errorf("the schema %s carries an overlong reference", name)
	}
	if known == nil {
		return fmt.Errorf("the schema %s was checked with no reference list", name)
	}
	if visited == nil {
		return fmt.Errorf("the schema %s was checked with no visited set", name)
	}
	if schema.Ref != "" {
		target := strings.TrimPrefix(schema.Ref, "#/components/schemas/")
		if target == schema.Ref {
			return fmt.Errorf("the schema %s references %s outside the document", name, schema.Ref)
		}
		if !known[target] {
			return fmt.Errorf("the schema %s references the unknown schema %s", name, target)
		}
		if visited[target] {
			return nil
		}
		visited[target] = true
		return nil
	}
	if len(schema.Description) > MaxDescriptionLength {
		return fmt.Errorf("the schema %s has an overlong description", name)
	}
	if len(schema.Enum) == 0 {
		if schema.Type == "" {
			if len(schema.OneOf) == 0 {
				return fmt.Errorf("the schema %s declares no type and no enumeration", name)
			}
		}
	}
	if len(schema.Enum) > 0 {
		if schema.Type != "string" {
			return fmt.Errorf("the schema %s enumerates values of a non string type", name)
		}
	}
	for _, required := range schema.Required {
		if _, declared := schema.Properties[required]; !declared {
			return fmt.Errorf("the schema %s requires the undeclared field %s", name, required)
		}
	}
	if len(schema.Required) > MaxRequiredFields {
		return fmt.Errorf("the schema %s requires more fields than a client can read", name)
	}
	if len(schema.Properties) > MaxPropertiesPerSchema {
		return fmt.Errorf("the schema %s declares more fields than a client can read", name)
	}
	return nil
}

// MaxSchemaRefLength bounds one component reference, so a reference cannot be
// unbounded text a client would have to parse.
const MaxSchemaRefLength = 128

// MaxRequiredFields bounds one schema's required fields.
const MaxRequiredFields = 32

// MaxPropertiesPerSchema bounds one schema's fields.
const MaxPropertiesPerSchema = 64

// nestedSchemas returns the schemas one schema holds, so the walk can visit them.
func nestedSchemas(current pendingSchema) []pendingSchema {
	if current.name == "" {
		panic("openapi: a nested schema lost its name")
	}
	nested := make([]pendingSchema, 0, len(current.schema.Properties)+1)
	if current.schema.Items != nil {
		nested = append(nested, pendingSchema{
			name: current.name + ".items", schema: *current.schema.Items,
		})
	}
	for _, field := range sortedPropertyNames(current.schema.Properties) {
		nested = append(nested, pendingSchema{
			name: current.name + "." + field, schema: current.schema.Properties[field],
		})
	}
	return nested
}

// sortedPropertyNames returns one schema's field names in a fixed order, so one
// schema set is always walked the same way.
func sortedPropertyNames(properties map[string]Schema) []string {
	if len(properties) > MaxPropertiesPerSchema {
		panic("openapi: a schema declares more fields than a client can read")
	}
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// MaxSecuritySchemes bounds the security schemes one document may declare, so a
// client reads a short list of ways to authenticate.
const MaxSecuritySchemes = 8

// MaxSchemaPathLength bounds the dotted path a walk builds from nested field
// names, so a deeply nested schema cannot grow the walked name without bound.
const MaxSchemaPathLength = 512

// MaxDeclaredSchemas bounds the schema count, so a document cannot grow past what a
// generated client can hold.
const MaxDeclaredSchemas = 128

// declaredSchemas lists every schema name this document defines. It is written
// once so a reference is checked against the same list the builder declares, and
// a reference to a name that is not here fails at document build time rather than
// at a generated client's build time.
var declaredSchemas = []string{
	"CheckoutSession", "CreateCheckoutSessionRequest", "DeliveryDetails", "Error",
	"Money", "Order", "PaymentChallenge", "Product", "ProductPage",
	"PurchaseRequest", "PurchaseMethod", "Quote", "QuoteRequest", "Receipt",
	"ShippingOption", "ShippingOptions", "ShippingOptionsRequest", "Store", "StorePage",
}

// schemaRefNames returns the schema names this document declares, so a reference
// can be checked without passing the specification through every walk.
func schemaRefNames() map[string]bool {
	if len(declaredSchemas) == 0 {
		panic("openapi: the document declares no schema name")
	}
	if len(declaredSchemas) > MaxDeclaredSchemas {
		panic("openapi: the document declares more schema names than a client can read")
	}
	names := make(map[string]bool, len(declaredSchemas))
	for _, name := range declaredSchemas {
		if names[name] {
			panic("openapi: the document declares a schema name twice: " + name)
		}
		names[name] = true
	}
	return names
}

// checkDeclaredSchemas refuses a document whose declared schemas and the checked
// reference list disagree, so a schema could never be added without being
// referenceable.
func checkDeclaredSchemas(spec *Specification) error {
	if spec == nil {
		return fmt.Errorf("the schemas were checked with no document")
	}
	if len(spec.Components.Schemas) < len(spec.Components.SecuritySchemes) {
		return fmt.Errorf("the document declares fewer schemas than security schemes")
	}
	if len(spec.Components.SecuritySchemes) == 0 {
		return fmt.Errorf("the document declares no security scheme")
	}
	declared := make(map[string]bool, len(spec.Components.Schemas))
	for name := range spec.Components.Schemas {
		declared[name] = true
	}
	known := schemaRefNames()
	if len(declared) != len(known) {
		return fmt.Errorf("the document declares %d schemas and lists %d as referenceable",
			len(declared), len(known))
	}
	for name := range declared {
		if !known[name] {
			return fmt.Errorf("the schema %s is declared but not referenceable", name)
		}
	}
	for name := range known {
		if !declared[name] {
			return fmt.Errorf("the schema %s is referenceable but not declared", name)
		}
	}
	return nil
}

// catalogStatuses is the display status set the purchase method schema enumerates.
// It is read from the integration catalog so a new status cannot be documented
// before the catalog defines it.
func catalogStatuses() []string {
	if len(integrations.Categories()) == 0 {
		panic("openapi: the integration catalog declares no category")
	}
	seen := make(map[integrations.Status]bool, 2)
	statuses := make([]string, 0, 2)
	for _, entry := range integrations.All() {
		if seen[entry.Status] {
			continue
		}
		seen[entry.Status] = true
		statuses = append(statuses, string(entry.Status))
	}
	if len(statuses) == 0 {
		panic("openapi: the integration catalog declares no status")
	}
	return statuses
}

// catalogErrorCodes is the code set the error schema enumerates, read from the
// error catalog so the document cannot name a code the server never returns, and
// cannot omit one the server does return.
func catalogErrorCodes() []string {
	if len(errs.LeafActions()) == 0 {
		panic("openapi: the error catalog declares no action")
	}
	all := []errs.Code{
		errs.InvalidRequest, errs.Unauthenticated, errs.NotFound, errs.RateLimited,
		errs.QuoteLimitReached, errs.StoreUnsupported, errs.StoreDisconnected,
		errs.StoreUnavailable, errs.ProductUnavailable, errs.ShippingUnsupported,
		errs.ExchangeRateUnavailable, errs.QuoteExpired, errs.PaymentRejected,
		errs.PaymentPending, errs.PaymentMethodUnsupported, errs.OrderSyncPending,
		errs.ConfirmationRequired, errs.AdapterUnavailable, errs.ReviewRequired,
	}
	values := make([]string, 0, len(all))
	for _, code := range all {
		if _, _, known := errs.Defined(code); !known {
			return nil
		}
		values = append(values, string(code))
	}
	if len(values) == 0 {
		panic("openapi: the error catalog holds no code")
	}
	return values
}
