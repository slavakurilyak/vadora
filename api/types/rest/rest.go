// Package rest holds the request and response documents of Vadora's REST API. The
// server and every client build from these types, so a client sends and reads
// exactly what the server decides. Amounts are integers: store currency amounts in
// minor units, US dollar amounts in cents, and stablecoin amounts in token base
// units (shared:36). A client never sends a price, a total, a network class, or a
// status, because the server computes and decides all of them (012:92).
package rest

import "github.com/vadora/vadora/api/types/domain"

// The three headers of the x402 version 2 HTTP flow. A purchase without a payment
// answers HTTP 402 with the challenge in the first header, the buyer pays in the
// second, and a confirmed settlement answers in the third (005:58, 005:66).
const (
	HeaderPaymentRequired  = "PAYMENT-REQUIRED"
	HeaderPaymentSignature = "PAYMENT-SIGNATURE"
	HeaderPaymentResponse  = "PAYMENT-RESPONSE"
	// HeaderIdempotencyKey is required on every mutating buyer call (001:328).
	HeaderIdempotencyKey = "Idempotency-Key"
)

// SignInCodeRequest asks Vadora to email a one time sign in code. A buyer request
// asks for a buyer credential, and any other request asks for an account session.
// The code is accepted only for the request that sent it (001:64).
type SignInCodeRequest struct {
	Email string `json:"email"`
	Buyer bool   `json:"buyer,omitempty"`
}

// SignInCode answers a code request. It carries no code: the code travels only by
// email, and the request ID binds the completion to this request.
type SignInCode struct {
	RequestID string `json:"request_id"`
	ExpiresAt string `json:"expires_at"`
}

// SignInRequest completes a sign in with the emailed code.
type SignInRequest struct {
	RequestID string `json:"request_id"`
	Email     string `json:"email"`
	Code      string `json:"code"`
	Buyer     bool   `json:"buyer,omitempty"`
}

// BuyerCredential is a freshly issued buyer credential. The server shows the secret
// once and stores only its hash (001:74).
type BuyerCredential struct {
	Credential string `json:"credential"`
	ID         string `json:"id"`
	ExpiresAt  string `json:"expires_at"`
}

// Account is the signed in account as a sign in reports it.
type Account struct {
	ID               string `json:"id"`
	Email            string `json:"email"`
	AccountMode      string `json:"account_mode"`
	PreferredCountry string `json:"preferred_country"`
}

// SessionSignIn is the answer of an account sign in. The session itself travels in
// a secure, HTTP only cookie, and the token guards every later state change.
type SessionSignIn struct {
	Account   Account `json:"account"`
	CSRFToken string  `json:"csrf_token"`
}

// CreateCheckoutSessionRequest opens a private buyer session for the store of one
// listing. The server fixes the network class from the store, never from the
// request (001:516).
type CreateCheckoutSessionRequest struct {
	ListingID string `json:"listing_id"`
}

// CheckoutSession is a private buyer session. Its token authorizes only that
// session and is returned once, to a client that authenticates with a buyer
// credential. The checkout page signs in with its own cookie and receives no
// token.
type CheckoutSession struct {
	ID           string `json:"id"`
	Token        string `json:"token,omitempty"`
	StoreID      string `json:"store_id"`
	NetworkClass string `json:"network_class"`
	ExpiresAt    string `json:"expires_at"`
}

// ShippingOptionsRequest asks for the shipping methods and totals of a destination.
type ShippingOptionsRequest struct {
	ListingID string                 `json:"listing_id"`
	Quantity  int                    `json:"quantity"`
	Delivery  domain.DeliveryDetails `json:"delivery"`
}

// ShippingOption is one shipping method with the platform's authoritative totals,
// in the store's own currency (001:322).
type ShippingOption struct {
	Handle        string `json:"handle"`
	Title         string `json:"title"`
	Description   string `json:"description,omitempty"`
	Currency      string `json:"currency"`
	SubtotalMinor int64  `json:"subtotal_minor"`
	ShippingMinor int64  `json:"shipping_minor"`
	TaxMinor      int64  `json:"tax_minor"`
	TotalMinor    int64  `json:"total_minor"`
}

// ShippingOptions lists the methods that the platform offers a destination.
type ShippingOptions struct {
	Options []ShippingOption `json:"options"`
}

// QuoteRequest asks for a quote within a checkout session. It carries the listing,
// the quantity, the delivery details, and the chosen shipping method. It carries no
// price (001:328).
type QuoteRequest struct {
	// SessionID names the checkout session when the caller is not a session token.
	SessionID      string                 `json:"session_id,omitempty"`
	ListingID      string                 `json:"listing_id"`
	Quantity       int                    `json:"quantity"`
	Delivery       domain.DeliveryDetails `json:"delivery"`
	ShippingMethod string                 `json:"shipping_method"`
}

// Offer is one payment option of a quote: one chain, one protocol, one recipient,
// one network, and one asset (shared:34).
type Offer struct {
	ChainID         string `json:"chain_id"`
	ProtocolID      string `json:"protocol_id"`
	Network         string `json:"network"`
	Asset           string `json:"asset"`
	Recipient       string `json:"recipient"`
	AmountBaseUnits int64  `json:"amount_base_units"`
}

// Quote is an immutable, expiring offer for one listing, quantity, destination, and
// shipping method.
type Quote struct {
	ID                  string  `json:"id"`
	SessionID           string  `json:"session_id"`
	StoreID             string  `json:"store_id"`
	ListingID           string  `json:"listing_id"`
	OrderID             string  `json:"order_id"`
	Quantity            int     `json:"quantity"`
	StoreCurrency       string  `json:"store_currency"`
	SubtotalMinor       int64   `json:"subtotal_minor"`
	ShippingMinor       int64   `json:"shipping_minor"`
	TaxMinor            int64   `json:"tax_minor"`
	ShippingDescription string  `json:"shipping_description,omitempty"`
	ExchangeRate        string  `json:"exchange_rate,omitempty"`
	ExchangeRateSource  string  `json:"exchange_rate_source,omitempty"`
	TotalUSDMinor       int64   `json:"total_usd_minor"`
	NetworkClass        string  `json:"network_class"`
	CreatedAt           string  `json:"created_at"`
	ExpiresAt           string  `json:"expires_at"`
	Offers              []Offer `json:"offers"`
}

// Receipt is the durable proof that a confirmed payment belongs to an order.
type Receipt struct {
	ID                string `json:"id"`
	OrderID           string `json:"order_id"`
	Network           string `json:"network"`
	Asset             string `json:"asset"`
	Payer             string `json:"payer"`
	Transaction       string `json:"transaction"`
	Slot              uint64 `json:"slot"`
	Confirmation      string `json:"confirmation"`
	Protocol          string `json:"protocol"`
	Seller            string `json:"seller"`
	NetworkClass      string `json:"network_class"`
	ExchangeRate      string `json:"exchange_rate,omitempty"`
	AmountBaseUnits   int64  `json:"amount_base_units"`
	FulfillmentStatus string `json:"fulfillment_status"`
	CreatedAt         string `json:"created_at"`
}

// Order is one order as its buyer reads it. A confirmed payment shows paid and
// sync_pending until the merchant's platform acknowledges the order, and received
// only afterwards. It carries no delivery detail (001:472).
type Order struct {
	ID                string   `json:"id"`
	QuoteID           string   `json:"quote_id"`
	StoreID           string   `json:"store_id"`
	PaymentStatus     string   `json:"payment_status"`
	FulfillmentStatus string   `json:"fulfillment_status"`
	RefundStatus      string   `json:"refund_status"`
	TotalUSDMinor     int64    `json:"total_usd_minor"`
	NetworkClass      string   `json:"network_class"`
	Test              bool     `json:"test"`
	Receipt           *Receipt `json:"receipt,omitempty"`
	SourceOrderRef    string   `json:"source_order_ref,omitempty"`
	StatusNote        string   `json:"status_note,omitempty"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
}

// OrderPage is one page of a buyer's orders.
type OrderPage struct {
	Orders []Order `json:"orders"`
	Next   string  `json:"next,omitempty"`
}

// Image is one merchant image, loaded directly from the platform's own domain.
type Image struct {
	URL string `json:"url"`
	Alt string `json:"alt,omitempty"`
}

// Product is the public view of one published listing.
type Product struct {
	ID              string            `json:"id"`
	StoreID         string            `json:"store_id"`
	StoreName       string            `json:"store_name"`
	StoreDomain     string            `json:"store_domain"`
	Title           string            `json:"title"`
	Description     string            `json:"description"`
	Category        string            `json:"category,omitempty"`
	Attributes      map[string]string `json:"attributes,omitempty"`
	Images          []Image           `json:"images"`
	ImageLimitation string            `json:"image_limitation,omitempty"`
	Price           string            `json:"price"`
	Currency        string            `json:"currency"`
	PriceUSDEst     string            `json:"price_usd_estimate,omitempty"`
	Available       bool              `json:"available"`
	SourceURL       string            `json:"source_url"`
	PurchaseMethods []string          `json:"purchase_methods"`
}

// ProductPage is one page of published products.
type ProductPage struct {
	Products []Product `json:"products"`
	Next     string    `json:"next,omitempty"`
}

// Store is one entry of the public merchant directory.
type Store struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Domain          string   `json:"domain"`
	Destinations    []string `json:"destinations"`
	Categories      []string `json:"categories"`
	PurchaseMethods []string `json:"purchase_methods"`
	Listings        int      `json:"published_listings"`
}

// StorePage is one page of the merchant directory.
type StorePage struct {
	Stores []Store `json:"stores"`
	Next   string  `json:"next,omitempty"`
}
