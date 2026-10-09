// Package mcp owns the MCP tool schemas every client and the server follow. It
// declares the shopper and merchant tool sets from the shared contract, so the
// hosted MCP server and a plugin's generated client are written from one
// definition and a tool that is renamed in one place cannot drift from the other.
//
// Every schema here is a description. Nothing in this package performs a tool
// call, signs a payment, or holds a credential.
package mcp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/vadora/vadora/api/openapi"
	"github.com/vadora/vadora/api/types/errs"
)

// ProtocolVersion is the Model Context Protocol revision these schemas target.
const ProtocolVersion = "2026-07-28"

// ContractVersion changes when a published tool schema changes.
const ContractVersion = "0.2.0"

// ServerName is the MCP server's name, as a host displays it.
const ServerName = "xyz.vadora/vadora"

// MaxToolNameLength bounds one tool name, so a generated client cannot carry an
// unbounded symbol.
const MaxToolNameLength = 64

// MaxDescriptionLength bounds one tool or parameter description.
const MaxDescriptionLength = 2000

// ToolSet is one named group of tools. The shopper and merchant sets are separate,
// because a session authenticated as one may never be offered the other's tools.
type ToolSet struct {
	// Name is the set's identifier, which a host uses to group its tools.
	Name string
	// Description explains what the set is for.
	Description string
	// Tools are the set's tools in the order the contract names them.
	Tools []Tool
}

// Tool is one MCP tool. Its InputSchema is the same Schema the REST document uses,
// so a field carries one description in both surfaces.
type Tool struct {
	Name        string
	Title       string
	Description string
	InputSchema openapi.Schema
	// ReadOnly marks a tool that changes no state. A read only tool is offered to
	// an unauthenticated host session for the catalog reads.
	ReadOnly bool
	// RequiresAccount marks a tool that needs a signed in account rather than a
	// plugin session alone.
	RequiresAccount bool
	// Destructive and OpenWorld describe the effects, separately from auth.
	Destructive bool
	OpenWorld   bool
	// Scope is the server enforced OAuth scope for a private tool.
	Scope string
}

// Names returns the set's tool names in order, so a test can assert the set holds
// exactly the tools the contract names.
func (t ToolSet) Names() []string {
	names := make([]string, 0, len(t.Tools))
	for _, tool := range t.Tools {
		if tool.Name == "" {
			panic("mcp: a tool set listed an unnamed tool")
		}
		names = append(names, tool.Name)
	}
	if len(names) != len(t.Tools) {
		panic("mcp: a tool set lost a tool while it listed its names")
	}
	return names
}

// MaxToolSetSize bounds one set's tool count, so a host's tool list stays readable.
const MaxToolSetSize = 16

// contractShopperTools are the shopper tool names the shared contract fixes.
var contractShopperTools = []string{
	"search_products", "list_stores", "get_product", "compare_products",
	"set_cart", "start_checkout", "get_order_status",
}

// contractMerchantTools are the merchant tool names the shared contract fixes.
var contractMerchantTools = []string{
	"get_onboarding_state", "start_store_connection", "sync_products",
	"get_store_preview", "publish_products", "pause_products",
	"get_readiness", "list_merchant_orders",
}

// ShopperToolSet returns the shopper tools. They browse, keep the cart, hand off to
// Vadora's own domain, or read status. None of them creates a quote or executes a
// payment, because a stablecoin payment is signed by a person on Vadora's own
// origin and never inside an agent host.
func ShopperToolSet() (ToolSet, error) {
	if len(contractShopperTools) > MaxToolSetSize {
		panic("mcp: the shopper contract names more tools than a host can list")
	}
	if ShopperSetName == "" {
		panic("mcp: the shopper set is named nothing")
	}
	set := ToolSet{
		Name: ShopperSetName,
		Description: strings.Join([]string{
			"Tools a shopper uses to find a merchant's products, keep a cart, hand off to",
			"Vadora's own domain to sign a payment, and read the status of an order it already",
			"bought. No tool in this set creates a quote or moves funds: a payment is signed by",
			"the shopper's own wallet on Vadora's own domain.",
		}, "\n"),
		Tools: []Tool{
			searchProductsTool(),
			listStoresTool(),
			getProductTool(),
			compareProductsTool(),
			setCartTool(),
			startCheckoutTool(),
			getOrderStatusTool(),
		},
	}
	if err := checkSet(&set, contractShopperTools); err != nil {
		return ToolSet{}, err
	}
	return set, nil
}

// MerchantToolSet returns the merchant tools. They read readiness, prepare the
// store connection, import and publish listings, and read the merchant's own
// orders. Publishing and pausing require an explicit confirmation, and refunds are
// recorded only in the hosted workspace, never through a tool.
func MerchantToolSet() (ToolSet, error) {
	if len(contractMerchantTools) > MaxToolSetSize {
		panic("mcp: the merchant contract names more tools than a host can list")
	}
	if MerchantSetName == "" {
		panic("mcp: the merchant set is named nothing")
	}
	set := ToolSet{
		Name: MerchantSetName,
		Description: strings.Join([]string{
			"Tools a merchant uses to read onboarding state, connect a store, import and select",
			"products, publish or pause listings with an explicit confirmation, and read the",
			"store's own orders. A refund is recorded only in the hosted workspace, never through",
			"a tool.",
		}, "\n"),
		Tools: []Tool{
			getOnboardingStateTool(),
			startStoreConnectionTool(),
			syncProductsTool(),
			getStorePreviewTool(),
			publishProductsTool(),
			pauseProductsTool(),
			getReadinessTool(),
			listMerchantOrdersTool(),
		},
	}
	if err := checkSet(&set, contractMerchantTools); err != nil {
		return ToolSet{}, err
	}
	return set, nil
}

// BothToolSets returns the shopper and merchant sets, so a host's discovery
// document can list both without importing this package's internals.
func BothToolSets() ([]ToolSet, error) {
	if ShopperSetName == MerchantSetName {
		panic("mcp: the two tool sets share a name")
	}
	shopper, err := ShopperToolSet()
	if err != nil {
		return nil, err
	}
	merchant, err := MerchantToolSet()
	if err != nil {
		return nil, err
	}
	sets := []ToolSet{shopper, merchant}
	if len(sets) != 2 {
		return nil, fmt.Errorf("the contract declares %d tool sets, want 2", len(sets))
	}
	return sets, nil
}

// The set names below are the identifiers a host groups tools by.
const (
	// ShopperSetName groups the shopper tools.
	ShopperSetName = "vadora_shopper"
	// MerchantSetName groups the merchant tools.
	MerchantSetName = "vadora_merchant"
)

// checkSet refuses a set that does not hold exactly the tools the shared contract
// names, in that order, because a renamed or missing tool would break every client
// that was generated from the contract.
func checkSet(set *ToolSet, contract []string) error {
	if set == nil {
		return fmt.Errorf("a tool set was checked with no set")
	}
	if len(contract) == 0 {
		return fmt.Errorf("the tool set %q was checked against an empty contract", set.Name)
	}
	if len(set.Tools) == 0 {
		return fmt.Errorf("the tool set %q holds no tool", set.Name)
	}
	if len(set.Tools) > MaxToolSetSize {
		return fmt.Errorf("the tool set %q holds more tools than a host can list", set.Name)
	}
	if len(set.Tools) != len(contract) {
		return fmt.Errorf("the tool set %q holds %d tools, the contract names %d",
			set.Name, len(set.Tools), len(contract))
	}
	for index, tool := range set.Tools {
		if tool.Name != contract[index] {
			return fmt.Errorf("the tool set %q holds %q where the contract names %q",
				set.Name, tool.Name, contract[index])
		}
		if err := checkTool(set.Name, tool); err != nil {
			return err
		}
	}
	return nil
}

// checkTool refuses one tool a host could not call: no name, no description, no
// schema, or a write tool that claims to be read only.
func checkTool(setName string, tool Tool) error {
	if setName == "" {
		return fmt.Errorf("a tool was checked against no set")
	}
	if tool.Name == "" {
		return fmt.Errorf("the tool set %q holds an unnamed tool", setName)
	}
	if len(tool.Name) > MaxToolNameLength {
		return fmt.Errorf("the tool %s carries an overlong name", tool.Name)
	}
	if tool.Description == "" {
		return fmt.Errorf("the tool %s carries no description", tool.Name)
	}
	if len(tool.Description) > MaxDescriptionLength {
		return fmt.Errorf("the tool %s carries an overlong description", tool.Name)
	}
	if tool.InputSchema.Type != SchemaObjectType {
		return fmt.Errorf("the tool %s declares an input schema that is not an object", tool.Name)
	}
	if tool.ReadOnly {
		if claimsWrite(tool.Description) {
			return fmt.Errorf("the read only tool %s describes a write", tool.Name)
		}
	}
	return nil
}

// claimsWrite reports whether a description says the tool acts. The phrases are
// the ways a description states an action, so a read only tool that said one would
// be a write in disguise.
func claimsWrite(description string) bool {
	if len(description) > MaxDescriptionLength {
		panic("mcp: a write check read an overlong description")
	}
	phrases := []string{
		"publishes the", "pauses the", "creates a quote", "creates the quote",
		"moves funds", "submits a payment", "submits the payment", "records a refund",
		"cancels an order", "starts a purchase",
	}
	for _, phrase := range phrases {
		if strings.Contains(description, phrase) {
			return true
		}
	}
	return false
}

// MaxClaimPhrases bounds the phrases the write check reads.
const MaxClaimPhrases = 16

// SchemaObjectType is the JSON Schema type every tool input schema carries, so a
// host always receives an object.
const SchemaObjectType = "object"

// boundedFieldLength bounds every free text field a tool accepts, so a model
// cannot ask for an unbounded value.
const boundedFieldLength = 128

// maxIdentifierLength bounds every identifier a tool accepts.
const maxIdentifierLength = 64

// stringField returns a bounded string field.
func stringField(description string) openapi.Schema {
	if description == "" {
		panic("mcp: a string field was declared with no description")
	}
	return openapi.Schema{
		Type:        "string",
		Description: description,
		MaxLength:   lengthPointer(boundedFieldLength),
	}
}

// identifierField returns a bounded identifier field.
func identifierField(description string) openapi.Schema {
	if description == "" {
		panic("mcp: an identifier field was declared with no description")
	}
	written := lengthPointer(maxIdentifierLength)
	if written == nil {
		panic("mcp: an identifier field was written with no bound")
	}
	if *written > MaxFieldBound {
		panic("mcp: the identifier bound exceeds the field bound")
	}
	return openapi.Schema{
		Type:        "string",
		Description: description,
		MaxLength:   written,
	}
}

// boundedCount returns a bounded count field.
func boundedCount(description string) openapi.Schema {
	low := int64(1)
	return openapi.Schema{
		Type:        "integer",
		Description: description,
		Minimum:     &low,
	}
}

// lengthPointer returns the addressable form of a bound, so a field can express a
// maximum without a second field type.
func lengthPointer(value int64) *int64 {
	if value <= 0 {
		panic("mcp: a field bound was written as not positive")
	}
	if value > MaxFieldBound {
		panic("mcp: a field bound above the field bound was written")
	}
	return &value
}

// MaxFieldBound bounds any bound a tool field declares, so a field can never exceed
// what a host stores in an integer.
const MaxFieldBound = 4096

// errorCodes returns the catalog codes a tool may return, so a host's client can
// handle every failure the tool names without reading the error catalog.
func errorCodes() []string {
	values := make([]string, 0, MaxToolErrorCodes)
	for _, code := range []errs.Code{
		errs.InvalidRequest, errs.Unauthenticated, errs.NotFound, errs.RateLimited,
		errs.StoreDisconnected, errs.StoreUnavailable, errs.ProductUnavailable,
		errs.ShippingUnsupported, errs.QuoteExpired, errs.PaymentPending,
		errs.ConfirmationRequired, errs.AdapterUnavailable,
	} {
		values = append(values, string(code))
	}
	return values
}

// MaxToolErrorCodes bounds the codes one tool may name in its description.
const MaxToolErrorCodes = 16

// sortedCodes returns the codes a tool may return in a fixed order, so two builds
// of this package describe a tool identically.
func sortedCodes() []string {
	codes := errorCodes()
	if len(codes) == 0 {
		panic("mcp: a tool description named no error code")
	}
	sort.Strings(codes)
	if len(codes) > MaxToolErrorCodes {
		panic("mcp: a tool description named more error codes than its bound")
	}
	return codes
}

// MinToolErrorCodes is the smallest set of codes a tool may name, so a tool always
// tells its caller at least how to correct its own input.
const MinToolErrorCodes = 2

// codeList returns the codes a tool's description names, so a reader can see which
// failures a tool reports without holding the catalog.
func codeList(codes []string) string {
	if len(codes) == 0 {
		panic("mcp: a tool named no error code")
	}
	if len(codes) < MinToolErrorCodes {
		panic("mcp: a tool named fewer error codes than its floor")
	}
	if len(codes) > MaxToolErrorCodes {
		panic("mcp: a tool named more error codes than its bound")
	}
	return strings.Join(codes, ", ")
}
