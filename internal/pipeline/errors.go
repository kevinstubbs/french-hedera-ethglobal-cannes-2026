package pipeline

import "errors"

// ErrInsufficientPrepaid is returned when start/resume requires more prepaid units than available.
var ErrInsufficientPrepaid = errors.New("insufficient prepaid balance")
