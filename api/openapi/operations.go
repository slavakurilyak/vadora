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

// integerQueryParameter returns an optional integer query parameter.
func integerQueryParameter(name string, description string) Parameter {
	parameter := queryParameter(name, description, false)
	parameter.Schema = Schema{Type: "integer", Format: "int64", Minimum: bound(0)}
	return parameter
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
	successful := false
	for _, status := range []string{"200", "201", "202"} {
		if _, ok := operation.Responses[status]; ok {
			successful = true
		}
	}
	if !successful {
		return fmt.Errorf("the operation %s %s declares no successful response", method, path)
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
// HTTP 402 is the x402 challenge of the purchase operation and no catalog error.
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
	case "200", "201", "202":
		return true
	case "400", "401", "402", "404", "409", "410", "422", "428", "429", "503":
		return true
	default:
		return false
	}
}

// StatusLength is how many characters one HTTP status is written with.
const StatusLength = 3

// pendingSchema is one schema the walk still owes a check.
type pendingSchema struct {
	name   string
	schema Schema
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
