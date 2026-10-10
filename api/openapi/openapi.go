// Package openapi owns the REST contract every client and the server follow. It
// builds the document from the same catalog and types the packages define, so the
// published document cannot drift from the error codes, statuses, and money type
// the code actually returns.
//
// The document is a description, not a server: nothing here performs a request.
// It exists so a generated client and the server's own validation are written
// from one source, per the shared contract.
package openapi

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/vadora/vadora/api/types/errs"
	"github.com/vadora/vadora/api/types/rest"
)

// APIVersion is the version prefix every implemented REST operation carries. The
// prefix is versioned separately from the document, so a compatible addition does
// not force a new prefix.
const APIVersion = "v1"

// Specification is the OpenAPI document Vadora publishes. It describes only the
// implemented launch operations; an operation that is not implemented is absent
// rather than declared and refused.
type Specification struct {
	// OpenAPI is the document version this structure follows.
	OpenAPI string
	// Info carries the document's title, description, and version.
	Info Info
	// Servers lists the origins the operations are served from.
	Servers []Server
	// Paths holds every implemented operation, keyed by path then method.
	Paths map[string]map[string]Operation
	// Components holds the schemas and the security scheme the paths reference.
	Components Components
	// Security lists the security requirements that apply to the whole document.
	Security []map[string][]string
	// Tags groups the operations for a generated client's readability.
	Tags []Tag
}

// DocumentVersion is the OpenAPI version this structure follows.
const DocumentVersion = "3.1.0"

// MaxDescriptionLength bounds one description, so a generated document cannot
// grow past what a reader can use.
const MaxDescriptionLength = 2000

// MaxFieldLength is the largest bound any string field this document declares, so
// an unbounded free text field can never reach the generated client.
const MaxFieldLength = 256

// Info is the document's own metadata.
type Info struct {
	Title       string
	Description string
	Version     string
}

// Server is one origin the operations are served from.
type Server struct {
	URL         string
	Description string
}

// Tag groups operations under one name.
type Tag struct {
	Name        string
	Description string
}

// Components holds the reusable schemas and security schemes.
type Components struct {
	Schemas         map[string]Schema
	SecuritySchemes map[string]SecurityScheme
}

// SecurityScheme names how a client proves who it is.
type SecurityScheme struct {
	Type         string
	Scheme       string
	Description  string
	BearerFormat string
}

// Schema is one JSON Schema object. It is deliberately small: the contract needs
// objects, arrays, strings, integers, and enumerations, and a partial JSON Schema
// would let a required field go undocumented.
type Schema struct {
	Type                 string
	Description          string
	Format               string
	Properties           map[string]Schema
	Required             []string
	Items                *Schema
	Enum                 []string
	Ref                  string
	Nullable             bool
	Minimum              *int64
	Maximum              *int64
	MinLength            *int64
	MaxLength            *int64
	MaxItems             *int64
	AdditionalProperties bool
	OneOf                []string
}

// Operation is one implemented HTTP operation.
type Operation struct {
	OperationID string
	Summary     string
	Description string
	Tags        []string
	Parameters  []Parameter
	RequestBody *RequestBody
	Responses   map[string]Response
	Security    []map[string][]string
}

// Parameter is one query, path, or header parameter.
type Parameter struct {
	Name        string
	In          string
	Description string
	Required    bool
	Schema      Schema
}

// RequestBody is one operation's request body.
type RequestBody struct {
	Description string
	Required    bool
	Content     map[string]MediaType
}

// Response is one operation's response for one status.
type Response struct {
	Description string
	// Headers maps a response header name to what it carries.
	Headers map[string]string
	Content map[string]MediaType
}

// MediaType is one content type of a request or response body.
type MediaType struct {
	Schema Schema
}

// Build returns the published document. It is derived from the error catalog, the
// integration catalog, and the domain types, so a code that has no catalog entry
// cannot appear in the document and a catalog entry cannot be missing from it.
func Build() (Specification, error) {
	spec := Specification{
		OpenAPI: DocumentVersion,
		Info: Info{
			Title: "Vadora API",
			Description: strings.Join([]string{
				"Vadora connects merchants to crypto agents: people and AI agents buy a merchant's",
				"existing physical products, paid in stablecoins, with the order landing in the store",
				"the merchant already uses. Every buyer API call other than public discovery is",
				"authenticated with a buyer credential or a checkout page sign in on Vadora's own",
				"domain. A plugin sign in is never accepted for a buyer call.",
			}, "\n"),
			Version: "0.2.1",
		},
		Servers: []Server{
			{URL: "https://api.vadora.xyz", Description: "The production API origin"},
		},
		Paths:      map[string]map[string]Operation{},
		Components: Components{Schemas: map[string]Schema{}, SecuritySchemes: map[string]SecurityScheme{}},
		Tags: []Tag{
			{Name: "auth", Description: "Email sign in for buyer credentials and account sessions"},
			{Name: "discovery", Description: "Public catalog reads"},
			{Name: "checkout", Description: "Buyer sessions, shipping, and quotes"},
			{Name: "payments", Description: "Quote purchase and settlement"},
			{Name: "orders", Description: "A buyer's own orders"},
		},
	}
	if err := addSecuritySchemes(&spec); err != nil {
		return Specification{}, err
	}
	if err := addSchemas(&spec); err != nil {
		return Specification{}, err
	}
	if err := addOperations(&spec); err != nil {
		return Specification{}, err
	}
	if err := checkRoutes(&spec); err != nil {
		return Specification{}, err
	}
	if err := checkSpec(&spec); err != nil {
		return Specification{}, err
	}
	return spec, nil
}

// addSecuritySchemes names how a buyer proves who it is on the API origin. The
// checkout page's cookie session lives on the checkout origin and is not part of
// this document.
func addSecuritySchemes(spec *Specification) error {
	spec.Components.SecuritySchemes["buyerCredential"] = SecurityScheme{
		Type:         "http",
		Scheme:       "bearer",
		BearerFormat: "Vadora buyer credential",
		Description: strings.Join([]string{
			"A revocable buyer credential that the server stores only as a hash. It authenticates an",
			"independent agent's buyer calls, and it is issued after an email sign in on Vadora's own",
			"domain. A plugin sign in is never accepted for a buyer call.",
		}, "\n"),
	}
	spec.Components.SecuritySchemes["checkoutSession"] = SecurityScheme{
		Type:         "http",
		Scheme:       "bearer",
		BearerFormat: "Vadora checkout session token",
		Description: strings.Join([]string{
			"The token that a checkout session returns once. It binds that session's quotes and",
			"purchases, and another session's token reads nothing.",
		}, "\n"),
	}
	spec.Components.SecuritySchemes["merchantToken"] = SecurityScheme{
		Type:         "http",
		Scheme:       "bearer",
		BearerFormat: "Vadora merchant access token",
		Description: strings.Join([]string{
			"The short lived access token that the CLI's sign in issues through OAuth with PKCE. It",
			"carries the merchant scope, and it is refreshed with a rotating refresh token. It",
			"administers only the stores that its account owns. A buyer credential and a checkout",
			"session token never work on a merchant operation.",
		}, "\n"),
	}
	if len(spec.Components.SecuritySchemes) != 3 {
		return fmt.Errorf("the document declares %d security schemes, want 3",
			len(spec.Components.SecuritySchemes))
	}
	return nil
}

// addSchemas declares every schema the paths reference. The request and response
// documents are derived from the shared wire types, and the money and error
// schemas come from the domain and the error catalog.
func addSchemas(spec *Specification) error {
	spec.Components.Schemas["Money"] = Schema{
		Type: "object",
		Description: strings.Join([]string{
			"An amount in a currency's minor units. The amount is an integer so no rounding ever",
			"reaches a payment, and the currency is an uppercase ISO 4217 code.",
		}, "\n"),
		Properties: map[string]Schema{
			"amount":   {Type: "integer", Format: "int64", Description: "The minor unit count."},
			"currency": {Type: "string", Description: "The uppercase ISO 4217 code."},
		},
		Required: []string{"amount", "currency"},
	}
	spec.Components.Schemas["Error"] = errorSchema()
	spec.Components.Schemas["PaymentRequired"] = paymentRequiredSchema()
	documents := rest.Documents()
	names := make(map[reflect.Type]string, len(documents))
	for _, document := range documents {
		names[reflect.TypeOf(document.Sample)] = document.Name
	}
	for _, document := range documents {
		schema, err := deriveSchema(document.Sample, document.Description, names)
		if err != nil {
			return fmt.Errorf("deriving the schema %s: %w", document.Name, err)
		}
		spec.Components.Schemas[document.Name] = schema
	}
	return checkSchemas(spec)
}

// paymentRequiredSchema describes the x402 version 2 challenge. The x402
// specification owns its shape, so the document names the members that a client
// reads and leaves the rest open.
func paymentRequiredSchema() Schema {
	return Schema{
		Type: "object",
		Description: strings.Join([]string{
			"The x402 version 2 challenge of an unpaid quote, as the PAYMENT-REQUIRED header carries it",
			"in base64. It lists exactly one option: USDC on the quote's Solana network, to the",
			"merchant's payout address, with the quote ID as the memo and the facilitator as fee payer.",
		}, "\n"),
		Properties: map[string]Schema{
			"x402Version": {Type: "integer", Description: "The protocol version, always 2."},
			"accepts": {Type: "array", Description: "The payment options, each an object of the exact scheme.",
				Items: &Schema{Type: "object", AdditionalProperties: true}},
			"resource":   {Type: "object", AdditionalProperties: true, Description: "The purchase endpoint of the quote."},
			"extensions": {Type: "object", AdditionalProperties: true, Description: "The protocol extensions, including the required payment identifier."},
		},
		Required: []string{"x402Version", "accepts"},
	}
}

// SchemaRefPrefix is the pointer prefix every component reference carries, so a
// client in another package can build one reference without restating the path.
const SchemaRefPrefix = "#/components/schemas/"

// SchemaRef returns the pointer that names one component schema.
func SchemaRef(name string) string {
	if name == "" {
		panic("openapi: a schema reference named nothing")
	}
	if len(name) > MaxSchemaNameLength {
		panic("openapi: a schema name is longer than its bound: " + name)
	}
	return SchemaRefPrefix + name
}

// MaxSchemaNameLength bounds one schema name, so a reference cannot be unbounded.
const MaxSchemaNameLength = 64

// checkSpec refuses a document that is not publishable: no version, no
// description, or no security scheme for the buyer authenticated operations.
func checkSpec(spec *Specification) error {
	if spec.OpenAPI != DocumentVersion {
		return fmt.Errorf("the document declares the OpenAPI version %q", spec.OpenAPI)
	}
	if spec.Info.Title == "" {
		return fmt.Errorf("the document has no title")
	}
	if spec.Info.Description == "" {
		return fmt.Errorf("the document has no description")
	}
	if len(spec.Servers) == 0 {
		return fmt.Errorf("the document declares no server")
	}
	if len(spec.SecuritySchemes()) == 0 {
		return fmt.Errorf("the document declares no security scheme")
	}
	if len(spec.Tags) == 0 {
		return fmt.Errorf("the document declares no tag")
	}
	return nil
}

// SecuritySchemes returns how many security schemes the document declares.
func (s Specification) SecuritySchemes() map[string]SecurityScheme {
	return s.Components.SecuritySchemes
}

// errorSchema describes the one error shape every failure returns. Its code,
// action, and status come from the catalog, so the enumerated codes are read from
// it rather than written twice.
func errorSchema() Schema {
	values := catalogErrorCodes()
	if len(values) == 0 {
		panic("openapi: the error schema enumerates no code")
	}
	actions := errs.LeafActions()
	actionValues := make([]string, 0, len(actions))
	for _, action := range actions {
		actionValues = append(actionValues, string(action))
	}
	schema := Schema{
		Type: "object",
		Description: strings.Join([]string{
			"The one error every operation returns. The action, the retry rule, and the HTTP",
			"status all come from the error catalog, never from the caller, so REST, MCP, and the",
			"CLI report the same thing for the same failure.",
		}, "\n"),
		Properties: map[string]Schema{
			"code": {
				Type:        "string",
				Description: "The catalog code for this failure.",
				Enum:        values,
			},
			"reason": {
				Type:        "string",
				Description: "The human readable cause, safe to show a person.",
			},
			"action": {
				Type:        "string",
				Description: "The single client action this failure asks for.",
				Enum:        actionValues,
			},
			"retries": {
				Type:        "boolean",
				Description: "Whether the catalog asks for a retry of the same request.",
			},
			"retry_after_seconds": {
				Type:        "integer",
				Description: "Set only for a code that carries a retry interval.",
				Nullable:    true,
			},
			"details": {
				Type: "object",
				Description: strings.Join([]string{
					"Catalog scoped context, such as the reason a store is unsupported or the cause",
					"a quote expired. It never carries delivery details.",
				}, "\n"),
				AdditionalProperties: true,
			},
		},
		Required: []string{"code", "reason", "action", "retries"}}

	if len(schema.Properties) == 0 {
		panic("openapi: errorSchema declares no property")
	}
	if _, declared := schema.Properties["code"]; !declared {
		panic("openapi: errorSchema declares no code property")
	}
	if len(schema.Required) == 0 {
		panic("openapi: errorSchema requires no field")
	}
	return schema

}

// The names below are the schema names this package declares and references. They
// are constants rather than literals so a reference and a declaration cannot name
// the same schema two different ways.
const (
	// StoreSchemaName is the public store directory entry.
	StoreSchemaName = "Store"
	// ProductSchemaName is one published listing.
	ProductSchemaName = "Product"
	// ProductPageSchemaName is one page of the public listing catalog.
	ProductPageSchemaName = "ProductPage"
	// StorePageSchemaName is one page of the public store directory.
	StorePageSchemaName = "StorePage"
	// CheckoutSessionSchemaName is a private buyer session.
	CheckoutSessionSchemaName = "CheckoutSession"
	// CreateSessionSchemaName is the body that creates a buyer session.
	CreateSessionSchemaName = "CreateCheckoutSessionRequest"
	// ShippingOptionsRequestName is the body that prices shipping.
	ShippingOptionsRequestName = "ShippingOptionsRequest"
	// ShippingOptionName is one shipping method and its total.
	ShippingOptionName = "ShippingOption"
	// ShippingOptionsName is the shipping methods available for a destination.
	ShippingOptionsName = "ShippingOptions"
	// QuoteRequestName is the body that creates a quote.
	QuoteRequestName = "QuoteRequest"
	// QuoteName is one issued quote.
	QuoteName = "Quote"
	// PurchaseRequestName is the body that submits a purchase.
	PurchaseRequestName = "PurchaseRequest"
	// PaymentChallengeName is the challenge a quote returns before payment.
	PaymentChallengeName = "PaymentChallenge"
	// PaymentMethodName is one method a listing or store offers.
	PaymentMethodName = "PurchaseMethod"
	// ReceiptName is a persisted receipt.
	ReceiptName = "Receipt"
	// OrderName is one buyer's order.
	OrderName = "Order"
	// ErrorName is the one error shape every failure returns.
	ErrorName = "Error"
	// MoneyName is an amount in a currency's minor units.
	MoneyName = "Money"
	// DeliveryDetailsName is the bounded delivery fields an order keeps.
	DeliveryDetailsName = "DeliveryDetails"
	// OrderPageSchemaName is one page of the buyer's own orders.
	OrderPageSchemaName = "OrderPage"
)

// The field name sets below are the fields each nested schema declares. A nested
// schema that declared none would be documented as an object a client cannot read,
// so each set is named once and its emptiness is an assertion.
var (
	variantFields  = []string{"id", "title", "price"}
	policyFields   = []string{"return_policy", "privacy_policy"}
	deliveryFields = []string{"name", "street_lines", "city", "region", "postal_code", "country", "phone"}
)

// itemOf returns the addressable form an array's item schema needs, so a list's
// element type is written as a call rather than a temporary variable.
func itemOf(schema Schema) *Schema {
	return &schema
}

// refSchema returns the schema that points at a named component.
func refSchema(name string) Schema {
	if name == "" {
		panic("openapi: a schema reference named nothing")
	}
	return Schema{Ref: "#/components/schemas/" + name}
}

// MarshalJSON writes the document with its object properties in a stable order,
// so a generated client and a published copy of the document do not differ only by
// Go's map iteration order.
func (s Specification) MarshalJSON() ([]byte, error) {
	if s.OpenAPI == "" {
		return nil, fmt.Errorf("the document declares no OpenAPI version")
	}
	if len(s.Paths) == 0 {
		return nil, fmt.Errorf("the document declares no operation")
	}
	if len(s.Components.Schemas) == 0 {
		return nil, fmt.Errorf("the document declares no schema")
	}
	return json.Marshal(buildDocumentShape(s))
}

// documentShape is the document json.Marshal writes. Every field carries the
// OpenAPI field name it is published under, and every map is replaced by a sorted
// list so a generated client and a published copy of the document differ only in
// the contract itself.
type documentShape struct {
	OpenAPI    string                `json:"openapi"`
	Info       infoShape             `json:"info"`
	Servers    []serverShape         `json:"servers"`
	Tags       []tagShape            `json:"tags"`
	Paths      []orderedPath         `json:"paths"`
	Components orderedComponents     `json:"components"`
	Security   []map[string][]string `json:"security,omitempty"`
}

// infoShape is the document's own metadata, under its published field names.
type infoShape struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Version     string `json:"version"`
}

// serverShape is one origin an operation is served from.
type serverShape struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

// tagShape groups operations under one name.
type tagShape struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// orderedPath is one path and its operations, ordered by path.
type orderedPath struct {
	Path       string                    `json:"path"`
	Operations map[string]operationShape `json:"operations"`
}

// operationShape is one operation, under its published field names.
type operationShape struct {
	OperationID string                   `json:"operationId"`
	Summary     string                   `json:"summary"`
	Description string                   `json:"description,omitempty"`
	Tags        []string                 `json:"tags,omitempty"`
	Parameters  []parameterShape         `json:"parameters,omitempty"`
	RequestBody *requestBodyShape        `json:"requestBody,omitempty"`
	Responses   map[string]responseShape `json:"responses"`
	Security    []map[string][]string    `json:"security,omitempty"`
}

// parameterShape is one parameter, under its published field names.
type parameterShape struct {
	Name        string      `json:"name"`
	In          string      `json:"in"`
	Description string      `json:"description"`
	Required    bool        `json:"required"`
	Schema      schemaShape `json:"schema"`
}

// requestBodyShape is one operation's request body.
type requestBodyShape struct {
	Description string                `json:"description"`
	Required    bool                  `json:"required"`
	Content     map[string]mediaShape `json:"content"`
}

// responseShape is one operation's response for one status.
type responseShape struct {
	Description string                 `json:"description"`
	Headers     map[string]headerShape `json:"headers,omitempty"`
	Content     map[string]mediaShape  `json:"content"`
}

// headerShape is one response header, under its published field names.
type headerShape struct {
	Description string      `json:"description"`
	Schema      schemaShape `json:"schema"`
}

// mediaShape is one content type of a request or response body.
type mediaShape struct {
	Schema schemaShape `json:"schema"`
}

// schemaShape is one JSON Schema object, under its published field names. Only
// the keywords this contract uses are carried, so a required field can never go
// undocumented.
type schemaShape struct {
	Type                 string                 `json:"type,omitempty"`
	Description          string                 `json:"description,omitempty"`
	Format               string                 `json:"format,omitempty"`
	Properties           map[string]schemaShape `json:"properties,omitempty"`
	Required             []string               `json:"required,omitempty"`
	Items                *schemaShape           `json:"items,omitempty"`
	Enum                 []string               `json:"enum,omitempty"`
	Ref                  string                 `json:"$ref,omitempty"`
	Nullable             bool                   `json:"nullable,omitempty"`
	Minimum              *int64                 `json:"minimum,omitempty"`
	Maximum              *int64                 `json:"maximum,omitempty"`
	MinLength            *int64                 `json:"minLength,omitempty"`
	MaxLength            *int64                 `json:"maxLength,omitempty"`
	MaxItems             *int64                 `json:"maxItems,omitempty"`
	AdditionalProperties bool                   `json:"additionalProperties,omitempty"`
	OneOf                []schemaShape          `json:"oneOf,omitempty"`
}

// orderedComponents is the components section with sorted keys.
type orderedComponents struct {
	Schemas         map[string]schemaShape   `json:"schemas"`
	SecuritySchemes map[string]securityShape `json:"securitySchemes"`
}

// securityShape is one security scheme, under its published field names.
type securityShape struct {
	Type         string `json:"type"`
	Scheme       string `json:"scheme"`
	Description  string `json:"description,omitempty"`
	BearerFormat string `json:"bearerFormat,omitempty"`
}

// buildDocumentShape builds the publishable document from the specification,
// sorting every map so the output is reproducible.
func buildDocumentShape(spec Specification) documentShape {
	return documentShape{
		OpenAPI: spec.OpenAPI,
		Info: infoShape{
			Title:       spec.Info.Title,
			Description: spec.Info.Description,
			Version:     spec.Info.Version,
		},
		Servers:    buildServerShapes(spec.Servers),
		Tags:       buildTagShapes(spec.Tags),
		Paths:      buildPathShapes(spec.Paths),
		Components: buildComponentShapes(spec.Components),
		Security:   spec.Security,
	}
}

// buildServerShapes returns the origins in their published form.
func buildServerShapes(servers []Server) []serverShape {
	if len(servers) == 0 {
		panic("openapi: the document declares no server")
	}
	shapes := make([]serverShape, 0, len(servers))
	for _, server := range servers {
		if server.URL == "" {
			panic("openapi: a server was declared with no URL")
		}
		if server.Description == "" {
			panic("openapi: a server was declared with no description: " + server.URL)
		}
		shapes = append(shapes, serverShape{URL: server.URL, Description: server.Description})
	}
	if len(shapes) != len(servers) {
		panic("openapi: a server lost its published form")
	}
	return shapes
}

// buildTagShapes returns the tags in their published form.
func buildTagShapes(tags []Tag) []tagShape {
	if len(tags) == 0 {
		panic("openapi: the document declares no tag")
	}
	shapes := make([]tagShape, 0, len(tags))
	for _, tag := range tags {
		if tag.Name == "" {
			panic("openapi: a tag was declared with no name")
		}
		if tag.Description == "" {
			panic("openapi: a tag was declared with no description: " + tag.Name)
		}
		shapes = append(shapes, tagShape{Name: tag.Name, Description: tag.Description})
	}
	if len(shapes) != len(tags) {
		panic("openapi: a tag lost its published form")
	}
	return shapes
}

// buildPathShapes returns every path and its operations, keyed by method.
func buildPathShapes(paths map[string]map[string]Operation) []orderedPath {
	if len(paths) == 0 {
		panic("openapi: the document declares no operation")
	}
	names := make([]string, 0, len(paths))
	for path := range paths {
		names = append(names, path)
	}
	sort.Strings(names)
	if len(names) != len(paths) {
		panic("openapi: a declared path lost its name")
	}
	shapes := make([]orderedPath, 0, len(names))
	for _, path := range names {
		operations := paths[path]
		if len(operations) == 0 {
			panic("openapi: a declared path carries no operation: " + path)
		}
		methods := make([]string, 0, len(operations))
		for method := range operations {
			methods = append(methods, method)
		}
		sort.Strings(methods)
		byMethod := make(map[string]operationShape, len(methods))
		for _, method := range methods {
			byMethod[method] = buildOperationShape(operations[method])
		}
		if len(byMethod) != len(methods) {
			panic("openapi: an operation lost its published form: " + path)
		}
		shapes = append(shapes, orderedPath{Path: path, Operations: byMethod})
	}
	if len(shapes) != len(names) {
		panic("openapi: a declared path lost its published form")
	}
	return shapes
}

// buildOperationShape returns one operation in its published form.
func buildOperationShape(operation Operation) operationShape {
	if operation.OperationID == "" {
		panic("openapi: an operation was published with no identifier")
	}
	if operation.Summary == "" {
		panic("openapi: an operation was published with no summary: " + operation.OperationID)
	}
	if len(operation.Responses) == 0 {
		panic("openapi: an operation was published with no response: " + operation.OperationID)
	}
	return operationShape{
		OperationID: operation.OperationID,
		Summary:     operation.Summary,
		Description: operation.Description,
		Tags:        operation.Tags,
		Parameters:  buildParameterShapes(operation.Parameters),
		RequestBody: buildRequestBodyShape(operation.RequestBody),
		Responses:   buildResponseShapes(operation.Responses),
		Security:    operation.Security,
	}
}

// buildParameterShapes returns the parameters in their published form.
func buildParameterShapes(parameters []Parameter) []parameterShape {
	if len(parameters) == 0 {
		return nil
	}
	if len(parameters) == 0 {
		return nil
	}
	shapes := make([]parameterShape, 0, len(parameters))
	for _, parameter := range parameters {
		if parameter.Name == "" {
			panic("openapi: a parameter was declared with no name")
		}
		if parameter.In == "" {
			panic("openapi: a parameter was declared with no location: " + parameter.Name)
		}
		shapes = append(shapes, parameterShape{
			Name:        parameter.Name,
			In:          parameter.In,
			Description: parameter.Description,
			Required:    parameter.Required,
			Schema:      buildSchemaShape(parameter.Schema),
		})
	}
	return shapes
}

// buildRequestBodyShape returns one request body in its published form.
func buildRequestBodyShape(body *RequestBody) *requestBodyShape {
	if body == nil {
		return nil
	}
	if body.Description == "" {
		panic("openapi: a request body carried no description")
	}
	if len(body.Content) == 0 {
		panic("openapi: a request body carried no content type")
	}
	return &requestBodyShape{
		Description: body.Description,
		Required:    body.Required,
		Content:     buildContentShapes(body.Content),
	}
}

// buildResponseShapes returns the responses keyed by their status.
func buildResponseShapes(responses map[string]Response) map[string]responseShape {
	if len(responses) == 0 {
		panic("openapi: an operation declares no response")
	}
	shapes := make(map[string]responseShape, len(responses))
	for status, response := range responses {
		if status == "" {
			panic("openapi: a response was declared with no status")
		}
		if response.Description == "" {
			panic("openapi: a response carried no description: " + status)
		}
		if len(response.Content) == 0 {
			panic("openapi: a response carried no content type: " + status)
		}
		shapes[status] = responseShape{
			Description: response.Description,
			Headers:     buildHeaderShapes(response.Headers),
			Content:     buildContentShapes(response.Content),
		}
	}
	return shapes
}

// oneOfShapes returns a choice between named schemas as references.
func oneOfShapes(names []string) []schemaShape {
	if len(names) == 0 {
		return nil
	}
	shapes := make([]schemaShape, 0, len(names))
	for _, name := range names {
		shapes = append(shapes, schemaShape{Ref: SchemaRefPrefix + name})
	}
	return shapes
}

// buildHeaderShapes returns the response headers of one status in their published form.
func buildHeaderShapes(headers map[string]string) map[string]headerShape {
	if len(headers) == 0 {
		return nil
	}
	shapes := make(map[string]headerShape, len(headers))
	for name, description := range headers {
		shapes[name] = headerShape{Description: description, Schema: schemaShape{Type: "string"}}
	}
	return shapes
}

// buildContentShapes returns the media types of one body in their published form.
func buildContentShapes(content map[string]MediaType) map[string]mediaShape {
	if len(content) == 0 {
		panic("openapi: a body carries no content type")
	}
	shapes := make(map[string]mediaShape, len(content))
	for mediaType, body := range content {
		if mediaType == "" {
			panic("openapi: a body was declared with no content type")
		}
		shapes[mediaType] = mediaShape{Schema: buildSchemaShape(body.Schema)}
	}
	if len(shapes) != len(content) {
		panic("openapi: a media type lost its published form")
	}
	return shapes
}

// buildSchemaShape returns one schema in its published form.
func buildSchemaShape(schema Schema) schemaShape {
	if schema.Type == "" {
		if schema.Ref == "" {
			if len(schema.OneOf) == 0 {
				panic("openapi: a schema was published with no type and no reference")
			}
		}
	}
	shape := schemaShape{
		Type:                 schema.Type,
		Description:          schema.Description,
		Format:               schema.Format,
		Properties:           buildPropertyShapes(schema.Properties),
		Required:             schema.Required,
		Items:                buildItemShape(schema.Items),
		Enum:                 schema.Enum,
		Ref:                  schema.Ref,
		Nullable:             schema.Nullable,
		Minimum:              schema.Minimum,
		Maximum:              schema.Maximum,
		MinLength:            schema.MinLength,
		MaxLength:            schema.MaxLength,
		MaxItems:             schema.MaxItems,
		AdditionalProperties: schema.AdditionalProperties,
		OneOf:                oneOfShapes(schema.OneOf),
	}
	shape.Properties = buildPropertyShapes(schema.Properties)
	return shape
}

// buildPropertyShapes returns one schema's properties in their published form.
func buildPropertyShapes(properties map[string]Schema) map[string]schemaShape {
	if len(properties) == 0 {
		return nil
	}
	shapes := make(map[string]schemaShape, len(properties))
	for name, property := range properties {
		if name == "" {
			panic("openapi: a schema declares an unnamed field")
		}
		shapes[name] = buildSchemaShape(property)
	}
	if len(shapes) != len(properties) {
		panic("openapi: a schema field lost its published form")
	}
	return shapes
}

// buildItemShape returns one array's element type in its published form.
func buildItemShape(items *Schema) *schemaShape {
	if items == nil {
		return nil
	}
	if items.Type == "" {
		if items.Ref == "" {
			if len(items.OneOf) == 0 {
				panic("openapi: an array was published with an element type of nothing")
			}
		}
	}
	shape := buildSchemaShape(*items)
	return &shape
}

// buildComponentShapes returns the components section with sorted keys.
func buildComponentShapes(components Components) orderedComponents {
	if len(components.Schemas) == 0 {
		panic("openapi: the document declares no schema")
	}
	if len(components.SecuritySchemes) == 0 {
		panic("openapi: the document declares no security scheme")
	}
	schemas := make(map[string]schemaShape, len(components.Schemas))
	for name, schema := range components.Schemas {
		if name == "" {
			panic("openapi: a component schema is unnamed")
		}
		schemas[name] = buildSchemaShape(schema)
	}
	schemes := make(map[string]securityShape, len(components.SecuritySchemes))
	for name, scheme := range components.SecuritySchemes {
		if scheme.Type == "" {
			panic("openapi: a security scheme declares no type: " + name)
		}
		if scheme.Scheme == "" {
			panic("openapi: a security scheme declares no scheme: " + name)
		}
		schemes[name] = securityShape{
			Type:         scheme.Type,
			Scheme:       scheme.Scheme,
			Description:  scheme.Description,
			BearerFormat: scheme.BearerFormat,
		}
	}
	return orderedComponents{Schemas: schemas, SecuritySchemes: schemes}
}
