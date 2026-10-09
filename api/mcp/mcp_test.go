package mcp

import (
	"strings"
	"testing"

	"github.com/vadora/vadora/api/openapi"
)

// The shopper set holds exactly the tools the shared contract names, in order, so a
// client generated from the contract still resolves every name.
func TestTheShopperSetHoldsTheContractTools(t *testing.T) {
	t.Parallel()
	set, err := ShopperToolSet()
	if err != nil {
		t.Fatalf("the shopper tool set did not build: %v", err)
	}
	if set.Name != ShopperSetName {
		t.Fatalf("the shopper set is named %q, want %q", set.Name, ShopperSetName)
	}
	got := set.Names()
	if len(got) != len(contractShopperTools) {
		t.Fatalf("the shopper set holds %d tools, the contract names %d",
			len(got), len(contractShopperTools))
	}
	for index, name := range contractShopperTools {
		if got[index] != name {
			t.Fatalf("the shopper set holds %q where the contract names %q", got[index], name)
		}
	}
}

// The merchant set holds exactly the tools the shared contract names, in order.
func TestTheMerchantSetHoldsTheContractTools(t *testing.T) {
	t.Parallel()
	set, err := MerchantToolSet()
	if err != nil {
		t.Fatalf("the merchant tool set did not build: %v", err)
	}
	if set.Name != MerchantSetName {
		t.Fatalf("the merchant set is named %q, want %q", set.Name, MerchantSetName)
	}
	got := set.Names()
	if len(got) != len(contractMerchantTools) {
		t.Fatalf("the merchant set holds %d tools, the contract names %d",
			len(got), len(contractMerchantTools))
	}
	for index, name := range contractMerchantTools {
		if got[index] != name {
			t.Fatalf("the merchant set holds %q where the contract names %q", got[index], name)
		}
	}
}

// No shopper tool creates a quote or moves a stablecoin, because a payment is
// signed by a person on Vadora's own domain and never inside an agent host.
func TestNoShopperToolSignsOrPays(t *testing.T) {
	t.Parallel()
	set, err := ShopperToolSet()
	if err != nil {
		t.Fatalf("the shopper tool set did not build: %v", err)
	}
	forbidden := []string{"create_quote", "purchase", "submit_payment", "sign", "send_payment"}
	for _, tool := range set.Tools {
		for _, name := range forbidden {
			if strings.Contains(tool.Name, name) {
				t.Fatalf("the shopper set offers the paying tool %q", tool.Name)
			}
		}
		assertToolNeverClaimsSigning(t, tool)
	}
}

// assertToolNeverClaimsSigning refuses a tool whose description tells a model to
// sign or move funds, because the checkout handoff is the only payment path.
func assertToolNeverClaimsSigning(t *testing.T, tool Tool) {
	t.Helper()
	for _, phrase := range []string{"sign the payment", "signs the payment", "moves funds",
		"submits the payment", "spends the"} {
		if strings.Contains(tool.Description, phrase) {
			t.Fatalf("the tool %s describes signing: %s", tool.Name, phrase)
		}
	}
}

// start_checkout hands off to Vadora's own domain and names no credential, because a
// plugin never carries a payment key of any kind.
func TestStartCheckoutHandsOffWithoutACredential(t *testing.T) {
	t.Parallel()
	set, err := ShopperToolSet()
	if err != nil {
		t.Fatalf("the shopper tool set did not build: %v", err)
	}
	tool, present := lookup(t, set, "start_checkout")
	if !present {
		t.Fatalf("the shopper set offers no start_checkout")
	}
	if tool.ReadOnly {
		t.Fatalf("start_checkout claims to be read only, but it writes a handoff")
	}
	if !strings.Contains(tool.Description, "own domain") {
		t.Fatalf("start_checkout does not hand off to Vadora's own domain: %s", tool.Description)
	}
	assertToolCarriesNoCredentialField(t, tool)
}

// assertToolCarriesNoCredentialField refuses a tool that accepts a secret, because
// no plugin tool surface collects a key, a code, or a signature.
func assertToolCarriesNoCredentialField(t *testing.T, tool Tool) {
	t.Helper()
	fields := []string{"private_key", "secret", "password", "seed_phrase", "api_key",
		"sign_in_code", "signature", "card_number"}
	for name := range tool.InputSchema.Properties {
		for _, forbidden := range fields {
			if strings.Contains(name, forbidden) {
				t.Fatalf("the tool %s accepts the credential field %q", tool.Name, name)
			}
		}
	}
}

// Publishing and pausing require an explicit confirmation and say so, because a
// model must never change what a merchant sells without the merchant's own act.
func TestPublicationConfirmationExistsOnlyInTheWorkspace(t *testing.T) {
	set, err := MerchantToolSet()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"publish_products", "pause_products"} {
		tool, found := lookup(t, set, name)
		if !found {
			t.Fatalf("missing %s", name)
		}
		if _, found := tool.InputSchema.Properties["confirm"]; found {
			t.Fatal("plugin accepts a confirmation")
		}
		if !strings.Contains(tool.Description, "CONFIRMATION_REQUIRED") {
			t.Fatal("missing confirmation fallback")
		}
		if _, found := tool.InputSchema.Properties["listing_ids"]; !found {
			t.Fatal("missing listing_ids")
		}
	}
}

// A refund is recorded only in the hosted workspace, so no merchant tool names one as
// something it performs.
func TestNoMerchantToolRecordsARefund(t *testing.T) {
	t.Parallel()
	set, err := MerchantToolSet()
	if err != nil {
		t.Fatalf("the merchant tool set did not build: %v", err)
	}
	for _, tool := range set.Tools {
		if strings.Contains(tool.Name, "refund") {
			t.Fatalf("the merchant set offers the refund tool %q", tool.Name)
		}
	}
	for _, tool := range set.Tools {
		for name := range tool.InputSchema.Properties {
			if strings.Contains(name, "refund") {
				t.Fatalf("the tool %s accepts the refund field %q", tool.Name, name)
			}
		}
	}
}

// start_store_connection writes no credential and returns none, because the
// merchant authorizes the app in the hosted workspace.
func TestStoreConnectionCarriesNoCredential(t *testing.T) {
	t.Parallel()
	set, err := MerchantToolSet()
	if err != nil {
		t.Fatalf("the merchant tool set did not build: %v", err)
	}
	tool, present := lookup(t, set, "start_store_connection")
	if !present {
		t.Fatalf("the merchant set offers no start_store_connection")
	}
	if !strings.Contains(tool.Description, "hosted workspace") {
		t.Fatalf("start_store_connection does not send the merchant to the workspace: %s",
			tool.Description)
	}
	assertToolCarriesNoCredentialField(t, tool)
}

// get_onboarding_state links each missing item to the hosted workspace, because the
// merchant completes setup there and never through a tool.
func TestOnboardingStateLinksTheHostedWorkspace(t *testing.T) {
	t.Parallel()
	set, err := MerchantToolSet()
	if err != nil {
		t.Fatalf("the merchant tool set did not build: %v", err)
	}
	tool, present := lookup(t, set, "get_onboarding_state")
	if !present {
		t.Fatalf("the merchant set offers no get_onboarding_state")
	}
	if !strings.Contains(tool.Description, "hosted workspace") {
		t.Fatalf("get_onboarding_state names no workspace link: %s", tool.Description)
	}
	if !tool.ReadOnly {
		t.Fatalf("get_onboarding_state claims to write, but it reads state")
	}
}

// Every read only tool is one a host may call without an account, so an
// unauthenticated session is never offered a write.
func TestOnlyCatalogToolsAreReadOnly(t *testing.T) {
	t.Parallel()
	sets, err := BothToolSets()
	if err != nil {
		t.Fatalf("the tool sets did not build: %v", err)
	}
	readOnly := map[string]bool{
		"search_products": true, "list_stores": true, "get_product": true,
		"compare_products": true, "get_order_status": true,
		"get_onboarding_state": true, "get_store_preview": true,
		"get_readiness": true, "list_merchant_orders": true,
	}
	for _, set := range sets {
		for _, tool := range set.Tools {
			if tool.ReadOnly != readOnly[tool.Name] {
				t.Fatalf("the tool %s reports read only %t, the contract says %t",
					tool.Name, tool.ReadOnly, readOnly[tool.Name])
			}
		}
	}
}

// Every tool declares an object schema with at least one field, because a host that
// receives an empty schema cannot call the tool meaningfully.
func TestEveryToolDeclaresAUsableInputSchema(t *testing.T) {
	t.Parallel()
	sets, err := BothToolSets()
	if err != nil {
		t.Fatalf("the tool sets did not build: %v", err)
	}
	for _, set := range sets {
		if len(set.Tools) > MaxToolSetSize {
			t.Fatalf("the tool set %q holds %d tools, the bound is %d",
				set.Name, len(set.Tools), MaxToolSetSize)
		}
		for _, tool := range set.Tools {
			if tool.InputSchema.Type != SchemaObjectType {
				t.Fatalf("the tool %s declares an input schema of type %q",
					tool.Name, tool.InputSchema.Type)
			}
			if len(tool.InputSchema.Properties) == 0 {
				t.Fatalf("the tool %s declares no input field", tool.Name)
			}
			if tool.Description == "" {
				t.Fatalf("the tool %s declares no description", tool.Name)
			}
			if len(tool.Description) > MaxDescriptionLength {
				t.Fatalf("the tool %s declares an overlong description", tool.Name)
			}
		}
	}
}

// Every required field of every tool is one the tool declares, because a host that
// is told to send an undeclared field cannot build a valid call.
func TestEveryRequiredFieldIsDeclared(t *testing.T) {
	t.Parallel()
	sets, err := BothToolSets()
	if err != nil {
		t.Fatalf("the tool sets did not build: %v", err)
	}
	for _, set := range sets {
		for _, tool := range set.Tools {
			for _, required := range tool.InputSchema.Required {
				if _, declared := tool.InputSchema.Properties[required]; !declared {
					t.Fatalf("the tool %s requires the undeclared field %q", tool.Name, required)
				}
			}
		}
	}
}

// Every free text field a tool accepts is bounded, so a model cannot ask for an
// unbounded value.
func TestEveryFreeTextFieldIsBounded(t *testing.T) {
	t.Parallel()
	sets, err := BothToolSets()
	if err != nil {
		t.Fatalf("the tool sets did not build: %v", err)
	}
	for _, set := range sets {
		for _, tool := range set.Tools {
			for name, field := range tool.InputSchema.Properties {
				if field.Type != "string" {
					continue
				}
				if field.MaxLength == nil {
					t.Fatalf("the tool %s accepts the unbounded field %q", tool.Name, name)
				}
				if *field.MaxLength > boundedFieldLength {
					t.Fatalf("the tool %s accepts %q with a bound above the field bound",
						tool.Name, name)
				}
			}
		}
	}
}

// Every tool names the catalog codes it may return, so a host's client can handle
// each failure without holding the error catalog.
func TestEveryToolNamesItsErrorCodes(t *testing.T) {
	t.Parallel()
	sets, err := BothToolSets()
	if err != nil {
		t.Fatalf("the tool sets did not build: %v", err)
	}
	for _, set := range sets {
		for _, tool := range set.Tools {
			if !strings.Contains(tool.Description, "May return:") {
				t.Fatalf("the tool %s names no error codes", tool.Name)
			}
			if !strings.Contains(tool.Description, "INVALID_REQUEST") {
				t.Fatalf("the tool %s does not name the invalid request code", tool.Name)
			}
		}
	}
}

// A tool that is described as read only never describes publishing or pausing,
// because a read only tool that changed a listing would be a write in disguise.
func TestAReadOnlyToolNeverDescribesAWrite(t *testing.T) {
	t.Parallel()
	sets, err := BothToolSets()
	if err != nil {
		t.Fatalf("the tool sets did not build: %v", err)
	}
	for _, set := range sets {
		for _, tool := range set.Tools {
			if !tool.ReadOnly {
				continue
			}
			for _, write := range []string{"publishes the named", "pauses the named"} {
				if strings.Contains(tool.Description, write) {
					t.Fatalf("the read only tool %s describes writing: %s", tool.Name, write)
				}
			}
		}
	}
}

// The two sets share no tool name, because a session authenticated as one may never
// be offered the other's tools.
func TestTheTwoSetsShareNoTool(t *testing.T) {
	t.Parallel()
	sets, err := BothToolSets()
	if err != nil {
		t.Fatalf("the tool sets did not build: %v", err)
	}
	if len(sets) != 2 {
		t.Fatalf("the contract declares %d tool sets, want 2", len(sets))
	}
	seen := make(map[string]bool)
	for _, set := range sets {
		if seen[set.Name] {
			t.Fatalf("the tool set name %q appears twice", set.Name)
		}
		seen[set.Name] = true
		for _, tool := range set.Tools {
			for _, other := range sets {
				if other.Name == set.Name {
					break
				}
				for _, candidate := range other.Tools {
					if candidate.Name == tool.Name {
						t.Fatalf("the tool %q appears in both sets", tool.Name)
					}
				}
			}
		}
	}
}

// The comparison and cart tools bound how many items they carry, so a model cannot
// ask for an unbounded table or an unbounded cart.
func TestTheComparisonAndSingleCartAreBounded(t *testing.T) {
	set, err := ShopperToolSet()
	if err != nil {
		t.Fatal(err)
	}
	compare, _ := lookup(t, set, "compare_products")
	ids := compare.InputSchema.Properties["product_ids"]
	if ids.Type != "array" {
		t.Fatal("comparison is not an array")
	}
	if *ids.MaxItems != 4 {
		t.Fatal("comparison exceeds four")
	}
	cart, _ := lookup(t, set, "set_cart")
	if _, found := cart.InputSchema.Properties["items"]; found {
		t.Fatal("cart permits multiple items")
	}
	if _, found := cart.InputSchema.Properties["checkout_session_id"]; found {
		t.Fatal("plugin accepts a checkout session")
	}
	if cart.Scope != "shopper" {
		t.Fatal("cart has no shopper scope")
	}
	if !cart.Destructive {
		t.Fatal("cart overwrite is not annotated")
	}
}

// lookup returns one tool by name, so a test can assert that tool's own contract.
func lookup(t *testing.T, set ToolSet, name string) (Tool, bool) {
	t.Helper()
	for _, tool := range set.Tools {
		if tool.Name == name {
			return tool, true
		}
	}
	return Tool{}, false
}

// The price a tool shows uses the same component schema the REST document
// publishes, so one price has one description across both surfaces.
func TestTheMoneySchemaIsSharedWithTheOpenAPIDocument(t *testing.T) {
	t.Parallel()
	spec, err := openapi.Build()
	if err != nil {
		t.Fatalf("the REST document did not build: %v", err)
	}
	if _, present := spec.Components.Schemas[openapi.MoneyName]; !present {
		t.Fatalf("the REST document declares no %q schema", openapi.MoneyName)
	}
	reference := moneySchemaRef()
	if reference.Ref != openapi.SchemaRef(openapi.MoneyName) {
		t.Fatalf("the money reference is %q, want %q",
			reference.Ref, openapi.SchemaRef(openapi.MoneyName))
	}
}
