package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/vadora/vadora/api/types/errs"
)

// SpendingPolicy is the owner's limit set for a buyer agent. The CLI enforces it
// before it signs, as a safeguard for the owner's own money. The server never relies
// on it and decides every request on its own evidence (012:92).
type SpendingPolicy struct {
	// MaxPerPurchaseUSDCents is the largest total that one purchase may reach.
	MaxPerPurchaseUSDCents int64 `json:"max_per_purchase_usd_cents"`
	// AllowedNetworks are the CAIP-2 identifiers that the owner permits.
	AllowedNetworks []string `json:"allowed_networks"`
	// AllowedAssets are the token mints that the owner permits.
	AllowedAssets []string `json:"allowed_assets"`
	// AllowedStores limits the merchants by store ID. An empty list allows any
	// merchant, and a list allows only the stores that it names.
	AllowedStores []string `json:"allowed_stores,omitempty"`
}

// policyFile names the policy file in the configuration directory.
const policyFile = "spending-policy.json"

// maxPolicyBytes bounds the policy file.
const maxPolicyBytes = 64 << 10

// maxPolicyTotalCents bounds one purchase maximum at one million US dollars.
const maxPolicyTotalCents = 100_000_000

// PolicyCheck is the part of a purchase that the policy decides on.
type PolicyCheck struct {
	TotalUSDCents int64
	Network       string
	Asset         string
	StoreID       string
}

// loadPolicy reads the owner's policy. A missing or malformed policy is an error,
// so a run without one stops before it signs instead of spending without a limit.
func loadPolicy(env *Env) (SpendingPolicy, error) {
	dir, err := configDir(env)
	if err != nil {
		return SpendingPolicy{}, err
	}
	path := filepath.Join(dir, policyFile)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return SpendingPolicy{}, errs.New(errs.InvalidRequest, "no spending policy is configured: create "+path+" before a purchase")
	}
	if err != nil {
		return SpendingPolicy{}, errs.New(errs.InvalidRequest, "the spending policy could not be read")
	}
	if len(raw) > maxPolicyBytes {
		return SpendingPolicy{}, errs.New(errs.InvalidRequest, "the spending policy could not be read")
	}
	info, err := os.Stat(path)
	if err != nil {
		return SpendingPolicy{}, errs.New(errs.InvalidRequest, "the spending policy is writable by another user: run chmod 644 on it")
	}
	if info.Mode().Perm()&0o022 != 0 {
		return SpendingPolicy{}, errs.New(errs.InvalidRequest, "the spending policy is writable by another user: run chmod 644 on it")
	}
	var policy SpendingPolicy
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&policy); err != nil {
		return SpendingPolicy{}, errs.New(errs.InvalidRequest, "the spending policy is not the policy document")
	}
	return policy, policy.validate()
}

// validate refuses an incomplete policy, because an incomplete policy never spends.
func (p SpendingPolicy) validate() error {
	if p.MaxPerPurchaseUSDCents <= 0 {
		return errs.New(errs.InvalidRequest, "the spending policy needs a maximum per purchase between one cent and one million dollars")
	}
	if p.MaxPerPurchaseUSDCents > maxPolicyTotalCents {
		return errs.New(errs.InvalidRequest, "the spending policy needs a maximum per purchase between one cent and one million dollars")
	}
	if len(p.AllowedNetworks) == 0 {
		return errs.New(errs.InvalidRequest, "the spending policy names no network")
	}
	if len(p.AllowedAssets) == 0 {
		return errs.New(errs.InvalidRequest, "the spending policy names no asset")
	}
	return nil
}

// Check decides whether a purchase is inside the policy. Every rejection names the
// limit that failed, and the CLI stops before it signs.
func (p SpendingPolicy) Check(c PolicyCheck) error {
	if err := p.validate(); err != nil {
		return err
	}
	if c.TotalUSDCents <= 0 {
		return errs.New(errs.InvalidRequest, "the quote has no total")
	}
	if c.TotalUSDCents > p.MaxPerPurchaseUSDCents {
		return errs.New(errs.InvalidRequest, "the quote total exceeds the spending policy maximum of "+usd(p.MaxPerPurchaseUSDCents))
	}
	if !slices.Contains(p.AllowedNetworks, c.Network) {
		return errs.New(errs.InvalidRequest, "the quote's network is outside the spending policy")
	}
	if !slices.Contains(p.AllowedAssets, c.Asset) {
		return errs.New(errs.InvalidRequest, "the quote's asset is outside the spending policy")
	}
	if len(p.AllowedStores) > 0 {
		if !slices.Contains(p.AllowedStores, c.StoreID) {
			return errs.New(errs.InvalidRequest, "the quote's merchant is outside the spending policy")
		}
	}
	return nil
}
