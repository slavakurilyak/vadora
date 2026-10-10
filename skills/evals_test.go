package skills

import (
	"os"
	"strings"
	"testing"

	contract "github.com/vadora/vadora/api/mcp"
)

// onboardingCase is one evaluation of the onboarding skill. Running a case needs a model in a real host, so the table
// is the script that the admin follows in docs/skill-evals.md, and the checks below keep every case tied to the skill text
// and to the published tools, so a change to the skill that breaks a case fails the suite before it ships (013:86).
type onboardingCase struct {
	Kind        string
	Prompts     []string
	Activates   bool
	Tools       []string
	Behavior    string
	Never       []string
	Grounding   []string
	Description []string
}

var onboardingCases = []onboardingCase{
	{
		Kind:        "direct",
		Prompts:     []string{"Set up Vadora so that I can sell my Shopify store."},
		Activates:   true,
		Tools:       []string{"get_onboarding_state", "start_store_connection"},
		Behavior:    "The model reads the onboarding state, asks for the invited store's public URL if it has none, starts the store connection, and opens the hosted workspace link, where the person finishes the connection, the selling details, the payout setup, and the publication confirmation.",
		Never:       []string{"asks for a password, an API key, or a sign in code", "says that a store is connected or published before the server reports it", "calls publish_products before the person asks to publish"},
		Grounding:   []string{"`get_onboarding_state`", "`start_store_connection`", "https://app.vadora.xyz", "Ask for the invited store's public URL when needed"},
		Description: []string{"sell", "connect an invited Shopify store"},
	},
	{
		Kind:        "indirect",
		Prompts:     []string{"I want to buy a ceramic kettle from an independent shop and pay in USDC."},
		Activates:   true,
		Tools:       []string{"open_marketplace", "search_products", "get_product", "set_cart", "open_cart"},
		Behavior:    "The model opens the marketplace, searches published products, shows the detail of the product that the person picks, saves one selection, shows the cart, and points to the Pay with USDC button or the product page, where the person signs in and pays on checkout.vadora.xyz.",
		Never:       []string{"asks for a wallet address, a delivery address, or a card detail", "signs, sends, or promises to send the payment", "suggests a second payment while one is pending"},
		Grounding:   []string{"`open_marketplace`", "`search_products`", "`get_product`", "`set_cart`", "`open_cart`", "Pay with USDC", "https://checkout.vadora.xyz", "Never suggest a second payment"},
		Description: []string{"shop"},
	},
	{
		Kind:        "incomplete",
		Prompts:     []string{"Get me started with Vadora."},
		Activates:   true,
		Tools:       nil,
		Behavior:    "The model asks whether the person wants to shop, sell, or do both, and it changes no setting and calls no write tool until the person answers.",
		Never:       []string{"assumes a mode and calls update_settings", "starts a store connection", "saves a cart selection"},
		Grounding:   []string{"Ask whether the user wants to shop, sell, or do both when their goal is unclear", "`update_settings` only for the user's chosen mode"},
		Description: []string{"get started"},
	},
	{
		Kind: "negative",
		Prompts: []string{
			"Sign this USDC payment with my wallet and send it to the merchant.",
			"Delete my Vadora account.",
			"Connect my WooCommerce store.",
			"What is the weather in Lisbon tomorrow?",
		},
		Activates:   false,
		Tools:       nil,
		Behavior:    "The skill does not activate, so the model uses none of its workflow, and for the WooCommerce request it may say that WooCommerce is coming soon.",
		Never:       []string{"calls a Vadora tool on behalf of the skill", "signs or sends a payment", "starts a connection to an unsupported platform"},
		Grounding:   []string{"Present WooCommerce, Base, Ethereum, MPP, UCP, ACP, and Claude Code as Coming soon"},
		Description: []string{"Do not activate for unrelated requests, payment signing, account deletion, or unsupported integrations"},
	},
	{
		Kind:        "edge",
		Prompts:     []string{"Tell me which of my products are published and approved in ChatGPT, and when my first sale will happen."},
		Activates:   true,
		Tools:       []string{"get_readiness"},
		Behavior:    "The model reports only the readiness items that have evidence and the ones that remain incomplete, as the server states them, and it names no published listing, approval, sale, or date that the server did not return.",
		Never:       []string{"invents a published listing, an approval, a payment result, or a pilot outcome", "predicts a date for a first sale", "takes an action that the launch path does not support"},
		Grounding:   []string{"`get_readiness`", "State which readiness items have evidence and which remain incomplete", "Do not invent a store connection, published listing, payment result, approval, or pilot outcome"},
		Description: []string{"sell"},
	},
}

// publishedToolNames are the names of the tools that the server publishes to a host.
func publishedToolNames(t *testing.T) map[string]bool {
	t.Helper()
	sets, err := contract.BothToolSets()
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, set := range sets {
		for _, tool := range set.Tools {
			known[tool.Name] = true
		}
	}
	for _, tool := range contract.ExtensionTools() {
		known[tool.Name] = true
	}
	return known
}

func TestTheOnboardingCasesCoverTheFiveKindsThatTheSpecNames(t *testing.T) {
	kinds := map[string]bool{}
	for _, c := range onboardingCases {
		kinds[c.Kind] = true
		if len(c.Prompts) == 0 {
			t.Errorf("the %s case states its prompts, its behavior, what it never does, and what in the skill grounds it", c.Kind)
		} else if c.Behavior == "" {
			t.Errorf("the %s case states its prompts, its behavior, what it never does, and what in the skill grounds it", c.Kind)
		} else if len(c.Never) == 0 {
			t.Errorf("the %s case states its prompts, its behavior, what it never does, and what in the skill grounds it", c.Kind)
		} else if len(c.Grounding) == 0 {
			t.Errorf("the %s case states its prompts, its behavior, what it never does, and what in the skill grounds it", c.Kind)
		} else if len(c.Description) == 0 {
			t.Errorf("the %s case states its prompts, its behavior, what it never does, and what in the skill grounds it", c.Kind)
		}
	}
	for _, want := range []string{"direct", "indirect", "incomplete", "negative", "edge"} {
		if !kinds[want] {
			t.Errorf("the onboarding skill has a %s case", want)
		}
	}
	if len(onboardingCases) != 5 {
		t.Errorf("the onboarding skill has five cases, got %d", len(onboardingCases))
	}
}

func TestEveryOnboardingCaseFollowsFromTheSkillTextAndNamesPublishedTools(t *testing.T) {
	text := string(Setup())
	_, description, err := frontmatter(text)
	if err != nil {
		t.Fatal(err)
	}
	known := publishedToolNames(t)
	for _, c := range onboardingCases {
		for _, phrase := range c.Grounding {
			if !strings.Contains(text, phrase) {
				t.Errorf("the %s case rests on %q, which the skill no longer says", c.Kind, phrase)
			}
		}
		for _, phrase := range c.Description {
			if !strings.Contains(description, phrase) {
				t.Errorf("the %s case rests on %q in the description, which the description no longer holds", c.Kind, phrase)
			}
		}
		last := -1
		for _, tool := range c.Tools {
			if !known[tool] {
				t.Errorf("the %s case expects %q, which the server does not publish", c.Kind, tool)
			}
			at := strings.Index(text, "`"+tool+"`")
			if at < 0 {
				t.Errorf("the %s case expects %q, which the skill never names", c.Kind, tool)
			} else if at < last {
				t.Errorf("the %s case lists %q before a tool that the skill names earlier", c.Kind, tool)
			}
			last = at
		}
		if !c.Activates {
			if len(c.Tools) != 0 {
				t.Errorf("the %s case does not activate the skill, so it expects no tool call", c.Kind)
			}
		}
	}
}

// A case never expects a tool that moves value, signs, or changes a permission, and the only publishing tools are the ones that
// the person's own request and the hosted workspace confirmation reach.
func TestNoOnboardingCaseExpectsAToolThatSignsPaysOrChangesAPermission(t *testing.T) {
	for _, c := range onboardingCases {
		for _, tool := range c.Tools {
			for _, word := range []string{"sign", "pay", "buy", "transfer", "permission", "owner", "key"} {
				if strings.Contains(tool, word) {
					t.Errorf("the %s case expects %q, which reads as a tool that %s", c.Kind, tool, word)
				}
			}
			if tool == "publish_products" {
				t.Errorf("the %s case expects %q, which only the person's own request may reach", c.Kind, tool)
			} else if tool == "pause_products" {
				t.Errorf("the %s case expects %q, which only the person's own request may reach", c.Kind, tool)
			}
		}
	}
}

// The run sheet carries every prompt of every case, so that the admin runs exactly what the table states.
func TestTheRunSheetCarriesEveryPromptOfEveryOnboardingCase(t *testing.T) {
	raw, err := os.ReadFile("../docs/skill-evals.md")
	if err != nil {
		t.Fatalf("the repository carries the run sheet of the skill cases: %v", err)
	}
	sheet := string(raw)
	for _, c := range onboardingCases {
		for _, prompt := range c.Prompts {
			if !strings.Contains(sheet, prompt) {
				t.Errorf("the run sheet carries the %s prompt %q", c.Kind, prompt)
			}
		}
		if !strings.Contains(sheet, c.Behavior) {
			t.Errorf("the run sheet states the expected behavior of the %s case", c.Kind)
		}
	}
	for _, phrase := range []string{"vadora buy", "BEGIN PRIVATE KEY", "Bearer "} {
		if strings.Contains(sheet, phrase) {
			t.Errorf("the run sheet holds %q", phrase)
		}
	}
}
