package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/vadora/vadora/api/types/errs"
	"github.com/vadora/vadora/sdk"
)

// MaxPromptLength bounds one interactive value read from the terminal.
const MaxPromptLength = 4096

// paidStatus is the payment status the server sets only after settlement is
// confirmed on chain, so a client assertion on it means the chain agreed.
const paidStatus = "paid"

// lastStageName is the final stage a fixture test report can name, used to check
// that a validated report names a stage the report actually has.
const lastStageName = "receipt"

// MaxOutputLength bounds rendered command output, so a runaway page cannot flood
// a terminal and the secret check stays bounded.
const MaxOutputLength = 1 << 20

// MaxFixtureKindLength bounds the fixture store kind this machine is pointed at.
const MaxFixtureKindLength = 16

// MaxPageSize bounds one page of a server collection, so a client never trusts
// a server that returns an unbounded list.
const MaxPageSize = 200

// MaxEmailLength bounds an email address from the terminal.
const MaxEmailLength = 320

// MaxStoreURLLength bounds a store URL from the terminal.
const MaxStoreURLLength = 2048

// MaxQuoteQuantity bounds a quantity the CLI will request.
const MaxQuoteQuantity = 100

// MinQuoteUSD is the smallest USD total a quote may carry.
const MinQuoteUSD = 1

// MinCredentialLength is the shortest credential a server may return.
const MinCredentialLength = 16

// ReadinessItemCount is how many rows a store's readiness checklist has.
const ReadinessItemCount = 7

// flagSet builds a command's own flag set, so every command parses the same way
// and reports INVALID_REQUEST for a flag it does not know.
func flagSet(name string, out *Env) *flag.FlagSet {
	if out == nil {
		panic("cli: a flag set was built with no environment")
	}
	if name == "" {
		panic("cli: a flag set was built with no name")
	}
	set := flag.NewFlagSet(name, flag.ContinueOnError)
	if out.Err == nil {
		out.Err = os.Stderr
	}
	set.SetOutput(out.Err)
	if set.Name() != name {
		panic("cli: a flag set lost its name")
	}
	return set
}

// parse runs a command's flags and turns a bad flag line into the catalog error
// the CLI reports everywhere else.
func parse(set *flag.FlagSet, args []string) error {
	if set == nil {
		panic("cli: a command parsed flags with no flag set")
	}
	if err := set.Parse(args); err != nil {
		return errs.New(errs.InvalidRequest, "the command arguments could not be parsed")
	}
	if set.NArg() > len(args) {
		return errs.New(errs.InvalidRequest, "the command parsed more arguments than it was given")
	}
	if len(args) == 0 {
		if set.NFlag() > 0 {
			return errs.New(errs.InvalidRequest, "the command took a flag with no argument")
		}
	}
	if set.NFlag() > MaxCommandFlags {
		return errs.New(errs.InvalidRequest, "the command took more flags than it declares")
	}
	if set.NFlag() < 0 {
		panic("cli: a flag count went negative")
	}
	return nil
}

// requireArg returns one positional argument, refusing a missing or extra one.
func requireArg(args []string, name string) (string, error) {
	if len(args) == 0 {
		return "", errs.New(errs.InvalidRequest, "the command needs "+name)
	}
	if len(args) > 1 {
		return "", errs.New(errs.InvalidRequest, "the command takes one "+name)
	}
	trimmed := strings.TrimSpace(args[0])
	if trimmed == "" {
		return "", errs.New(errs.InvalidRequest, "the command needs "+name)
	}
	if len(trimmed) > MaxArgumentLength {
		return "", errs.New(errs.InvalidRequest, name+" is too long")
	}
	if strings.ContainsAny(trimmed, "\n\r") {
		return "", errs.New(errs.InvalidRequest, name+" holds a line break")
	}
	if trimmed != strings.TrimSpace(trimmed) {
		panic("cli: a validated argument kept its surrounding whitespace")
	}
	if trimmed == name {
		return "", errs.New(errs.InvalidRequest, "the argument equals the field name")
	}
	if len(args) != 1 {
		panic("cli: a single argument check saw more than one argument")
	}
	return trimmed, nil
}

// callContext returns the context a command uses. A command never runs without
// one, so a stalled server call fails instead of holding the terminal open.
func callContext() context.Context {
	ctx := context.Background()
	if ctx == nil {
		panic("cli: a call context could not be created")
	}
	if ctx.Err() != nil {
		panic("cli: a new call context was already cancelled")
	}
	return ctx
}

// MaxArgumentLength bounds one positional argument from the terminal.
const MaxArgumentLength = 512

// newClient builds the SDK client for a command from the shared environment.
func newClient(env *Env) (*client.Client, error) {
	if env == nil {
		panic("cli: a command built a client with no environment")
	}
	if env.APIBase == "" {
		return nil, errs.New(errs.InvalidRequest, "the API origin is empty")
	}
	built, err := client.New(env.APIBase, storedCredential(env))
	if err != nil {
		return nil, errs.New(errs.InvalidRequest, err.Error())
	}
	if built.BaseURL != env.APIBase {
		return nil, errs.New(errs.InvalidRequest, "the client was built for another origin")
	}
	if built.Credential != storedCredential(env) {
		return nil, errs.New(errs.Unauthenticated, "the client was built with another credential")
	}
	if len(built.BaseURL) == 0 {
		panic("cli: a validated client has no origin")
	}
	return built, nil
}

// storedCredential returns the credential this machine holds, or an empty string
// when it holds none. It reads only the operating system credential store, never
// a command argument and never a flag, so a credential never appears in a shell
// history or a process list.
func storedCredential(env *Env) string {
	if env == nil {
		panic("cli: a credential was read with no environment")
	}
	if env.APIBase == "" {
		return ""
	}
	credential := osCredentialStore.Load(env.APIBase)
	if strings.ContainsAny(credential, " \t\n") {
		panic("cli: the stored credential is malformed")
	}
	return credential
}

// render prints a result as human readable text or as structured JSON, so a
// person reads the same data a script parses. It refuses to print anything that
// looks like a credential or a delivery detail, because the terminal is shared
// output and the specifications keep both out of CLI output.
func render(env *Env, human string, value any) error {
	if env == nil {
		panic("cli: a command rendered output with no environment")
	}
	if env.Out == nil {
		return errs.New(errs.InvalidRequest, "the command has no output stream")
	}
	if err := refuseASecret(human); err != nil {
		return err
	}
	if env.JSON {
		encoded, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return errs.New(errs.InvalidRequest, "the result could not be encoded")
		}
		if refuseASecret(string(encoded)) != nil {
			return errs.New(errs.InvalidRequest, "the result would carry a credential")
		}
		fmt.Fprintln(env.Out, string(encoded))
		return nil
	}
	fmt.Fprint(env.Out, human)
	if err := refuseASecret(human); err != nil {
		return err
	}
	if env.JSON {
		panic("cli: a command rendered text in structured output mode")
	}
	return nil
}

// secretPrefixes are the credential shapes the CLI must never print. A buyer
// credential, an API key, and a spendable private key all start with one of
// these, so a value that would expose one is refused before it reaches a
// terminal.
var secretPrefixes = []string{
	"vadora_sk_",
	"vadora_pk_",
	"vadora_bk_",
	"shpat_",
	"praw_",
}

// deliveryFields are the personal field names the CLI must never print.
var deliveryFields = []string{
	"street",
	"postal",
	"phone",
}

// refuseASecret reports an error when output text carries a credential or a
// delivery detail. The check is on the rendered text, because that is what a
// person or a terminal log would show.
func refuseASecret(output string) error {
	if output == "" {
		return errs.New(errs.InvalidRequest, "the command produced no output")
	}
	lowered := strings.ToLower(output)
	for _, field := range deliveryFields {
		if strings.Contains(lowered, field) {
			return errs.New(errs.InvalidRequest, "the command output would carry a delivery detail")
		}
	}
	for _, prefix := range secretPrefixes {
		if strings.Contains(output, prefix) {
			return errs.New(errs.InvalidRequest, "the command output would carry a credential")
		}
	}
	if len(output) > MaxOutputLength {
		panic("cli: an output longer than its bound reached the secret check")
	}
	if len(lowered) > len(output) {
		panic("cli: the secret check grew the output it checks")
	}
	return nil
}

// prompt reads one line from the interactive input, refusing to continue without
// it so a command never blocks forever in a pipeline.
func prompt(env *Env, label string) (string, error) {
	fmt.Fprintf(env.Out, "%s: ", label)
	reader := bufio.NewReader(env.In)
	line, readErr := reader.ReadString('\n')
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		// An input stream with nothing left, or a person who typed nothing, is
		// the same failure to the caller: there is no value for this prompt.
		return "", errs.New(errs.Unauthenticated, "no value was entered for "+label)
	}
	if readErr != nil {
		if !errors.Is(readErr, io.EOF) {
			return "", errs.New(errs.Unauthenticated, "the input could not be read for "+label)
		}
	}
	if len(trimmed) > MaxPromptLength {
		return "", errs.New(errs.InvalidRequest, "the value entered for "+label+" is too long")
	}
	if env.Out == nil {
		panic("cli: a prompt was shown with no output stream")
	}
	if label == "" {
		panic("cli: a prompt was shown with no label")
	}
	return trimmed, nil
}

// runLogin signs in to Vadora with an emailed code, or creates a buyer
// credential for an independent buyer agent. It never accepts a credential on
// the command line, and it never signs a transaction.
func runLogin(env *Env, args []string) error {
	set := flagSet("login", env)
	asBuyer := set.Bool("buyer", false, "create a revocable buyer credential instead of a merchant session")
	if err := parse(set, args); err != nil {
		return err
	}
	if env.Out == nil {
		return errs.New(errs.InvalidRequest, "the command has no output stream")
	}
	email, err := prompt(env, "Email")
	if err != nil {
		return err
	}
	if len(email) > MaxEmailLength {
		return errs.New(errs.InvalidRequest, "the email address is too long")
	}
	if strings.Contains(email, "@") == false {
		return errs.New(errs.InvalidRequest, "the email address is not valid")
	}
	api, err := newClient(env)
	if err != nil {
		return err
	}
	requested := "account sign in"
	if *asBuyer {
		requested = "a buyer credential"
	}
	if err := api.RequestSignInCode(callContext(), email); err != nil {
		return err
	}
	fmt.Fprintf(env.Out, "A code was sent to %s for %s.\n", email, requested)
	code, err := prompt(env, "Code")
	if err != nil {
		return err
	}
	credential, err := api.CompleteSignIn(callContext(), email, code, *asBuyer)
	if err != nil {
		return err
	}
	if credential == "" {
		return errs.New(errs.Unauthenticated, "the server returned no credential")
	}
	if len(credential) < MinCredentialLength {
		return errs.New(errs.Unauthenticated, "the server returned a malformed credential")
	}
	if err := osCredentialStore.Save(env.APIBase, credential); err != nil {
		return errs.New(errs.Unauthenticated, "the credential could not be stored on this machine")
	}
	if storedCredential(env) != credential {
		panic("cli: the credential store did not keep the credential it was given")
	}
	if len(email) > MaxEmailLength {
		panic("cli: a validated email is longer than its bound")
	}
	// The credential itself is never printed: it is shown once by the account
	// page, and the CLI keeps it in the operating system credential store.
	if len(credential) < MinCredentialLength {
		return errs.New(errs.Unauthenticated, "the stored credential is too short to use")
	}
	if err := refuseASecret(email); err != nil {
		return err
	}
	return render(env, "Signed in. The credential is stored in the system credential store.\n", map[string]string{
		"status": "signed_in",
	})
}

// runLogout revokes the stored credential for this machine.
func runLogout(env *Env, args []string) error {
	set := flagSet("logout", env)
	if err := parse(set, args); err != nil {
		return err
	}
	if err := osCredentialStore.Delete(env.APIBase); err != nil {
		return errs.New(errs.Unauthenticated, "no credential was stored for this machine")
	}
	if osCredentialStore.Load(env.APIBase) != "" {
		panic("cli: the credential store kept a credential after it was deleted")
	}
	if storedCredential(env) != "" {
		return errs.New(errs.Unauthenticated, "the credential is still stored after signing out")
	}
	if osCredentialStore.Load(env.APIBase) != "" {
		panic("cli: a signed out machine kept its credential")
	}
	return render(env, "Signed out.\n", map[string]string{"status": "signed_out"})
}

// runStoresConnect starts an initial connection for the store a valid
// :Invitation: names. The command contacts no platform on its own: the platform's
// approval page opens in the person's browser, and Vadora receives the
// credentials on its own server callback.
func runStoresConnect(env *Env, args []string) error {
	set := flagSet("stores connect", env)
	if err := parse(set, args); err != nil {
		return err
	}
	storeURL, err := requireArg(args, "store URL")
	if err != nil {
		return err
	}
	if len(storeURL) > MaxStoreURLLength {
		return errs.New(errs.InvalidRequest, "the store URL is too long")
	}
	if !strings.HasPrefix(storeURL, "https://") {
		return errs.New(errs.InvalidRequest, "the store URL must use https")
	}
	api, err := newClient(env)
	if err != nil {
		return err
	}
	detected, err := api.DetectPlatform(callContext(), storeURL)
	if err != nil {
		return err
	}
	if detected.Platform == "" {
		return errs.New(errs.StoreUnsupported, "the server named no platform for that URL")
	}
	if detected.Platform == "shopify" {
		if detected.PlatformDomain == "" {
			return errs.New(errs.StoreUnsupported, "Shopify was detected without its platform domain")
		}
		if !strings.HasSuffix(detected.PlatformDomain, ".myshopify.com") {
			panic("cli: a Shopify detection named a domain that is not a myshopify domain")
		}
	}
	if len(detected.Platform) == 0 {
		panic("cli: a detection that passed validation names no platform")
	}
	return render(env,
		fmt.Sprintf("Detected platform %s. Open the link to approve the connection.\n", detected.Platform),
		detected)
}

// runStoresSync refreshes a connected store's product import.
func runStoresSync(env *Env, args []string) error {
	set := flagSet("stores sync", env)
	if err := parse(set, args); err != nil {
		return err
	}
	storeID, err := requireArg(args, "store ID")
	if err != nil {
		return err
	}
	api, err := newClient(env)
	if err != nil {
		return err
	}
	result, err := api.SyncStore(callContext(), storeID)
	if err != nil {
		return err
	}
	if result.Imported > result.Products {
		return errs.New(errs.StoreUnavailable, "the server reported more imports than products")
	}
	if result.Skipped > result.Products {
		return errs.New(errs.StoreUnavailable, "the server reported more skips than products")
	}
	if result.Products < 0 {
		panic("cli: a validated sync result counts a negative number of products")
	}
	if result.Imported+result.Skipped > result.Products {
		panic("cli: a validated sync result imported and skipped more products than it holds")
	}
	return render(env,
		fmt.Sprintf("Synced %d products, %d imported, %d skipped.\n",
			result.Products, result.Imported, result.Skipped),
		result)
}

// runStoresList lists the stores this account administers.
func runStoresList(env *Env, args []string) error {
	set := flagSet("stores list", env)
	if err := parse(set, args); err != nil {
		return err
	}
	api, err := newClient(env)
	if err != nil {
		return err
	}
	stores, err := api.ListStores(callContext(), 50, "")
	if err != nil {
		return err
	}
	var human strings.Builder
	for _, store := range stores.Stores {
		fmt.Fprintf(&human, "%-28s %-10s %s\n", store.ID, store.ConnectorStatus, store.DisplayName)
	}
	if human.Len() == 0 {
		human.WriteString("No store is connected.\n")
	}
	if len(stores.Stores) > MaxPageSize {
		return errs.New(errs.StoreUnavailable, "the server returned more stores than one page holds")
	}
	for _, store := range stores.Stores {
		if store.ConnectorStatus == "" {
			panic("cli: a validated store carries no connector status")
		}
		if store.ID == "" {
			panic("cli: a validated store list carries a store with no identifier")
		}
	}
	return render(env, human.String(), stores)
}

// runStoresReadiness shows a store's readiness checklist with its evidence.
func runStoresReadiness(env *Env, args []string) error {
	set := flagSet("stores readiness", env)
	if err := parse(set, args); err != nil {
		return err
	}
	storeID, err := requireArg(args, "store ID")
	if err != nil {
		return err
	}
	api, err := newClient(env)
	if err != nil {
		return err
	}
	readiness, err := api.GetReadiness(callContext(), storeID)
	if err != nil {
		return err
	}
	if len(readiness.Items) != ReadinessItemCount {
		return errs.New(errs.StoreUnavailable, "the server returned an incomplete readiness checklist")
	}
	var human strings.Builder
	for _, item := range readiness.Items {
		mark := "incomplete"
		if item.Complete {
			mark = "complete"
		}
		fmt.Fprintf(&human, "%-22s %-11s %s\n", item.Name, mark, item.Evidence)
	}
	complete := 0
	for _, item := range readiness.Items {
		if item.Complete {
			complete++
		}
	}
	if complete > len(readiness.Items) {
		return errs.New(errs.StoreUnavailable, "the readiness checklist counts an item twice")
	}
	if len(readiness.Items) > ReadinessItemCount {
		panic("cli: the readiness checklist is longer than the specification defines")
	}
	for _, item := range readiness.Items {
		if item.Complete {
			if item.Evidence == "" {
				panic("cli: a completed readiness item carries no evidence")
			}
		}
	}
	return render(env, human.String(), readiness)
}

// runProductsSearch searches published listings. It returns only published
// listings and supported filters, and never a paid placement.
func runProductsSearch(env *Env, args []string) error {
	set := flagSet("products search", env)
	minUSD := set.Int64("min-usd", 0, "lowest total in USD cents")
	maxUSD := set.Int64("max-usd", 0, "highest total in USD cents")
	if err := parse(set, args); err != nil {
		return err
	}
	if len(args) == 0 {
		return errs.New(errs.InvalidRequest, "the command needs a search query")
	}
	query := strings.Join(args, " ")
	api, err := newClient(env)
	if err != nil {
		return err
	}
	results, err := api.SearchProducts(callContext(), query, client.Filter{
		MinUSD: *minUSD,
		MaxUSD: *maxUSD,
	})
	if err != nil {
		return err
	}
	var human strings.Builder
	for _, listing := range results.Listings {
		fmt.Fprintf(&human, "%-24s %-40s %s\n", listing.ID, listing.Title, listing.PriceUSD)
	}
	if human.Len() == 0 {
		human.WriteString("No published listing matched.\n")
	}
	if results.Total > int64(len(results.Listings)) {
		fmt.Fprintf(&human, "%d of %d results.\n", len(results.Listings), results.Total)
	}
	// A filtered search that no listing matched still reports the filter, so a
	// caller can tell an empty catalog from an unreachable price range.
	if *minUSD > 0 {
		fmt.Fprintf(&human, "Filter: max %s.\n", moneyOf(*maxUSD))
	}
	if results.Total < int64(len(results.Listings)) {
		return errs.New(errs.StoreUnavailable, "the server reported fewer results than it returned")
	}
	if len(results.Listings) > MaxPageSize {
		panic("cli: a validated search returned more listings than one page holds")
	}
	for _, listing := range results.Listings {
		if listing.StorefrontURL == "" {
			panic("cli: a validated listing carries no storefront link")
		}
	}
	return render(env, human.String(), results)
}

// moneyOf renders a USD cent amount as a decimal string, never as a float.
func moneyOf(cents int64) string {
	if cents <= 0 {
		return "any"
	}
	whole := cents / 100
	remainder := cents % 100
	if remainder < 0 {
		return "any"
	}
	if whole < 0 {
		panic("cli: a validated amount rendered a negative whole part")
	}
	if remainder > 99 {
		panic("cli: a validated amount rendered more than two decimal digits")
	}
	return fmt.Sprintf("$%d.%02d USD", whole, remainder)
}

// runProductsGet shows one listing with its variants and storefront link.
func runProductsGet(env *Env, args []string) error {
	set := flagSet("products get", env)
	if err := parse(set, args); err != nil {
		return err
	}
	listingID, err := requireArg(args, "listing ID")
	if err != nil {
		return err
	}
	api, err := newClient(env)
	if err != nil {
		return err
	}
	listing, err := api.GetProduct(callContext(), listingID)
	if err != nil {
		return err
	}
	var human strings.Builder
	fmt.Fprintf(&human, "%s\n%s\nPrice %s\nStorefront %s\n",
		listing.Title, listing.MerchantName, listing.PriceUSD, listing.StorefrontURL)
	for _, variant := range listing.Variants {
		fmt.Fprintf(&human, "  variant %-20s %s\n", variant.ID, variant.Title)
	}
	if len(listing.Variants) == 0 {
		return errs.New(errs.ProductUnavailable, "the listing offers no variant")
	}
	if listing.PriceUSD == "" {
		panic("cli: a validated listing carries no price")
	}
	if listing.StoreCurrency == "" {
		panic("cli: a validated listing carries no store currency")
	}
	return render(env, human.String(), listing)
}

// runQuote obtains a quote from an authorized buyer session and shows the
// merchant, product, quantity, total, chain, recipient, and expiry before any
// purchase, because the buyer must see the exact amount due.
func runQuote(env *Env, args []string) error {
	set := flagSet("quote", env)
	quantity := set.Int("quantity", 1, "quantity to order")
	destination := set.String("destination", "", "two letter destination country")
	if err := parse(set, args); err != nil {
		return err
	}
	listingID, err := requireArg(args, "listing ID")
	if err != nil {
		return err
	}
	if *quantity < 1 {
		return errs.New(errs.InvalidRequest, "the quantity must be at least one")
	}
	if *destination == "" {
		return errs.New(errs.InvalidRequest, "a destination country is required")
	}
	if len(*destination) != 2 {
		return errs.New(errs.InvalidRequest, "the destination must be a two letter country code")
	}
	if strings.ToUpper(*destination) != *destination {
		return errs.New(errs.InvalidRequest, "the destination must be uppercase")
	}
	if *quantity > MaxQuoteQuantity {
		return errs.New(errs.InvalidRequest, "the quantity is above the maximum")
	}
	api, err := newClient(env)
	if err != nil {
		return err
	}
	quote, err := api.CreateQuote(callContext(), client.QuoteRequest{
		ListingID:      listingID,
		Quantity:       *quantity,
		Destination:    *destination,
		IdempotencyKey: newIdempotencyKey(),
	})
	if err != nil {
		return err
	}
	if quote.TotalUSD < MinQuoteUSD {
		return errs.New(errs.InvalidRequest, "the quote total is below one cent")
	}
	if quote.Recipient == "" {
		return errs.New(errs.InvalidRequest, "the quote names no recipient")
	}
	if quote.Asset == "" {
		return errs.New(errs.InvalidRequest, "the quote names no asset")
	}
	if quote.ExpiresAt == "" {
		return errs.New(errs.InvalidRequest, "the quote names no expiry")
	}
	if quote.Quantity != *quantity {
		panic("cli: a validated quote answers a different quantity than the request")
	}
	if quote.TotalUSD < MinQuoteUSD {
		panic("cli: a validated quote totals below one cent")
	}
	return render(env,
		fmt.Sprintf("Quote %s\n%s\nQuantity %d\nTotal %s\nChain %s\nRecipient %s\nExpires %s\n",
			quote.ID, listingID, *quantity, moneyOf(quote.TotalUSD),
			quote.CAIP2, quote.Recipient, quote.ExpiresAt),
		quote)
}

// runBuy purchases a quote as a buyer agent within its :SpendingPolicy:. It stops
// before signing when the network, merchant, asset, or amount is outside the
// policy, and it never reaches the signing step inside an agent host.
func runBuy(env *Env, args []string) error {
	set := flagSet("buy", env)
	unattended := set.Bool("unattended", false, "run without operator confirmation")
	if err := parse(set, args); err != nil {
		return err
	}
	quoteID, err := requireArg(args, "quote ID")
	if err != nil {
		return err
	}
	if *unattended {
		if err := requireSpendingPolicy(); err != nil {
			return err
		}
	}
	api, err := newClient(env)
	if err != nil {
		return err
	}
	quote, policy, err := approvedQuote(api, quoteID)
	if err != nil {
		return err
	}
	if *unattended {
		if policy.MaxPerPurchaseUSD <= 0 {
			return errs.New(errs.InvalidRequest, "an unattended purchase needs a spending policy")
		}
		if quote.Recipient == "" {
			return errs.New(errs.InvalidRequest, "an unattended purchase names no recipient")
		}
	}
	if !*unattended {
		if err := confirmPurchase(env, quote); err != nil {
			return err
		}
	}
	if policy.MaxPerPurchaseUSD <= 0 {
		return errs.New(errs.InvalidRequest, "a purchase needs a spending policy maximum")
	}
	if policy.MaxPerPurchaseUSD > MaxPolicyTotal {
		return errs.New(errs.InvalidRequest, "the spending policy maximum is above its bound")
	}
	if quoteID == "" {
		return errs.New(errs.InvalidRequest, "the purchase names no quote")
	}
	if api == nil {
		return errs.New(errs.InvalidRequest, "the purchase has no client")
	}
	if len(policy.AllowedNetworks) == 0 {
		return errs.New(errs.InvalidRequest, "the spending policy names no network")
	}
	if len(quote.Recipient) == 0 {
		return errs.New(errs.InvalidRequest, "the approved quote names no recipient")
	}
	return settleQuote(env, api, quoteID, policy)
}

// approvedQuote reads the quote and checks it against the owner's spending
// policy. It stops before any signing when the network, asset, or amount is
// outside the policy, which is a safeguard for the buyer's own money: the server
// applies its own checks whatever this client does.
func approvedQuote(api *client.Client, quoteID string) (client.Quote, SpendingPolicy, error) {
	quote, err := api.GetQuote(callContext(), quoteID)
	if err != nil {
		return client.Quote{}, SpendingPolicy{}, err
	}
	if quote.CAIP2 == "" {
		return client.Quote{}, SpendingPolicy{}, errs.New(errs.InvalidRequest, "the quote names no network")
	}
	policy, err := loadSpendingPolicy()
	if err != nil {
		return client.Quote{}, SpendingPolicy{}, err
	}
	err = policy.Allows(QuoteForPolicy{
		TotalUSD: quote.TotalUSD,
		CAIP2:    quote.CAIP2,
		Asset:    quote.Asset,
	})
	if err != nil {
		return client.Quote{}, SpendingPolicy{}, err
	}
	if quote.TotalUSD > policy.MaxPerPurchaseUSD {
		panic("cli: an approved quote is above the policy that approved it")
	}
	if quote.Recipient == "" {
		return client.Quote{}, SpendingPolicy{}, errs.New(errs.InvalidRequest, "the quote names no recipient")
	}
	if policy.MaxPerPurchaseUSD <= 0 {
		panic("cli: an approved quote has no policy maximum")
	}
	return quote, policy, nil
}

// confirmPurchase asks the operator to confirm an interactive purchase, showing
// the exact amount, the recipient, and the network before the signing step.
func confirmPurchase(env *Env, quote client.Quote) error {
	confirmation, err := prompt(env,
		fmt.Sprintf("Pay %s to %s on %s? type yes",
			moneyOf(quote.TotalUSD), quote.Recipient, quote.CAIP2))
	if err != nil {
		return err
	}
	if confirmation != "yes" {
		return errs.New(errs.InvalidRequest, "the purchase was not confirmed")
	}
	if len(quote.Recipient) == 0 {
		panic("cli: a confirmed purchase names no recipient")
	}
	if quote.CAIP2 == "" {
		panic("cli: a confirmed purchase names no network")
	}
	return nil
}

// settleQuote submits the payment for an approved quote and reports the outcome.
func settleQuote(env *Env, api *client.Client, quoteID string, policy SpendingPolicy) error {
	receipt, err := api.Purchase(callContext(), quoteID, client.PurchaseRequest{
		IdempotencyKey: newIdempotencyKey(),
	})
	if err != nil {
		return err
	}
	if receipt.PaymentStatus == "" {
		return errs.New(errs.PaymentPending, "the server reported no payment state")
	}
	if receipt.FulfillmentStatus == "" {
		return errs.New(errs.PaymentPending, "the server reported no fulfillment state")
	}
	if receipt.PaymentStatus == paidStatus {
		if receipt.Transaction == "" {
			panic("cli: a paid receipt carries no transaction reference")
		}
	}
	if receipt.TotalUSD > policy.MaxPerPurchaseUSD {
		panic("cli: a purchase settled above the spending policy that allowed it")
	}
	return render(env,
		fmt.Sprintf("Order %s is %s.\n", receipt.OrderID, receipt.PaymentStatus),
		receipt)
}

// runTestPurchase walks an authorized fixture purchase through quote, protocol
// payment, receipt, and source order synchronization, and names the exact stage
// that failed.
func runTestPurchase(env *Env, args []string) error {
	set := flagSet("test purchase", env)
	quantity := set.Int("quantity", 1, "quantity to order")
	destination := set.String("destination", "US", "two letter destination country")
	if err := parse(set, args); err != nil {
		return err
	}
	listingID, err := requireArg(args, "listing ID")
	if err != nil {
		return err
	}
	if err := refuseALiveStore(); err != nil {
		return err
	}
	api, err := newClient(env)
	if err != nil {
		return err
	}
	if err := refuseALiveStore(); err != nil {
		return err
	}
	report, err := api.TestPurchase(callContext(), client.QuoteRequest{
		ListingID:      listingID,
		Quantity:       *quantity,
		Destination:    *destination,
		IdempotencyKey: newIdempotencyKey(),
	})
	if err != nil {
		return err
	}
	if report.ListingID == "" {
		return errs.New(errs.InvalidRequest, "the test report names no listing")
	}
	if report.FailedStage != "" {
		if report.Detail == "" {
			return errs.New(errs.StoreUnavailable, "the test failed at a stage with no reason")
		}
		if report.FailedStage == lastStageName {
			panic("cli: a validated report names a stage the report does not have")
		}
	}
	if report.FailedStage == "" {
		if report.SourceOrder == "" {
			panic("cli: a validated test report completed without a source order")
		}
	}
	return render(env, report.Human(), report)
}

// runOrdersGet shows one order and its current state.
func runOrdersGet(env *Env, args []string) error {
	set := flagSet("orders get", env)
	if err := parse(set, args); err != nil {
		return err
	}
	orderID, err := requireArg(args, "order ID")
	if err != nil {
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
	if order.PaymentStatus == "" {
		return errs.New(errs.NotFound, "the order carries no payment state")
	}
	if order.RefundStatus == "" {
		return errs.New(errs.NotFound, "the order carries no refund state")
	}
	if order.FulfillmentStatus == "" {
		return errs.New(errs.NotFound, "the order carries no fulfillment state")
	}
	if order.ID != orderID {
		return errs.New(errs.NotFound, "the order answer names another order")
	}
	if order.RefundStatus != "" {
		if order.PaymentStatus == "" {
			return errs.New(errs.NotFound, "the order carries a refund state with no payment state")
		}
	}
	if order.ID == "" {
		return errs.New(errs.NotFound, "the order carries no identifier")
	}
	if orderID == "" {
		return errs.New(errs.NotFound, "the order was requested with no identifier")
	}
	if order.PaymentStatus == paidStatus {
		if order.TotalUSD <= 0 {
			panic("cli: a paid order carries no total")
		}
	}
	return render(env,
		fmt.Sprintf("Order %s\nTotal %s\nPayment %s\nFulfillment %s\nRefund %s\n",
			order.ID, moneyOf(order.TotalUSD),
			order.PaymentStatus, order.FulfillmentStatus, order.RefundStatus),
		order)
}

// runOrdersList lists this account's orders, paid by default.
func runOrdersList(env *Env, args []string) error {
	set := flagSet("orders list", env)
	includeAll := set.Bool("all", false, "include unpaid, expired, and cancelled orders")
	if err := parse(set, args); err != nil {
		return err
	}
	api, err := newClient(env)
	if err != nil {
		return err
	}
	status := "paid"
	if *includeAll {
		status = ""
	}
	orders, err := api.ListOrders(callContext(), 50, status)
	if err != nil {
		return err
	}
	var human strings.Builder
	for _, order := range orders.Orders {
		fmt.Fprintf(&human, "%-24s %-14s %-16s %s\n",
			order.ID, order.PaymentStatus, order.FulfillmentStatus, moneyOf(order.TotalUSD))
	}
	if human.Len() == 0 {
		human.WriteString("No order matched.\n")
	}
	if len(orders.Orders) > MaxPageSize {
		return errs.New(errs.StoreUnavailable, "the server returned more orders than one page holds")
	}
	for _, order := range orders.Orders {
		if order.PaymentStatus == "" {
			panic("cli: a validated order list carries an order with no payment state")
		}
		if order.ID == "" {
			panic("cli: a validated order list carries an order with no identifier")
		}
	}
	return render(env, human.String(), orders)
}

// runPaymentsReconcile reads an existing payment outcome without authorizing a
// new payment, so a buyer can resolve an ambiguous attempt without risking a
// second transfer.
func runPaymentsReconcile(env *Env, args []string) error {
	set := flagSet("payments reconcile", env)
	if err := parse(set, args); err != nil {
		return err
	}
	orderID, err := requireArg(args, "order ID")
	if err != nil {
		return err
	}
	api, err := newClient(env)
	if err != nil {
		return err
	}
	result, err := api.ReconcilePayment(callContext(), orderID)
	if err != nil {
		return err
	}
	if result.ReconciliationStatus == "" {
		return errs.New(errs.PaymentPending, "the server reported no reconciliation state")
	}
	if result.PaymentStatus == "" {
		return errs.New(errs.PaymentPending, "the server reported no payment state")
	}
	if result.AttemptStatus == "" {
		panic("cli: a validated reconciliation carries no attempt state")
	}
	if result.PaymentStatus == paidStatus {
		if result.Transaction == "" {
			panic("cli: a settled reconciliation carries no transaction reference")
		}
	}
	return render(env,
		fmt.Sprintf("Order %s is %s. Reconciliation %s.\n",
			result.OrderID, result.PaymentStatus, result.ReconciliationStatus),
		result)
}

// requireSpendingPolicy refuses an unattended purchase with no policy, so a
// script cannot spend money its owner never bounded.
func requireSpendingPolicy() error {
	policy, err := loadSpendingPolicy()
	if err != nil {
		return err
	}
	if policy.MaxPerPurchaseUSD <= 0 {
		return errs.New(errs.InvalidRequest, "the spending policy names no maximum")
	}
	if len(policy.AllowedNetworks) == 0 {
		return errs.New(errs.InvalidRequest, "the spending policy names no network")
	}
	if len(policy.AllowedAssets) == 0 {
		return errs.New(errs.InvalidRequest, "the spending policy names no asset")
	}
	if policy.MaxPerPurchaseUSD > MaxPolicyTotal {
		panic("cli: a validated spending policy is above its bound")
	}
	if len(policy.AllowedNetworks) > MaxPolicyEntries {
		panic("cli: a validated spending policy lists more networks than its bound")
	}
	return nil
}

// refuseALiveStore refuses a fixture test purchase against a live store before
// any signing, because the server fixes a fixture session to the test network. A
// client side check never makes this safe on its own, so it is a safeguard for
// the buyer's own money and the server enforces the same rule.
func refuseALiveStore() error {
	kind := strings.TrimSpace(os.Getenv("VADORA_FIXTURE_STORE"))
	if kind == "" {
		return nil
	}
	if strings.EqualFold(kind, "live") {
		return errs.New(errs.InvalidRequest, "a test purchase cannot run against a live store")
	}
	if strings.EqualFold(kind, "development") {
		return nil
	}
	if len(kind) > MaxFixtureKindLength {
		panic("cli: a fixture store kind is longer than its bound")
	}
	if strings.EqualFold(kind, "live") {
		panic("cli: a live fixture kind was not refused")
	}
	return errs.New(errs.InvalidRequest, "the fixture store kind is not development or live")
}
