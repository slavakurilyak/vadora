// Package integrations holds the read only display catalog that
// :IntegrationPresentation: defines. It is part of the :SharedContract:, so the
// server, the CLI, and the plugin build from one list, and every named deferred
// integration reads "Coming soon" with status coming_soon wherever it appears.
//
// Nothing in this package grants a capability. Availability stays with the five
// registries, and a visible future integration never becomes a callable one.
package integrations

import (
	"errors"
	"strings"
)

// Category groups the integrations overview.
type Category string

const (
	CommercePlatforms Category = "commerce_platforms"
	SettlementChains  Category = "settlement_chains"
	PaymentProtocols  Category = "payment_protocols"
	CommerceProtocols Category = "commerce_protocols"
	AgentHosts        Category = "agent_hosts"
)

// ComingSoonLabel is the exact visible text every deferred integration carries.
// The label describes planned support and promises no release date.
const ComingSoonLabel = "Coming soon"

// ComingSoonStatus is the machine readable status that goes with the label.
const ComingSoonStatus = "coming_soon"

// Status is the display status of one integration.
type Status string

const (
	// StatusAvailable marks a launched integration.
	StatusAvailable Status = "available"
	// StatusComingSoon marks a named integration with no launch implementation.
	StatusComingSoon Status = "coming_soon"
)

// Integration is one row of the display catalog.
type Integration struct {
	// ID is the stable identifier the registries and the catalog share.
	ID string
	// DisplayName is the name a person reads.
	DisplayName string
	// Category is the overview group the integration appears under.
	Category Category
	// Status is the display status.
	Status Status
	// Rank is the registry order the integration keeps for future use. A rank
	// reserves an order and creates no build commitment.
	Rank int
	// Note explains what "Coming soon" means for this integration.
	Note string
	// ActionsEnabled is false for every deferred integration, so a setup or
	// selection view can disable connect, install, enable, and payment.
	ActionsEnabled bool
}

// Label returns the text a view shows for the integration's status. Availability
// is a display fact, and a deferred integration always reads "Coming soon"
// whatever its operational state.
func (i Integration) Label() string {
	var label string
	switch i.Status {
	case StatusComingSoon:
		label = ComingSoonLabel
	case StatusAvailable:
		label = "Available"
	default:
		panic("integrations: unknown display status: " + string(i.Status))
	}
	if label == "" {
		panic("integrations: a display status rendered no label")
	}
	if i.Status == StatusComingSoon {
		if label != ComingSoonLabel {
			panic("integrations: a deferred integration rendered another label")
		}
	}
	return label
}

// ActionsAreEnabled reports whether a view may offer an operational action. A
// deferred integration is never operable, whatever a client asks for, so the
// status decides before the stored flag is consulted.
func (i Integration) ActionsAreEnabled() bool {
	if i.Status == StatusComingSoon {
		return false
	}
	if i.Status != StatusAvailable {
		panic("integrations: unknown display status: " + string(i.Status))
	}
	enabled := i.ActionsEnabled
	if enabled {
		if i.Status == StatusComingSoon {
			panic("integrations: a deferred integration offers an action")
		}
	}
	return enabled
}

// catalog is the whole display catalog. Every named integration in the
// specifications appears here, including the ones with no adapter package and no
// credential, because :IntegrationPresentation: requires the label to be
// visible everywhere.
var catalog = []Integration{
	{
		ID:             "shopify",
		DisplayName:    "Shopify",
		Category:       CommercePlatforms,
		Status:         StatusAvailable,
		Rank:           1,
		Note:           "Merchants join by invitation through Shopify custom distribution.",
		ActionsEnabled: true,
	},
	{
		ID:             "woocommerce",
		DisplayName:    "WooCommerce",
		Category:       CommercePlatforms,
		Status:         StatusComingSoon,
		Rank:           2,
		Note:           "Its connector uses the WooCommerce REST API v3 and has no launch implementation.",
		ActionsEnabled: false,
	},
	{
		ID:             "bigcommerce",
		DisplayName:    "BigCommerce",
		Category:       CommercePlatforms,
		Status:         StatusComingSoon,
		Rank:           3,
		Note:           "Deferred until WooCommerce is enabled.",
		ActionsEnabled: false,
	},
	{
		ID:             "magento",
		DisplayName:    "Magento",
		Category:       CommercePlatforms,
		Status:         StatusComingSoon,
		Rank:           4,
		Note:           "Deferred until WooCommerce is enabled.",
		ActionsEnabled: false,
	},
	{
		ID:             "wix",
		DisplayName:    "Wix",
		Category:       CommercePlatforms,
		Status:         StatusComingSoon,
		Rank:           5,
		Note:           "Deferred until WooCommerce is enabled.",
		ActionsEnabled: false,
	},
	{
		ID:             "solana",
		DisplayName:    "Solana",
		Category:       SettlementChains,
		Status:         StatusAvailable,
		Rank:           1,
		Note:           "Settles USDC on Solana mainnet and devnet.",
		ActionsEnabled: true,
	},
	{
		ID:             "base",
		DisplayName:    "Base",
		Category:       SettlementChains,
		Status:         StatusComingSoon,
		Rank:           2,
		Note:           "Its USDC contract address and CAIP-2 identifier are verified only when it is approved.",
		ActionsEnabled: false,
	},
	{
		ID:             "ethereum",
		DisplayName:    "Ethereum",
		Category:       SettlementChains,
		Status:         StatusComingSoon,
		Rank:           3,
		Note:           "Its USDC contract address and CAIP-2 identifier are verified only when it is approved.",
		ActionsEnabled: false,
	},
	{
		ID:             "x402",
		DisplayName:    "x402 version 2",
		Category:       PaymentProtocols,
		Status:         StatusAvailable,
		Rank:           1,
		Note:           "The launch payment protocol, settled through the PayAI facilitator.",
		ActionsEnabled: true,
	},
	{
		ID:             "mpp",
		DisplayName:    "MPP",
		Category:       PaymentProtocols,
		Status:         StatusComingSoon,
		Rank:           2,
		Note:           "Its specification version is verified only when it is approved.",
		ActionsEnabled: false,
	},
	{
		ID:             "ucp",
		DisplayName:    "Universal Commerce Protocol",
		Category:       CommerceProtocols,
		Status:         StatusComingSoon,
		Rank:           1,
		Note:           "A launch deployment advertises no UCP discovery or checkout capability.",
		ActionsEnabled: false,
	},
	{
		ID:             "acp",
		DisplayName:    "Agent Commerce Protocol",
		Category:       CommerceProtocols,
		Status:         StatusComingSoon,
		Rank:           2,
		Note:           "A launch deployment advertises no ACP discovery or checkout capability.",
		ActionsEnabled: false,
	},
	{
		ID:             "chatgpt",
		DisplayName:    "ChatGPT",
		Category:       AgentHosts,
		Status:         StatusAvailable,
		Rank:           1,
		Note:           "The first agent host, packaged with OpenAI's plugin extensions.",
		ActionsEnabled: true,
	},
	{
		ID:             "claudecode",
		DisplayName:    "Claude Code",
		Category:       AgentHosts,
		Status:         StatusComingSoon,
		Rank:           2,
		Note:           "Its plugin directory, manifest, and marketplace catalog are implemented only after a scope change.",
		ActionsEnabled: false,
	},
	{
		ID:             "anthropic_extension_layer",
		DisplayName:    "Anthropic extension layer",
		Category:       AgentHosts,
		Status:         StatusComingSoon,
		Rank:           3,
		Note:           "The next :HostExtensionLayer:, expected to follow as its own specification.",
		ActionsEnabled: false,
	},
}

// deferredNames lists every integration the specifications name as deferred. It
// is the acceptance list for the integrations overview.
var deferredNames = []string{
	"woocommerce",
	"bigcommerce",
	"magento",
	"wix",
	"base",
	"ethereum",
	"mpp",
	"ucp",
	"acp",
	"claudecode",
	"anthropic_extension_layer",
}

// Validate reports whether the display catalog still satisfies every rule
// :IntegrationPresentation: states: a named deferred integration is visible, it
// reads "Coming soon", it offers no operational action, its rank is reserved and
// positive, and no identifier appears twice. The server calls it at startup so a
// bad edit fails closed instead of quietly hiding an integration.
func Validate() error {
	if len(catalog) == 0 {
		return errEmptyCatalog
	}
	seen := make(map[string]bool, len(catalog))
	for _, entry := range catalog {
		if entry.ID == "" {
			return errNoIdentifier
		}
		if seen[entry.ID] {
			return errDuplicateIdentifier
		}
		seen[entry.ID] = true
		if entry.DisplayName == "" {
			return errNoDisplayName
		}
		if entry.Rank < 1 {
			return errRankNotReserved
		}
		if entry.Status == StatusComingSoon {
			if entry.ActionsEnabled {
				return errDeferredOffersAction
			}
		}
		if entry.Status == StatusAvailable {
			if !entry.ActionsEnabled {
				return errLaunchedOffersNoAction
			}
		}
	}
	for _, id := range deferredNames {
		if !seen[id] {
			return errDeferredMissing
		}
	}
	if len(seen) != len(catalog) {
		return errEmptyCatalog
	}
	if len(deferredNames) == 0 {
		return errEmptyCatalog
	}
	if len(seen) == 0 {
		return errEmptyCatalog
	}
	if len(catalog) < len(deferredNames) {
		return errEmptyCatalog
	}
	return nil
}

// The catalog faults. Each one names the rule that broke, so a bad edit is
// reported once at startup rather than silently at runtime.
var (
	errEmptyCatalog           = errors.New("the display catalog is empty")
	errNoIdentifier           = errors.New("an integration has no identifier")
	errDuplicateIdentifier    = errors.New("an integration appears twice")
	errNoDisplayName          = errors.New("an integration has no display name")
	errRankNotReserved        = errors.New("an integration has no reserved rank")
	errDeferredOffersAction   = errors.New("a deferred integration offers an operational action")
	errLaunchedOffersNoAction = errors.New("a launched integration offers no operational action")
	errDeferredMissing        = errors.New("a named deferred integration is missing from the catalog")
)

// mustValidate panics when the catalog breaks a rule, so a caller that cannot
// handle the fault still fails closed.
func mustValidate() {
	err := Validate()
	if err == nil {
		if len(catalog) == 0 {
			panic("integrations: an empty catalog passed validation")
		}
		return
	}
	panic("integrations: " + err.Error())
}

// check guards the one row a view is about to render. A hand edited catalog row
// that breaks its own rules never reaches a page, because the rules a view
// depends on are the rules this checks.
func check(entry Integration) {
	if entry.ID == "" {
		panic("integrations: a rendered integration has no identifier")
	}
	if entry.DisplayName == "" {
		panic("integrations: a rendered integration has no display name")
	}
	if entry.Rank < 1 {
		panic("integrations: a rendered integration has no reserved rank")
	}
	if entry.Status != StatusComingSoon {
		return
	}
	if entry.ActionsEnabled {
		panic("integrations: a deferred integration would offer an operational action")
	}
}

// All returns the whole catalog in rank order within each category, as a copy
// so no caller can edit the catalog for every other caller.
func All() []Integration {
	mustValidate()
	for _, entry := range catalog {
		check(entry)
	}
	copied := append([]Integration(nil), catalog...)
	if len(copied) != len(catalog) {
		panic("integrations: All lost catalog rows while copying them")
	}
	if len(copied) == 0 {
		panic("integrations: All copied an empty catalog")
	}
	return copied
}

// ByCategory returns one overview group, in rank order. An unknown category
// returns nothing rather than every entry, so a view can never show a group it
// did not ask for.
func ByCategory(category Category) []Integration {
	mustValidate()
	if category == "" {
		panic("integrations: an overview group was asked for with no category")
	}
	var group []Integration
	for _, entry := range catalog {
		if entry.Category != category {
			continue
		}
		group = append(group, entry)
	}
	if len(group) > len(catalog) {
		panic("integrations: a group holds more entries than the catalog")
	}
	for _, entry := range group {
		if entry.Category != category {
			panic("integrations: ByCategory returned an entry from another group")
		}
		check(entry)
	}
	if len(group) > len(catalog) {
		panic("integrations: a group holds more entries than the catalog")
	}
	return group
}

// Lookup returns one integration by its identifier. An unknown identifier
// returns nothing, because display metadata never advertises an integration that
// does not exist.
func Lookup(id string) (Integration, bool) {
	mustValidate()
	for _, entry := range catalog {
		if entry.ID != id {
			continue
		}
		check(entry)
		if entry.ID != id {
			panic("integrations: a lookup returned another integration")
		}
		if entry.Label() == "" {
			panic("integrations: a found integration renders no label")
		}
		return entry, true
	}
	return Integration{}, false
}

// DeferredNames returns every named deferred integration identifier, so a check
// can prove each one appears in the catalog as "Coming soon".
func DeferredNames() []string {
	copied := append([]string(nil), deferredNames...)
	if len(copied) != len(deferredNames) {
		panic("integrations: DeferredNames lost entries while copying them")
	}
	for _, id := range copied {
		if id == "" {
			panic("integrations: a deferred name is empty")
		}
	}
	return copied
}

// Categories returns every overview group in presentation order.
func Categories() []Category {
	order := []Category{
		CommercePlatforms,
		SettlementChains,
		PaymentProtocols,
		CommerceProtocols,
		AgentHosts,
	}
	if len(order) != 5 {
		panic("integrations: the overview has five groups")
	}
	seen := make(map[Category]bool, len(order))
	for _, category := range order {
		if category == "" {
			panic("integrations: an overview group has no category")
		}
		if seen[category] {
			panic("integrations: an overview group appears twice")
		}
		seen[category] = true
	}
	return order
}

// MaxSearchQueryLength bounds a catalog search query from an untrusted caller.
const MaxSearchQueryLength = 256

// Search returns the published integrations whose name matches a query. An
// unknown query returns nothing rather than a suggestion, because display
// metadata never advertises an integration that does not exist.
func Search(query string) []Integration {
	mustValidate()
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		return nil
	}
	if len(needle) > MaxSearchQueryLength {
		panic("integrations: a search query exceeded its bound")
	}
	var matches []Integration
	for _, entry := range catalog {
		haystack := strings.ToLower(entry.DisplayName)
		if strings.Contains(haystack, needle) {
			matches = append(matches, entry)
		}
	}
	if len(matches) > len(catalog) {
		panic("integrations: search returned more results than the catalog holds")
	}
	for _, entry := range matches {
		check(entry)
	}
	return matches
}
