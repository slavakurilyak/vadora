package chatgpt

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	contract "github.com/vadora/vadora/api/mcp"
	"github.com/vadora/vadora/api/types/errs"
)

// update rewrites the golden fixtures from the adapter: go test ./plugins/chatgpt -update.
var update = flag.Bool("update", false, "rewrite the golden fixtures")

func testAdapter(t *testing.T) *Adapter {
	t.Helper()
	adapter, err := New(Config{APIOrigin: "https://api.vadora.test", UIOrigin: "https://ui.vadora.test", AssetOrigin: "https://assets.vadora.test",
		CheckoutOrigin: "https://checkout.vadora.test", WorkspaceOrigin: "https://app.vadora.test", ImageDomains: []string{"cdn.shopify.com"}})
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

// golden compares a value with the fixture of the same name, or rewrites the fixture.
func golden(t *testing.T, name string, value any) {
	t.Helper()
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, encoded, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the golden fixture %s is missing: run go test ./plugins/chatgpt -update (%v)", name, err)
	}
	if !bytes.Equal(want, encoded) {
		t.Fatalf("the output no longer matches the golden fixture %s:\n%s", name, encoded)
	}
}

// toolNames lists every tool that the plugin registers, in a fixed order.
func toolNames(t *testing.T) []string {
	t.Helper()
	sets, err := contract.BothToolSets()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, set := range sets {
		for _, tool := range set.Tools {
			names = append(names, tool.Name)
		}
	}
	for _, tool := range contract.ExtensionTools() {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	return names
}

// The registered metadata of open_marketplace names ui://vadora/marketplace with a global
// entrypoint, the metadata of open_cart names ui://vadora/cart with a thread entrypoint, and
// each matches its golden fixture. The same fixtures hold the metadata of every other tool,
// so a change to what the host sees is a change that a person reviews (011:102, 011:108).
func TestTheToolMetadataMatchesItsGoldenFixtures(t *testing.T) {
	adapter := testAdapter(t)
	all := map[string]any{}
	for _, name := range toolNames(t) {
		all[name] = adapter.Metadata(name)
	}
	golden(t, "tool_metadata.golden.json", all)
	golden(t, "open_marketplace.golden.json", adapter.Metadata("open_marketplace"))
	golden(t, "open_cart.golden.json", adapter.Metadata("open_cart"))
	marketplace := adapter.Metadata("open_marketplace")
	if marketplace["ui"].(map[string]any)["resourceUri"] != "ui://vadora/marketplace" {
		t.Fatalf("the global entrypoint names the marketplace resource, got %v", marketplace["ui"])
	}
	if marketplace["openai/ui"].(map[string]any)["entrypoints"].([]any)[0].(map[string]any)["type"] != "global" {
		t.Fatalf("open_marketplace is the global entrypoint, got %v", marketplace["openai/ui"])
	}
	cart := adapter.Metadata("open_cart")
	if cart["ui"].(map[string]any)["resourceUri"] != "ui://vadora/cart" {
		t.Fatalf("the thread entrypoint names the cart resource, got %v", cart["ui"])
	}
	if cart["openai/ui"].(map[string]any)["entrypoints"].([]any)[0].(map[string]any)["type"] != "thread" {
		t.Fatalf("open_cart is the thread entrypoint, got %v", cart["openai/ui"])
	}
}

// The two UI resources declare the display modes and the domains that the host enforces: inline
// first with fullscreen available, the API origin for connections, and the plugin's own asset
// domain plus the merchant image domains of the enabled connectors for images (011:140, 010:114).
func TestTheUIResourcesDeclareTheirModesAndOnlyTheirOwnDomains(t *testing.T) {
	adapter := testAdapter(t)
	resources := adapter.Resources()
	if len(resources) != 2 {
		t.Fatalf("the plugin registers the marketplace and the cart, got %d resources", len(resources))
	}
	summary := map[string]any{}
	for _, resource := range resources {
		if resource.MIMEType != "text/html;profile=mcp-app" {
			t.Fatalf("a UI resource is an MCP app page, got %q", resource.MIMEType)
		}
		if strings.Contains(resource.Text, "{{ASSET_ORIGIN}}") {
			t.Fatal("the asset origin is filled in before the page is served")
		}
		if strings.Contains(resource.Text, "<script>") {
			t.Fatal("a UI page carries no inline script")
		}
		if !strings.Contains(resource.Text, "https://assets.vadora.test/chatgpt/widget.js") {
			t.Fatalf("the page loads its one script from the plugin's own asset origin, got %s", resource.Text)
		}
		summary[resource.URI] = resource.Meta
		modes := resource.Meta["openai/ui"].(map[string]any)
		if modes["preferredDisplayMode"] != "inline" {
			t.Fatalf("the app opens inline first, got %v", modes)
		}
		if len(modes["availableDisplayModes"].([]any)) != 2 {
			t.Fatalf("inline and fullscreen are available, got %v", modes)
		}
	}
	golden(t, "resources.golden.json", summary)
}

func strings1(values any) []string {
	var out []string
	for _, value := range values.([]any) {
		out = append(out, value.(string))
	}
	return out
}

// Images load only from the declared domains and only as raster formats, and a configuration
// with a plain origin, a path, or a credential is refused (010:114, 011:180).
func TestImagesLoadOnlyFromDeclaredDomainsAndOriginsMustBeBare(t *testing.T) {
	adapter := testAdapter(t)
	allowed := []string{"https://cdn.shopify.com/s/files/1/a.png", "https://cdn.shopify.com/a.JPG", "https://assets.vadora.test/chatgpt/logo.webp"}
	for _, address := range allowed {
		if !adapter.AllowedImage(address) {
			t.Errorf("%s is a raster image on a declared domain", address)
		}
	}
	refused := []string{"https://evil.example/a.png", "http://cdn.shopify.com/a.png", "https://cdn.shopify.com:8443/a.png", "https://user@cdn.shopify.com/a.png",
		"https://cdn.shopify.com/a.svg", "https://cdn.shopify.com/a", "https://cdn.shopify.com.evil.example/a.png", "javascript:alert(1)", "data:image/png;base64,AAAA", ""}
	for _, address := range refused {
		if adapter.AllowedImage(address) {
			t.Errorf("%s was allowed", address)
		}
	}
	for _, bad := range []string{"http://api.vadora.test", "https://api.vadora.test/path", "https://user@api.vadora.test", "https://api.vadora.test?x=1", "https://api.vadora.test#x", "", "not a url"} {
		if _, err := New(Config{APIOrigin: bad, UIOrigin: "https://ui.vadora.test", AssetOrigin: "https://assets.vadora.test",
			CheckoutOrigin: "https://checkout.vadora.test", WorkspaceOrigin: "https://app.vadora.test"}); err == nil {
			t.Errorf("the origin %q was accepted", bad)
		}
	}
	resources := adapter.Resources()
	csp := resources[0].Meta["ui"].(map[string]any)["csp"].(map[string]any)
	if strings.Join(strings1(csp["connectDomains"]), ",") != "https://api.vadora.test" {
		t.Fatalf("the app connects to the API origin only, got %v", csp["connectDomains"])
	}
	if strings.Join(strings1(csp["resourceDomains"]), ",") != "https://assets.vadora.test,https://cdn.shopify.com" {
		t.Fatalf("the app loads resources from its asset origin and the declared image domains only, got %v", csp["resourceDomains"])
	}
}

// The plugin requests no filesystem capability: it registers no file entrypoint, declares only
// the settings capability, and its UI source and bundle never call the local file opening
// method or touch a host file resource (011:182, 011:186).
func TestThePluginRequestsNoFilesystemCapabilityAndNeverOpensAFile(t *testing.T) {
	adapter := testAdapter(t)
	capabilities := adapter.Capabilities()
	if len(capabilities) != 1 {
		t.Fatalf("the plugin declares the settings capability only, got %v", capabilities)
	}
	if _, settings := capabilities["openai/settings"]; !settings {
		t.Fatalf("the plugin declares the settings capability, got %v", capabilities)
	}
	for _, name := range toolNames(t) {
		encoded, err := json.Marshal(adapter.Metadata(name))
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{"file", "filesystem", "openai/files"} {
			if strings.Contains(strings.ToLower(string(encoded)), `"`+banned) {
				t.Errorf("the tool %s declares %s in its metadata: %s", name, banned, encoded)
			}
		}
	}
	for _, path := range []string{"ui/src/app.ts", "ui/src/state.ts", "assets/widget.js"} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{"files?.open", "files.open", ".files.", "files?.", "readFile", "writeFile"} {
			if strings.Contains(string(source), banned) {
				t.Errorf("%s uses %q, a host file feature", path, banned)
			}
		}
	}
}

// A deep link opens only the app relative paths that Vadora defines, and anything else opens the
// root: a path that does not begin with a slash, a fragment, a token or any other query, an
// encoded character, a host, and an unknown path (011:128, 011:132).
func TestADeepLinkOpensOnlyThePathsThatVadoraDefinesAndAnythingElseOpensTheRoot(t *testing.T) {
	open := map[string]DeepLink{
		"/":                                    {Route: "/"},
		"/stores":                              {Route: "/stores"},
		"/stores/shop_1":                       {Route: "/stores", IDs: []string{"shop_1"}},
		"/products/lst_42":                     {Route: "/products", IDs: []string{"lst_42"}},
		"/compare?ids=a1,b2,c3,d4":             {Route: "/compare", IDs: []string{"a1", "b2", "c3", "d4"}},
		"/cart":                                {Route: "/cart"},
		"/sell":                                {Route: "/sell"},
		"/sell/readiness":                      {Route: "/sell/readiness"},
		"/products/lst-9_x":                    {Route: "/products", IDs: []string{"lst-9_x"}},
		"/compare?ids=only":                    {Route: "/compare", IDs: []string{"only"}},
		"/compare?ids=a,b":                     {Route: "/compare", IDs: []string{"a", "b"}},
		"/products/" + strings.Repeat("a", 64): {Route: "/products", IDs: []string{strings.Repeat("a", 64)}},
	}
	for raw, want := range open {
		got := ParseDeepLink(raw)
		if got.Route != want.Route {
			t.Errorf("%q opens %+v, want %+v", raw, got, want)
		} else if strings.Join(got.IDs, ",") != strings.Join(want.IDs, ",") {
			t.Errorf("%q opens %+v, want %+v", raw, got, want)
		}
	}
	root := []string{"", "products/lst_1", "//evil.example/products/x", "https://evil.example/cart", "/products/lst_1#fragment", "/products/lst_1?token=abc", "/cart?x=1",
		"/stores?q=1", "/compare?ids=a,b,c,d,e", "/compare?ids=a&token=1", "/compare", "/compare?ids=", "/compare?ids=a,,b", "/compare?ids=a%2Fb", "/products/lst%2E1",
		"/products/", "/products/a/b", "/products/" + strings.Repeat("a", 65), "/admin", "/checkout", "/sell/publish", "/products/lst 1", "/products/lst\n1",
		"/" + strings.Repeat("a", 1100), "javascript:alert(1)", "\\\\evil", "/products/lst_1;x"}
	for _, raw := range root {
		got := ParseDeepLink(raw)
		if got.Route != "/" {
			t.Errorf("%q opens %+v, want the root", raw, got)
		} else if len(got.IDs) != 0 {
			t.Errorf("%q opens %+v, want the root", raw, got)
		}
	}
}

// Where the host reports no deep links, such as Android, the storefront web link replaces the
// deep link, and a link that is built never carries anything but the plugin and listing
// identifiers (011:134, 011:138).
func TestTheOpenInChatGPTLinkFallsBackToTheStorefrontWhereDeepLinksAreUnavailable(t *testing.T) {
	storefront := "https://vadora.test/products/lst_1"
	with := Capabilities{DeepLinks: true}
	link := ProductLink("vadora", "lst_1", storefront, with)
	if link != "https://chatgpt.com/plugins/vadora/app/open_marketplace?path=%2Fproducts%2Flst_1" {
		t.Fatalf("a host with deep links opens the listing in the app, got %q", link)
	}
	if got := ParseDeepLink("/products/lst_1"); got.Route != "/products" {
		t.Fatal("the deep link that the page publishes is one that the app opens")
	}
	if got := ProductLink("vadora", "lst_1", storefront, Capabilities{}); got != storefront {
		t.Fatalf("a host without deep links gets the storefront page, got %q", got)
	}
	for _, bad := range []struct{ plugin, listing string }{{"vad ora", "lst_1"}, {"vadora", "lst_1?token=1"}, {"vadora", ""}, {"", "lst_1"}, {"vadora", "../x"}} {
		if got := ProductLink(bad.plugin, bad.listing, storefront, with); got != storefront {
			t.Errorf("%+v builds %q instead of the storefront page", bad, got)
		}
	}
	android := PlatformSupport("android", Capabilities{DeepLinks: true, Mentions: true, Forms: true, Thumbnails: true, NewConversation: true, Settings: true})
	if android.DeepLinks {
		t.Fatal("Android has no deep links")
	}
	if got := ProductLink("vadora", "lst_1", storefront, android); got != storefront {
		t.Fatalf("on Android the storefront link replaces the deep link, got %q", got)
	}
}

// Each platform keeps only what it supports: deep links are absent on Android, mention search is
// desktop only, rich forms are desktop and web only, thumbnails are absent on iOS, and a new
// conversation is a desktop and web feature (011:94). An unknown platform gets nothing.
func TestThePlatformCapabilitySetDropsWhatEachPlatformLacks(t *testing.T) {
	everything := Capabilities{VisualUI: true, Entrypoints: true, DeepLinks: true, Settings: true, Mentions: true, Forms: true, Thumbnails: true, NewConversation: true, ExternalLinks: true}
	want := map[string]Capabilities{
		"desktop": everything,
		"web":     {VisualUI: true, Entrypoints: true, DeepLinks: true, Settings: true, Forms: true, Thumbnails: true, NewConversation: true, ExternalLinks: true},
		"ios":     {VisualUI: true, Entrypoints: true, DeepLinks: true, Settings: true, ExternalLinks: true},
		"android": {VisualUI: true, Entrypoints: true, Settings: true, Thumbnails: true, ExternalLinks: true},
		"watch":   {},
	}
	for platform, expected := range want {
		if got := PlatformSupport(platform, everything); got != expected {
			t.Errorf("%s: got %+v, want %+v", platform, got, expected)
		}
	}
	if got := PlatformSupport("desktop", Capabilities{}); got != (Capabilities{}) {
		t.Fatalf("a feature that the host does not advertise stays off, got %+v", got)
	}
}

// Where the host advertises forms, a selection form offers only the identifiers that the server
// offered, with a thumbnail only for an image on a declared domain, so a form option whose
// image sits elsewhere is offered without one and stays selectable. Where the host has no form
// capability, the form falls back to the UI with no partial form, and a result outside the
// offered set is refused (011:174, 011:178, 011:179, 011:180).
func TestASelectionFormOffersOnlyWhatTheServerOfferedAndFallsBackWithoutFormSupport(t *testing.T) {
	adapter := testAdapter(t)
	choices := formChoices()
	form, fallback, err := adapter.SelectionForm("listing_ids", choices, true, Capabilities{Forms: true, Thumbnails: true})
	if err != nil {
		t.Fatalf("a host with forms gets a form, got %+v %q %v", form, fallback, err)
	}
	if fallback != "" {
		t.Fatalf("a host with forms gets a form, got %+v %q %v", form, fallback, err)
	}
	if form == nil {
		t.Fatalf("a host with forms gets a form, got %+v %q %v", form, fallback, err)
	}
	field := form.Properties["listing_ids"]
	if field.Type != "array" {
		t.Fatalf("a multiple choice offers every product, got %+v", field)
	}
	if field.Items == nil {
		t.Fatalf("a multiple choice offers every product, got %+v", field)
	}
	if len(field.Items.OneOf) != 3 {
		t.Fatalf("a multiple choice offers every product, got %+v", field)
	}
	if field.Items.OneOf[0].Thumbnail == nil {
		t.Fatal("an image on a declared domain is a thumbnail")
	}
	if field.Items.OneOf[1].Thumbnail != nil {
		t.Fatalf("an image on an undeclared domain is offered without a thumbnail and stays selectable, got %+v", field.Items.OneOf[1])
	}
	if field.Items.OneOf[1].Const != "lst_2" {
		t.Fatalf("an image on an undeclared domain is offered without a thumbnail and stays selectable, got %+v", field.Items.OneOf[1])
	}
	if len(form.Required) != 1 {
		t.Fatalf("the form requires its one field and allows no other, got %+v", form)
	}
	if form.Required[0] != "listing_ids" {
		t.Fatalf("the form requires its one field and allows no other, got %+v", form)
	}
	if form.AdditionalProperties {
		t.Fatalf("the form requires its one field and allows no other, got %+v", form)
	}
	golden(t, "selection_form.golden.json", form)
}

// formChoices are the products that the server offers in the selection form tests.
func formChoices() []Choice {
	return []Choice{{ID: "lst_1", Title: "Kettle", Image: "https://cdn.shopify.com/a.png"}, {ID: "lst_2", Title: "Scarf", Image: "https://evil.example/b.png"}, {ID: "lst_3", Title: "Hat"}}
}

// A host with no form capability gets the fallback text and no partial form, and a form asks
// for a listing or a variant from a bounded list of safe identifiers only.
func TestASelectionFormFallsBackWithoutFormSupportAndRefusesWhatItMayNotAsk(t *testing.T) {
	adapter := testAdapter(t)
	choices := formChoices()
	none, text, err := adapter.SelectionForm("listing_ids", choices, true, Capabilities{})
	if err != nil {
		t.Fatalf("a host with no form capability gets the fallback text and no partial form, got %+v %q %v", none, text, err)
	}
	if none != nil {
		t.Fatalf("a host with no form capability gets the fallback text and no partial form, got %+v %q %v", none, text, err)
	}
	if text == "" {
		t.Fatalf("a host with no form capability gets the fallback text and no partial form, got %+v %q %v", none, text, err)
	}
	if _, _, err := adapter.SelectionForm("note", choices, false, Capabilities{Forms: true}); err == nil {
		t.Fatal("a form asks for a listing or a variant only")
	}
	if _, _, err := adapter.SelectionForm("variant_id", []Choice{{ID: "../x", Title: "x"}}, false, Capabilities{Forms: true}); err == nil {
		t.Fatal("an identifier outside the allowed pattern is not offered")
	}
	many := make([]Choice, 51)
	for i := range many {
		many[i] = Choice{ID: "id" + strings.Repeat("a", i%10), Title: "x"}
	}
	if _, _, err := adapter.SelectionForm("listing_ids", many, true, Capabilities{Forms: true}); err == nil {
		t.Fatal("a form offers at most 50 choices")
	}
}

// A form result outside the offered set is refused as INVALID_REQUEST.
func TestAFormResultOutsideTheOfferedSetIsRefused(t *testing.T) {
	choices := formChoices()
	for name, selected := range map[string][]string{"nothing": nil, "an unoffered id": {"lst_9"}, "a repeat": {"lst_1", "lst_1"}, "more than offered": {"lst_1", "lst_2", "lst_3", "lst_4"}} {
		failure, ok := errs.As(ValidateSelection(choices, selected))
		if !ok {
			t.Errorf("%s: the result is refused as INVALID_REQUEST, got %v", name, failure)
		} else if failure.Code != errs.InvalidRequest {
			t.Errorf("%s: the result is refused as INVALID_REQUEST, got %v", name, failure)
		}
	}
	if err := ValidateSelection(choices, []string{"lst_1", "lst_3"}); err != nil {
		t.Fatalf("an offered selection is accepted: %v", err)
	}
}
