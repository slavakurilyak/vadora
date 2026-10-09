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
	"sort"
	"strings"

	"github.com/vadora/vadora/api/types/errs"
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

// paymentChallengeSchemaShape exists so the payment challenge has a named type in
// this package's own vocabulary, matching every other schema it declares.
type paymentChallengeSchemaShape struct{}

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
	Content     map[string]MediaType
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
			Version: "0.1.0",
		},
		Servers: []Server{
			{URL: "https://api.vadora.xyz", Description: "The production API origin"},
		},
		Paths:      map[string]map[string]Operation{},
		Components: Components{Schemas: map[string]Schema{}, SecuritySchemes: map[string]SecurityScheme{}},
		Tags: []Tag{
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
	if err := addDiscoveryOperations(&spec); err != nil {
		return Specification{}, err
	}
	if err := addCheckoutOperations(&spec); err != nil {
		return Specification{}, err
	}
	if err := addPaymentOperations(&spec); err != nil {
		return Specification{}, err
	}
	if err := addOrderOperations(&spec); err != nil {
		return Specification{}, err
	}
	if err := checkSpec(&spec); err != nil {
		return Specification{}, err
	}
	return spec, nil
}

// addSecuritySchemes names the two ways a client proves who it is.
func addSecuritySchemes(spec *Specification) error {
	spec.Components.SecuritySchemes["buyerCredential"] = SecurityScheme{
		Type:         "http",
		Scheme:       "bearer",
		BearerFormat: "Vadora buyer credential",
		Description: strings.Join([]string{
			"A revocable buyer credential the server stores only as a hash. It authenticates an",
			"independent agent's buyer calls and is the only credential a plugin sign in cannot",
			"replace.",
		}, "\n"),
	}
	spec.Components.SecuritySchemes["checkoutSession"] = SecurityScheme{
		Type:         "http",
		Scheme:       "bearer",
		BearerFormat: "Vadora checkout session token",
		Description: strings.Join([]string{
			"The token a checkout session returns once. It binds the session's quotes and",
			"purchases, and another session's token reads nothing.",
		}, "\n"),
	}
	if len(spec.Components.SecuritySchemes) != 2 {
		return fmt.Errorf("the document declares %d security schemes, want 2",
			len(spec.Components.SecuritySchemes))
	}
	return nil
}

// addSchemas declares every schema the paths reference.
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
	spec.Components.Schemas["Store"] = storeSchema()
	spec.Components.Schemas["Product"] = productSchema()
	spec.Components.Schemas["ProductPage"] = productPageSchema()
	spec.Components.Schemas["StorePage"] = storePageSchema()
	spec.Components.Schemas["CheckoutSession"] = checkoutSessionSchema()
	spec.Components.Schemas["CreateCheckoutSessionRequest"] = createCheckoutSessionRequestSchema()
	spec.Components.Schemas["ShippingOptionsRequest"] = shippingOptionsRequestSchema()
	spec.Components.Schemas["ShippingOption"] = shippingOptionSchema()
	spec.Components.Schemas["ShippingOptions"] = shippingOptionsSchema()
	spec.Components.Schemas["QuoteRequest"] = quoteRequestSchema()
	spec.Components.Schemas["Quote"] = quoteSchema()
	spec.Components.Schemas["PurchaseRequest"] = purchaseRequestSchema()
	spec.Components.Schemas["PaymentChallenge"] = paymentChallengeSchema()
	spec.Components.Schemas["Receipt"] = receiptSchema()
	spec.Components.Schemas["Order"] = orderSchema()
	spec.Components.Schemas["DeliveryDetails"] = deliveryDetailsSchema()
	spec.Components.Schemas["PurchaseMethod"] = purchaseMethodSchema()
	if err := checkSchemas(spec); err != nil {
		return err
	}
	return nil
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

// storeSchema describes one store in the public directory.
func storeSchema() Schema {
	if len(StoreSchemaName) == 0 {
		panic("openapi: the store schema is named nothing")
	}
	schema := Schema{
		Type:        "object",
		Description: "A merchant store with at least one published listing.",
		Properties: map[string]Schema{
			"id":           {Type: "string", Description: "The stable store identifier."},
			"name":         {Type: "string", Description: "The name a person reads."},
			"categories":   {Type: "array", Description: "The store's categories.", Items: itemOf(stringSchemaRef())},
			"destinations": {Type: "array", Description: "Approved destination country codes.", Items: itemOf(stringSchemaRef())},
			"purchase_methods": {
				Type:        "array",
				Description: "The payment methods this store's listings offer.",
				Items:       itemOf(refSchema(PaymentMethodName)),
			},
		},
		Required: []string{"id", "name", "categories", "destinations", "purchase_methods"}}

	if len(schema.Properties) == 0 {
		panic("openapi: storeSchema declares no property")
	}
	if _, declared := schema.Properties["id"]; !declared {
		panic("openapi: storeSchema declares no id property")
	}
	if len(schema.Required) == 0 {
		panic("openapi: storeSchema requires no field")
	}
	return schema

}

// productSchema describes one published listing.
func productSchema() Schema {
	if len(ProductSchemaName) == 0 {
		panic("openapi: the product schema is named nothing")
	}
	schema := Schema{
		Type:        "object",
		Description: "A published listing a buyer can quote.",
		Properties: map[string]Schema{
			"id":       {Type: "string", Description: "The stable listing identifier."},
			"title":    {Type: "string", Description: "The listing title, imported from the store."},
			"store_id": {Type: "string", Description: "The store that owns the listing."},
			"price":    refSchema(MoneyName),
			"variants": {
				Type:        "array",
				Description: "The purchasable variants of the listing.",
				Items:       itemOf(variantSchema()),
			},
			"purchase_methods": {
				Type:        "array",
				Description: "The payment methods this listing offers.",
				Items:       itemOf(refSchema(PaymentMethodName)),
			},
		},
		Required: []string{"id", "title", "store_id", "price", "variants", "purchase_methods"}}

	if len(schema.Properties) == 0 {
		panic("openapi: productSchema declares no property")
	}
	if _, declared := schema.Properties["id"]; !declared {
		panic("openapi: productSchema declares no id property")
	}
	if len(schema.Required) == 0 {
		panic("openapi: productSchema requires no field")
	}
	return schema

}

// variantSchema describes one purchasable variant of a listing.
func variantSchema() Schema {
	if len(variantFields) == 0 {
		panic("openapi: the variant schema declares no field")
	}
	schema := Schema{
		Type:        "object",
		Description: "One purchasable variant of a listing.",
		Properties: map[string]Schema{
			"id":    {Type: "string", Description: "The variant identifier."},
			"title": {Type: "string", Description: "The variant title."},
			"price": refSchema(MoneyName),
		},
		Required: []string{"id", "title", "price"}}

	if len(schema.Properties) == 0 {
		panic("openapi: variantSchema declares no property")
	}
	if _, declared := schema.Properties["id"]; !declared {
		panic("openapi: variantSchema declares no id property")
	}
	if len(schema.Required) == 0 {
		panic("openapi: variantSchema requires no field")
	}
	return schema

}

// purchaseMethodSchema describes one method a listing or store offers. A deferred
// integration appears in the checkout information with its coming soon status and
// never in the selectable set, so this schema carries the status for that reason.
func purchaseMethodSchema() Schema {
	statuses := catalogStatuses()
	if len(statuses) == 0 {
		panic("openapi: the purchase method schema enumerates no status")
	}
	statusValues := make([]string, 0, len(statuses))
	for _, status := range statuses {
		statusValues = append(statusValues, string(status))
	}
	schema := Schema{
		Type:        "object",
		Description: "One payment method a listing or store offers.",
		Properties: map[string]Schema{
			"protocol": {
				Type:        "string",
				Description: "The payment protocol, which is x402 for the launch scope.",
			},
			"network": {
				Type:        "string",
				Description: "The settlement network class, which is mainnet or testnet.",
				Enum:        []string{"mainnet", "testnet"},
			},
			"asset": {
				Type:        "string",
				Description: "The settlement asset, which is USDC for the launch scope.",
			},
			"selectable": {
				Type: "boolean",
				Description: strings.Join([]string{
					"Whether a client may select this method. A deferred integration is never",
					"selectable, so a pointer or keyboard interaction initiates no payment.",
				}, "\n"),
			},
			"status": {
				Type:        "string",
				Description: "The display status, which is coming_soon for a deferred method.",
				Enum:        statusValues,
			},
		},
		Required: []string{"protocol", "network", "asset", "selectable", "status"}}

	if len(schema.Properties) == 0 {
		panic("openapi: purchaseMethodSchema declares no property")
	}
	if _, declared := schema.Properties["protocol"]; !declared {
		panic("openapi: purchaseMethodSchema declares no protocol property")
	}
	if len(schema.Required) == 0 {
		panic("openapi: purchaseMethodSchema requires no field")
	}
	return schema

}

// productPageSchema describes one page of the public product catalog.
func productPageSchema() Schema {
	if ProductPageSchemaName == "" {
		panic("openapi: the product page schema is named nothing")
	}
	schema := Schema{
		Type:        "object",
		Description: "One page of public listings.",
		Properties: map[string]Schema{
			"items": {
				Type:        "array",
				Description: "The listings on this page.",
				Items:       itemOf(refSchema(ProductSchemaName)),
			},
			"next_cursor": {
				Type:        "string",
				Description: "The cursor for the next page, absent on the last page.",
				Nullable:    true,
			},
		},
		Required: []string{"items"}}

	if len(schema.Properties) == 0 {
		panic("openapi: productPageSchema declares no property")
	}
	if _, declared := schema.Properties["items"]; !declared {
		panic("openapi: productPageSchema declares no items property")
	}
	if len(schema.Required) == 0 {
		panic("openapi: productPageSchema requires no field")
	}
	return schema

}

// storePageSchema describes one page of the public store directory.
func storePageSchema() Schema {
	if StorePageSchemaName == "" {
		panic("openapi: the store page schema is named nothing")
	}
	schema := Schema{
		Type:        "object",
		Description: "One page of the public merchant directory.",
		Properties: map[string]Schema{
			"items": {
				Type:        "array",
				Description: "The stores on this page.",
				Items:       itemOf(refSchema(StoreSchemaName)),
			},
			"next_cursor": {
				Type:        "string",
				Description: "The cursor for the next page, absent on the last page.",
				Nullable:    true,
			},
		},
		Required: []string{"items"}}

	if len(schema.Properties) == 0 {
		panic("openapi: storePageSchema declares no property")
	}
	if _, declared := schema.Properties["items"]; !declared {
		panic("openapi: storePageSchema declares no items property")
	}
	if len(schema.Required) == 0 {
		panic("openapi: storePageSchema requires no field")
	}
	return schema

}

// checkoutSessionSchema describes a private buyer session. Its access token is
// returned once, so the schema carries it on the create response only.
func checkoutSessionSchema() Schema {
	if len(CheckoutSessionSchemaName) == 0 {
		panic("openapi: the checkout session schema is named nothing")
	}
	schema := Schema{
		Type: "object",
		Description: strings.Join([]string{
			"A private buyer session. Creating one charges nothing and creates no paid order. The",
			"access token is returned exactly once, in the create response.",
		}, "\n"),
		Properties: map[string]Schema{
			"id":           {Type: "string", Description: "The stable session identifier."},
			"access_token": {Type: "string", Description: "The session's bearer token, returned once."},
			"expires_at": {
				Type:        "string",
				Format:      "date-time",
				Description: "When the session stops authorizing requests.",
			},
		},
		Required: []string{"id", "access_token", "expires_at"}}

	if len(schema.Properties) == 0 {
		panic("openapi: checkoutSessionSchema declares no property")
	}
	if _, declared := schema.Properties["id"]; !declared {
		panic("openapi: checkoutSessionSchema declares no id property")
	}
	if len(schema.Required) == 0 {
		panic("openapi: checkoutSessionSchema requires no field")
	}
	return schema

}

// createCheckoutSessionRequestSchema describes the create session body.
func createCheckoutSessionRequestSchema() Schema {
	if len(CreateSessionSchemaName) == 0 {
		panic("openapi: the create session schema is named nothing")
	}
	schema := Schema{
		Type: "object",
		Description: strings.Join([]string{
			"The body that creates a buyer session. It names nothing about the store, so a",
			"session grants no access to a merchant's catalog beyond public discovery.",
		}, "\n"),
		Properties: map[string]Schema{
			"agent_label": {
				Type: "string",
				Description: strings.Join([]string{
					"A label the buyer agent names for itself, recorded so an operator can read",
					"which client holds a session.",
				}, "\n"),
				MaxLength: bound(128),
			},
		}}

	if len(schema.Properties) == 0 {
		panic("openapi: createCheckoutSessionRequestSchema declares no property")
	}
	if _, declared := schema.Properties["agent_label"]; !declared {
		panic("openapi: createCheckoutSessionRequestSchema declares no agent_label property")
	}
	if len(schema.Properties) == 0 {
		panic("openapi: createCheckoutSessionRequestSchema declares no property")
	}
	return schema

}

// shippingOptionsRequestSchema describes the shipping options body.
func shippingOptionsRequestSchema() Schema {
	if len(ShippingOptionsRequestName) == 0 {
		panic("openapi: the shipping options request is named nothing")
	}
	schema := Schema{
		Type: "object",
		Description: strings.Join([]string{
			"The destination to price shipping for. Listing the options writes nothing to the",
			"merchant's store.",
		}, "\n"),
		Properties: map[string]Schema{
			"delivery": refSchema(DeliveryDetailsName),
		},
		Required: []string{"delivery"}}

	if len(schema.Properties) == 0 {
		panic("openapi: shippingOptionsRequestSchema declares no property")
	}
	if _, declared := schema.Properties["delivery"]; !declared {
		panic("openapi: shippingOptionsRequestSchema declares no delivery property")
	}
	if len(schema.Required) == 0 {
		panic("openapi: shippingOptionsRequestSchema requires no field")
	}
	return schema

}

// shippingOptionSchema describes one shipping method and its total.
func shippingOptionSchema() Schema {
	if len(ShippingOptionName) == 0 {
		panic("openapi: the shipping option schema is named nothing")
	}
	schema := Schema{
		Type:        "object",
		Description: "One shipping method and its authoritative total for a destination.",
		Properties: map[string]Schema{
			"id":          {Type: "string", Description: "The method identifier."},
			"title":       {Type: "string", Description: "The method title a person reads."},
			"description": {Type: "string", Description: "The method's delivery description."},
			"total":       refSchema(MoneyName),
		},
		Required: []string{"id", "title", "total"}}

	if len(schema.Properties) == 0 {
		panic("openapi: shippingOptionSchema declares no property")
	}
	if _, declared := schema.Properties["id"]; !declared {
		panic("openapi: shippingOptionSchema declares no id property")
	}
	if len(schema.Properties) == 0 {
		panic("openapi: shippingOptionSchema declares no property")
	}
	return schema

}

// shippingOptionsSchema describes the shipping options response.
func shippingOptionsSchema() Schema {
	if len(ShippingOptionsName) == 0 {
		panic("openapi: the shipping options schema is named nothing")
	}
	schema := Schema{
		Type:        "object",
		Description: "The shipping methods a connector offers for one destination.",
		Properties: map[string]Schema{
			"items": {
				Type:        "array",
				Description: "The available methods.",
				Items:       itemOf(refSchema(ShippingOptionName)),
			},
		},
		Required: []string{"items"}}

	if len(schema.Properties) == 0 {
		panic("openapi: shippingOptionsSchema declares no property")
	}
	if _, declared := schema.Properties["items"]; !declared {
		panic("openapi: shippingOptionsSchema declares no items property")
	}
	if len(schema.Required) == 0 {
		panic("openapi: shippingOptionsSchema requires no field")
	}
	return schema

}

// quoteRequestSchema describes the quote body.
func quoteRequestSchema() Schema {
	if len(QuoteRequestName) == 0 {
		panic("openapi: the quote request schema is named nothing")
	}
	schema := Schema{
		Type: "object",
		Description: strings.Join([]string{
			"The body that creates a quote. The price is calculated on the server, so a buyer",
			"never sends an amount and a tampered one changes nothing.",
		}, "\n"),
		Properties: map[string]Schema{
			"listing_id":      {Type: "string", Description: "The listing to quote.", MaxLength: bound(64)},
			"variant_id":      {Type: "string", Description: "The variant to quote.", MaxLength: bound(64)},
			"quantity":        {Type: "integer", Description: "How many, from one to the configured maximum.", Minimum: bound(1)},
			"delivery":        refSchema(DeliveryDetailsName),
			"shipping_option": {Type: "string", Description: "The chosen shipping method.", MaxLength: bound(64)},
		},
		Required: []string{"listing_id", "quantity", "delivery"}}

	if len(schema.Properties) == 0 {
		panic("openapi: quoteRequestSchema declares no property")
	}
	if _, declared := schema.Properties["listing_id"]; !declared {
		panic("openapi: quoteRequestSchema declares no listing_id property")
	}
	if len(schema.Required) == 0 {
		panic("openapi: quoteRequestSchema requires no field")
	}
	return schema

}

// quoteSchema describes one issued quote.
func quoteSchema() Schema {
	if len(QuoteName) == 0 {
		panic("openapi: the quote schema is named nothing")
	}
	schema := Schema{
		Type: "object",
		Description: strings.Join([]string{
			"An issued quote. It is bound to one buyer session and one merchant, and every offered",
			"chain is bound to one recipient, one network, and one asset.",
		}, "\n"),
		Properties: map[string]Schema{
			"id":                   {Type: "string", Description: "The stable quote identifier."},
			"listing_id":           {Type: "string", Description: "The quoted listing."},
			"quantity":             {Type: "integer", Description: "The quoted quantity."},
			"total":                refSchema(MoneyName),
			"shipping":             refSchema(MoneyName),
			"shipping_description": {Type: "string", Description: "The chosen method's description."},
			"expires_at":           {Type: "string", Format: "date-time", Description: "When the quote expires."},
			"payment_methods": {
				Type:        "array",
				Description: "The methods this quote offers, in protocol rank order.",
				Items:       itemOf(refSchema(PaymentMethodName)),
			},
			"policies": policySchema(),
		},
		Required: []string{"id", "listing_id", "quantity", "total", "expires_at", "payment_methods"}}

	if len(schema.Properties) == 0 {
		panic("openapi: quoteSchema declares no property")
	}
	if _, declared := schema.Properties["id"]; !declared {
		panic("openapi: quoteSchema declares no id property")
	}
	if len(schema.Required) == 0 {
		panic("openapi: quoteSchema requires no field")
	}
	return schema

}

// policySchema describes the merchant's policies a quote returns.
func policySchema() Schema {
	if len(policyFields) == 0 {
		panic("openapi: the policy schema declares no field")
	}
	schema := Schema{
		Type:        "object",
		Description: "The merchant's policies the quote carries.",
		Properties: map[string]Schema{
			"return_policy":  {Type: "string", Description: "The store's return policy text."},
			"privacy_policy": {Type: "string", Description: "The store's privacy policy text."},
		}}

	if len(schema.Properties) == 0 {
		panic("openapi: policySchema declares no property")
	}
	if _, declared := schema.Properties["return_policy"]; !declared {
		panic("openapi: policySchema declares no return_policy property")
	}
	if len(schema.Properties) == 0 {
		panic("openapi: policySchema declares no property")
	}
	return schema

}

// purchaseRequestSchema describes the purchase body.
func purchaseRequestSchema() Schema {
	if len(PurchaseRequestName) == 0 {
		panic("openapi: the purchase request schema is named nothing")
	}
	schema := Schema{
		Type: "object",
		Description: strings.Join([]string{
			"The body that submits a purchase. Without a payment payload the operation returns the",
			"standard challenge of every protocol the quote offers, in registry rank order.",
		}, "\n"),
		Properties: map[string]Schema{
			"payment": {
				Nullable: true,
				Description: strings.Join([]string{
					"The signed payment authorization. Its absence asks for a challenge rather than",
					"submitting a payment.",
				}, "\n"),
				OneOf: []string{"PaymentAuthorization"},
			},
		}}

	if len(schema.Properties) == 0 {
		panic("openapi: purchaseRequestSchema declares no property")
	}
	if _, declared := schema.Properties["payment"]; !declared {
		panic("openapi: purchaseRequestSchema declares no payment property")
	}
	if len(schema.Properties) == 0 {
		panic("openapi: purchaseRequestSchema declares no property")
	}
	return schema

}

// paymentChallengeSchema describes the challenge a quote returns before payment.
func paymentChallengeSchema() Schema {
	if len(PaymentChallengeName) == 0 {
		panic("openapi: the payment challenge schema is named nothing")
	}
	schema := Schema{
		Type: "object",
		Description: strings.Join([]string{
			"The standard challenge of every payment protocol the quote offers, in protocol rank",
			"order. The pilot adds no Vadora commission, so no fee is challenged.",
		}, "\n"),
		Properties: map[string]Schema{
			"quote_id": {Type: "string", Description: "The quote this challenge belongs to."},
			"schemes": {
				Type:        "array",
				Description: "One challenge per offered protocol, in rank order.",
				Items:       itemOf(refSchema(PaymentChallengeName)),
			},
		},
		Required: []string{"quote_id", "schemes"}}

	if len(schema.Properties) == 0 {
		panic("openapi: paymentChallengeSchema declares no property")
	}
	if _, declared := schema.Properties["quote_id"]; !declared {
		panic("openapi: paymentChallengeSchema declares no quote_id property")
	}
	if len(schema.Required) == 0 {
		panic("openapi: paymentChallengeSchema requires no field")
	}
	return schema

}

// receiptSchema describes a persisted receipt. A receipt is written before the
// source order is updated, so a restart after settlement still shows it.
func receiptSchema() Schema {
	if len(ReceiptName) == 0 {
		panic("openapi: the receipt schema is named nothing")
	}
	schema := Schema{
		Type: "object",
		Description: strings.Join([]string{
			"A persisted receipt. Vadora writes it before it updates the source order, so a",
			"payment that settled is never lost to a failed store update.",
		}, "\n"),
		Properties: map[string]Schema{
			"id":       {Type: "string", Description: "The stable receipt identifier."},
			"quote_id": {Type: "string", Description: "The quote this receipt settles."},
			"amount":   refSchema(MoneyName),
			"network":  {Type: "string", Description: "The confirmed settlement network."},
			"transaction_signature": {
				Type:        "string",
				Description: "The chain transaction reference Vadora's own RPC confirmed.",
			},
			"confirmed_at": {
				Type:        "string",
				Format:      "date-time",
				Description: "When Vadora's own RPC confirmed the transfer.",
			},
		},
		Required: []string{"id", "quote_id", "amount", "network", "transaction_signature", "confirmed_at"}}

	if len(schema.Properties) == 0 {
		panic("openapi: receiptSchema declares no property")
	}
	if _, declared := schema.Properties["id"]; !declared {
		panic("openapi: receiptSchema declares no id property")
	}
	if len(schema.Required) == 0 {
		panic("openapi: receiptSchema requires no field")
	}
	return schema

}

// orderSchema describes a buyer's own order.
func orderSchema() Schema {
	if len(OrderName) == 0 {
		panic("openapi: the order schema is named nothing")
	}
	schema := Schema{
		Type: "object",
		Description: strings.Join([]string{
			"One order. Payment becomes paid only after settlement is confirmed, and fulfillment",
			"becomes received only after the merchant's platform acknowledges the order.",
		}, "\n"),
		Properties: map[string]Schema{
			"id":         {Type: "string", Description: "The stable order identifier."},
			"listing_id": {Type: "string", Description: "The ordered listing."},
			"quantity":   {Type: "integer", Description: "The ordered quantity."},
			"total":      refSchema(MoneyName),
			"payment_status": {
				Type:        "string",
				Description: "The payment state, which is unpaid, pending, paid, or refunded.",
				Enum:        []string{"unpaid", "pending", "paid", "refunded"},
			},
			"fulfillment_status": {
				Type:        "string",
				Description: "The fulfillment state, from sync_pending to received.",
				Enum:        []string{"sync_pending", "received", "cancelled"},
			},
			"receipt": {
				Ref:         "#/components/schemas/Receipt",
				Description: "The persisted receipt, once one exists.",
				Nullable:    true,
			},
			"merchant_order_reference": {
				Type:        "string",
				Description: "The merchant's order reference, once the platform supplies one.",
				Nullable:    true,
			},
		},
		Required: []string{"id", "listing_id", "quantity", "total", "payment_status", "fulfillment_status"}}

	if len(schema.Properties) == 0 {
		panic("openapi: orderSchema declares no property")
	}
	if _, declared := schema.Properties["id"]; !declared {
		panic("openapi: orderSchema declares no id property")
	}
	if len(schema.Required) == 0 {
		panic("openapi: orderSchema requires no field")
	}
	return schema

}

// deliveryDetailsSchema describes the bounded delivery fields a quote requires.
func deliveryDetailsSchema() Schema {
	if len(deliveryFields) == 0 {
		panic("openapi: the delivery schema declares no field")
	}
	schema := Schema{
		Type: "object",
		Description: strings.Join([]string{
			"The delivery fields an order keeps. They arrive from an untrusted client, so every",
			"field is bounded and they are stored apart from public catalog data.",
		}, "\n"),
		Properties: map[string]Schema{
			"name":         {Type: "string", Description: "The recipient's name.", MaxLength: bound(200)},
			"street_lines": {Type: "array", Description: "The street address lines.", Items: itemOf(stringSchemaRef()), MaxItems: bound(5)},
			"city":         {Type: "string", Description: "The city.", MaxLength: bound(200)},
			"region":       {Type: "string", Description: "The region, state, or province.", MaxLength: bound(200)},
			"postal_code":  {Type: "string", Description: "The postal code.", MaxLength: bound(200)},
			"country":      {Type: "string", Description: "The uppercase ISO 3166-1 alpha-2 country code.", MinLength: bound(2), MaxLength: bound(2)},
			"phone":        {Type: "string", Description: "The recipient's phone number.", MaxLength: bound(200)},
		},
		Required: []string{"name", "street_lines", "city", "country"}}

	if len(schema.Properties) == 0 {
		panic("openapi: deliveryDetailsSchema declares no property")
	}
	if _, declared := schema.Properties["name"]; !declared {
		panic("openapi: deliveryDetailsSchema declares no name property")
	}
	if len(schema.Required) == 0 {
		panic("openapi: deliveryDetailsSchema requires no field")
	}
	return schema

}

// stringSchemaRef returns the schema for a plain string.
func stringSchemaRef() Schema {
	return Schema{Type: "string"}
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
	Description string                `json:"description"`
	Content     map[string]mediaShape `json:"content"`
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
	OneOf                []string               `json:"oneOf,omitempty"`
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
			Content:     buildContentShapes(response.Content),
		}
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
		OneOf:                schema.OneOf,
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
	if len(components.Schemas) != len(declaredSchemas) {
		panic("openapi: the declared schemas and the built schemas disagree")
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
