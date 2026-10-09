package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vadora/vadora/api/types/errs"
)

func wantReason(t *testing.T, err error, fragment string) {
	t.Helper()
	catalog, ok := errs.As(err)
	if !ok {
		t.Fatalf("got %v, want a catalog error mentioning %q", err, fragment)
	}
	if !strings.Contains(catalog.Reason, fragment) {
		t.Fatalf("got %q, want a reason mentioning %q", catalog.Reason, fragment)
	}
}

func goodPolicy() SpendingPolicy {
	return SpendingPolicy{MaxPerPurchaseUSDCents: 1000, AllowedNetworks: []string{"solana:devnet"}, AllowedAssets: []string{"MINT"}}
}

func goodCheck() PolicyCheck {
	return PolicyCheck{TotalUSDCents: 550, Network: "solana:devnet", Asset: "MINT", StoreID: "store-1"}
}

// The policy decides on the amount, the network, the asset, and the merchant, and
// every refusal names the limit that failed (001:544).
func TestThePolicyDecidesOnEveryLimit(t *testing.T) {
	cases := []struct {
		name   string
		policy func(*SpendingPolicy)
		check  func(*PolicyCheck)
		reason string
	}{
		{"inside every limit", func(*SpendingPolicy) {}, func(*PolicyCheck) {}, ""},
		{"the maximum itself", func(*SpendingPolicy) {}, func(c *PolicyCheck) { c.TotalUSDCents = 1000 }, ""},
		{"above the maximum", func(*SpendingPolicy) {}, func(c *PolicyCheck) { c.TotalUSDCents = 1001 }, "exceeds the spending policy maximum"},
		{"another network", func(*SpendingPolicy) {}, func(c *PolicyCheck) { c.Network = "solana:mainnet" }, "network is outside"},
		{"another asset", func(*SpendingPolicy) {}, func(c *PolicyCheck) { c.Asset = "OTHER" }, "asset is outside"},
		{"a merchant list that names the store", func(p *SpendingPolicy) { p.AllowedStores = []string{"store-1"} }, func(*PolicyCheck) {}, ""},
		{"a merchant list without the store", func(p *SpendingPolicy) { p.AllowedStores = []string{"store-2"} }, func(*PolicyCheck) {}, "merchant is outside"},
		{"a quote with no total", func(*SpendingPolicy) {}, func(c *PolicyCheck) { c.TotalUSDCents = 0 }, "no total"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			policy, check := goodPolicy(), goodCheck()
			tc.policy(&policy)
			tc.check(&check)
			err := policy.Check(check)
			if tc.reason == "" {
				if err != nil {
					t.Fatalf("the purchase is inside the policy: %v", err)
				}
				return
			}
			wantReason(t, err, tc.reason)
		})
	}
}

// An incomplete policy never spends.
func TestAnIncompletePolicyNeverSpends(t *testing.T) {
	for name, policy := range map[string]SpendingPolicy{
		"no maximum":         {AllowedNetworks: []string{"n"}, AllowedAssets: []string{"a"}},
		"a negative maximum": {MaxPerPurchaseUSDCents: -1, AllowedNetworks: []string{"n"}, AllowedAssets: []string{"a"}},
		"no network":         {MaxPerPurchaseUSDCents: 100, AllowedAssets: []string{"a"}},
		"no asset":           {MaxPerPurchaseUSDCents: 100, AllowedNetworks: []string{"n"}},
		"a maximum above one million dollars": {MaxPerPurchaseUSDCents: maxPolicyTotalCents + 1,
			AllowedNetworks: []string{"n"}, AllowedAssets: []string{"a"}},
	} {
		if err := policy.Check(goodCheck()); err == nil {
			t.Fatalf("%s: an incomplete policy allowed a purchase", name)
		}
	}
}

func policyEnv(t *testing.T, content string, mode os.FileMode) *Env {
	t.Helper()
	dir := t.TempDir()
	if content != "" {
		path := filepath.Join(dir, policyFile)
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
	return &Env{ConfigDir: dir}
}

func TestThePolicyLoaderStopsOnAMissingLooseOrUnknownPolicy(t *testing.T) {
	_, err := loadPolicy(policyEnv(t, "", 0))
	wantReason(t, err, "no spending policy is configured")

	valid := `{"max_per_purchase_usd_cents":1000,"allowed_networks":["n"],"allowed_assets":["a"]}`
	if _, err := loadPolicy(policyEnv(t, valid, 0o644)); err != nil {
		t.Fatalf("a valid policy loads: %v", err)
	}
	_, err = loadPolicy(policyEnv(t, valid, 0o666))
	wantReason(t, err, "writable by another user")

	// A field that the policy document does not define is refused, never ignored.
	loose := `{"max_per_purchase_usd_cents":1000,"allowed_networks":["n"],"allowed_assets":["a"],"spending_limit":99999}`
	_, err = loadPolicy(policyEnv(t, loose, 0o644))
	wantReason(t, err, "not the policy document")

	_, err = loadPolicy(policyEnv(t, `{"allowed_networks":["n"],"allowed_assets":["a"]}`, 0o644))
	wantReason(t, err, "maximum per purchase")
}
