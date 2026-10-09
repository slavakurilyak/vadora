package mcp

import (
	"strconv"
	"strings"

	"github.com/vadora/vadora/api/openapi"
)

// maxComparisonItems bounds how many listings one comparison carries, so a
// comparison cannot become an unbounded table a model must read.
const maxComparisonItems = 4

// maxCartItems bounds how many listings a cart keeps.
const maxCartItems = 1

// searchProductsTool lets a shopper search the public catalog. It is read only and
// needs no credential, because public discovery exposes no buyer detail.
func searchProductsTool() Tool {
	codes := sortedCodes()
	zero := int64(0)
	return Tool{
		Name:     "search_products",
		Title:    "Search products",
		ReadOnly: true,
		Description: strings.Join([]string{
			"Searches Vadora's public product catalog and returns published listings with their",
			"prices and the payment methods each offers. It creates no quote, no cart entry, and",
			"no order. May return: " + codeList(codes),
		}, "\n"),
		InputSchema: openapi.Schema{
			Type:        SchemaObjectType,
			Description: "The search to run. An empty query returns the newest published listings.",
			Properties: map[string]openapi.Schema{
				"merchant":            stringField("Restrict the search to a merchant name."),
				"cursor":              stringField("Continue the previous page."),
				"min_price_usd":       {Type: "number", Minimum: &zero},
				"max_price_usd":       {Type: "number", Minimum: &zero},
				"destination_country": {Type: "string", MaxLength: lengthPointer(2)},
				"attributes":          {Type: "object", AdditionalProperties: true},
				"query":               stringField("Words to match against a listing's title and category."),
				"store_id":            identifierField("Restrict the search to one store."),
				"category":            stringField("Restrict the search to one category."),
				"limit":               boundedCount("How many listings to return, at most one page."),
			},
		},
	}
}

// listStoresTool lets a shopper read the public merchant directory. It is read only
// and needs no credential.
func listStoresTool() Tool {
	codes := sortedCodes()
	return Tool{
		Name:     "list_stores",
		Title:    "List stores",
		ReadOnly: true,
		Description: strings.Join([]string{
			"Lists the merchants with at least one published listing, with each store's categories,",
			"approved destinations, and available purchase methods. A store with no published",
			"listing never appears. May return: " + codeList(codes),
		}, "\n"),
		InputSchema: openapi.Schema{
			Type:        SchemaObjectType,
			Description: "Which stores to list and how many of them.",
			Properties: map[string]openapi.Schema{
				"category": stringField("Restrict the listing to one category."),
				"cursor":   stringField("The page cursor from a previous call."),
				"limit":    boundedCount("How many stores to return."),
			},
		},
	}
}

// getProductTool lets a shopper read one listing. It is read only.
func getProductTool() Tool {
	codes := sortedCodes()
	return Tool{
		Name:     "get_product",
		Title:    "Get a product",
		ReadOnly: true,
		Description: strings.Join([]string{
			"Returns one published listing with its variants, its price, and the payment methods it",
			"offers. An unpublished listing returns NOT_FOUND, so a shopper cannot probe for a",
			"listing a merchant has not published. May return: " + codeList(codes),
		}, "\n"),
		InputSchema: openapi.Schema{
			Type:        SchemaObjectType,
			Description: "The listing to read.",
			Properties: map[string]openapi.Schema{
				"product_id": identifierField("The listing to read."),
			},
			Required: []string{"product_id"},
		},
	}
}

// compareProductsTool lets a shopper compare listings side by side. It is read only
// and returns data, never a recommendation to buy.
func compareProductsTool() Tool {
	codes := sortedCodes()
	return Tool{
		Name:     "compare_products",
		Title:    "Compare products",
		ReadOnly: true,
		Description: strings.Join([]string{
			"Returns several listings side by side with their prices, variants, and purchase",
			"methods so a shopper can compare them. It returns the listings as the merchants",
			"published them and adds no recommendation, so imported product text can never",
			"authorize a purchase. May return: " + codeList(codes),
		}, "\n"),
		InputSchema: openapi.Schema{
			Type:        SchemaObjectType,
			Description: "The listings to compare.",
			Properties: map[string]openapi.Schema{
				"product_ids": {
					Type: "array",
					Description: "The listings to compare, at most " +
						itoa(maxComparisonItems) + " of them.",
					Items:    &openapi.Schema{Type: "string", MaxLength: lengthPointer(maxIdentifierLength)},
					MaxItems: lengthPointer(maxComparisonItems),
				},
			},
			Required: []string{"product_ids"},
		},
	}
}

// setCartTool keeps the shopper's cart inside Vadora. It writes only the cart: it
// creates no quote and moves no funds.
func setCartTool() Tool {
	return Tool{
		Name: "set_cart", Title: "Set the cart", RequiresAccount: true, Scope: "shopper", Destructive: true,
		Description: "Replaces the account's single cart selection. It creates no quote or payment. May return: " + codeList(sortedCodes()),
		InputSchema: openapi.Schema{Type: "object", Properties: map[string]openapi.Schema{
			"listing_id": identifierField("The published listing to select."),
			"variant_id": identifierField("The variant belonging to the listing."),
			"quantity":   {Type: "integer", Minimum: lengthPointer(1), Maximum: lengthPointer(100)},
		}, Required: []string{"listing_id", "quantity"}},
	}
}

// startCheckoutTool returns the link a shopper finishes on Vadora's own domain. It
// hands off rather than paying: the plugin never signs and never moves funds.
func startCheckoutTool() Tool {
	return Tool{
		Name: "start_checkout", Title: "Start checkout", RequiresAccount: true, Scope: "shopper",
		Description: "Returns a checkout link on Vadora's own domain for the cart or an explicit selection. It creates no session, quote, payment attempt, or source order. Pay only on checkout.vadora.xyz. May return: " + codeList(sortedCodes()),
		InputSchema: openapi.Schema{Type: "object", Properties: map[string]openapi.Schema{
			"listing_id": identifierField("The published listing to select, with variant and quantity."),
			"variant_id": identifierField("The variant belonging to the listing."),
			"quantity":   {Type: "integer", Minimum: lengthPointer(1), Maximum: lengthPointer(100)},
		}},
	}
}

// getOrderStatusTool lets a shopper read an order it already bought. It is read only,
// and it returns another buyer's order as NOT_FOUND.
func getOrderStatusTool() Tool {
	codes := sortedCodes()
	return Tool{
		Name:            "get_order_status",
		RequiresAccount: true, Scope: "shopper",
		Title:    "Get an order's status",
		ReadOnly: true,
		Description: strings.Join([]string{
			"Returns one order with its payment state, its fulfillment state, and its receipt. A",
			"confirmed payment shows as paid and sync_pending until the merchant's platform",
			"acknowledges the order, and as received only afterwards. Another buyer's order",
			"returns NOT_FOUND. May return: " + codeList(codes),
		}, "\n"),
		InputSchema: openapi.Schema{
			Type:        SchemaObjectType,
			Description: "The order to read.",
			Properties: map[string]openapi.Schema{
				"order_id": identifierField("The order to read."),
			},
			Required: []string{"order_id"},
		},
	}
}

// itoa writes a count as text, so a bound appears in a description without the
// description repeating the number and drifting from the constant it names.
func itoa(value int) string {
	if value < 0 {
		panic("mcp: a negative count was written as text")
	}
	if value > MaxWrittenCount {
		panic("mcp: a count above its bound was written as text")
	}
	written := strconv.Itoa(value)
	if written == "" {
		panic("mcp: a count was written as empty text")
	}
	return written
}

// MaxWrittenCount bounds the count a description may write as text. It is above
// every count the contract names, so a bound that grows past it is caught here
// rather than silently truncated in a tool description.
const MaxWrittenCount = 100

// moneySchemaRef returns the schema for a price, so every tool that shows a price
// shows it in the same shape the REST document publishes.
func moneySchemaRef() openapi.Schema {
	if openapi.MoneyName == "" {
		panic("mcp: the money schema is named nothing")
	}
	return openapi.Schema{Ref: openapi.SchemaRefPrefix + openapi.MoneyName}
}
