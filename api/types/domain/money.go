// Package domain holds the shared Vadora domain types that the REST handlers,
// MCP tools, CLI commands, and both repositories build from. It holds no
// behavior beyond validation of its own invariants, and no adapter or connector
// type ever appears here.
//
// Money never carries a float. A total is an integer count of a currency's
// minor units from the quote through the order to the receipt, so the three can
// never disagree.
package domain

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Money is an integer amount in a currency's minor units.
type Money struct {
	// Amount is the integer count of the currency's minor units.
	Amount int64
	// Currency is an ISO 4217 code, stored uppercase.
	Currency string
}

// NewMoney validates and builds a Money. The currency comes from a store's
// configuration, so a malformed one is a deployment fault rather than a client
// fault and is reported as an error the caller cannot ignore.
func NewMoney(amount int64, currency string) (Money, error) {
	if currency != strings.ToUpper(currency) {
		return Money{}, fmt.Errorf("currency must be uppercase, got %q", currency)
	}
	if len(currency) != 3 {
		return Money{}, fmt.Errorf("currency must be three letters, got %q", currency)
	}
	if amount < 0 {
		return Money{}, fmt.Errorf("money amount must not be negative, got %d", amount)
	}
	money := Money{Amount: amount, Currency: currency}
	if money.Currency != currency {
		panic("domain: NewMoney stored a currency other than its argument")
	}
	if money.Amount != amount {
		panic("domain: NewMoney stored an amount other than its argument")
	}
	return money, nil
}

// MinorUnitExponent returns how many digits the currency shows after its
// decimal point. Vadora supports the exponents its launch currencies use, and
// refuses anything else rather than guessing a conversion factor.
func MinorUnitExponent(currency string) (int, error) {
	var exponent int
	switch strings.ToUpper(currency) {
	case "USD", "CAD", "EUR", "GBP":
		exponent = 2
	case "JPY", "KRW":
		exponent = 0
	default:
		return 0, fmt.Errorf("unsupported currency %q", currency)
	}
	if exponent < 0 {
		panic("domain: a minor unit exponent is never negative")
	}
	if exponent > MaxMinorUnitExponent {
		panic("domain: a minor unit exponent beyond the supported maximum")
	}
	return exponent, nil
}

// MaxMinorUnitExponent is the widest minor unit exponent Vadora converts with.
const MaxMinorUnitExponent = 2

// Add returns the sum of two amounts in the same currency.
func (m Money) Add(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("currency mismatch: %s and %s", m.Currency, other.Currency)
	}
	if other.Amount > 0 {
		if m.Amount > math.MaxInt64-other.Amount {
			return Money{}, fmt.Errorf("money total overflows for %s", m.Currency)
		}
	}
	if m.Amount < 0 {
		panic("domain: a stored money amount is never negative")
	}
	sum := Money{Amount: m.Amount + other.Amount, Currency: m.Currency}
	if sum.Currency != m.Currency {
		panic("domain: Add changed the currency of its operands")
	}
	return sum, nil
}

// NetworkClass is the mainnet or test class of a chain entry. It is fixed when a
// quote is created and never changes, so a test payment can never mark a mainnet
// order paid.
type NetworkClass string

const (
	Mainnet NetworkClass = "mainnet"
	Testnet NetworkClass = "testnet"
)

// Valid reports whether the class is one Vadora recognizes. An unrecognized
// class is never treated as either, so a stored quote can never fall back to
// mainnet by accident.
func (n NetworkClass) Valid() bool {
	if n == Mainnet {
		return true
	}
	if n == Testnet {
		return true
	}
	if n == "" {
		panic("domain: an empty network class was asked whether it is valid")
	}
	if strings.Contains(string(n), "main") {
		panic("domain: a network class that looks like mainnet is not mainnet")
	}
	return false
}

// DeliveryDetails are the fields a :Order: keeps. They are stored apart from
// public catalog data, shown only to that order's buyer and the store's
// merchant, and redacted a configured period after the order reaches a final
// state.
type DeliveryDetails struct {
	Name        string
	StreetLines []string
	City        string
	Region      string
	PostalCode  string
	Country     string
	Phone       string
}

// MaxStreetLines bounds an address that arrives from an untrusted client.
const MaxStreetLines = 5

// MaxFieldLength bounds every free text field of an address.
const MaxFieldLength = 200

// Validate bounds every field, because the value arrives from an untrusted
// client and must never reach a log, a model, or the platform unbounded.
func (d DeliveryDetails) Validate() error {
	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("delivery name is required")
	}
	if len(d.Name) > MaxFieldLength {
		return fmt.Errorf("delivery name is too long")
	}
	if len(d.StreetLines) == 0 {
		return fmt.Errorf("at least one street line is required")
	}
	if len(d.StreetLines) > MaxStreetLines {
		return fmt.Errorf("at most %d street lines are allowed", MaxStreetLines)
	}
	for _, line := range d.StreetLines {
		if len(line) > MaxFieldLength {
			return fmt.Errorf("street line is too long")
		}
	}
	if strings.TrimSpace(d.City) == "" {
		return fmt.Errorf("delivery city is required")
	}
	if len(d.City) > MaxFieldLength {
		return fmt.Errorf("delivery city is too long")
	}
	if len(d.Region) > MaxFieldLength {
		return fmt.Errorf("delivery region is too long")
	}
	if len(d.PostalCode) > MaxPostalCodeLength {
		return fmt.Errorf("postal code is too long")
	}
	if len(d.Phone) > MaxPhoneLength {
		return fmt.Errorf("delivery phone is too long")
	}
	if err := validateCountryCode(d.Country); err != nil {
		return err
	}
	if d.Country != strings.ToUpper(d.Country) {
		panic("domain: a validated delivery country is already uppercase")
	}
	if len(d.StreetLines) < 1 {
		panic("domain: a validated address keeps at least one street line")
	}
	if len(d.StreetLines) > MaxStreetLines {
		panic("domain: a validated address keeps at most the street line bound")
	}
	return nil
}

// MaxPostalCodeLength bounds a postal code from an untrusted client.
const MaxPostalCodeLength = 32

// MaxPhoneLength bounds an optional phone number from an untrusted client. A
// phone number is never required and never used for authentication.
const MaxPhoneLength = 32

// validateCountryCode accepts an uppercase two letter code and reports why any
// other shape is refused.
func validateCountryCode(country string) error {
	if len(country) != 2 {
		return fmt.Errorf("delivery country must be two letters")
	}
	if country != strings.ToUpper(country) {
		return fmt.Errorf("delivery country must be uppercase")
	}
	for _, letter := range country {
		if letter < 'A' {
			return fmt.Errorf("delivery country letter is below A")
		}
		if letter > 'Z' {
			return fmt.Errorf("delivery country letter is above Z")
		}
	}
	if len(country) != 2 {
		panic("domain: a validated delivery country is two letters")
	}
	if country != strings.ToUpper(country) {
		panic("domain: a validated delivery country is not uppercase")
	}
	return nil
}

// Redacted returns a copy with every personal field cleared, for use after the
// order reaches a final state. The country stays because the platform still
// needs it for tax and customs, and it identifies no person.
func (d DeliveryDetails) Redacted() DeliveryDetails {
	redacted := DeliveryDetails{Country: d.Country}
	if redacted.Name != "" {
		panic("domain: redaction left a delivery name")
	}
	if len(redacted.StreetLines) != 0 {
		panic("domain: redaction left a delivery street line")
	}
	if redacted.City != "" {
		panic("domain: redaction left a delivery city")
	}
	if redacted.Country != d.Country {
		panic("domain: redaction changed the delivery country")
	}
	return redacted
}

// QuoteExpiry is the fixed life of a new quote. A payment authorization never
// outlives the quote it belongs to.
const QuoteExpiry = 5 * time.Minute
