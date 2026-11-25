package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hellprise/interline-engine/internal/inventory"
)

// TestHoldRaceLastSeats verifies that concurrent holds never oversell the last seats.
func TestHoldRaceLastSeats(t *testing.T) {
	repo, pool := openRepo(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const buyers = 10
	errs := make([]error, buyers)
	var wg sync.WaitGroup

	for i := range buyers {
		// Use wg.Go instead of wg.Add(1) + defer wg.Done() since wg.Go wraps them.
		wg.Go(func() {
			errs[i] = repo.Hold(ctx, "V8-220-2026-11-02", "economy", 1)
		})
	}

	wg.Wait()

	var held, refused int
	for i, err := range errs {
		switch {
		case err == nil:
			held++
		case errors.Is(err, inventory.ErrNotEnoughSeats):
			refused++
		default:
			t.Errorf("buyer %d: unexpected error: %v", i, err)
		}
	}

	if held != 2 || refused != buyers-2 {
		t.Errorf("held %d, refused %d, want 2 and %d", held, refused, buyers-2)
	}

	got := heldNow(t, pool)["V8-220-2026-11-02/economy"]
	if got != 150 {
		t.Errorf("seats_held = %d, want 150", got)
	}
}
