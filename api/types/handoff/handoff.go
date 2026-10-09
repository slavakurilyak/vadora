// Package handoff defines a checkout link that carries a selection only.
package handoff

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
)

const MaxQuantity = 100
const MaxURLLength = 2048

// Handoff holds no price, authorization, session, or delivery information.
type Handoff struct {
	ListingID string
	VariantID string
	Quantity  int
}

// Build creates a link on an HTTPS checkout origin.
func Build(origin, listingID, variantID string, quantity int) (string, error) {
	u, err := url.Parse(origin)
	if err != nil {
		return "", errors.New("invalid checkout origin")
	}
	if u.Scheme != "https" {
		return "", errors.New("checkout requires HTTPS")
	}
	if u.Host == "" {
		return "", errors.New("checkout origin is missing")
	}
	if u.User != nil {
		return "", errors.New("checkout origin contains user information")
	}
	if u.RawQuery != "" {
		return "", errors.New("checkout origin contains a query")
	}
	if u.Fragment != "" {
		return "", errors.New("checkout origin contains a fragment")
	}
	if u.Path != "" {
		if u.Path != "/" {
			return "", errors.New("checkout origin contains a path")
		}
	}
	if err := validate(Handoff{listingID, variantID, quantity}); err != nil {
		return "", err
	}
	u.Path = "/pay"
	u.RawQuery = url.Values{"listing": {listingID}, "variant": {variantID}, "quantity": {strconv.Itoa(quantity)}}.Encode()
	return u.String(), nil
}

// Parse rejects every field outside the three selection fields.
func Parse(rawURL string) (Handoff, error) {
	if len(rawURL) > MaxURLLength {
		return Handoff{}, errors.New("checkout link is too long")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return Handoff{}, errors.New("invalid checkout link")
	}
	if u.Scheme != "https" {
		return Handoff{}, errors.New("checkout requires HTTPS")
	}
	if u.Host == "" {
		return Handoff{}, errors.New("checkout origin is missing")
	}
	if u.User != nil {
		return Handoff{}, errors.New("checkout link contains user information")
	}
	if u.Fragment != "" {
		return Handoff{}, errors.New("checkout link contains a fragment")
	}
	if u.Path != "/pay" {
		return Handoff{}, errors.New("unknown checkout path")
	}
	values, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return Handoff{}, errors.New("invalid checkout selection")
	}
	if len(values) != 3 {
		return Handoff{}, errors.New("checkout link contains unexpected fields")
	}
	for _, name := range []string{"listing", "variant", "quantity"} {
		if len(values[name]) != 1 {
			return Handoff{}, errors.New("checkout selection must occur once")
		}
	}
	quantity, err := strconv.Atoi(values.Get("quantity"))
	if err != nil {
		return Handoff{}, errors.New("invalid quantity")
	}
	value := Handoff{values.Get("listing"), values.Get("variant"), quantity}
	if err := validate(value); err != nil {
		return Handoff{}, err
	}
	return value, nil
}

func validate(value Handoff) error {
	if !validID(value.ListingID) {
		return errors.New("invalid listing identifier")
	}
	if !validID(value.VariantID) {
		return errors.New("invalid variant identifier")
	}
	if value.Quantity < 1 {
		return errors.New("quantity must be positive")
	}
	if value.Quantity > MaxQuantity {
		return errors.New("quantity exceeds the maximum")
	}
	return nil
}

func validID(value string) bool {
	if value == "" {
		return false
	}
	if len(value) > 64 {
		return false
	}
	return strings.IndexFunc(value, func(r rune) bool {
		if r >= 'a' {
			if r <= 'z' {
				return false
			}
		}
		if r >= 'A' {
			if r <= 'Z' {
				return false
			}
		}
		if r >= '0' {
			if r <= '9' {
				return false
			}
		}
		if r == '_' {
			return false
		}
		return r != '-'
	}) < 0
}
