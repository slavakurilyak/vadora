package chatgpt

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	contract "github.com/vadora/vadora/api/mcp"
	"net/url"
	"strings"
)

const Version = "0.1.0"
const MarketplaceURI = "ui://vadora/marketplace"
const CartURI = "ui://vadora/cart"

//go:embed assets/*
var assets embed.FS

type Config struct {
	APIOrigin       string
	UIOrigin        string
	AssetOrigin     string
	CheckoutOrigin  string
	WorkspaceOrigin string
	ImageDomains    []string
}
type Adapter struct{ config Config }

func New(config Config) (*Adapter, error) {
	for _, origin := range []string{config.APIOrigin, config.UIOrigin, config.AssetOrigin, config.CheckoutOrigin, config.WorkspaceOrigin} {
		u, err := url.Parse(origin)
		if err != nil {
			return nil, errors.New("plugin origin is invalid")
		}
		if u.Scheme != "https" {
			return nil, errors.New("plugin origin requires HTTPS")
		}
		if u.Host == "" {
			return nil, errors.New("plugin origin is missing")
		}
		if u.Path != "" {
			return nil, errors.New("plugin origin contains a path")
		}
		if u.User != nil {
			return nil, errors.New("plugin origin contains user information")
		}
		if u.RawQuery != "" {
			return nil, errors.New("plugin origin contains a query")
		}
		if u.Fragment != "" {
			return nil, errors.New("plugin origin contains a fragment")
		}
	}
	return &Adapter{config: config}, nil
}

func (a *Adapter) Metadata(name string) map[string]any {
	meta := Metadata{SecuritySchemes: []SecurityScheme{{Type: "oauth2", Scopes: []string{"shopper"}}}}
	if strings.HasPrefix(name, "get_store") {
		meta.SecuritySchemes[0].Scopes = []string{"merchant"}
	}
	for _, n := range []string{"get_readiness", "get_onboarding_state", "sync_products", "publish_products", "pause_products", "start_store_connection", "list_merchant_orders"} {
		if name == n {
			meta.SecuritySchemes[0].Scopes = []string{"merchant"}
		}
	}
	for _, n := range []string{"search_products", "list_stores", "get_product", "compare_products", "open_marketplace"} {
		if name == n {
			meta.SecuritySchemes = []SecurityScheme{{Type: "noauth"}, {Type: "oauth2", Scopes: []string{"shopper"}}}
		}
	}
	if name == "open_marketplace" {
		meta.UI = &ToolUI{ResourceURI: MarketplaceURI}
		meta.OpenAIUI = &Entrypoints{[]Entrypoint{{"global"}}}
	}
	if name == "open_cart" {
		meta.UI = &ToolUI{ResourceURI: CartURI}
		meta.OpenAIUI = &Entrypoints{[]Entrypoint{{"thread"}}}
	}
	if name == "get_product" {
		meta.UI = &ToolUI{ResourceURI: MarketplaceURI}
	}
	if name == "compare_products" {
		meta.UI = &ToolUI{ResourceURI: MarketplaceURI}
	}
	if name == "start_checkout" {
		meta.UI = &ToolUI{Visibility: []string{"app"}}
	}
	if name == "search_mentions" {
		meta.UI = &ToolUI{Visibility: []string{"app"}}
		meta.Extensions = &ToolExtensions{MentionSearch: &struct{}{}}
	}
	return object(meta)
}

func (a *Adapter) Capabilities() map[string]any {
	return map[string]any{"openai/settings": SettingsCapability{"read_settings", "update_settings"}}
}

func (a *Adapter) Resources() []contract.Resource {
	html, err := assets.ReadFile("assets/widget.html")
	if err != nil {
		panic("embedded UI is missing")
	}
	resources := make([]contract.Resource, 0, 2)
	for _, uri := range []string{MarketplaceURI, CartURI} {
		meta := ResourceMetadata{UI: ResourceUI{Domain: a.config.UIOrigin, CSP: CSP{
			ConnectDomains: []string{a.config.APIOrigin}, ResourceDomains: append([]string{a.config.AssetOrigin}, a.imageOrigins()...)}, PrefersBorder: true},
			OpenAIUI:  DisplayModes{Available: []string{"inline", "fullscreen"}, Preferred: "inline"},
			LegacyCSP: LegacyCSP{ConnectDomains: []string{a.config.APIOrigin}, ResourceDomains: append([]string{a.config.AssetOrigin}, a.imageOrigins()...), RedirectDomains: []string{a.config.CheckoutOrigin, a.config.WorkspaceOrigin}}}
		text := strings.ReplaceAll(string(html), "{{ASSET_ORIGIN}}", a.config.AssetOrigin)
		resources = append(resources, contract.Resource{URI: uri, MIMEType: "text/html;profile=mcp-app", Text: text, Meta: object(meta)})
	}
	return resources
}

func (a *Adapter) imageOrigins() []string {
	values := make([]string, 0, len(a.config.ImageDomains))
	for _, domain := range a.config.ImageDomains {
		values = append(values, "https://"+domain)
	}
	return values
}

func (a *Adapter) Icons(name string) []contract.Icon {
	if name != "open_marketplace" {
		if name != "open_cart" {
			return nil
		}
	}
	svg, err := assets.ReadFile("assets/cart.svg")
	if err != nil {
		panic("embedded icon is missing")
	}
	return []contract.Icon{{Src: "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(svg), MIMEType: "image/svg+xml", Sizes: []string{"20x20"}}}
}

// Asset exposes only embedded public browser assets.
func (a *Adapter) Asset(name string) ([]byte, error) {
	for _, allowed := range []string{"widget.html", "widget.js", "widget.css", "cart.svg"} {
		if name == allowed {
			return assets.ReadFile("assets/" + name)
		}
	}
	return nil, errors.New("unknown asset")
}

func object(value any) map[string]any {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic("plugin metadata cannot encode")
	}
	var meta map[string]any
	if err := json.Unmarshal(encoded, &meta); err != nil {
		panic("plugin metadata is not an object")
	}
	return meta
}
