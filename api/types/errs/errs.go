// Package errs holds Vadora's one shared error type and the :ErrorCatalog: it
// is built from. Every interface, REST, MCP, and CLI, returns the same code,
// reason, and client action for the same failure, so a specification adds no
// code outside the catalog.
package errs

import (
	"errors"
	"fmt"
	"net/http"
)

// Code is one entry of the :ErrorCatalog:.
type Code string

const (
	InvalidRequest           Code = "INVALID_REQUEST"
	Unauthenticated          Code = "UNAUTHENTICATED"
	NotFound                 Code = "NOT_FOUND"
	RateLimited              Code = "RATE_LIMITED"
	QuoteLimitReached        Code = "QUOTE_LIMIT_REACHED"
	StoreUnsupported         Code = "STORE_UNSUPPORTED"
	StoreDisconnected        Code = "STORE_DISCONNECTED"
	StoreUnavailable         Code = "STORE_UNAVAILABLE"
	ProductUnavailable       Code = "PRODUCT_UNAVAILABLE"
	ShippingUnsupported      Code = "SHIPPING_UNSUPPORTED"
	ExchangeRateUnavailable  Code = "EXCHANGE_RATE_UNAVAILABLE"
	QuoteExpired             Code = "QUOTE_EXPIRED"
	PaymentRejected          Code = "PAYMENT_REJECTED"
	PaymentPending           Code = "PAYMENT_PENDING"
	PaymentMethodUnsupported Code = "PAYMENT_METHOD_UNSUPPORTED"
	OrderSyncPending         Code = "ORDER_SYNC_PENDING"
	ConfirmationRequired     Code = "CONFIRMATION_REQUIRED"
	AdapterUnavailable       Code = "ADAPTER_UNAVAILABLE"
	ReviewRequired           Code = "REVIEW_REQUIRED"
)

// ClientAction is the single action the catalog asks a client to take.
type ClientAction string

const (
	ActionCorrectInput       ClientAction = "correct_input"
	ActionAuthenticate       ClientAction = "authenticate"
	ActionWait               ClientAction = "wait"
	ActionNewQuote           ClientAction = "request_new_quote"
	ActionChooseOtherMethod  ClientAction = "choose_another_method"
	ActionConfirmInWorkspace ClientAction = "confirm_in_workspace"
	ActionContactMerchant    ClientAction = "contact_merchant"
)

// entry is one row of the :ErrorCatalog:.
type entry struct {
	// action is the one client action the catalog asks for.
	action ClientAction
	// retries records whether the catalog asks for a retry, and under what
	// condition. A pending settlement never asks for one.
	retries bool
	// carriesInterval marks the codes whose retry waits on an interval the
	// error itself returns.
	carriesInterval bool
	// status is the HTTP status the REST layer returns, chosen so that another
	// merchant's store or order looks exactly like a nonexistent one.
	status int
}

// catalog is the whole :ErrorCatalog:. Every Code constant has exactly one
// entry, and a code with no entry is a programming error rather than a client
// error, so New refuses it.
var catalog = map[Code]entry{
	InvalidRequest:           {ActionCorrectInput, true, false, http.StatusBadRequest},
	Unauthenticated:          {ActionAuthenticate, true, false, http.StatusUnauthorized},
	NotFound:                 {ActionCorrectInput, true, false, http.StatusNotFound},
	RateLimited:              {ActionWait, true, true, http.StatusTooManyRequests},
	QuoteLimitReached:        {ActionWait, true, true, http.StatusTooManyRequests},
	StoreUnsupported:         {ActionChooseOtherMethod, true, false, http.StatusUnprocessableEntity},
	StoreDisconnected:        {ActionContactMerchant, true, false, http.StatusUnprocessableEntity},
	StoreUnavailable:         {ActionWait, true, true, http.StatusServiceUnavailable},
	ProductUnavailable:       {ActionCorrectInput, true, false, http.StatusBadRequest},
	ShippingUnsupported:      {ActionCorrectInput, true, false, http.StatusBadRequest},
	ExchangeRateUnavailable:  {ActionWait, true, true, http.StatusServiceUnavailable},
	QuoteExpired:             {ActionNewQuote, true, false, http.StatusGone},
	PaymentRejected:          {ActionNewQuote, true, false, http.StatusConflict},
	PaymentPending:           {ActionWait, false, false, http.StatusTooManyRequests},
	PaymentMethodUnsupported: {ActionChooseOtherMethod, true, false, http.StatusUnprocessableEntity},
	OrderSyncPending:         {ActionWait, false, false, http.StatusTooManyRequests},
	ConfirmationRequired:     {ActionConfirmInWorkspace, false, false, http.StatusPreconditionRequired},
	AdapterUnavailable:       {ActionChooseOtherMethod, true, false, http.StatusUnprocessableEntity},
	ReviewRequired:           {ActionWait, false, false, http.StatusTooManyRequests},
}

// intervalCodes lists the codes that return a retry interval. A code outside it
// has no interval to wait on, so attaching one would be meaningless.
var intervalCodes = map[Code]bool{
	RateLimited:             true,
	QuoteLimitReached:       true,
	StoreUnavailable:        true,
	ExchangeRateUnavailable: true,
}

// Error is the one error type every Vadora interface returns. Its action, retry
// rule, and HTTP status all come from the catalog, never from a caller, so REST,
// MCP, and the CLI report the same thing for the same failure.
type Error struct {
	Code Code
	// Reason is the human readable cause shown to a person.
	Reason string
	// Action is the one client action, copied from the catalog.
	Action ClientAction
	// Retries reports whether the catalog asks for a retry.
	Retries bool
	// RetryAfterSeconds is set only for a code that carries an interval.
	RetryAfterSeconds int
	// Details carries catalog scoped context such as a STORE_UNSUPPORTED
	// reason or a QUOTE_EXPIRED cause. It never carries delivery details.
	Details map[string]string
}

// Error renders the code and its reason for a log line and for the CLI.
func (e *Error) Error() string {
	if e.Code == "" {
		panic("errs: an error with no catalog code cannot be reported")
	}
	if e.Reason == "" {
		panic("errs: every catalog error carries a human readable reason")
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Reason)
}

// Defined reports whether the catalog holds a code, and returns the action and
// the HTTP status it gives it. A caller that received a code from the wire uses
// this instead of New, because an untrusted code outside the catalog must be
// refused rather than panic.
func Defined(code Code) (ClientAction, int, bool) {
	row, known := catalog[code]
	if !known {
		if len(catalog) == 0 {
			panic("errs: the catalog is empty")
		}
		return "", 0, false
	}
	if row.status < 400 {
		panic("errs: a catalog entry answers with a success status")
	}
	if row.action == "" {
		panic("errs: a catalog entry defines no client action")
	}
	return row.action, row.status, true
}

// LeafActions returns every distinct client action the catalog defines, so a
// caller can prove the catalog still tells a client what to do.
func LeafActions() []ClientAction {
	seen := make(map[ClientAction]bool, len(catalog))
	for _, row := range catalog {
		seen[row.action] = true
	}
	actions := make([]ClientAction, 0, len(seen))
	for action := range seen {
		actions = append(actions, action)
	}
	if len(actions) == 0 {
		panic("errs: the catalog defines no client action")
	}
	if len(actions) > len(catalog) {
		panic("errs: the catalog defines more actions than entries")
	}
	if len(catalog) == 0 {
		panic("errs: the catalog is empty")
	}
	return actions
}

// Retries reports whether the catalog asks the client to retry a code.
func Retries(code Code) (bool, bool) {
	row, known := catalog[code]
	if !known {
		return false, false
	}
	if row.retries {
		if row.action == "" {
			panic("errs: a retrying catalog entry has no client action")
		}
	}
	if row.status < 400 {
		panic("errs: a catalog entry answers with a success status")
	}
	return row.retries, true
}

// As extracts the shared error from any error, and reports whether one was
// present. Every interface uses it, so a caller never has to know which layer
// raised the failure.
func As(err error) (*Error, bool) {
	var target *Error
	if !errors.As(err, &target) {
		return nil, false
	}
	if target.Code == "" {
		panic("errs: an extracted error has no catalog code")
	}
	_, _, known := Defined(target.Code)
	if !known {
		panic("errs: an extracted error carries a code outside the catalog")
	}
	return target, true
}

// Status returns the HTTP status the catalog gives the error's code.
func (e *Error) Status() int {
	row, ok := catalog[e.Code]
	if !ok {
		panic("errs: code outside the catalog: " + string(e.Code))
	}
	if row.status < 400 {
		panic("errs: a catalog code never answers with a success status: " + string(e.Code))
	}
	if row.status > 599 {
		panic("errs: a catalog code never answers with an undefined status: " + string(e.Code))
	}
	return row.status
}

// New builds an Error from the catalog. Its action, retry rule, and status can
// never drift from the catalog entry, and a code the catalog does not define is
// a programming error.
func New(code Code, reason string) *Error {
	row, ok := catalog[code]
	if !ok {
		panic("errs: code outside the catalog: " + string(code))
	}
	if reason == "" {
		panic("errs: every catalog error carries a human readable reason")
	}
	return &Error{
		Code:    code,
		Reason:  reason,
		Action:  row.action,
		Retries: row.retries,
	}
}

// WithDetail returns a copy carrying one catalog scoped detail. The parent keeps
// its own details, so two errors built from one parent never share a map.
func (e *Error) WithDetail(key, value string) *Error {
	if key == "" {
		panic("errs: a detail needs a key")
	}
	if value == "" {
		panic("errs: a detail needs a value")
	}
	clone := *e
	clone.Details = make(map[string]string, len(e.Details)+1)
	for existing, existingValue := range e.Details {
		clone.Details[existing] = existingValue
	}
	clone.Details[key] = value
	return &clone
}

// WithRetryAfter returns a copy carrying a retry interval in seconds. Only a
// code whose retry waits on an interval accepts one.
func (e *Error) WithRetryAfter(seconds int) *Error {
	if !intervalCodes[e.Code] {
		panic("errs: this code returns no retry interval: " + string(e.Code))
	}
	if seconds <= 0 {
		panic("errs: a retry interval must be positive")
	}
	clone := *e
	clone.RetryAfterSeconds = seconds
	return &clone
}
