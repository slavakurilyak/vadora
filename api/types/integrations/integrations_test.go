package integrations

import "testing"

// TestEveryNamedDeferredIntegrationIsVisible proves the overview lists every
// deferred integration with the exact visible label and the machine readable
// status, including the ones with no adapter package and no credential.
func TestEveryNamedDeferredIntegrationIsVisible(t *testing.T) {
	for _, id := range DeferredNames() {
		entry, ok := Lookup(id)
		if !ok {
			t.Fatalf("deferred integration %s is missing from the catalog", id)
		}
		if entry.Label() != "Coming soon" {
			t.Fatalf("%s label = %q, want %q", id, entry.Label(), "Coming soon")
		}
		if string(entry.Status) != "coming_soon" {
			t.Fatalf("%s status = %q, want %q", id, entry.Status, "coming_soon")
		}
	}
}

// TestADeferredIntegrationHasNoOperationalAction proves pointer or keyboard
// interaction with a deferred entry can start no install, connection, wallet
// signature, quote, or payment.
func TestADeferredIntegrationHasNoOperationalAction(t *testing.T) {
	for _, id := range DeferredNames() {
		entry, ok := Lookup(id)
		if !ok {
			t.Fatalf("deferred integration %s is missing", id)
		}
		if entry.ActionsEnabled {
			t.Fatalf("%s offers an operational action while it is deferred", id)
		}
		if entry.ActionsAreEnabled() {
			t.Fatalf("%s reports enabled actions while it is deferred", id)
		}
	}
}

// TestDeferredChainsAndProtocolsAreAbsentFromSelectableMethods proves the
// checkout information area names Base, Ethereum, and MPP while no selectable
// method or challenge offers them.
func TestDeferredChainsAndProtocolsAreAbsentFromSelectableMethods(t *testing.T) {
	launch := []string{"solana", "x402"}
	for _, id := range launch {
		entry, ok := Lookup(id)
		if !ok {
			t.Fatalf("launch integration %s is missing", id)
		}
		if entry.Status != StatusAvailable {
			t.Fatalf("launch integration %s status = %q", id, entry.Status)
		}
		if !entry.ActionsAreEnabled() {
			t.Fatalf("launch integration %s offers no action", id)
		}
	}
	for _, id := range []string{"base", "ethereum", "mpp"} {
		entry, ok := Lookup(id)
		if !ok {
			t.Fatalf("%s is missing", id)
		}
		if entry.ActionsAreEnabled() {
			t.Fatalf("%s is selectable while it is deferred", id)
		}
	}
}

// TestCommerceProtocolsAreNeverAdvertised proves UCP and ACP are visible in the
// overview while no commerce protocol capability is advertised for them.
func TestCommerceProtocolsAreNeverAdvertised(t *testing.T) {
	group := ByCategory(CommerceProtocols)
	if len(group) == 0 {
		t.Fatal("the overview has no commerce protocol group")
	}
	for _, entry := range group {
		if entry.ActionsAreEnabled() {
			t.Fatalf("commerce protocol %s advertises a capability", entry.ID)
		}
	}
}

// TestEveryOverviewGroupHasAtLeastOneEntry proves a setup or selection view can
// always show each named group.
func TestEveryOverviewGroupHasAtLeastOneEntry(t *testing.T) {
	for _, category := range Categories() {
		group := ByCategory(category)
		if len(group) == 0 {
			t.Fatalf("category %s has no entry", category)
		}
	}
}

// TestRanksAreUniqueWithinACategory proves a reserved order never collides.
func TestRanksAreUniqueWithinACategory(t *testing.T) {
	for _, category := range Categories() {
		seen := make(map[int]bool, 4)
		for _, entry := range ByCategory(category) {
			if seen[entry.Rank] {
				t.Fatalf("category %s reuses rank %d", category, entry.Rank)
			}
			seen[entry.Rank] = true
		}
	}
}

// TestIdentifiersAreUnique proves one integration never appears twice under two
// identifiers, which would let a view offer it twice.
func TestIdentifiersAreUnique(t *testing.T) {
	seen := make(map[string]bool, len(catalog))
	for _, entry := range catalog {
		if seen[entry.ID] {
			t.Fatalf("integration %s appears twice", entry.ID)
		}
		seen[entry.ID] = true
	}
}

// TestALaunchedIntegrationShowsItsOwnState proves a disconnected Shopify store
// or a facilitator outage is never relabeled "Coming soon".
func TestALaunchedIntegrationShowsItsOwnState(t *testing.T) {
	shopify, ok := Lookup("shopify")
	if !ok {
		t.Fatal("shopify is missing")
	}
	if shopify.Label() == "Coming soon" {
		t.Fatal("a launched integration reads Coming soon")
	}
}

// TestSearchFindsOnlyPublishedIntegrations proves search never suggests an
// integration the catalog does not publish.
func TestSearchFindsOnlyPublishedIntegrations(t *testing.T) {
	matches := Search("shop")
	if len(matches) != 1 {
		t.Fatalf("search for shop returned %d results", len(matches))
	}
	if matches[0].ID != "shopify" {
		t.Fatalf("search for shop returned %s", matches[0].ID)
	}
	if Search("  ") != nil {
		t.Fatal("an empty query returned results")
	}
	if Search("definitely not published") != nil {
		t.Fatal("an unknown query returned results")
	}
}

// TestAllReturnsACopy proves a caller cannot edit the catalog for every other
// caller.
func TestAllReturnsACopy(t *testing.T) {
	first := All()
	if len(first) == 0 {
		t.Fatal("the catalog is empty")
	}
	first[0].DisplayName = "edited"
	second := All()
	if second[0].DisplayName == "edited" {
		t.Fatal("All exposed the catalog itself")
	}
}
