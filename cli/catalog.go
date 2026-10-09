package cli

import (
	"fmt"
	"strings"

	"github.com/vadora/vadora/api/types/errs"
	"github.com/vadora/vadora/api/types/rest"
	"github.com/vadora/vadora/sdk"
)

// runProductsSearch searches the public catalog. It needs no credential, because a
// shopper reads the catalog before signing in, and it returns only what the server
// published.
func runProductsSearch(env *Env, args []string) error {
	set := flagSet("products search", env)
	store := set.String("store", "", "only this store ID")
	merchant := set.String("merchant", "", "only this merchant, by name or domain")
	category := set.String("category", "", "only this category")
	destination := set.String("destination", "", "only listings that ship to this two letter country")
	minPrice := set.String("min-price", "", "the lowest price in US dollars, such as 10.00")
	maxPrice := set.String("max-price", "", "the highest price in US dollars, such as 50.00")
	limit := set.Int("limit", 0, "how many results to show, up to 100")
	cursor := set.String("cursor", "", "the page cursor of an earlier search")
	positional, err := parse(set, args)
	if err != nil {
		return err
	}
	filter := client.Filter{StoreID: *store, Merchant: *merchant, Category: *category,
		Destination: strings.ToUpper(*destination), Cursor: *cursor, Limit: *limit}
	if *minPrice != "" {
		if filter.MinUSDCents, err = dollarsToCents(*minPrice); err != nil {
			return err
		}
	}
	if *maxPrice != "" {
		if filter.MaxUSDCents, err = dollarsToCents(*maxPrice); err != nil {
			return err
		}
	}
	api, err := newClient(env)
	if err != nil {
		return err
	}
	results, err := api.SearchProducts(callContext(), strings.Join(positional, " "), filter)
	if err != nil {
		return err
	}
	var human strings.Builder
	for _, product := range results.Products {
		fmt.Fprintf(&human, "%s  %s  %s %s  %s  %s\n", product.ID, product.Title, product.Price, product.Currency,
			product.StoreName, availability(product))
	}
	if len(results.Products) == 0 {
		human.WriteString("No listing matches.\n")
	}
	if results.Next != "" {
		fmt.Fprintf(&human, "More results: add --cursor %s\n", results.Next)
	}
	return render(env, human.String(), results)
}

func availability(product rest.Product) string {
	if product.Available {
		return "available"
	}
	return "unavailable"
}

// runProductsGet shows one published listing.
func runProductsGet(env *Env, args []string) error {
	set := flagSet("products get", env)
	positional, err := parse(set, args)
	if err != nil {
		return err
	}
	id, err := oneArgument(positional, "listing ID")
	if err != nil {
		return err
	}
	api, err := newClient(env)
	if err != nil {
		return err
	}
	product, err := api.GetProduct(callContext(), id)
	if err != nil {
		return err
	}
	return render(env, describeProduct(product), product)
}

// describeProduct renders a listing for a person. Merchant text is data: it is
// printed as text and never interpreted (001:496).
func describeProduct(product rest.Product) string {
	var human strings.Builder
	fmt.Fprintf(&human, "%s\n", product.Title)
	fmt.Fprintf(&human, "  Listing     %s\n", product.ID)
	fmt.Fprintf(&human, "  Merchant    %s (%s)\n", product.StoreName, product.StoreDomain)
	fmt.Fprintf(&human, "  Price       %s %s", product.Price, product.Currency)
	if product.PriceUSDEst != "" {
		fmt.Fprintf(&human, " (about %s USD)", product.PriceUSDEst)
	}
	fmt.Fprintf(&human, "\n  Available   %v\n", product.Available)
	fmt.Fprintf(&human, "  Pay with    %s\n", strings.Join(product.PurchaseMethods, ", "))
	if product.Description != "" {
		fmt.Fprintf(&human, "\n%s\n", product.Description)
	}
	return human.String()
}

// runStoresList reads the public merchant directory.
func runStoresList(env *Env, args []string) error {
	set := flagSet("stores list", env)
	category := set.String("category", "", "only this category")
	limit := set.Int("limit", 0, "how many stores to show, up to 100")
	cursor := set.String("cursor", "", "the page cursor of an earlier listing")
	if _, err := parse(set, args); err != nil {
		return err
	}
	api, err := newClient(env)
	if err != nil {
		return err
	}
	stores, err := api.ListStores(callContext(), *category, *cursor, *limit)
	if err != nil {
		return err
	}
	var human strings.Builder
	for _, store := range stores.Stores {
		fmt.Fprintf(&human, "%s  %s  %s  %d listings  ships to %s\n", store.ID, store.Name, store.Domain, store.Listings,
			strings.Join(store.Destinations, ", "))
	}
	if len(stores.Stores) == 0 {
		human.WriteString("No store has published a listing yet.\n")
	}
	if stores.Next != "" {
		fmt.Fprintf(&human, "More stores: add --cursor %s\n", stores.Next)
	}
	return render(env, human.String(), stores)
}

var _ = errs.New
