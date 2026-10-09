package mcp

import (
	"strings"

	"github.com/vadora/vadora/api/openapi"
)

// maxPublishItems bounds how many listings one publication adds, so a publication
// confirmation stays readable in the hosted workspace.
const maxPublishItems = 50

// maxPauseItems bounds how many listings one bulk pause names.
const maxPauseItems = 50

// minReadinessItems is how many readiness items the checklist holds at minimum.
const minReadinessItems = 1

// getOnboardingStateTool reports what a merchant still owes before publishing. It
// links each missing item to the hosted workspace, because the merchant completes
// setup there and never through a tool.
func getOnboardingStateTool() Tool {
	codes := sortedCodes()
	return Tool{
		Name:            "get_onboarding_state",
		RequiresAccount: true, Scope: "merchant",
		Title:    "Get onboarding state",
		ReadOnly: true,
		Description: strings.Join([]string{
			"Returns the merchant's current onboarding state and every missing readiness item with",
			"a link to the hosted workspace where the merchant completes it. It never returns a",
			"credential, and it writes nothing. May return: " + codeList(codes),
		}, "\n"),
		InputSchema: openapi.Schema{
			Type:        SchemaObjectType,
			Description: "Which store to report on.",
			Properties: map[string]openapi.Schema{
				"store_id": identifierField("The store to report on. Defaults to the caller's only store."),
			},
		},
	}
}

// startStoreConnectionTool prepares a store connection. The merchant completes the
// authorization in the hosted workspace, so no tool ever holds a merchant's token.
func startStoreConnectionTool() Tool {
	codes := sortedCodes()
	return Tool{
		Name:            "start_store_connection",
		RequiresAccount: true, Scope: "merchant",
		Title: "Start a store connection",
		Description: strings.Join([]string{
			"Prepares a store connection and returns the hosted workspace link where the merchant",
			"completes the platform authorization. It writes no credential and returns none: the",
			"merchant approves the app in the hosted workspace, never in an agent host.",
			"May return: " + codeList(codes),
		}, "\n"),
		InputSchema: openapi.Schema{
			Type:        SchemaObjectType,
			Description: "The store to connect. Only a platform in the launch scope is accepted; a",
			Properties: map[string]openapi.Schema{
				"store_url": stringField("The invited merchant public store URL."),
			},
			Required: []string{"store_url"},
		},
	}
}

// syncProductsTool imports the store's products. It reads the store and writes Vadora's
// own listing records; it publishes nothing.
func syncProductsTool() Tool {
	codes := sortedCodes()
	return Tool{
		Name:            "sync_products",
		RequiresAccount: true, Scope: "merchant", Destructive: true,
		Title: "Sync products",
		Description: strings.Join([]string{
			"Imports the store's products into Vadora's listing records so a merchant can select",
			"what to publish. It imports nothing as an instruction: product text reaches the",
			"records as data only. It publishes nothing, and it writes to the store nothing.",
			"May return: " + codeList(codes),
		}, "\n"),
		InputSchema: openapi.Schema{
			Type:        SchemaObjectType,
			Description: "Which store to import from.",
			Properties: map[string]openapi.Schema{
				"store_id": identifierField("The store to import from."),
				"cursor":   stringField("The page cursor from a previous call."),
			},
			Required: []string{"store_id"},
		},
	}
}

// getStorePreviewTool shows a merchant the storefront a shopper would see. It is read
// only and returns exactly what a public shopper sees.
func getStorePreviewTool() Tool {
	codes := sortedCodes()
	return Tool{
		Name:            "get_store_preview",
		RequiresAccount: true, Scope: "merchant",
		Title:    "Get a store preview",
		ReadOnly: true,
		Description: strings.Join([]string{
			"Returns the storefront a shopper would see for the store, with the listings a shopper",
			"can reach. It returns exactly the public view, so a merchant cannot use it to read",
			"another store's data or a buyer's details. May return: " + codeList(codes),
		}, "\n"),
		InputSchema: openapi.Schema{
			Type:        SchemaObjectType,
			Description: "Which store to preview.",
			Properties: map[string]openapi.Schema{
				"store_id": identifierField("The store to preview."),
			},
		},
	}
}

// publishProductsTool publishes selected listings. It requires the merchant's explicit
// confirmation, so a model cannot publish on a merchant's behalf.
func publishProductsTool() Tool {
	return publicationTool("publish_products", "Request product publication", false, true,
		"Requests publication of selected listings. Additions return CONFIRMATION_REQUIRED with a hosted workspace link. A plugin cannot confirm publication.")
}

func pauseProductsTool() Tool {
	return publicationTool("pause_products", "Pause products", true, false,
		"Pauses individual listings. Pausing a whole store returns CONFIRMATION_REQUIRED with a hosted workspace link. A plugin cannot confirm a bulk pause.")
}

func publicationTool(name, title string, destructive, openWorld bool, description string) Tool {
	return Tool{
		Name: name, Title: title, RequiresAccount: true, Scope: "merchant", Destructive: destructive, OpenWorld: openWorld,
		Description: description + " May return: " + codeList(sortedCodes()),
		InputSchema: openapi.Schema{Type: "object", Properties: map[string]openapi.Schema{
			"store_id":    identifierField("The store that owns these listings."),
			"listing_ids": {Type: "array", Items: &openapi.Schema{Type: "string", MaxLength: lengthPointer(maxIdentifierLength)}, MaxItems: lengthPointer(50)},
		}, Required: []string{"store_id", "listing_ids"}},
	}
}

// getReadinessTool returns the whole readiness checklist with the evidence for each
// completed item. It is read only and never blocks on the agent surface line.
func getReadinessTool() Tool {
	codes := sortedCodes()
	return Tool{
		Name:            "get_readiness",
		RequiresAccount: true, Scope: "merchant",
		Title:    "Get readiness",
		ReadOnly: true,
		Description: strings.Join([]string{
			"Returns the merchant's readiness checklist in its defined order with the evidence for",
			"each completed item and a hosted workspace link for each missing one. The checklist",
			"holds at least " + itoa(minReadinessItems) + " item and at most " +
				itoa(MaxReadinessItems) + ". The store's agent surface line is shown on the read",
			"only line with its limitations, and it never blocks publication. May return: " +
				codeList(codes),
		}, "\n"),
		InputSchema: openapi.Schema{
			Type:        SchemaObjectType,
			Description: "Which store's checklist to return.",
			Properties: map[string]openapi.Schema{
				"store_id": identifierField("The store whose checklist to return."),
			},
		},
	}
}

// listMerchantOrdersTool returns the merchant's own orders, showing paid orders by
// default and unpaid, expired, and cancelled orders only on request.
func listMerchantOrdersTool() Tool {
	codes := sortedCodes()
	return Tool{
		Name:            "list_merchant_orders",
		RequiresAccount: true, Scope: "merchant",
		Title:    "List the merchant's orders",
		ReadOnly: true,
		Description: strings.Join([]string{
			"Returns the orders from the merchant's own stores, showing paid orders by default and",
			"unpaid, expired, and cancelled orders only when asked for another status. It never",
			"returns a buyer's delivery details, and it records no",
			"refund: a refund is recorded only in the hosted workspace. May return: " +
				codeList(codes),
		}, "\n"),
		InputSchema: openapi.Schema{
			Type:        SchemaObjectType,
			Description: "Which orders to list and for which store.",
			Properties: map[string]openapi.Schema{
				"store_id":       identifierField("Restrict the listing to one store."),
				"payment_status": stringField("The payment status to list: unpaid, pending, paid, or refunded."),
				"cursor":         stringField("The page cursor from a previous call."),
				"limit":          boundedCount("How many orders to return."),
			},
		},
	}
}

// MaxReadinessItems bounds how many readiness items a checklist may hold, so the
// hosted workspace checklist stays a page a merchant can read.
const MaxReadinessItems = 64
