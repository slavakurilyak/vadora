package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"

	"github.com/vadora/vadora/api/types/errs"
)

// purchaseRecord is the durable memory of one purchase: the idempotency key and the
// signed payment, written before the payment is sent. A retry after a crash or a
// timeout sends the same key and the same signed payload, so it asks the server
// for the existing outcome and never authorizes a second payment (001:543).
type purchaseRecord struct {
	QuoteID   string `json:"quote_id"`
	Key       string `json:"idempotency_key"`
	PaymentID string `json:"payment_id"`
	// Signature is the PAYMENT-SIGNATURE header that the buyer signed.
	Signature string `json:"payment_signature"`
	OrderID   string `json:"order_id,omitempty"`
}

var safeName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// recordPath returns the file of one quote's record.
func recordPath(env *Env, quoteID string) (string, error) {
	if !safeName.MatchString(quoteID) {
		return "", errs.New(errs.InvalidRequest, "the quote ID is malformed")
	}
	dir, err := configDir(env)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "purchases", quoteID+".json"), nil
}

// loadRecord reads the record of a quote, and reports whether one exists.
func loadRecord(env *Env, quoteID string) (purchaseRecord, bool, error) {
	path, err := recordPath(env, quoteID)
	if err != nil {
		return purchaseRecord{}, false, err
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return purchaseRecord{}, false, nil
	}
	if err != nil {
		return purchaseRecord{}, false, errs.New(errs.InvalidRequest, "the purchase record could not be read")
	}
	var record purchaseRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return purchaseRecord{}, false, errs.New(errs.InvalidRequest, "the purchase record is damaged")
	} else if record.QuoteID != quoteID {
		return purchaseRecord{}, false, errs.New(errs.InvalidRequest, "the purchase record is damaged")
	}
	return record, true, nil
}

// saveRecord writes the record of a quote.
func saveRecord(env *Env, record purchaseRecord) error {
	path, err := recordPath(env, record.QuoteID)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if err := writePrivate(path, raw); err != nil {
		return errs.New(errs.InvalidRequest, "the purchase record could not be written: nothing was sent")
	}
	return nil
}

// findRecordByOrder finds the record of the quote that created an order.
func findRecordByOrder(env *Env, orderID string) (purchaseRecord, bool) {
	dir, err := configDir(env)
	if err != nil {
		return purchaseRecord{}, false
	}
	entries, err := os.ReadDir(filepath.Join(dir, "purchases"))
	if err != nil {
		return purchaseRecord{}, false
	}
	for _, entry := range entries {
		raw, err := os.ReadFile(filepath.Join(dir, "purchases", entry.Name()))
		if err != nil {
			continue
		}
		var record purchaseRecord
		if json.Unmarshal(raw, &record) == nil {
			if record.OrderID == orderID {
				if record.Signature != "" {
					return record, true
				}
			}
		}
	}
	return purchaseRecord{}, false
}
