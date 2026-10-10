package chatgpt

import (
	"net/url"
	"regexp"
	"strings"
)

var identifier = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// DeepLink is navigation only. Unknown input falls back to the root.
type DeepLink struct {
	Route string
	IDs   []string
}

func ParseDeepLink(raw string) DeepLink {
	root := DeepLink{Route: "/"}
	if len(raw) > 1024 {
		return root
	}
	if !strings.HasPrefix(raw, "/") {
		return root
	}
	if strings.HasPrefix(raw, "//") {
		return root
	}
	u, err := url.Parse(raw)
	if err != nil {
		return root
	}
	if u.Fragment != "" {
		return root
	}
	if u.Host != "" {
		return root
	}
	if strings.Contains(u.Path, "%") {
		return root
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return root
	}
	if u.Path == "/compare" {
		if len(q) != 1 {
			return root
		}
		if len(q["ids"]) != 1 {
			return root
		}
		ids := strings.Split(q.Get("ids"), ",")
		if len(ids) > 4 {
			return root
		}
		for _, id := range ids {
			if !identifier.MatchString(id) {
				return root
			}
		}
		return DeepLink{Route: "/compare", IDs: ids}
	}
	if len(q) > 0 {
		return root
	}
	for _, route := range []string{"/", "/stores", "/cart", "/sell", "/sell/readiness"} {
		if u.Path == route {
			return DeepLink{Route: route}
		}
	}
	for _, prefix := range []string{"/products/", "/stores/"} {
		if strings.HasPrefix(u.Path, prefix) {
			id := strings.TrimPrefix(u.Path, prefix)
			if identifier.MatchString(id) {
				return DeepLink{Route: strings.TrimSuffix(prefix, "/"), IDs: []string{id}}
			}
		}
	}
	return root
}

// ProductLink falls back to the storefront when the host lacks deep links.
func ProductLink(pluginID, listingID, storefrontURL string, capabilities Capabilities) string {
	if !capabilities.DeepLinks {
		return storefrontURL
	}
	if !identifier.MatchString(pluginID) {
		return storefrontURL
	}
	if !identifier.MatchString(listingID) {
		return storefrontURL
	}
	return "https://chatgpt.com/plugins/" + url.PathEscape(pluginID) + "/app/open_marketplace?path=" + url.QueryEscape("/products/"+listingID)
}

// StoreLink is the "Open in ChatGPT" web link of a store. It opens the store in the app where the host has deep links, and it
// falls back to the storefront page where the host has none, so a link that a merchant publishes always leads somewhere
// (011:134). The link carries the plugin and store identifiers and nothing else.
func StoreLink(pluginID, storeID, storefrontURL string, capabilities Capabilities) string {
	if !capabilities.DeepLinks {
		return storefrontURL
	}
	if !identifier.MatchString(pluginID) {
		return storefrontURL
	}
	if !identifier.MatchString(storeID) {
		return storefrontURL
	}
	return "https://chatgpt.com/plugins/" + url.PathEscape(pluginID) + "/app/open_marketplace?path=" + url.QueryEscape("/stores/"+storeID)
}
