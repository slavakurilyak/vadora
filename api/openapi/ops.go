package openapi

import (
	"fmt"
	"strings"
)

// The two ways a buyer authenticates on the API origin. A session may be opened
// only with a buyer credential, because a checkout session token names an
// existing session and cannot create one.
var (
	credentialOnly = []map[string][]string{{"buyerCredential": {}}}
	buyerSecurity  = []map[string][]string{{"buyerCredential": {}}, {"checkoutSession": {}}}
	// merchantSecurity is the access token that the CLI's sign in issues. A merchant call
	// needs the merchant scope, and no buyer credential and no checkout session token works.
	merchantSecurity = []map[string][]string{{"merchantToken": {}}}
)

// Header names of the x402 version 2 flow (005:58, 005:66).
const (
	headerPaymentRequired  = "PAYMENT-REQUIRED"
	headerPaymentSignature = "PAYMENT-SIGNATURE"
	headerPaymentResponse  = "PAYMENT-RESPONSE"
)

// addOperations declares every operation of the REST API. The document describes
// exactly the routes that the server mounts on the API origin, and a test of the
// server compares the two lists.
func addOperations(spec *Specification) error {
	groups := [][]route{authRoutes(), discoveryRoutes(), checkoutRoutes(), paymentRoutes(), orderRoutes(), merchantRoutes()}
	for _, group := range groups {
		for _, entry := range group {
			if spec.Paths[entry.path] == nil {
				spec.Paths[entry.path] = map[string]Operation{}
			}
			spec.Paths[entry.path][entry.method] = entry.operation
			if err := checkOperation(entry.path, entry.method, entry.operation); err != nil {
				return err
			}
		}
	}
	return nil
}

// route is one operation at one path and method.
type route struct {
	path      string
	method    string
	operation Operation
}

// authRoutes describe the email sign in. Both steps are public and rate limited per
// email address and per client address, and Vadora names its own checkout domain in
// every sign in email as the only place to pay (001:64).
func authRoutes() []route {
	codes := Operation{
		OperationID: "requestSignInCode",
		Summary:     "Email a one time sign in code",
		Description: strings.Join([]string{
			"Sends a one time code to the address. The answer carries no code. A request with buyer set",
			"to true asks for a buyer credential, and any other request asks for an account session.",
		}, "\n"),
		Tags: []string{"auth"},
		RequestBody: &RequestBody{Description: "The address to sign in.", Required: true,
			Content: map[string]MediaType{"application/json": {Schema: refSchema("SignInCodeRequest")}}},
		Responses: map[string]Response{
			"202": jsonResponse("The code was sent, and the request ID binds the completion.", refSchema("SignInCode")),
			"400": errorResponse("The address or the request could not be read."),
			"429": errorResponse("The code request limit was reached for the address or the client."),
		},
	}
	complete := Operation{
		OperationID: "completeSignIn",
		Summary:     "Complete a sign in with the emailed code",
		Description: strings.Join([]string{
			"The code works once, only for the request that sent it, and stops working after a configured",
			"number of wrong attempts. A buyer sign in answers with a buyer credential, shown once. Any",
			"other sign in starts a session in a secure, HTTP only cookie and answers with the account and",
			"the token that guards later state changes.",
		}, "\n"),
		Tags: []string{"auth"},
		RequestBody: &RequestBody{Description: "The request ID, the address, and the code.", Required: true,
			Content: map[string]MediaType{"application/json": {Schema: refSchema("SignInRequest")}}},
		Responses: map[string]Response{
			"200": jsonResponse("A buyer credential for a buyer sign in, or an account session otherwise.",
				Schema{OneOf: []string{"BuyerCredential", "SessionSignIn"}}),
			"400": errorResponse("The request could not be read."),
			"401": errorResponse("The code is wrong, expired, or already used."),
			"429": errorResponse("Too many attempts."),
		},
	}
	return []route{{"/v1/auth/sign-in/codes", MethodPost, codes}, {"/v1/auth/sign-in", MethodPost, complete}}
}

func discoveryRoutes() []route {
	listStores := Operation{
		OperationID: "listStores",
		Summary:     "List stores with at least one published listing",
		Description: strings.Join([]string{
			"Returns the public merchant directory. Each store carries its categories, its approved",
			"destinations, and the purchase methods its listings offer. A store with no published",
			"listing never appears, and no buyer detail is exposed.",
		}, "\n"),
		Tags:       []string{"discovery"},
		Parameters: []Parameter{queryParameter("category", "Filter to one category.", false), cursorParameter("The page cursor, absent on the first page."), pageSizeParameter()},
		Responses: map[string]Response{
			"200": jsonResponse("One page of stores.", refSchema("StorePage")),
			"400": errorResponse("The request could not be read."),
		},
	}
	listProducts := Operation{
		OperationID: "listProducts",
		Summary:     "Search published listings",
		Description: strings.Join([]string{
			"Returns the public product catalog, ranked by relevance to the query. Each listing carries",
			"its price, its availability, and the purchase methods it offers. An attribute filter is",
			"written attr.color=red, and a price bound is a whole number of US cents.",
		}, "\n"),
		Tags: []string{"discovery"},
		Parameters: []Parameter{
			queryParameter("q", "The search text.", false),
			queryParameter("store_id", "Filter to one store.", false),
			queryParameter("merchant", "Filter to one merchant by name or domain.", false),
			queryParameter("category", "Filter to one category.", false),
			queryParameter("destination", "Filter to listings that ship to a two letter country.", false),
			integerQueryParameter("min_usd_cents", "The lowest price in US cents."),
			integerQueryParameter("max_usd_cents", "The highest price in US cents."),
			cursorParameter("The page cursor, absent on the first page."),
			pageSizeParameter(),
		},
		Responses: map[string]Response{
			"200": jsonResponse("One page of listings.", refSchema("ProductPage")),
			"400": errorResponse("The request could not be read."),
		},
	}
	getProduct := Operation{
		OperationID: "getProduct",
		Summary:     "Read one listing",
		Description: strings.Join([]string{
			"Returns one published listing. An unpublished listing returns NOT_FOUND, so a buyer cannot",
			"probe for a listing that the merchant has not published.",
		}, "\n"),
		Tags:       []string{"discovery"},
		Parameters: []Parameter{idParameter("id", "The listing identifier.")},
		Responses: map[string]Response{
			"200": jsonResponse("The listing.", refSchema("Product")),
			"404": errorResponse("The listing is not published."),
		},
	}
	return []route{{"/v1/stores", MethodGet, listStores}, {"/v1/products", MethodGet, listProducts}, {"/v1/products/{id}", MethodGet, getProduct}}
}

func checkoutRoutes() []route {
	create := Operation{
		OperationID: "createCheckoutSession",
		Summary:     "Open a buyer session",
		Description: strings.Join([]string{
			"Opens a private buyer session for the store of one listing and returns its token once. The",
			"server fixes the network class from the store, so a test store settles on a test network.",
			"Opening a session charges nothing and creates no order. A buyer credential holds one open",
			"session per store, so a second call returns that session without a token. A plugin sign in",
			"cannot open one.",
		}, "\n"),
		Tags: []string{"checkout"},
		RequestBody: &RequestBody{Description: "The listing whose store the session is for.", Required: true,
			Content: map[string]MediaType{"application/json": {Schema: refSchema("CreateCheckoutSessionRequest")}}},
		Security: credentialOnly,
		Responses: map[string]Response{
			"201": jsonResponse("The session and its token, returned once.", refSchema("CheckoutSession")),
			"200": jsonResponse("The open session of this credential for the store, returned again without a token.", refSchema("CheckoutSession")),
			"400": errorResponse("The request could not be read."),
			"401": errorResponse("No valid buyer credential was presented."),
			"404": errorResponse("The listing is not published, or the store is not available to this buyer."),
			"422": errorResponse("The store is disconnected or cannot sell this way."),
			"428": errorResponse("The buyer terms must be accepted on Vadora's own domain first."),
			"429": errorResponse("The session rate limit was reached."),
		},
	}
	shipping := Operation{
		OperationID: "listShippingOptions",
		Summary:     "List shipping options for a destination",
		Description: strings.Join([]string{
			"Returns the shipping methods and their authoritative totals for a destination, read through",
			"the connector. Listing the options writes nothing to the merchant's store, so a buyer can",
			"price shipping before deciding.",
		}, "\n"),
		Tags:       []string{"checkout"},
		Parameters: []Parameter{idParameter("id", "The checkout session identifier.")},
		RequestBody: &RequestBody{Description: "The destination to price.", Required: true,
			Content: map[string]MediaType{"application/json": {Schema: refSchema("ShippingOptionsRequest")}}},
		Security: buyerSecurity,
		Responses: map[string]Response{
			"200": jsonResponse("The available methods.", refSchema("ShippingOptions")),
			"400": errorResponse("The destination or the listing could not be accepted."),
			"401": errorResponse("No valid credential was presented."),
			"404": errorResponse("The session belongs to another buyer, or does not exist."),
			"422": errorResponse("The store cannot ship to that destination."),
			"429": errorResponse("The shipping rate limit was reached."),
			"503": errorResponse("The platform could not be reached."),
		},
	}
	return []route{{"/v1/checkout-sessions", MethodPost, create}, {"/v1/checkout-sessions/{id}/shipping-options", MethodPost, shipping}}
}

func paymentRoutes() []route {
	create := Operation{
		OperationID: "createQuote",
		Summary:     "Create a quote",
		Description: strings.Join([]string{
			"Creates a quote for a published listing of a connected, payment configured store. The server",
			"calculates the total from the platform's authoritative shipping and tax and from the locked",
			"exchange rate, and a body that names a price is not part of the contract. Creating a quote",
			"writes nothing to the merchant's store. A repeated Idempotency-Key with the same request",
			"returns the original quote, and a second quote in the same session replaces the first.",
		}, "\n"),
		Tags:       []string{"payments"},
		Parameters: []Parameter{idempotencyParameter()},
		RequestBody: &RequestBody{Description: "The quote to create.", Required: true,
			Content: map[string]MediaType{"application/json": {Schema: refSchema("QuoteRequest")}}},
		Security: buyerSecurity,
		Responses: map[string]Response{
			"201": jsonResponse("The issued quote.", refSchema("Quote")),
			"400": errorResponse("The request, the destination, or the product could not be accepted."),
			"401": errorResponse("No valid credential was presented."),
			"404": errorResponse("The listing or the session was not found."),
			"422": errorResponse("The store or the payment method is unavailable."),
			"428": errorResponse("The buyer terms must be accepted on Vadora's own domain first."),
			"429": errorResponse("A rate or quote limit was reached, or an earlier payment is still pending."),
			"503": errorResponse("The platform or the exchange rate could not be reached."),
		},
	}
	get := Operation{
		OperationID: "getQuote",
		Summary:     "Read one quote",
		Description: "Returns a quote of the caller's own session, with its offers and its expiry.",
		Tags:        []string{"payments"},
		Parameters:  []Parameter{idParameter("id", "The quote identifier.")},
		Security:    buyerSecurity,
		Responses: map[string]Response{
			"200": jsonResponse("The quote.", refSchema("Quote")),
			"401": errorResponse("No valid credential was presented."),
			"404": errorResponse("The quote belongs to another buyer, or does not exist."),
		},
	}
	return []route{{"/v1/quotes", MethodPost, create}, {"/v1/quotes/{id}", MethodGet, get}, {"/v1/quotes/{id}/purchase", MethodPost, purchase()}}
}

// purchase describes the x402 purchase of one quote. A request without a payment
// answers HTTP 402 with the challenge, a request with a payment in
// PAYMENT-SIGNATURE settles it, and every other answer is a catalog error
// (005:58, 005:66, 001:344).
func purchase() Operation {
	signature := Parameter{
		Name: headerPaymentSignature, In: "header", Required: false,
		Description: "The base64 x402 version 2 payment payload that the buyer signed with their own wallet. Without it the operation answers with the challenge.",
		Schema:      Schema{Type: "string", MaxLength: bound(65536)},
	}
	return Operation{
		OperationID: "purchaseQuote",
		Summary:     "Pay a quote with x402",
		Description: strings.Join([]string{
			"Answers HTTP 402 with the x402 challenge in the PAYMENT-REQUIRED header for an unpaid,",
			"unexpired quote that has no claimed attempt. A request that carries the signed payment in",
			"PAYMENT-SIGNATURE claims the quote once, verifies the payment, creates the source order,",
			"settles, and answers with the order only after Vadora's own RPC confirms the transfer. The",
			"outcome travels in PAYMENT-RESPONSE. While an earlier attempt may have settled the operation",
			"answers PAYMENT_PENDING and never offers a second payment. The version 1 X-PAYMENT header is",
			"refused.",
		}, "\n"),
		Tags:       []string{"payments"},
		Parameters: []Parameter{idParameter("id", "The quote identifier."), idempotencyParameter(), signature},
		Security:   buyerSecurity,
		Responses: map[string]Response{
			"200": Response{Description: "The order after a confirmed settlement, or the existing order of a settled quote.",
				Headers: map[string]string{headerPaymentResponse: "The base64 settlement outcome that the facilitator reported."},
				Content: map[string]MediaType{"application/json": {Schema: refSchema("Order")}}},
			"402": Response{Description: "The x402 challenge of an unpaid quote.",
				Headers: map[string]string{headerPaymentRequired: "The same challenge document, base64 encoded."},
				Content: map[string]MediaType{"application/json": {Schema: refSchema("PaymentRequired")}}},
			"400": errorResponse("The request, the Idempotency-Key, or the payment could not be read."),
			"401": errorResponse("No valid credential was presented."),
			"404": errorResponse("The quote belongs to another buyer, or does not exist."),
			"409": errorResponse("The payment was rejected before any funds moved."),
			"410": errorResponse("The quote expired, so the buyer requests a new quote."),
			"422": errorResponse("The quote offered no supported payment method, or the payment uses a method that the quote did not offer."),
			"429": errorResponse("A payment attempt is pending: wait, and never pay again."),
		},
	}
}

func orderRoutes() []route {
	list := Operation{
		OperationID: "listOrders",
		Summary:     "List this buyer's orders",
		Description: "Returns the orders that the caller's account owns, newest first. Another buyer's order is never reachable.",
		Tags:        []string{"orders"},
		Parameters: []Parameter{
			queryParameter("payment_status", "Filter to one payment status.", false),
			cursorParameter("The page cursor, absent on the first page."), pageSizeParameter(),
		},
		Security: buyerSecurity,
		Responses: map[string]Response{
			"200": jsonResponse("One page of orders.", refSchema("OrderPage")),
			"400": errorResponse("The request could not be read."),
			"401": errorResponse("No valid credential was presented."),
		},
	}
	get := Operation{
		OperationID: "getOrder",
		Summary:     "Read one order",
		Description: strings.Join([]string{
			"Returns one order with its payment status, its fulfillment status, and its receipt. A",
			"confirmed payment shows paid and sync_pending before the merchant's platform acknowledges",
			"the order, and received only afterwards. A test network order is labeled a test.",
		}, "\n"),
		Tags:       []string{"orders"},
		Parameters: []Parameter{idParameter("id", "The order identifier.")},
		Security:   buyerSecurity,
		Responses: map[string]Response{
			"200": jsonResponse("The order.", refSchema("Order")),
			"401": errorResponse("No valid credential was presented."),
			"404": errorResponse("The order belongs to another buyer, or does not exist."),
			"429": errorResponse("Too many order reads for this account, credential, or address in the last hour. Retry after the interval that the answer gives."),
		},
	}
	return []route{{"/v1/orders", MethodGet, list}, {"/v1/orders/{id}", MethodGet, get}}
}

// checkRoutes refuses a document in which two operations share an identifier.
func checkRoutes(spec *Specification) error {
	seen := map[string]string{}
	for path, operations := range spec.Paths {
		for method, operation := range operations {
			if other, duplicate := seen[operation.OperationID]; duplicate {
				return fmt.Errorf("the operations %s and %s %s share the identifier %s", other, method, path, operation.OperationID)
			}
			seen[operation.OperationID] = method + " " + path
		}
	}
	return nil
}

// checkSchemas refuses a schema that names a reference that does not exist or a
// required field that it does not declare, because a generated client would fail
// on either at build time rather than at document time.
func checkSchemas(spec *Specification) error {
	if len(spec.Components.Schemas) == 0 {
		return fmt.Errorf("the document declares no schema")
	}
	for name, schema := range spec.Components.Schemas {
		if err := checkSchema(name, schema, spec.Components.Schemas); err != nil {
			return err
		}
	}
	return nil
}

// checkSchema walks one schema and every schema nested inside it with an explicit
// worklist, so a deeply nested schema cannot grow the stack.
func checkSchema(name string, schema Schema, known map[string]Schema) error {
	type pending struct {
		name   string
		schema Schema
	}
	work := []pending{{name, schema}}
	for len(work) > 0 {
		current := work[len(work)-1]
		work = work[:len(work)-1]
		if current.schema.Ref != "" {
			target := strings.TrimPrefix(current.schema.Ref, SchemaRefPrefix)
			if target == current.schema.Ref {
				return fmt.Errorf("the schema %s references %s outside the document", current.name, current.schema.Ref)
			}
			if _, declared := known[target]; !declared {
				return fmt.Errorf("the schema %s references the unknown schema %s", current.name, target)
			}
			continue
		}
		if current.schema.Type == "" {
			if len(current.schema.OneOf) == 0 {
				return fmt.Errorf("the schema %s declares no type, reference, or choice", current.name)
			}
		}
		for _, required := range current.schema.Required {
			if _, declared := current.schema.Properties[required]; !declared {
				return fmt.Errorf("the schema %s requires the undeclared field %s", current.name, required)
			}
		}
		if current.schema.Items != nil {
			work = append(work, pending{current.name + ".items", *current.schema.Items})
		}
		for field, property := range current.schema.Properties {
			work = append(work, pending{current.name + "." + field, property})
		}
	}
	return nil
}

// merchantRoutes describe the store operations of a signed in merchant: the optional
// equivalents of the hosted workspace's connection and refresh (001:46). Each answers only
// to the access token of an account that owns the store, with the merchant scope. They
// accept no spending input and move no money, and publication, the payout wallet, and
// every refund stay in the hosted workspace (001:247).
func merchantRoutes() []route {
	unauthorized := errorResponse("No valid merchant access token was presented.")
	return []route{{"/v1/merchant/stores", MethodGet, listMyStoresOperation(unauthorized)},
		{"/v1/stores/detect", MethodPost, detectOperation(unauthorized)},
		{"/v1/stores/connect", MethodPost, connectOperation(unauthorized)},
		{"/v1/stores/{id}/sync", MethodPost, syncOperation(unauthorized)},
		{"/v1/stores/{id}/readiness", MethodGet, readinessOperation(unauthorized)}}
}

func listMyStoresOperation(unauthorized Response) Operation {
	return Operation{
		OperationID: "listMyStores",
		Summary:     "List the stores that this merchant owns",
		Description: "Returns the stores that the signed in account owns, oldest first, with their connection status and listing counts. Another merchant's store never appears.",
		Tags:        []string{"merchant"},
		Security:    merchantSecurity,
		Responses: map[string]Response{
			"200": jsonResponse("The merchant's stores.", refSchema("MerchantStorePage")),
			"401": unauthorized,
		},
	}
}

func detectOperation(unauthorized Response) Operation {
	return Operation{
		OperationID: "detectPlatform",
		Summary:     "Detect the commerce platform behind a store URL",
		Description: strings.Join([]string{
			"Reads bounded public documents from the store's own origin and reports the evidence. It",
			"contacts no platform for access and follows no redirect. An address that is not a public",
			"HTTPS origin is refused, a named deferred platform is reported with the status",
			"coming_soon, and a store that does not answer is reported with the status unreachable.",
		}, "\n"),
		Tags: []string{"merchant"},
		RequestBody: &RequestBody{Description: "The store's public URL.", Required: true,
			Content: map[string]MediaType{"application/json": {Schema: refSchema("StoreURLRequest")}}},
		Security: merchantSecurity,
		Responses: map[string]Response{
			"200": jsonResponse("The public evidence.", refSchema("PlatformDetection")),
			"400": errorResponse("The store URL is not an approved public origin."),
			"401": unauthorized,
			"429": errorResponse("Too many detections for this account in the last hour. Retry after the interval that the answer gives."),
		},
	}
}

func connectOperation(unauthorized Response) Operation {
	return Operation{
		OperationID: "connectStore",
		Summary:     "Start the connection of an invited store",
		Description: strings.Join([]string{
			"Starts the connection of a store that Vadora invited this account to connect. The answer",
			"carries the platform's approval link, which the merchant opens in their own browser. A",
			"store without an invitation for this account's email returns STORE_UNSUPPORTED with the",
			"reason invitation_required, and a store on a named deferred platform returns the reason",
			"platform_coming_soon. Both refusals offer to record the merchant's interest at the",
			"hosted workspace address that the answer names. The merchant agreement must be accepted",
			"first.",
		}, "\n"),
		Tags: []string{"merchant"},
		RequestBody: &RequestBody{Description: "The store's public URL.", Required: true,
			Content: map[string]MediaType{"application/json": {Schema: refSchema("StoreURLRequest")}}},
		Security: merchantSecurity,
		Responses: map[string]Response{
			"200": jsonResponse("The approval links.", refSchema("StoreConnection")),
			"400": errorResponse("The store URL is not an approved public origin."),
			"401": unauthorized,
			"422": errorResponse("The store has no invitation for this account, or the platform is not supported yet."),
			"428": errorResponse("The merchant agreement must be accepted in the hosted workspace first."),
		},
	}
}

func syncOperation(unauthorized Response) Operation {
	return Operation{
		OperationID: "syncStore",
		Summary:     "Read the store's products again",
		Description: strings.Join([]string{
			"Reads the connected store's physical products into the merchant's private preview. Every",
			"new listing starts unpublished, and publishing needs the merchant's confirmation in the",
			"hosted workspace. Repeating the call changes nothing beyond what the platform changed.",
		}, "\n"),
		Tags:       []string{"merchant"},
		Parameters: []Parameter{idParameter("id", "The store identifier.")},
		Security:   merchantSecurity,
		Responses: map[string]Response{
			"200": jsonResponse("What the refresh did.", refSchema("StoreSync")),
			"401": unauthorized,
			"404": errorResponse("The store belongs to another merchant, or does not exist."),
			"429": errorResponse("Too many refreshes for this account in the last hour. Retry after the interval that the answer gives."),
		},
	}
}

func readinessOperation(unauthorized Response) Operation {
	return Operation{
		OperationID: "getStoreReadiness",
		Summary:     "Read the store's readiness checklist",
		Description: "Returns the checklist in its fixed order. Each completed item carries the evidence that completed it, read from the records themselves.",
		Tags:        []string{"merchant"},
		Parameters:  []Parameter{idParameter("id", "The store identifier.")},
		Security:    merchantSecurity,
		Responses: map[string]Response{
			"200": jsonResponse("The checklist.", refSchema("StoreReadiness")),
			"401": unauthorized,
			"404": errorResponse("The store belongs to another merchant, or does not exist."),
		},
	}
}
