package errs

import "testing"

// allCodes lists every Code constant, so a new constant without a catalog entry
// fails here as well as in the production path.
var allCodes = []Code{
	InvalidRequest,
	Unauthenticated,
	NotFound,
	RateLimited,
	QuoteLimitReached,
	StoreUnsupported,
	StoreDisconnected,
	StoreUnavailable,
	ProductUnavailable,
	ShippingUnsupported,
	ExchangeRateUnavailable,
	QuoteExpired,
	PaymentRejected,
	PaymentPending,
	PaymentMethodUnsupported,
	OrderSyncPending,
	ConfirmationRequired,
	AdapterUnavailable,
	ReviewRequired,
}

// TestCatalogCoversEveryCode proves every Code constant has exactly one catalog
// entry, so no specification can add a code with no client action, no retry
// rule, and no status.
func TestCatalogCoversEveryCode(t *testing.T) {
	for _, code := range allCodes {
		row, ok := catalog[code]
		if !ok {
			t.Fatalf("code %s has no catalog entry", code)
		}
		if row.action == "" {
			t.Fatalf("code %s has no client action", code)
		}
		if row.status < 400 {
			t.Fatalf("code %s has status %d, which is a success status", code, row.status)
		}
		if row.status > 599 {
			t.Fatalf("code %s has status %d, which is undefined", code, row.status)
		}
	}
	if len(catalog) != len(allCodes) {
		t.Fatalf("catalog holds %d entries for %d codes", len(catalog), len(allCodes))
	}
}

// TestPendingSettlementNeverAsksForARetry proves the catalog never instructs a
// payment retry while a settlement may still land.
func TestPendingSettlementNeverAsksForARetry(t *testing.T) {
	for _, code := range []Code{
		PaymentPending,
		OrderSyncPending,
		ConfirmationRequired,
		ReviewRequired,
	} {
		if New(code, "held").Retries {
			t.Fatalf("code %s must not ask for a retry", code)
		}
	}
}

// TestRetryIntervalCodesAgreeWithTheCatalog proves the set of codes that accept
// a retry interval is exactly the set the catalog marks as carrying one.
func TestRetryIntervalCodesAgreeWithTheCatalog(t *testing.T) {
	for _, code := range allCodes {
		row := catalog[code]
		if intervalCodes[code] != row.carriesInterval {
			t.Fatalf("code %s: intervalCodes says %t, catalog says %t",
				code, intervalCodes[code], row.carriesInterval)
		}
	}
}

// TestNotFoundStatusHidesExistence proves another merchant's store or order
// returns the same status a nonexistent one returns.
func TestNotFoundStatusHidesExistence(t *testing.T) {
	missing := New(NotFound, "no such store").Status()
	for _, code := range allCodes {
		if code == NotFound {
			continue
		}
		if catalog[code].status != missing {
			continue
		}
		t.Fatalf("code %s shares the NOT_FOUND status %d, so existence leaks", code, missing)
	}
}

// TestActionAndStatusComeFromTheCatalog proves a caller cannot attach an action
// or a status of its own, which is what keeps REST, MCP, and the CLI equal.
func TestActionAndStatusComeFromTheCatalog(t *testing.T) {
	err := New(QuoteExpired, "quote expired")
	if err.Action != ActionNewQuote {
		t.Fatalf("QUOTE_EXPIRED action = %q, want %q", err.Action, ActionNewQuote)
	}
	if err.Status() != catalog[QuoteExpired].status {
		t.Fatalf("status = %d, want %d", err.Status(), catalog[QuoteExpired].status)
	}
}

// TestDetailDoesNotMutateTheParent proves two errors built from one parent keep
// their own details.
func TestDetailDoesNotMutateTheParent(t *testing.T) {
	parent := New(StoreUnsupported, "store cannot be connected")
	child := parent.WithDetail("reason", "invitation_required")
	if _, ok := parent.Details["reason"]; ok {
		t.Fatal("WithDetail mutated the parent")
	}
	if child.Details["reason"] != "invitation_required" {
		t.Fatalf("child detail = %q", child.Details["reason"])
	}
}

// TestRetryAfterSetsTheInterval proves an interval code carries the seconds and
// the parent keeps none.
func TestRetryAfterSetsTheInterval(t *testing.T) {
	parent := New(RateLimited, "slow down")
	child := parent.WithRetryAfter(30)
	if parent.RetryAfterSeconds != 0 {
		t.Fatalf("parent interval = %d, want 0", parent.RetryAfterSeconds)
	}
	if child.RetryAfterSeconds != 30 {
		t.Fatalf("child interval = %d, want 30", child.RetryAfterSeconds)
	}
}

// TestPanicOutsideTheCatalog proves a code the catalog does not define is caught
// as a programming error rather than reported to a client.
func TestPanicOutsideTheCatalog(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("New accepted a code outside the catalog")
		}
	}()
	New(Code("NOT_A_CATALOG_CODE"), "no such code")
}

// TestErrorRendersCodeAndReason proves the log and CLI rendering carries both.
func TestErrorRendersCodeAndReason(t *testing.T) {
	rendered := New(StoreDisconnected, "reconnect the store").Error()
	if rendered != "STORE_DISCONNECTED: reconnect the store" {
		t.Fatalf("rendered = %q", rendered)
	}
}
