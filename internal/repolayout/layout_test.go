// Package repolayout holds the checks of the public repository's own layout: the license, the client
// policy, the documentation, the fixtures, and the directories that the source model allows (012:38,
// 012:136).
package repolayout

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// root is the repository root, two directories above this package.
const root = "../.."

func read(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatalf("the repository carries %s: %v", name, err)
	}
	return string(raw)
}

// The repository carries the Apache 2.0 license and the client policy documents at its root, and the
// policy says that it grants no trust (012:45, 012:69, 012:136).
func TestTheRepositoryCarriesItsLicenseAndItsClientPolicy(t *testing.T) {
	license := read(t, "LICENSE")
	if !strings.Contains(license, "Apache License") {
		t.Fatal("the license is the Apache License")
	}
	if !strings.Contains(license, "Version 2.0, January 2004") {
		t.Fatal("the license is version 2.0")
	}
	if len(license) < 10000 {
		t.Fatalf("the license is the full text, got %d bytes", len(license))
	}
	policy := read(t, "CLIENT_POLICY.md")
	for _, want := range []string{"Apache License, Version 2.0", "Vadora name", "official", "unofficial", "grants no trust", "substitute for that verification"} {
		if !strings.Contains(policy, want) {
			t.Errorf("the client policy says %q", want)
		}
	}
	if !strings.Contains(read(t, "CONTRIBUTING.md"), "Apache License, Version 2.0") {
		t.Error("the contribution rules name the license of a contribution")
	}
	if !strings.Contains(read(t, "docs/README.md"), "decided on the server") {
		t.Error("the developer documentation states that the server decides every request")
	}
}

// The fixtures are labeled test stores: every one says TEST, none counts as revenue or as a live customer, and
// none holds a credential (012:46, 009:77).
func TestTheFixturesAreLabeledTestStoresThatHoldNoCredential(t *testing.T) {
	var store struct {
		Label                string `json:"label"`
		Platform             string `json:"platform"`
		CountsAsRevenue      bool   `json:"counts_as_revenue"`
		CountsAsLiveCustomer bool   `json:"counts_as_live_customer"`
	}
	if err := json.Unmarshal([]byte(read(t, "fixtures/shopify/development-store.json")), &store); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(store.Label, "TEST") {
		t.Fatalf("the store fixture is labeled TEST, got %q", store.Label)
	}
	if store.Platform != "shopify" {
		t.Fatalf("the store fixture is a Shopify test store that counts as nothing, got %+v", store)
	}
	if store.CountsAsRevenue {
		t.Fatalf("the store fixture is a Shopify test store that counts as nothing, got %+v", store)
	}
	if store.CountsAsLiveCustomer {
		t.Fatalf("the store fixture is a Shopify test store that counts as nothing, got %+v", store)
	}
	var catalog struct {
		Label    string `json:"label"`
		Products []struct {
			Title string `json:"title"`
		} `json:"products"`
	}
	if err := json.Unmarshal([]byte(read(t, "fixtures/shopify/products.json")), &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Products) == 0 {
		t.Fatal("the fixture catalog lists products")
	}
	for _, product := range catalog.Products {
		if !strings.HasPrefix(product.Title, "TEST ") {
			t.Errorf("every fixture product title starts with TEST, got %q", product.Title)
		}
	}
	secret := regexp.MustCompile(`(?i)BEGIN [A-Z ]*PRIVATE KEY|shpat_[0-9a-f]{20,}|shpss_[0-9a-f]{20,}|bearer\s+[a-z0-9._~+/=-]{16,}|\b[1-9A-HJ-NP-Za-km-z]{80,}\b|client_secret|access_token`)
	err := filepath.Walk(filepath.Join(root, "fixtures"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if found := secret.FindString(string(raw)); found != "" {
			t.Errorf("%s holds what looks like a credential: %q", path, found)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The source model allows only the directories of the layout: no server, no operator command, and no
// directory for a host that is not approved (012:38, 012:44).
func TestTheRepositoryHoldsNoServerNoOperatorCommandAndNoUnapprovedHostPackage(t *testing.T) {
	for _, forbidden := range []string{"plugins/claudecode", "cmd/serve", "cmd/operator", "internal/server", "migrations", "deploy"} {
		if _, err := os.Stat(filepath.Join(root, forbidden)); err == nil {
			t.Errorf("the public repository has %s, which belongs elsewhere or waits for an approval", forbidden)
		}
	}
	for _, required := range []string{"cmd/vadora", "cli", "sdk", "sdk/install", "api/types", "api/openapi", "api/mcp", "skills", "plugins/chatgpt", "docs", "fixtures"} {
		if info, err := os.Stat(filepath.Join(root, required)); err != nil {
			t.Errorf("the public repository has the directory %s", required)
		} else if !info.IsDir() {
			t.Errorf("the public repository has the directory %s", required)
		}
	}
}
