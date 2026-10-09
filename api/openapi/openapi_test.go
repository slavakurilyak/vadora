package openapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/vadora/vadora/api/types/errs"
)

// The document builds from the catalog and the domain types, with no edit needed.
func TestBuildProducesTheContract(t *testing.T) {
	t.Parallel()
	spec, err := Build()
	if err != nil {
		t.Fatalf("the document did not build: %v", err)
	}
	if spec.OpenAPI != DocumentVersion {
		t.Fatalf("the document declares OpenAPI %q, want %q", spec.OpenAPI, DocumentVersion)
	}
	if len(spec.Paths) != len(implementedPaths) {
		t.Fatalf("the document declares %d paths, want the %d implemented ones",
			len(spec.Paths), len(implementedPaths))
	}
	for _, path := range implementedPaths {
		if _, present := spec.Paths[path]; !present {
			t.Fatalf("the document declares no operation at %s", path)
		}
	}
}

// implementedPaths is every REST operation the specifications name. A path here
// that the document does not declare, or a path there that is not here, means the
// document and the specifications disagree.
var implementedPaths = []string{
	"/v1/auth/sign-in/codes",
	"/v1/auth/sign-in",
	"/v1/stores",
	"/v1/products",
	"/v1/products/{id}",
	"/v1/checkout-sessions",
	"/v1/checkout-sessions/{id}/shipping-options",
	"/v1/quotes",
	"/v1/quotes/{id}",
	"/v1/quotes/{id}/purchase",
	"/v1/orders",
	"/v1/orders/{id}",
}

// Every declared path is one the specifications name, and every path the
// specifications name is declared.
func TestEveryPathIsImplementedAndOnlyThose(t *testing.T) {
	t.Parallel()
	spec, err := Build()
	if err != nil {
		t.Fatalf("the document did not build: %v", err)
	}
	declared := make(map[string]bool, len(spec.Paths))
	for path := range spec.Paths {
		declared[path] = true
	}
	for _, path := range implementedPaths {
		if !declared[path] {
			t.Fatalf("the document declares no operation at %s", path)
		}
	}
	if len(declared) != len(implementedPaths) {
		t.Fatalf("the document declares %d paths, want exactly %d",
			len(declared), len(implementedPaths))
	}
}

// Public discovery needs no credential, because it exposes no buyer detail.
func TestDiscoveryOperationsArePublic(t *testing.T) {
	t.Parallel()
	spec, err := Build()
	if err != nil {
		t.Fatalf("the document did not build: %v", err)
	}
	public := []string{"/v1/stores", "/v1/products", "/v1/products/{id}"}
	for _, path := range public {
		operation, present := spec.Paths[path][MethodGet]
		if !present {
			t.Fatalf("%s declares no read operation", path)
		}
		if len(operation.Security) != 0 {
			t.Fatalf("the public read %s requires a credential", path)
		}
	}
}

// Every buyer operation requires a credential, and never a plugin sign in.
func TestBuyerOperationsRequireACredential(t *testing.T) {
	t.Parallel()
	spec, err := Build()
	if err != nil {
		t.Fatalf("the document did not build: %v", err)
	}
	guarded := []string{
		"/v1/checkout-sessions",
		"/v1/checkout-sessions/{id}/shipping-options",
		"/v1/quotes",
		"/v1/quotes/{id}",
		"/v1/quotes/{id}/purchase",
		"/v1/orders",
		"/v1/orders/{id}",
	}
	allowed := map[string]bool{"buyerCredential": true, "checkoutSession": true}
	for _, path := range guarded {
		for method, operation := range spec.Paths[path] {
			if len(operation.Security) == 0 {
				t.Fatalf("the buyer operation %s %s requires no credential", method, path)
			}
			for _, requirement := range operation.Security {
				for scheme := range requirement {
					if !allowed[scheme] {
						t.Fatalf("the operation %s %s accepts the scheme %q",
							method, path, scheme)
					}
				}
			}
		}
	}
}

// The document declares no status the error catalog does not give a code, so a
// client reading the document finds the same statuses the server returns.
func TestEveryStatusComesFromTheCatalog(t *testing.T) {
	t.Parallel()
	spec, err := Build()
	if err != nil {
		t.Fatalf("the document did not build: %v", err)
	}
	for path, operations := range spec.Paths {
		for method, operation := range operations {
			for status := range operation.Responses {
				if isKnownStatus(status) {
					continue
				}
				t.Fatalf("the operation %s %s declares the unknown status %s", method, path, status)
			}
		}
	}
}

// HTTP 402 is the x402 challenge and nothing else: only the purchase declares it,
// and it carries the challenge in the PAYMENT-REQUIRED header and the challenge
// document as its body. Every other answer of every operation is a catalog error
// (005:58, 001:344).
func TestOnlyThePurchaseDeclaresHTTP402WithItsChallengeHeader(t *testing.T) {
	t.Parallel()
	spec, err := Build()
	if err != nil {
		t.Fatalf("the document did not build: %v", err)
	}
	for path, operations := range spec.Paths {
		for method, operation := range operations {
			response, declares := operation.Responses["402"]
			if !declares {
				continue
			}
			if path != "/v1/quotes/{id}/purchase" {
				t.Fatalf("the operation %s %s declares HTTP 402", method, path)
			}
			if method != MethodPost {
				t.Fatalf("the operation %s %s declares HTTP 402", method, path)
			}
			if _, named := response.Headers["PAYMENT-REQUIRED"]; !named {
				t.Fatal("the 402 answer names no PAYMENT-REQUIRED header")
			}
			if response.Content["application/json"].Schema.Ref != SchemaRef("PaymentRequired") {
				t.Fatal("the 402 answer does not carry the challenge document")
			}
		}
	}
	if _, declared := spec.Paths["/v1/quotes/{id}/purchase"][MethodPost].Responses["402"]; !declared {
		t.Fatal("the purchase must declare its HTTP 402 challenge")
	}
}

// The purchase takes its payment in the PAYMENT-SIGNATURE header, answers a settled
// payment with PAYMENT-RESPONSE, and takes no request body, so a version 1 body
// payload can never be mistaken for the payment.
func TestThePurchaseUsesTheVersionTwoHeaders(t *testing.T) {
	t.Parallel()
	spec, err := Build()
	if err != nil {
		t.Fatalf("the document did not build: %v", err)
	}
	operation := spec.Paths["/v1/quotes/{id}/purchase"][MethodPost]
	if operation.RequestBody != nil {
		t.Fatal("the purchase declares a request body: the payment travels in PAYMENT-SIGNATURE")
	}
	var signature *Parameter
	for index := range operation.Parameters {
		if operation.Parameters[index].Name == "PAYMENT-SIGNATURE" {
			signature = &operation.Parameters[index]
		}
	}
	if signature == nil {
		t.Fatalf("PAYMENT-SIGNATURE is an optional header, got %+v", signature)
	}
	if signature.In != "header" {
		t.Fatalf("PAYMENT-SIGNATURE is an optional header, got %+v", signature)
	}
	if signature.Required {
		t.Fatalf("PAYMENT-SIGNATURE is an optional header, got %+v", signature)
	}
	if _, named := operation.Responses["200"].Headers["PAYMENT-RESPONSE"]; !named {
		t.Fatal("the 200 answer names no PAYMENT-RESPONSE header")
	}
}

// Every status the document declares that is not a success is one the catalog
// gives a code, so the client sees a documented body for every failure.
func TestEveryFailureStatusHasACatalogCode(t *testing.T) {
	t.Parallel()
	spec, err := Build()
	if err != nil {
		t.Fatalf("the document did not build: %v", err)
	}
	for path, operations := range spec.Paths {
		for method, operation := range operations {
			for status, response := range operation.Responses {
				if isSuccessStatus(status) {
					continue
				}
				if !carriesError(response) {
					t.Fatalf("the failure %s %s %s returns no error body",
						method, path, status)
				}
			}
		}
	}
}

// isSuccessStatus reports whether a status answers the operation.
func isSuccessStatus(status string) bool {
	return status == "200" || status == "201" || status == "202" || status == "402"
}

// carriesError reports whether one response body is the catalog error shape.
func carriesError(response Response) bool {
	body, present := response.Content["application/json"]
	if !present {
		return false
	}
	return body.Schema.Ref == "#/components/schemas/Error"
}

// The quote and the purchase require an Idempotency-Key, so a retried request
// never creates a second payment.
func TestPaymentOperationsRequireAnIdempotencyKey(t *testing.T) {
	t.Parallel()
	spec, err := Build()
	if err != nil {
		t.Fatalf("the document did not build: %v", err)
	}
	for _, path := range []string{"/v1/quotes", "/v1/quotes/{id}/purchase"} {
		operation := spec.Paths[path][MethodPost]
		if operation.OperationID == "" {
			t.Fatalf("%s declares no create operation", path)
		}
		if !hasIdempotencyKey(operation) {
			t.Fatalf("the operation %s requires no Idempotency-Key", operation.OperationID)
		}
	}
}

// hasIdempotencyKey reports whether one operation requires the header.
func hasIdempotencyKey(operation Operation) bool {
	for _, parameter := range operation.Parameters {
		if parameter.In != "header" {
			continue
		}
		if parameter.Name != "Idempotency-Key" {
			continue
		}
		if parameter.Required {
			return true
		}
	}
	return false
}

// Every schema the document declares is reachable from an operation or is one the
// error shape enumerates, so nothing is documented that no client receives.
func TestEverySchemaIsReachable(t *testing.T) {
	t.Parallel()
	spec, err := Build()
	if err != nil {
		t.Fatalf("the document did not build: %v", err)
	}
	reachable := make(map[string]bool)
	collectReferences(spec.Components.Schemas, reachable)
	for _, operations := range spec.Paths {
		for _, operation := range operations {
			for _, response := range operation.Responses {
				for _, body := range response.Content {
					collectReferences(map[string]Schema{"body": body.Schema}, reachable)
				}
			}
			if operation.RequestBody == nil {
				continue
			}
			for _, body := range operation.RequestBody.Content {
				collectReferences(map[string]Schema{"body": body.Schema}, reachable)
			}
		}
	}
	// Money is shared with the MCP tool schemas, which show a price in that shape.
	sharedWithTools := map[string]bool{"Money": true}
	for name := range spec.Components.Schemas {
		if !reachable[name] {
			if !sharedWithTools[name] {
				t.Fatalf("the schema %s is declared but no operation returns it", name)
			}
		}
	}
}

// collectReferences records every named schema a schema set reaches, at any depth.
// It keeps an explicit worklist rather than calling itself, so a deeply nested
// schema cannot grow the stack without bound.
func collectReferences(schemas map[string]Schema, into map[string]bool) {
	pending := make([]Schema, 0, len(schemas))
	for _, schema := range schemas {
		pending = append(pending, schema)
	}
	for len(pending) > 0 {
		current := pending[0]
		pending = pending[1:]
		collectOneReference(current, into)
		if current.Items != nil {
			pending = append(pending, *current.Items)
		}
		for _, property := range current.Properties {
			pending = append(pending, property)
		}
	}
}

// collectOneReference records one named schema reference, if it names one.
func collectOneReference(schema Schema, into map[string]bool) {
	for _, name := range schema.OneOf {
		into[name] = true
	}
	if schema.Ref == "" {
		return
	}
	name := strings.TrimPrefix(schema.Ref, "#/components/schemas/")
	if name == schema.Ref {
		return
	}
	into[name] = true
}

// The error schema enumerates every code the catalog defines, so a generated
// client cannot fail to handle a code the server returns.
func TestTheErrorSchemaEnumeratesTheCatalog(t *testing.T) {
	t.Parallel()
	spec, err := Build()
	if err != nil {
		t.Fatalf("the document did not build: %v", err)
	}
	schema, present := spec.Components.Schemas["Error"]
	if !present {
		t.Fatalf("the document declares no error schema")
	}
	code := schema.Properties["code"]
	if len(code.Enum) == 0 {
		t.Fatalf("the error schema enumerates no code")
	}
	for _, name := range code.Enum {
		if _, _, known := errs.Defined(errs.Code(name)); !known {
			t.Fatalf("the error schema names the unknown code %q", name)
		}
	}
}

// The document marshals to valid JSON with a stable ordering, so a generated
// client and a published copy do not differ only by map iteration order.
func TestMarshalIsStableAndComplete(t *testing.T) {
	t.Parallel()
	spec, err := Build()
	if err != nil {
		t.Fatalf("the document did not build: %v", err)
	}
	first, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("the document did not marshal: %v", err)
	}
	second, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("the document did not marshal a second time: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("two marshals of one document differ")
	}
	var decoded map[string]any
	if err := json.Unmarshal(first, &decoded); err != nil {
		t.Fatalf("the marshalled document is not valid JSON: %v", err)
	}
	for _, key := range []string{"openapi", "info", "servers", "paths", "components"} {
		if _, present := decoded[key]; !present {
			t.Fatalf("the marshalled document carries no %s section", key)
		}
	}
	checkPublishedFieldNames(t, decoded)
}

// checkPublishedFieldNames refuses a document that published a Go field name
// instead of the contract field name, because a generated client reads the
// contract names and would find nothing.
func checkPublishedFieldNames(t *testing.T, decoded map[string]any) {
	t.Helper()
	info, ok := decoded["info"].(map[string]any)
	if !ok {
		t.Fatalf("the marshalled document carries no info object")
	}
	for _, key := range []string{"title", "description", "version"} {
		if _, present := info[key]; !present {
			t.Fatalf("the info section publishes no %q field", key)
		}
	}
	if _, leaked := info["Title"]; leaked {
		t.Fatalf("the info section publishes a Go field name")
	}
	paths, ok := decoded["paths"].([]any)
	if !ok {
		t.Fatalf("the marshalled document carries no paths array")
	}
	if len(paths) == 0 {
		t.Fatalf("the marshalled document declares no path")
	}
	entry, ok := paths[0].(map[string]any)
	if !ok {
		t.Fatalf("a marshalled path is not an object")
	}
	operations, ok := entry["operations"].(map[string]any)
	if !ok {
		t.Fatalf("a marshalled path carries no operations object")
	}
	for _, raw := range operations {
		operation, isObject := raw.(map[string]any)
		if !isObject {
			t.Fatalf("a marshalled operation is not an object")
		}
		for _, key := range []string{"operationId", "summary", "responses"} {
			if _, present := operation[key]; !present {
				t.Fatalf("an operation publishes no %q field", key)
			}
		}
		if _, leaked := operation["OperationID"]; leaked {
			t.Fatalf("an operation publishes a Go field name")
		}
	}
	components, ok := decoded["components"].(map[string]any)
	if !ok {
		t.Fatalf("the marshalled document carries no components object")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		t.Fatalf("the components section publishes no schemas object")
	}
	for _, raw := range schemas {
		schema, isObject := raw.(map[string]any)
		if !isObject {
			t.Fatalf("a marshalled schema is not an object")
		}
		if _, leaked := schema["Properties"]; leaked {
			t.Fatalf("a schema publishes a Go field name")
		}
		if _, leaked := schema["Type"]; leaked {
			t.Fatalf("a schema publishes a Go field name")
		}
	}
}

// A document with nothing to describe is refused, so an empty contract is never
// published as if it were complete.
func TestAnEmptyDocumentIsRefused(t *testing.T) {
	t.Parallel()
	if _, err := (Specification{}).MarshalJSON(); err == nil {
		t.Fatalf("an empty document was published")
	}
}

// A schema that requires a field it never declares is refused, because a client
// would then be told to send something the document never describes.
func TestAnUndeclaredRequiredFieldIsRefused(t *testing.T) {
	t.Parallel()
	spec, err := Build()
	if err != nil {
		t.Fatalf("the document did not build: %v", err)
	}
	spec.Components.Schemas["Broken"] = Schema{
		Type:       "object",
		Properties: map[string]Schema{"id": {Type: "string"}},
		Required:   []string{"id", "absent"},
	}
	if err := checkSchemas(&spec); err == nil {
		t.Fatalf("a schema requiring an undeclared field was accepted")
	}
}

// A schema referencing a name the document does not declare is refused, because a
// generated client would fail at build time rather than at document time.
func TestAnUnknownReferenceIsRefused(t *testing.T) {
	t.Parallel()
	spec, err := Build()
	if err != nil {
		t.Fatalf("the document did not build: %v", err)
	}
	spec.Components.Schemas["Dangling"] = Schema{Ref: "#/components/schemas/NotDeclared"}
	if err := checkSchemas(&spec); err == nil {
		t.Fatalf("a schema referencing an undeclared schema was accepted")
	}
}

// A schema declaring no type and no enumeration is refused, because a generated
// client could not read a field of unknown shape.
func TestATypelessSchemaIsRefused(t *testing.T) {
	t.Parallel()
	spec, err := Build()
	if err != nil {
		t.Fatalf("the document did not build: %v", err)
	}
	spec.Components.Schemas["Shapeless"] = Schema{Description: "A field of unknown shape."}
	if err := checkSchemas(&spec); err == nil {
		t.Fatalf("a schema declaring no type was accepted")
	}
}

// An operation with no identifier, summary, response, or tag is refused, because
// such an operation is documented but not implementable.
func TestAnIncompleteOperationIsRefused(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		operation Operation
	}{
		{name: "no identifier", operation: Operation{
			Summary: "A summary", Tags: []string{"discovery"},
			Responses: map[string]Response{"200": jsonResponse("ok", refSchema("Product"))},
		}},
		{name: "no summary", operation: Operation{
			OperationID: "op", Tags: []string{"discovery"},
			Responses: map[string]Response{"200": jsonResponse("ok", refSchema("Product"))},
		}},
		{name: "no tag", operation: Operation{
			OperationID: "op", Summary: "A summary",
			Responses: map[string]Response{"200": jsonResponse("ok", refSchema("Product"))},
		}},
		{name: "no response", operation: Operation{
			OperationID: "op", Summary: "A summary", Tags: []string{"discovery"},
			Responses: map[string]Response{},
		}},
		{name: "no success", operation: Operation{
			OperationID: "op", Summary: "A summary", Tags: []string{"discovery"},
			Responses: map[string]Response{"400": errorResponse("bad")},
		}},
	}
	for _, testCase := range cases {
		if err := checkOperation("/v1/products", MethodGet, testCase.operation); err == nil {
			t.Fatalf("an operation with %s was accepted", testCase.name)
		}
	}
}

// A path outside the versioned prefix is refused, so a client always knows which
// contract version it is reading.
func TestAPathOutsideThePrefixIsRefused(t *testing.T) {
	t.Parallel()
	operation := Operation{
		OperationID: "op", Summary: "A summary", Tags: []string{"discovery"},
		Responses: map[string]Response{"200": jsonResponse("ok", refSchema("Product"))},
	}
	if err := checkOperation("/products", MethodGet, operation); err == nil {
		t.Fatalf("a path outside the versioned prefix was accepted")
	}
	if err := checkOperation("/v1/products", MethodGet, operation); err != nil {
		t.Fatalf("a versioned path was refused: %v", err)
	}
}
