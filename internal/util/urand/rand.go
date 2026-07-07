package urand

import (
	"context"
	"math/rand"
	"time"
)

func Range(rng *rand.Rand, left int, right int) int {
	if right <= left {
		return left
	}
	return left + rng.Intn(right-left+1)
}

func SleepMillis(ctx context.Context, rng *rand.Rand, left int, right int) error {
	delay := time.Duration(Range(rng, left, right)) * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

