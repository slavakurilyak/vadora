package client

import (
	"context"
	"strings"

	"github.com/vadora/vadora/api/types/errs"
)

// MaxStoreURLLength bounds a store URL the SDK sends.
const MaxStoreURLLength = 2048

// MaxReadinessItems is how many rows a readiness checklist has.
const MaxReadinessItems = 7

// checkStoreURL refuses a URL the SDK will not send, so detection never reaches
// a non public address or a plain HTTP origin.
func checkStoreURL(storeURL string) error {
	trimmed := strings.TrimSpace(storeURL)
	if trimmed == "" {
		return errs.New(errs.InvalidRequest, "the store URL is empty")
	}
	if len(trimmed) > MaxStoreURLLength {
		return errs.New(errs.InvalidRequest, "the store URL is too long")
	}
	if !strings.HasPrefix(trimmed, "https://") {
		return errs.New(errs.InvalidRequest, "the store URL must use https")
	}
	host := strings.TrimPrefix(trimmed, "https://")
	if host == "" {
		return errs.New(errs.InvalidRequest, "the store URL names no host")
	}
	if strings.HasPrefix(host, "/") {
		return errs.New(errs.InvalidRequest, "the store URL names no host")
	}
	if strings.Contains(host, "@") {
		return errs.New(errs.InvalidRequest, "the store URL carries credentials")
	}
	if strings.HasPrefix(host, "127.0.0.1") {
		return errs.New(errs.InvalidRequest, "the store URL names a private address")
	}
	if strings.HasPrefix(host, "[::1]") {
		return errs.New(errs.InvalidRequest, "the store URL names a private address")
	}
	if strings.HasPrefix(host, "localhost") {
		return errs.New(errs.InvalidRequest, "the store URL names a loopback address")
	}
	if strings.HasPrefix(host, "10.") {
		return errs.New(errs.InvalidRequest, "the store URL names a private address")
	}
	return nil
}

// Detection is the result of asking the server which platform a store URL names.
// The server answers from public, unauthenticated requests to the store's own
// origin, and never contacts a platform for access.
type Detection struct {
	// Platform is shopify, unsupported, unreachable, or store_closed.
	Platform string `json:"platform"`
	// PlatformDomain is the myshopify domain when the platform is Shopify.
	PlatformDomain string `json:"platform_domain,omitempty"`
	// Message explains the outcome to the merchant.
	Message string `json:"message"`
}

// Store is one store this account administers.
type Store struct {
	ID              string `json:"id"`
	DisplayName     string `json:"display_name"`
	CanonicalDomain string `json:"canonical_domain"`
	Currency        string `json:"currency"`
	ConnectorStatus string `json:"connector_status"`
}

// StoreList is a page of stores.
type StoreList struct {
	Stores []Store `json:"stores"`
	Next   string  `json:"next,omitempty"`
}

// SyncResult reports what a product refresh imported and skipped.
type SyncResult struct {
	StoreID  string `json:"store_id"`
	Products int    `json:"products"`
	Imported int    `json:"imported"`
	Skipped  int    `json:"skipped"`
}

// ReadinessItem is one row of a store's readiness checklist.
type ReadinessItem struct {
	Name     string `json:"name"`
	Complete bool   `json:"complete"`
	Evidence string `json:"evidence"`
}

// Readiness is a store's whole checklist.
type Readiness struct {
	StoreID string          `json:"store_id"`
	Items   []ReadinessItem `json:"items"`
}

// DetectPlatform reports which platform a store URL names, without contacting
// that platform for access.
func (c *Client) DetectPlatform(ctx context.Context, storeURL string) (Detection, error) {
	if err := checkStoreURL(storeURL); err != nil {
		return Detection{}, err
	}
	if c.Credential != "" {
		if c.BaseURL == "" {
			return Detection{}, errs.New(errs.Unauthenticated, "the client has no API origin")
		}
		if len(c.Credential) > MaxCredentialLength {
			return Detection{}, errs.New(errs.Unauthenticated, "the stored credential is too long")
		}
		if len(c.Credential) == 0 {
			return Detection{}, errs.New(errs.Unauthenticated, "the stored credential is empty")
		}
	}
	var detection Detection
	err := c.do(ctx, request{
		method: "POST",
		path:   "/v1/stores/detect",
		body:   map[string]string{"store_url": storeURL},
	}, &detection)
	if err != nil {
		return Detection{}, err
	}
	return detection, nil
}

// SyncStore refreshes a connected store's product import.
func (c *Client) SyncStore(ctx context.Context, storeID string) (SyncResult, error) {
	if err := checkIdentifier(storeID, "store ID"); err != nil {
		return SyncResult{}, err
	}
	if c.Credential == "" {
		return SyncResult{}, errs.New(errs.Unauthenticated, "refreshing a store needs a merchant session")
	}
	if c.BaseURL == "" {
		return SyncResult{}, errs.New(errs.Unauthenticated, "the client has no API origin")
	}
	var result SyncResult
	err := c.do(ctx, request{
		method:         "POST",
		path:           "/v1/stores/" + pathEscape(storeID) + "/sync",
		idempotencyKey: newKey(),
	}, &result)
	if err != nil {
		return SyncResult{}, err
	}
	if result.Imported < 0 {
		return SyncResult{}, errs.New(errs.StoreUnavailable, "the sync result counts do not add up")
	}
	if result.Skipped < 0 {
		return SyncResult{}, errs.New(errs.StoreUnavailable, "the sync result counts do not add up")
	}
	if result.Imported+result.Skipped > result.Products {
		return SyncResult{}, errs.New(errs.StoreUnavailable, "the sync result counts do not add up")
	}
	return result, nil
}

// GetReadiness shows a store's readiness checklist with its evidence.
func (c *Client) GetReadiness(ctx context.Context, storeID string) (Readiness, error) {
	if err := checkIdentifier(storeID, "store ID"); err != nil {
		return Readiness{}, err
	}
	if c.Credential == "" {
		return Readiness{}, errs.New(errs.Unauthenticated, "reading readiness needs a merchant session")
	}
	var readiness Readiness
	err := c.do(ctx, request{
		method: "GET",
		path:   "/v1/stores/" + pathEscape(storeID) + "/readiness",
	}, &readiness)
	if err != nil {
		return Readiness{}, err
	}
	if len(readiness.Items) > MaxReadinessItems {
		return Readiness{}, errs.New(errs.StoreUnavailable, "the readiness answer lists more items than the checklist defines")
	}
	if readiness.StoreID != storeID {
		return Readiness{}, errs.New(errs.StoreUnavailable, "the readiness answer names another store")
	}
	for _, item := range readiness.Items {
		if item.Complete {
			if item.Evidence == "" {
				return Readiness{}, errs.New(errs.StoreUnavailable, "a completed readiness item carries no evidence")
			}
		}
	}
	return readiness, nil
}
