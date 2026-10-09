package cli

import (
	"fmt"
	"strings"

	"github.com/vadora/vadora/api/types/errs"
)

// The merchant commands are optional equivalents of the hosted workspace: connect an
// invited store, refresh its products, and read its readiness checklist (001:531).

// runStoresConnect detects a store's platform from public requests and tells the
// merchant how to approve the connection. It contacts no platform for access: the
// platform's approval page opens in the person's own browser, and Vadora receives the
// credentials on its own server callback.
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
	if !strings.HasPrefix(storeURL, "https://") {
		return errs.New(errs.InvalidRequest, "the store URL must use https")
	}
	api, err := newClient(env)
	if err != nil {
		return err
	}
	detected, err := api.DetectPlatform(callContext(), storeURL)
	if err != nil {
		return err
	}
	return render(env, fmt.Sprintf("Detected platform %s. Open the link to approve the connection.\n", detected.Platform), detected)
}

// runStoresSync refreshes a connected store's product import.
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
	api, err := newClient(env)
	if err != nil {
		return err
	}
	result, err := api.SyncStore(callContext(), storeID)
	if err != nil {
		return err
	}
	return render(env, fmt.Sprintf("Synced %d products, %d imported, %d skipped.\n", result.Products, result.Imported, result.Skipped), result)
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
	api, err := newClient(env)
	if err != nil {
		return err
	}
	readiness, err := api.GetReadiness(callContext(), storeID)
	if err != nil {
		return err
	}
	var human strings.Builder
	for _, item := range readiness.Items {
		mark := "incomplete"
		if item.Complete {
			mark = "complete"
		}
		fmt.Fprintf(&human, "%-22s %-11s %s\n", item.Name, mark, item.Evidence)
	}
	return render(env, human.String(), readiness)
}
