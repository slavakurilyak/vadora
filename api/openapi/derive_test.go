package openapi

import (
	"reflect"
	"slices"
	"testing"

	"github.com/vadora/vadora/api/types/rest"
)

// The schemas come from the shared wire types, so a field the server returns is a
// field the document declares.
func TestEverySchemaIsDerivedFromItsWireType(t *testing.T) {
	t.Parallel()
	spec, err := Build()
	if err != nil {
		t.Fatalf("the document did not build: %v", err)
	}
	for _, document := range rest.Documents() {
		schema, present := spec.Components.Schemas[document.Name]
		if !present {
			t.Fatalf("the document declares no schema for %s", document.Name)
		}
		kind := reflect.TypeOf(document.Sample)
		for index := 0; index < kind.NumField(); index++ {
			name, optional, skip := jsonName(kind.Field(index))
			if skip {
				continue
			}
			if _, declared := schema.Properties[name]; !declared {
				t.Fatalf("%s drops the field %s", document.Name, name)
			}
			if slices.Contains(schema.Required, name) == optional {
				t.Fatalf("%s.%s is required=%v but the type says optional=%v", document.Name, name, !optional, optional)
			}
		}
		if schema.Description == "" {
			t.Fatalf("%s carries no description", document.Name)
		}
	}
}

// A buyer reads the order before the platform acknowledges it, so the receipt is
// optional, and a quote always lists its offers.
func TestTheQuoteAndOrderSchemasStateWhatIsAlwaysPresent(t *testing.T) {
	t.Parallel()
	spec, err := Build()
	if err != nil {
		t.Fatalf("the document did not build: %v", err)
	}
	quote := spec.Components.Schemas["Quote"]
	for _, field := range []string{"id", "order_id", "total_usd_minor", "network_class", "expires_at", "offers"} {
		if !slices.Contains(quote.Required, field) {
			t.Fatalf("a quote always carries %s", field)
		}
	}
	order := spec.Components.Schemas["Order"]
	if slices.Contains(order.Required, "receipt") {
		t.Fatal("an unpaid order has no receipt, so the receipt is optional")
	}
	for _, field := range []string{"payment_status", "fulfillment_status", "refund_status", "test"} {
		if !slices.Contains(order.Required, field) {
			t.Fatalf("an order always carries %s", field)
		}
	}
	if order.Properties["receipt"].Ref != SchemaRef("Receipt") {
		t.Fatal("the order's receipt refers to the Receipt schema")
	}
}

// A request carries no field that the server decides (012:92).
func TestNoRequestSchemaDeclaresAPriceATotalOrAStatus(t *testing.T) {
	t.Parallel()
	spec, err := Build()
	if err != nil {
		t.Fatalf("the document did not build: %v", err)
	}
	forbidden := []string{"price", "total", "total_usd_minor", "amount", "amount_base_units", "network_class", "status", "payment_status", "recipient"}
	for _, name := range []string{"CreateCheckoutSessionRequest", "ShippingOptionsRequest", "QuoteRequest"} {
		for field := range spec.Components.Schemas[name].Properties {
			if slices.Contains(forbidden, field) {
				t.Fatalf("%s declares the server decided field %s", name, field)
			}
		}
	}
}

// A Go type without a JSON Schema form is refused instead of documented wrongly.
func TestADerivationOfAnUnsupportedTypeIsRefused(t *testing.T) {
	t.Parallel()
	for _, sample := range []any{struct{ Ratio float64 }{}, struct{ Channel chan int }{}, struct{}{}} {
		if _, err := deriveSchema(sample, "x", nil); err == nil {
			t.Fatalf("a schema was derived from %T", sample)
		}
	}
}
