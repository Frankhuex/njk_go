package urand

import (
	"context"
	"math/rand"
	"time"
)

func NewRandom() *rand.Rand {
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}

func Range(left int, right int) int {
	rng := NewRandom()
	if right <= left {
		return left
	}
	return left + rng.Intn(right-left+1)
}

func SleepMillis(ctx context.Context, left int, right int) error {
	delay := time.Duration(Range(left, right)) * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
