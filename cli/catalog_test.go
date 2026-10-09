package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vadora/vadora/api/types/rest"
)

// testCatalog serves one labeled test product and one labeled test store, and one live
// product and store, so the commands can show the difference.
func testCatalog(t *testing.T) *httptest.Server {
	t.Helper()
	products := map[string]rest.Product{
		"test-1": {ID: "test-1", Title: "TEST kettle", StoreName: "Dev store", StoreDomain: "dev.example", Price: "5.00",
			Currency: "USD", Available: true, PurchaseMethods: []string{"x402_usdc_solana"}, Test: true},
		"live-1": {ID: "live-1", Title: "Wool scarf", StoreName: "Live store", StoreDomain: "live.example", Price: "25.00",
			Currency: "USD", Available: true, PurchaseMethods: []string{"merchant_checkout"}},
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/products":
			_ = json.NewEncoder(w).Encode(rest.ProductPage{Products: []rest.Product{products["test-1"], products["live-1"]}})
		case strings.HasPrefix(r.URL.Path, "/v1/products/"):
			_ = json.NewEncoder(w).Encode(products[strings.TrimPrefix(r.URL.Path, "/v1/products/")])
		case r.URL.Path == "/v1/stores":
			_ = json.NewEncoder(w).Encode(rest.StorePage{Stores: []rest.Store{
				{ID: "s-dev", Name: "Dev store", Domain: "dev.example", Destinations: []string{"US"}, Listings: 4, Test: true},
				{ID: "s-live", Name: "Live store", Domain: "live.example", Destinations: []string{"US"}, Listings: 2},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestTheCatalogCommandsLabelTestListingsAndStores(t *testing.T) {
	server := testCatalog(t)
	defer server.Close()
	out, errOut, code := runMain(t, "products", "search", "kettle", "--api", server.URL)
	if code != 0 {
		t.Fatalf("search failed with %d: %s", code, errOut)
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.Contains(line, "TEST kettle") {
			if !strings.HasSuffix(line, "[TEST]") {
				t.Fatalf("a test listing carries the test label: %q", line)
			}
		}
		if strings.Contains(line, "Wool scarf") {
			if strings.Contains(line, "[TEST]") {
				t.Fatalf("a live listing never carries the test label: %q", line)
			}
		}
	}
	out, errOut, code = runMain(t, "products", "get", "test-1", "--api", server.URL)
	if code != 0 {
		t.Fatalf("get failed with %d: %s", code, errOut)
	}
	if !strings.HasPrefix(out, "TEST kettle  [TEST]\n") {
		t.Fatalf("a test listing carries the test label in its heading: %q", out)
	}
	out, errOut, code = runMain(t, "stores", "list", "--api", server.URL)
	if code != 0 {
		t.Fatalf("list failed with %d: %s", code, errOut)
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.Contains(line, "s-dev") {
			if !strings.HasSuffix(line, "[TEST]") {
				t.Fatalf("a test store carries the test label: %q", line)
			}
		}
		if strings.Contains(line, "s-live") {
			if strings.Contains(line, "[TEST]") {
				t.Fatalf("a live store never carries the test label: %q", line)
			}
		}
	}
}
