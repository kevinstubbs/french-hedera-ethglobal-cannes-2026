package billing

import (
	"context"
	"time"
)

// RunEach invokes fn every interval until ctx is done.
func RunEach(ctx context.Context, interval time.Duration, fn func()) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn()
		}
	}
}
