package rest

import "github.com/vadora/vadora/api/types/domain"

// Document is one request or response document under its published schema name.
type Document struct {
	// Name is the schema name in the OpenAPI document.
	Name string
	// Description says what the document is, in one or two sentences.
	Description string
	// Sample is a zero value of the Go type that the server and every client use.
	Sample any
}

// Documents lists every document of the REST API in a fixed order. The OpenAPI
// document derives its schemas from these types, so the server's responses, the
// clients, and the published contract follow one definition and cannot drift
// apart (012:132).
func Documents() []Document {
	return []Document{
		{"SignInCodeRequest", "Asks Vadora to email a one time sign in code. The code works once and only for the request that sent it.", SignInCodeRequest{}},
		{"SignInCode", "Answers a code request. It carries no code, because the code travels only by email.", SignInCode{}},
		{"SignInRequest", "Completes a sign in with the emailed code.", SignInRequest{}},
		{"BuyerCredential", "A freshly issued buyer credential. The secret is shown once and stored only as a hash.", BuyerCredential{}},
		{"Account", "The signed in account as a sign in reports it.", Account{}},
		{"SessionSignIn", "The answer of an account sign in. The session travels in a secure, HTTP only cookie.", SessionSignIn{}},
		{"DeliveryDetails", "The delivery fields of one order, each bounded because they arrive from an untrusted client.", domain.DeliveryDetails{}},
		{"Image", "One merchant image, loaded directly from the platform's own domain.", Image{}},
		{"Store", "One entry of the public merchant directory.", Store{}},
		{"StorePage", "One page of the merchant directory.", StorePage{}},
		{"Product", "The public view of one published listing.", Product{}},
		{"ProductPage", "One page of published listings.", ProductPage{}},
		{"CreateCheckoutSessionRequest", "Opens a private buyer session for the store of one listing. The server fixes the network class from the store.", CreateCheckoutSessionRequest{}},
		{"CheckoutSession", "A private buyer session. Its token is returned once, to a caller that authenticates with a buyer credential.", CheckoutSession{}},
		{"ShippingOptionsRequest", "Asks for the shipping methods and totals of one destination.", ShippingOptionsRequest{}},
		{"ShippingOption", "One shipping method with the platform's authoritative totals, in the store's own currency.", ShippingOption{}},
		{"ShippingOptions", "The methods that the platform offers a destination.", ShippingOptions{}},
		{"QuoteRequest", "Asks for a quote within a checkout session. It carries no price, because the server computes every amount.", QuoteRequest{}},
		{"Offer", "One payment option of a quote: one chain, one protocol, one recipient, one network, and one asset.", Offer{}},
		{"Quote", "An immutable, expiring offer for one listing, quantity, destination, and shipping method.", Quote{}},
		{"Receipt", "The durable proof that a payment confirmed through Vadora's own RPC belongs to an order.", Receipt{}},
		{"Order", "One order as its buyer reads it. A confirmed payment shows paid and sync_pending until the merchant's platform acknowledges the order, and received only afterwards.", Order{}},
		{"OrderPage", "One page of a buyer's orders.", OrderPage{}},
	}
}
