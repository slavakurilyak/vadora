package client

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/vadora/vadora/api/types/errs"
	"github.com/vadora/vadora/api/types/rest"
)

// The calls below are the merchant side of the REST contract: the optional equivalents
// of the hosted workspace's store connection and product refresh (001:46). Each one
// carries the access token of the CLI's account session, and none sends a price, a
// status, a payout detail, or a publication, because the workspace alone takes those.

// MaxStoreURLLength bounds a store URL the SDK sends.
const MaxStoreURLLength = 2048

// MaxReadinessItems is how many rows a readiness checklist has.
const MaxReadinessItems = 7

// requireSession refuses a merchant call that has no account session to carry.
func (c *Client) requireSession(what string) error {
	if c.Credential == "" {
		return errs.New(errs.Unauthenticated, what+" needs a signed in account: run vadora login")
	}
	if c.BaseURL == "" {
		return errs.New(errs.Unauthenticated, "the client has no API origin")
	}
	return nil
}

// checkStoreURL refuses a URL the SDK will not send: it must be an HTTPS address that
// names a host and carries no credentials. The server decides again which origins it
// may read, so this is a courtesy to the person who typed it.
func checkStoreURL(storeURL string) (string, error) {
	trimmed := strings.TrimSpace(storeURL)
	if trimmed == "" {
		return "", errs.New(errs.InvalidRequest, "the store URL is empty")
	}
	if len(trimmed) > MaxStoreURLLength {
		return "", errs.New(errs.InvalidRequest, "the store URL is too long")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", errs.New(errs.InvalidRequest, "the store URL is malformed")
	}
	if parsed.Scheme != "https" {
		return "", errs.New(errs.InvalidRequest, "the store URL must use https")
	}
	if parsed.Host == "" {
		return "", errs.New(errs.InvalidRequest, "the store URL names no host")
	}
	if parsed.User != nil {
		return "", errs.New(errs.InvalidRequest, "the store URL carries credentials")
	}
	return trimmed, nil
}

// MyStores lists the stores that the signed in account owns and no one else's.
func (c *Client) MyStores(ctx context.Context) (rest.MerchantStorePage, error) {
	if err := c.requireSession("listing your stores"); err != nil {
		return rest.MerchantStorePage{}, err
	}
	var page rest.MerchantStorePage
	if err := c.do(ctx, request{method: http.MethodGet, path: "/v1/merchant/stores"}, &page); err != nil {
		return rest.MerchantStorePage{}, err
	}
	for _, store := range page.Stores {
		if store.ID == "" {
			return rest.MerchantStorePage{}, errs.New(errs.StoreUnavailable, "the store list carries a store with no identifier")
		}
	}
	return page, nil
}

// DetectPlatform reports which platform a store URL names, from public requests that the
// server makes to the store's own origin. It contacts no platform for access.
func (c *Client) DetectPlatform(ctx context.Context, storeURL string) (rest.PlatformDetection, error) {
	checked, err := checkStoreURL(storeURL)
	if err != nil {
		return rest.PlatformDetection{}, err
	}
	if err := c.requireSession("detecting a platform"); err != nil {
		return rest.PlatformDetection{}, err
	}
	var detection rest.PlatformDetection
	if err := c.do(ctx, request{method: http.MethodPost, path: "/v1/stores/detect", body: rest.StoreURLRequest{StoreURL: checked}}, &detection); err != nil {
		return rest.PlatformDetection{}, err
	}
	if detection.Platform == "" {
		return rest.PlatformDetection{}, errs.New(errs.StoreUnavailable, "the detection answer names no platform")
	}
	return detection, nil
}

// ConnectStore starts the connection of an invited store. The answer carries the platform's
// approval link, which the merchant opens in their own browser: the SDK completes nothing,
// because the platform returns to Vadora's own server.
func (c *Client) ConnectStore(ctx context.Context, storeURL string) (rest.StoreConnection, error) {
	checked, err := checkStoreURL(storeURL)
	if err != nil {
		return rest.StoreConnection{}, err
	}
	if err := c.requireSession("connecting a store"); err != nil {
		return rest.StoreConnection{}, err
	}
	var connection rest.StoreConnection
	if err := c.do(ctx, request{method: http.MethodPost, path: "/v1/stores/connect", body: rest.StoreURLRequest{StoreURL: checked}}, &connection); err != nil {
		return rest.StoreConnection{}, err
	}
	if connection.StoreID == "" {
		return rest.StoreConnection{}, errs.New(errs.StoreUnavailable, "the connection answer names no store")
	}
	if !httpsAddress(connection.AuthorizationURL) {
		return rest.StoreConnection{}, errs.New(errs.StoreUnavailable, "the connection answer carries an approval link that is not an https address")
	}
	if connection.InstallURL != "" {
		if !httpsAddress(connection.InstallURL) {
			return rest.StoreConnection{}, errs.New(errs.StoreUnavailable, "the connection answer carries an install link that is not an https address")
		}
	}
	return connection, nil
}

// httpsAddress reports whether text is an absolute HTTPS address with a host and no
// credentials, which is the only link that the CLI prints for a person to open.
func httpsAddress(text string) bool {
	parsed, err := url.Parse(text)
	if err != nil {
		return false
	}
	if parsed.Scheme != "https" {
		return false
	}
	if parsed.Host == "" {
		return false
	}
	return parsed.User == nil
}

// SyncStore reads a connected store's products again. Every new listing starts
// unpublished, and the merchant publishes in the hosted workspace.
func (c *Client) SyncStore(ctx context.Context, storeID string) (rest.StoreSync, error) {
	if err := checkIdentifier(storeID, "store ID"); err != nil {
		return rest.StoreSync{}, err
	}
	if err := c.requireSession("refreshing a store"); err != nil {
		return rest.StoreSync{}, err
	}
	var result rest.StoreSync
	if err := c.do(ctx, request{method: http.MethodPost, path: "/v1/stores/" + pathEscape(storeID) + "/sync"}, &result); err != nil {
		return rest.StoreSync{}, err
	}
	if result.StoreID != storeID {
		return rest.StoreSync{}, errs.New(errs.StoreUnavailable, "the refresh answer names another store")
	}
	if result.Created < 0 {
		return rest.StoreSync{}, errs.New(errs.StoreUnavailable, "the refresh answer counts a negative number of listings")
	}
	if result.Updated < 0 {
		return rest.StoreSync{}, errs.New(errs.StoreUnavailable, "the refresh answer counts a negative number of listings")
	}
	return result, nil
}

// GetReadiness shows a store's readiness checklist with its evidence.
func (c *Client) GetReadiness(ctx context.Context, storeID string) (rest.StoreReadiness, error) {
	if err := checkIdentifier(storeID, "store ID"); err != nil {
		return rest.StoreReadiness{}, err
	}
	if err := c.requireSession("reading readiness"); err != nil {
		return rest.StoreReadiness{}, err
	}
	var readiness rest.StoreReadiness
	if err := c.do(ctx, request{method: http.MethodGet, path: "/v1/stores/" + pathEscape(storeID) + "/readiness"}, &readiness); err != nil {
		return rest.StoreReadiness{}, err
	}
	if len(readiness.Items) > MaxReadinessItems {
		return rest.StoreReadiness{}, errs.New(errs.StoreUnavailable, "the readiness answer lists more items than the checklist defines")
	}
	if readiness.StoreID != storeID {
		return rest.StoreReadiness{}, errs.New(errs.StoreUnavailable, "the readiness answer names another store")
	}
	for _, item := range readiness.Items {
		if item.Complete {
			if item.Evidence == "" {
				return rest.StoreReadiness{}, errs.New(errs.StoreUnavailable, "a completed readiness item carries no evidence")
			}
		}
	}
	return readiness, nil
}
