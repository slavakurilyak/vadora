package domain

import (
	"strings"
	"testing"
)

// TestMoneyRejectsAFloatShapedAmount proves an amount is validated as an
// integer minor unit count, so a total never drifts.
func TestMoneyValidatesItsCurrencyAndAmount(t *testing.T) {
	if _, err := NewMoney(100, "usd"); err == nil {
		t.Fatal("NewMoney accepted a lowercase currency")
	}
	if _, err := NewMoney(100, "US"); err == nil {
		t.Fatal("NewMoney accepted a two letter currency")
	}
	if _, err := NewMoney(-1, "USD"); err == nil {
		t.Fatal("NewMoney accepted a negative amount")
	}
	money, err := NewMoney(1999, "USD")
	if err != nil {
		t.Fatalf("NewMoney(1999, USD) failed: %v", err)
	}
	if money.Amount != 1999 {
		t.Fatalf("amount = %d, want 1999", money.Amount)
	}
}

// TestMinorUnitExponentCoversTheLaunchCurrencies proves the exponents Vadora
// converts with are fixed rather than guessed.
func TestMinorUnitExponentCoversTheLaunchCurrencies(t *testing.T) {
	for _, currency := range []string{"USD", "CAD"} {
		exponent, err := MinorUnitExponent(currency)
		if err != nil {
			t.Fatalf("MinorUnitExponent(%s) failed: %v", currency, err)
		}
		if exponent != 2 {
			t.Fatalf("MinorUnitExponent(%s) = %d, want 2", currency, exponent)
		}
	}
	if _, err := MinorUnitExponent("XYZ"); err == nil {
		t.Fatal("MinorUnitExponent accepted an unknown currency")
	}
}

// TestAddRefusesMixedCurrencies proves a total never sums two currencies.
func TestAddRefusesMixedCurrencies(t *testing.T) {
	usd, err := NewMoney(100, "USD")
	if err != nil {
		t.Fatalf("NewMoney failed: %v", err)
	}
	cad, err := NewMoney(100, "CAD")
	if err != nil {
		t.Fatalf("NewMoney failed: %v", err)
	}
	if _, err := usd.Add(cad); err == nil {
		t.Fatal("Add mixed two currencies")
	}
	total, err := usd.Add(usd)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if total.Amount != 200 {
		t.Fatalf("total = %d, want 200", total.Amount)
	}
}

// TestDeliveryDetailsValidationBoundsEveryField proves an untrusted delivery
// payload cannot be empty, unbounded, or carry a non country code.
func TestDeliveryDetailsValidationBoundsEveryField(t *testing.T) {
	valid := DeliveryDetails{
		Name:        "Ada Lovelace",
		StreetLines: []string{"1 Analytical Engine Way"},
		City:        "London",
		Region:      "LDN",
		PostalCode:  "N1 9GU",
		Country:     "GB",
		Phone:       "+442071234567",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate rejected valid details: %v", err)
	}

	cases := map[string]DeliveryDetails{
		"missing name":      {City: "London", Country: "GB", StreetLines: []string{"1 Way"}},
		"missing street":    {Name: "Ada", City: "London", Country: "GB"},
		"missing city":      {Name: "Ada", Country: "GB", StreetLines: []string{"1 Way"}},
		"bad country":       {Name: "Ada", City: "London", Country: "gbr", StreetLines: []string{"1 Way"}},
		"long name":         {Name: strings.Repeat("a", 201), City: "London", Country: "GB", StreetLines: []string{"1 Way"}},
		"many street lines": {Name: "Ada", City: "London", Country: "GB", StreetLines: make([]string, 6)},
		"long street line":  {Name: "Ada", City: "London", Country: "GB", StreetLines: []string{strings.Repeat("a", 201)}},
		"long city":         {Name: "Ada", City: strings.Repeat("a", 201), Country: "GB", StreetLines: []string{"1 Way"}},
		"long region":       {Name: "Ada", City: "London", Region: strings.Repeat("a", 201), Country: "GB", StreetLines: []string{"1 Way"}},
		"long postal code":  {Name: "Ada", City: "London", PostalCode: strings.Repeat("1", 33), Country: "GB", StreetLines: []string{"1 Way"}},
		"long phone":        {Name: "Ada", City: "London", Phone: strings.Repeat("1", 33), Country: "GB", StreetLines: []string{"1 Way"}},
	}
	for name, details := range cases {
		if err := details.Validate(); err == nil {
			t.Fatalf("Validate accepted %s", name)
		}
	}
}

// TestRedactedKeepsNoPersonalField proves redaction clears every personal value
// while keeping the country, which the platform still needs.
func TestRedactedKeepsNoPersonalField(t *testing.T) {
	details := DeliveryDetails{
		Name:        "Ada Lovelace",
		StreetLines: []string{"1 Analytical Engine Way"},
		City:        "London",
		Region:      "LDN",
		PostalCode:  "N1 9GU",
		Country:     "GB",
		Phone:       "+442071234567",
	}
	redacted := details.Redacted()
	if redacted.Name != "" {
		t.Fatalf("redacted name = %q", redacted.Name)
	}
	if len(redacted.StreetLines) != 0 {
		t.Fatalf("redacted street lines = %v", redacted.StreetLines)
	}
	for field, value := range map[string]string{
		"city":        redacted.City,
		"region":      redacted.Region,
		"postal code": redacted.PostalCode,
		"phone":       redacted.Phone,
	} {
		if value != "" {
			t.Fatalf("redacted still carries a %s: %q", field, value)
		}
	}
	if redacted.Country != "GB" {
		t.Fatalf("redacted country = %q, want GB", redacted.Country)
	}
}

// TestQuoteExpiryIsFiveMinutes proves the quote life the specification fixes is
// the one the code uses.
func TestQuoteExpiryIsFiveMinutes(t *testing.T) {
	if QuoteExpiry.Minutes() != 5 {
		t.Fatalf("QuoteExpiry = %v, want 5m", QuoteExpiry)
	}
}

// TestNetworkClassesAreDistinct proves a test quote and a mainnet quote can
// never carry the same class value.
func TestNetworkClassesAreDistinct(t *testing.T) {
	if !Mainnet.Valid() {
		t.Fatal("Mainnet is not a valid class")
	}
	if !Testnet.Valid() {
		t.Fatal("Testnet is not a valid class")
	}
	if NetworkClass("local").Valid() {
		t.Fatal("an unknown network class reported itself valid")
	}
	if string(Mainnet) != "mainnet" {
		t.Fatalf("Mainnet = %q", string(Mainnet))
	}
	if string(Testnet) != "testnet" {
		t.Fatalf("Testnet = %q", string(Testnet))
	}
}
