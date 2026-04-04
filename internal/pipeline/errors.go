package pipeline

import (
	"errors"
	"fmt"
)

// ErrInsufficientPrepaid is returned when start/resume lacks runway, or GET status while running/paused with zero balance.
var ErrInsufficientPrepaid = errors.New("insufficient prepaid balance")

// InsufficientPrepaidError is the concrete error for [ErrInsufficientPrepaid] with balance details.
type InsufficientPrepaidError struct {
	RemainingUnits int64
	RequiredUnits  int64
}

func (e *InsufficientPrepaidError) Error() string {
	if e == nil {
		return ErrInsufficientPrepaid.Error()
	}
	return fmt.Sprintf("insufficient prepaid balance: have %d need %d", e.RemainingUnits, e.RequiredUnits)
}

func (e *InsufficientPrepaidError) Unwrap() error {
	return ErrInsufficientPrepaid
}

// ErrInvalidNaryoEvent is returned when a Naryo webhook payload is missing required fields.
var ErrInvalidNaryoEvent = errors.New("invalid naryo event")
