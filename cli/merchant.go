package cli

import (
	"fmt"
	"strings"

	"github.com/vadora/vadora/api/types/rest"
)

// The merchant commands are optional equivalents of the hosted workspace: connect an
// invited store, refresh its products, and read its readiness checklist (001:46, 001:535).
// They run with the account session of vadora login. None of them publishes a listing,
// takes a payout wallet, or records a refund, because the workspace alone does (001:247).

// shortText bounds a value that a store supplied when it is printed in one line.
const shortText = 120

// runStoresConnect starts the connection of an invited store and prints the platform's
// approval link. It completes nothing: the merchant opens the link in their own browser,
// and the platform returns to Vadora's own server, which verifies the callback and
// connects the store.
func runStoresConnect(env *Env, args []string) error {
	set := flagSet("stores connect", env)
	positional, err := parse(set, args)
	if err != nil {
		return err
	}
	storeURL, err := oneArgument(positional, "store URL")
	if err != nil {
		return err
	}
	api, err := newMerchantClient(env)
	if err != nil {
		return err
	}
	connection, err := api.ConnectStore(callContext(), storeURL)
	if err != nil {
		return err
	}
	return render(env, connectionText(connection), connection)
}

// connectionText tells the merchant what Vadora detected, what it asks for, and where to
// approve it. The approval link is an HTTPS address, which the SDK checked.
func connectionText(connection rest.StoreConnection) string {
	var text strings.Builder
	fmt.Fprintf(&text, "Store %s is on %s (%s).\n", printable(connection.CanonicalDomain, shortText), printable(connection.Platform, shortText),
		printable(connection.PlatformDomain, shortText))
	if len(connection.RequestedScopes) > 0 {
		fmt.Fprintf(&text, "Vadora asks for this access: %s.\n", printable(strings.Join(connection.RequestedScopes, ", "), 400))
	}
	text.WriteString("Open this link in your own browser to approve the connection:\n")
	fmt.Fprintf(&text, "%s\n", connection.AuthorizationURL)
	text.WriteString("Vadora connects the store only after the platform returns to Vadora's own server.\n")
	fmt.Fprintf(&text, "Then run: vadora stores readiness %s\n", printable(connection.StoreID, shortText))
	if connection.WorkspaceURL != "" {
		fmt.Fprintf(&text, "Your workspace: %s\n", printable(connection.WorkspaceURL, 400))
	}
	return text.String()
}

// runStoresSync reads a connected store's products again.
func runStoresSync(env *Env, args []string) error {
	set := flagSet("stores sync", env)
	positional, err := parse(set, args)
	if err != nil {
		return err
	}
	storeID, err := oneArgument(positional, "store ID")
	if err != nil {
		return err
	}
	api, err := newMerchantClient(env)
	if err != nil {
		return err
	}
	result, err := api.SyncStore(callContext(), storeID)
	if err != nil {
		return err
	}
	return render(env, syncText(result), result)
}

// syncText words what a refresh did, and says that only the workspace publishes.
func syncText(result rest.StoreSync) string {
	var text strings.Builder
	fmt.Fprintf(&text, "Refreshed store %s: %d new, %d updated, %d skipped.\n", printable(result.StoreID, shortText), result.Created, result.Updated, len(result.Skipped))
	text.WriteString("New listings start unpublished. Publish them in your Vadora workspace.\n")
	for _, skipped := range result.Skipped {
		fmt.Fprintf(&text, "Skipped %s: %s\n", printable(skipped.Title, shortText), printable(skipped.Reason, shortText))
	}
	return text.String()
}

// runStoresReadiness shows a store's readiness checklist with its evidence.
func runStoresReadiness(env *Env, args []string) error {
	set := flagSet("stores readiness", env)
	positional, err := parse(set, args)
	if err != nil {
		return err
	}
	storeID, err := oneArgument(positional, "store ID")
	if err != nil {
		return err
	}
	api, err := newMerchantClient(env)
	if err != nil {
		return err
	}
	readiness, err := api.GetReadiness(callContext(), storeID)
	if err != nil {
		return err
	}
	return render(env, readinessText(readiness), readiness)
}

// readinessText words the checklist in its fixed order, then the limitations that the
// connector declares.
func readinessText(readiness rest.StoreReadiness) string {
	var human strings.Builder
	for _, item := range readiness.Items {
		mark := "incomplete"
		if item.Complete {
			mark = "complete"
		}
		fmt.Fprintf(&human, "%-22s %-11s %s\n", printable(item.Name, shortText), mark, printable(item.Evidence, shortText))
	}
	if len(readiness.Limitations) > 0 {
		fmt.Fprintf(&human, "Limitations: %s.\n", printable(strings.Join(readiness.Limitations, ", "), 400))
	}
	return human.String()
}

// runMyStores lists the stores that the signed in account owns.
func runMyStores(env *Env) error {
	api, err := newMerchantClient(env)
	if err != nil {
		return err
	}
	page, err := api.MyStores(callContext())
	if err != nil {
		return err
	}
	var human strings.Builder
	for _, store := range page.Stores {
		fmt.Fprintf(&human, "%s  %s  %s  %s  %s  %d listings, %d published%s\n", printable(store.ID, shortText), printable(store.Name, shortText),
			printable(store.Domain, shortText), printable(store.Platform, shortText), printable(store.Status, shortText), store.Listings, store.Published,
			testLabel(store.Test))
	}
	if len(page.Stores) == 0 {
		human.WriteString("Your account owns no store yet. Run vadora stores connect with the store address that Vadora invited you to connect.\n")
	}
	return render(env, human.String(), page)
}
