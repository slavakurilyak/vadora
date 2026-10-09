package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/vadora/vadora/api/types/domain"
	"github.com/vadora/vadora/api/types/errs"
	"github.com/vadora/vadora/api/types/rest"
	"github.com/vadora/vadora/sdk"
)

// maxQuantity bounds the quantity that the CLI asks for. The server applies its own
// bound and every other rule, whatever this client does (012:92).
const maxQuantity = 100

// requestQuote takes a buyer from a listing to a quote: it opens a session, reads
// the shipping options of the destination, picks the method, and asks for the quote.
// Nothing is charged and nothing is written to the merchant's store.
func requestQuote(env *Env, api *client.Client, listingID string, quantity int, shipping string, delivery domain.DeliveryDetails) (rest.Product, rest.Quote, error) {
	if quantity < 1 {
		return rest.Product{}, rest.Quote{}, errs.New(errs.InvalidRequest, "the quantity must be between 1 and 100")
	}
	if quantity > maxQuantity {
		return rest.Product{}, rest.Quote{}, errs.New(errs.InvalidRequest, "the quantity must be between 1 and 100")
	}
	product, err := api.GetProduct(callContext(), listingID)
	if err != nil {
		return rest.Product{}, rest.Quote{}, err
	}
	session, err := api.CreateCheckoutSession(callContext(), listingID)
	if err != nil {
		return rest.Product{}, rest.Quote{}, err
	}
	options, err := api.ShippingOptions(callContext(), session.ID, rest.ShippingOptionsRequest{ListingID: listingID, Quantity: quantity, Delivery: delivery})
	if err != nil {
		return rest.Product{}, rest.Quote{}, err
	}
	handle, err := chooseShipping(options, shipping)
	if err != nil {
		return rest.Product{}, rest.Quote{}, err
	}
	quote, err := api.CreateQuote(callContext(), client.NewKey(), rest.QuoteRequest{SessionID: session.ID, ListingID: listingID,
		Quantity: quantity, Delivery: delivery, ShippingMethod: handle})
	return product, quote, err
}

// chooseShipping picks the method that the buyer named, or the only one.
func chooseShipping(options rest.ShippingOptions, wanted string) (string, error) {
	if len(options.Options) == 0 {
		return "", errs.New(errs.ShippingUnsupported, "the store offers no shipping method to that destination")
	}
	var listing []string
	for _, option := range options.Options {
		if wanted != "" {
			if option.Handle == wanted {
				return option.Handle, nil
			}
		}
		listing = append(listing, fmt.Sprintf("%s (%s, %s %s total)", option.Handle, option.Title,
			minorString(option.TotalMinor), option.Currency))
	}
	if wanted == "" {
		if len(options.Options) == 1 {
			return options.Options[0].Handle, nil
		}
	}
	return "", errs.New(errs.InvalidRequest, "choose one shipping method with --shipping: "+strings.Join(listing, "; "))
}

// minorString renders minor units with two decimals, which every launch currency but
// the zero decimal ones uses. The server prices every amount, and this is only a label.
func minorString(minor int64) string { return fmt.Sprintf("%d.%02d", minor/100, minor%100) }

// describeQuote renders a quote for the buyer to read before any purchase: the
// merchant, the product, the quantity, the total, the chain, the recipient, and the
// expiry.
func describeQuote(product rest.Product, quote rest.Quote) string {
	var human strings.Builder
	fmt.Fprintf(&human, "Quote %s\n", quote.ID)
	fmt.Fprintf(&human, "  Merchant    %s (%s)\n", product.StoreName, product.StoreDomain)
	fmt.Fprintf(&human, "  Product     %s\n", product.Title)
	fmt.Fprintf(&human, "  Quantity    %d\n", quote.Quantity)
	fmt.Fprintf(&human, "  Subtotal    %s %s\n", minorString(quote.SubtotalMinor), quote.StoreCurrency)
	fmt.Fprintf(&human, "  Shipping    %s %s\n", minorString(quote.ShippingMinor), quote.StoreCurrency)
	fmt.Fprintf(&human, "  Tax         %s %s\n", minorString(quote.TaxMinor), quote.StoreCurrency)
	fmt.Fprintf(&human, "  Total       %s\n", usd(quote.TotalUSDMinor))
	for _, offer := range quote.Offers {
		fmt.Fprintf(&human, "  Pay         %s on %s\n", usdc(offer.AmountBaseUnits), offer.Network)
		fmt.Fprintf(&human, "  Recipient   %s\n", offer.Recipient)
	}
	fmt.Fprintf(&human, "  Expires     %s\n", quote.ExpiresAt)
	if quote.NetworkClass == "testnet" {
		human.WriteString("  TEST        this quote settles on a test network: no real funds move and it is never revenue\n")
	}
	return human.String()
}

// runQuote obtains a quote and shows it before any purchase.
func runQuote(env *Env, args []string) error {
	set := flagSet("quote", env)
	quantity := set.Int("quantity", 1, "how many to order")
	shipping := set.String("shipping", "", "the shipping method handle, when the store offers more than one")
	delivery := addDeliveryFlags(set)
	positional, err := parse(set, args)
	if err != nil {
		return err
	}
	listingID, err := oneArgument(positional, "listing ID")
	if err != nil {
		return err
	}
	details, err := delivery.delivery()
	if err != nil {
		return err
	}
	if err := requireCredential(env); err != nil {
		return err
	}
	api, err := newClient(env)
	if err != nil {
		return err
	}
	product, quote, err := requestQuote(env, api, listingID, *quantity, *shipping, details)
	if err != nil {
		return err
	}
	text := describeQuote(product, quote) + "Next: vadora buy " + quote.ID + "\n"
	return render(env, text, quote)
}

// newPaymentID returns a payment identifier for the x402 payment identifier
// extension: 16 to 128 characters of letters, digits, hyphens, and underscores.
func newPaymentID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		panic("cli: a payment identifier could not be generated: " + err.Error())
	}
	return "pay_" + hex.EncodeToString(buffer)
}

// purchase pays one quote. It checks the spending policy before it asks the server
// for the challenge, checks that the challenge is the quote's own offer, asks the
// operator to confirm unless the run is unattended, signs with the buyer's own
// signing interface, writes the signed payment and the idempotency key to disk, and
// only then sends the payment. A retry sends the same key and the same payload.
func purchase(env *Env, api *client.Client, signer Signer, policy SpendingPolicy, product rest.Product, quote rest.Quote, unattended bool) (rest.Order, error) {
	if len(quote.Offers) != 1 {
		return rest.Order{}, errs.New(errs.PaymentMethodUnsupported, "the quote offers no single payment method")
	}
	offer := quote.Offers[0]
	if err := policy.Check(PolicyCheck{TotalUSDCents: quote.TotalUSDMinor, Network: offer.Network, Asset: offer.Asset, StoreID: quote.StoreID}); err != nil {
		return rest.Order{}, err
	}
	record, found, err := loadRecord(env, quote.ID)
	if err != nil {
		return rest.Order{}, err
	}
	if !found {
		record = purchaseRecord{QuoteID: quote.ID, Key: client.NewKey(), PaymentID: newPaymentID(), OrderID: quote.OrderID}
		challenged, err := api.Purchase(callContext(), quote.ID, client.NewKey(), "")
		if err != nil {
			return rest.Order{}, err
		}
		if challenged.Order != nil {
			return *challenged.Order, nil
		}
		if err := matchOffer(challenged.Challenge, quote.ID, offer.Network, offer.Asset, offer.Recipient, offer.AmountBaseUnits); err != nil {
			return rest.Order{}, err
		}
		if !unattended {
			if err := confirm(env, product, quote); err != nil {
				return rest.Order{}, err
			}
		}
		header, err := createPayment(callContext(), signer, env.getenv(settingRPC), challenged.Challenge, record.PaymentID)
		if err != nil {
			return rest.Order{}, err
		}
		env.secrets = append(env.secrets, header)
		record.Signature = header
		if err := saveRecord(env, record); err != nil {
			return rest.Order{}, err
		}
	} else {
		env.secrets = append(env.secrets, record.Signature)
		line(env, "Resending the signed payment of %s with its own idempotency key: nothing new is authorized.", quote.ID)
	}
	result, err := api.Purchase(callContext(), quote.ID, record.Key, record.Signature)
	if err != nil {
		return rest.Order{}, err
	}
	if result.Order == nil {
		return rest.Order{}, errs.New(errs.StoreUnavailable, "the server answered a payment with a challenge")
	}
	return *result.Order, nil
}

// confirm asks the operator to confirm an interactive purchase, showing the exact
// amount, the recipient, and the network before anything is signed.
func confirm(env *Env, product rest.Product, quote rest.Quote) error {
	offer := quote.Offers[0]
	fmt.Fprintf(env.Out, "Pay %s (%s) to %s for %q from %s on %s.\n", usdc(offer.AmountBaseUnits), usd(quote.TotalUSDMinor),
		offer.Recipient, product.Title, product.StoreName, offer.Network)
	answer, err := prompt(env, "Type yes to sign this payment with your own wallet")
	if err != nil {
		return err
	}
	if answer != "yes" {
		return errs.New(errs.InvalidRequest, "the purchase was not confirmed: nothing was signed")
	}
	return nil
}

// waitSettings returns how often to read an order and for how long.
func waitSettings(env *Env) (time.Duration, time.Duration) {
	interval, limit := env.PollInterval, env.WaitFor
	if interval <= 0 {
		interval = 2 * time.Second
	}
	if limit <= 0 {
		limit = 90 * time.Second
	}
	return interval, limit
}

// finished reports whether an order will not change by itself any more.
func finished(order rest.Order) bool {
	switch order.PaymentStatus {
	case "rejected", "expired":
		return true
	case "paid":
		switch order.FulfillmentStatus {
		case "received", "fulfilled", "cancelled", "needs_attention":
			return true
		}
	}
	return false
}

// waitForOrder reads an order until the merchant's platform acknowledged it or
// the time ran out, and returns the last order that it read.
func waitForOrder(env *Env, api *client.Client, order rest.Order) rest.Order {
	interval, limit := waitSettings(env)
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if finished(order) {
			break
		}
		time.Sleep(interval)
		latest, err := api.GetOrder(callContext(), order.ID)
		if err != nil {
			return order
		}
		order = latest
	}
	return order
}

// explorer links a confirmed transfer on the public Solana explorer.
func explorer(order rest.Order) string {
	if order.Receipt == nil {
		return ""
	}
	if order.Receipt.Transaction == "" {
		return ""
	}
	link := "https://explorer.solana.com/tx/" + order.Receipt.Transaction
	if order.NetworkClass == "testnet" {
		link += "?cluster=devnet"
	}
	return link
}

// describeOrder renders an order as the buyer reads it. A confirmed payment is
// paid, and the order is received only after the merchant's platform acknowledged it.
func describeOrder(order rest.Order) string {
	var human strings.Builder
	fmt.Fprintf(&human, "Order %s\n", order.ID)
	fmt.Fprintf(&human, "  Payment      %s\n", order.PaymentStatus)
	fmt.Fprintf(&human, "  Fulfillment  %s\n", order.FulfillmentStatus)
	fmt.Fprintf(&human, "  Refund       %s\n", order.RefundStatus)
	fmt.Fprintf(&human, "  Total        %s\n", usd(order.TotalUSDMinor))
	if order.Test {
		human.WriteString("  TEST         a test network order: no real funds moved and it is never revenue\n")
	}
	if order.Receipt != nil {
		fmt.Fprintf(&human, "  Receipt      %s (%s at %s)\n", order.Receipt.Transaction, order.Receipt.Confirmation, order.Receipt.Network)
		fmt.Fprintf(&human, "  Paid         %s from %s\n", usdc(order.Receipt.AmountBaseUnits), order.Receipt.Payer)
		if link := explorer(order); link != "" {
			fmt.Fprintf(&human, "  Explorer     %s\n", link)
		}
	}
	if order.SourceOrderRef != "" {
		fmt.Fprintf(&human, "  Source order %s\n", order.SourceOrderRef)
	}
	if order.StatusNote != "" {
		fmt.Fprintf(&human, "  Note         %s\n", order.StatusNote)
	}
	return human.String()
}

// runBuy pays a quote as a buyer agent. It needs the buyer credential, a configured
// signing interface, and an owner authorized spending policy, and it stops before
// it signs when the quote is outside the policy, so the server then receives no
// purchase request. An interactive run asks the operator to confirm, and an
// unattended run needs the policy and nothing else (001:543).
func runBuy(env *Env, args []string) error {
	set := flagSet("buy", env)
	unattended := set.Bool("unattended", false, "run without operator confirmation")
	wait := set.Bool("wait", true, "wait until the merchant's platform acknowledged the order")
	positional, err := parse(set, args)
	if err != nil {
		return err
	}
	quoteID, err := oneArgument(positional, "quote ID")
	if err != nil {
		return err
	}
	policy, err := loadPolicy(env)
	if err != nil {
		return err
	}
	signer, err := loadSigner(env)
	if err != nil {
		return err
	}
	if err := requireCredential(env); err != nil {
		return err
	}
	api, err := newClient(env)
	if err != nil {
		return err
	}
	quote, err := api.GetQuote(callContext(), quoteID)
	if err != nil {
		return err
	}
	product, err := api.GetProduct(callContext(), quote.ListingID)
	if err != nil {
		return err
	}
	order, err := purchase(env, api, signer, policy, product, quote, *unattended)
	if err != nil {
		return err
	}
	if *wait {
		order = waitForOrder(env, api, order)
	}
	return render(env, describeOrder(order), order)
}

// stage is one step of a test purchase.
type stage struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// testReport walks a test purchase through its stages and names the one that failed.
type testReport struct {
	ListingID   string  `json:"listing_id"`
	Test        bool    `json:"test"`
	Stages      []stage `json:"stages"`
	FailedStage string  `json:"failed_stage,omitempty"`
	Detail      string  `json:"detail,omitempty"`
}

// testStages are the stages of a test purchase, in order (001:553).
var testStages = []string{"quote", "payment", "receipt", "source order"}

func newReport(listingID string) *testReport {
	report := &testReport{ListingID: listingID, Test: true}
	for _, name := range testStages {
		report.Stages = append(report.Stages, stage{Name: name, Status: "not reached"})
	}
	return report
}

func (r *testReport) pass(name, detail string) {
	for i := range r.Stages {
		if r.Stages[i].Name == name {
			r.Stages[i].Status, r.Stages[i].Detail = "ok", detail
		}
	}
}

func (r *testReport) fail(name string, err error) error {
	reason := err.Error()
	if catalog, ok := errs.As(err); ok {
		reason = string(catalog.Code) + ": " + catalog.Reason
	}
	for i := range r.Stages {
		if r.Stages[i].Name == name {
			r.Stages[i].Status, r.Stages[i].Detail = "failed", reason
		}
	}
	r.FailedStage, r.Detail = name, reason
	return err
}

func (r *testReport) human() string {
	var text strings.Builder
	fmt.Fprintf(&text, "TEST purchase of %s on a labeled development store. It is never revenue.\n", r.ListingID)
	for _, s := range r.Stages {
		fmt.Fprintf(&text, "  %-13s %-11s %s\n", s.Name, s.Status, s.Detail)
	}
	if r.FailedStage != "" {
		fmt.Fprintf(&text, "Failed at the %s stage: %s\n", r.FailedStage, r.Detail)
	} else {
		text.WriteString("Every stage completed.\n")
	}
	return text.String()
}

// runTestPurchase walks a test purchase of a labeled development store through
// quote, protocol payment, receipt, and source order synchronization, with the
// buyer credential and the spending policy, and names the exact stage that failed.
// It never pays on a live network.
func runTestPurchase(env *Env, args []string) error {
	set := flagSet("test purchase", env)
	quantity := set.Int("quantity", 1, "how many to order")
	shipping := set.String("shipping", "", "the shipping method handle, when the store offers more than one")
	delivery := addDeliveryFlags(set)
	positional, err := parse(set, args)
	if err != nil {
		return err
	}
	listingID, err := oneArgument(positional, "listing ID")
	if err != nil {
		return err
	}
	details, err := delivery.delivery()
	if err != nil {
		return err
	}
	policy, err := loadPolicy(env)
	if err != nil {
		return err
	}
	signer, err := loadSigner(env)
	if err != nil {
		return err
	}
	if err := requireCredential(env); err != nil {
		return err
	}
	api, err := newClient(env)
	if err != nil {
		return err
	}
	report := newReport(listingID)
	failure := walkTestPurchase(env, api, signer, policy, report, listingID, *quantity, *shipping, details)
	if renderErr := render(env, report.human(), report); renderErr != nil {
		return renderErr
	}
	return failure
}

// walkTestPurchase runs the four stages and records each in the report.
func walkTestPurchase(env *Env, api *client.Client, signer Signer, policy SpendingPolicy, report *testReport, listingID string, quantity int, shipping string, details domain.DeliveryDetails) error {
	product, quote, err := requestQuote(env, api, listingID, quantity, shipping, details)
	if err != nil {
		return report.fail("quote", err)
	}
	if quote.NetworkClass != "testnet" {
		return report.fail("quote", errs.New(errs.InvalidRequest, "a test purchase never pays on a live network: this quote is "+quote.NetworkClass))
	}
	report.pass("quote", quote.ID)
	order, err := purchase(env, api, signer, policy, product, quote, true)
	if err != nil {
		return report.fail("payment", err)
	}
	if order.PaymentStatus != "paid" {
		return report.fail("payment", errs.New(errs.PaymentPending, "the payment is "+order.PaymentStatus+" and has no receipt yet"))
	}
	if order.Receipt == nil {
		return report.fail("payment", errs.New(errs.PaymentPending, "the payment is "+order.PaymentStatus+" and has no receipt yet"))
	}
	report.pass("payment", order.Receipt.Transaction)
	report.pass("receipt", fmt.Sprintf("%s at %s, %s", order.Receipt.ID, order.Receipt.Confirmation, explorer(order)))
	order = waitForOrder(env, api, order)
	if order.FulfillmentStatus != "received" {
		if order.FulfillmentStatus != "fulfilled" {
			return report.fail("source order", errs.New(errs.OrderSyncPending, "the merchant's platform has not acknowledged the order: it is "+order.FulfillmentStatus))
		}
	}
	report.pass("source order", order.SourceOrderRef+" acknowledged by the merchant's platform")
	return nil
}

// runOrdersGet reads one order.
func runOrdersGet(env *Env, args []string) error {
	set := flagSet("orders get", env)
	positional, err := parse(set, args)
	if err != nil {
		return err
	}
	id, err := oneArgument(positional, "order ID")
	if err != nil {
		return err
	}
	if err := requireCredential(env); err != nil {
		return err
	}
	api, err := newClient(env)
	if err != nil {
		return err
	}
	order, err := api.GetOrder(callContext(), id)
	if err != nil {
		return err
	}
	return render(env, describeOrder(order), order)
}

// runOrdersList lists this buyer's orders.
func runOrdersList(env *Env, args []string) error {
	set := flagSet("orders list", env)
	status := set.String("status", "", "only this payment status")
	limit := set.Int("limit", 0, "how many orders to show, up to 100")
	cursor := set.String("cursor", "", "the page cursor of an earlier listing")
	if _, err := parse(set, args); err != nil {
		return err
	}
	if err := requireCredential(env); err != nil {
		return err
	}
	api, err := newClient(env)
	if err != nil {
		return err
	}
	orders, err := api.ListOrders(callContext(), *status, *cursor, *limit)
	if err != nil {
		return err
	}
	var human strings.Builder
	for _, order := range orders.Orders {
		fmt.Fprintf(&human, "%s  payment %s  fulfillment %s  %s\n", order.ID, order.PaymentStatus, order.FulfillmentStatus, usd(order.TotalUSDMinor))
	}
	if len(orders.Orders) == 0 {
		human.WriteString("No order matches.\n")
	}
	if orders.Next != "" {
		fmt.Fprintf(&human, "More orders: add --cursor %s\n", orders.Next)
	}
	return render(env, human.String(), orders)
}

// runPaymentsReconcile reads the outcome of an existing payment without authorizing
// a new one. When this machine holds the signed payment of the order, it sends that
// same payment again with its own key, which asks the server for the existing
// outcome and moves no more funds (001:543).
func runPaymentsReconcile(env *Env, args []string) error {
	set := flagSet("payments reconcile", env)
	positional, err := parse(set, args)
	if err != nil {
		return err
	}
	orderID, err := oneArgument(positional, "order ID")
	if err != nil {
		return err
	}
	if err := requireCredential(env); err != nil {
		return err
	}
	api, err := newClient(env)
	if err != nil {
		return err
	}
	order, err := api.GetOrder(callContext(), orderID)
	if err != nil {
		return err
	}
	if order.PaymentStatus != "paid" {
		if record, found := findRecordByOrder(env, orderID); found {
			env.secrets = append(env.secrets, record.Signature)
			if result, err := api.Purchase(callContext(), record.QuoteID, record.Key, record.Signature); err == nil {
				if result.Order != nil {
					order = *result.Order
				}
			}
		}
	}
	return render(env, describeOrder(order), order)
}
