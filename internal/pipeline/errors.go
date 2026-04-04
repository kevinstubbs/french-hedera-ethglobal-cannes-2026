package pipeline

import "errors"

// ErrInsufficientPrepaid is returned when start/resume requires more prepaid units than available.
var ErrInsufficientPrepaid = errors.New("insufficient prepaid balance")

// ErrInvalidNaryoEvent is returned when a Naryo webhook payload is missing required fields.
var ErrInvalidNaryoEvent = errors.New("invalid naryo event")
