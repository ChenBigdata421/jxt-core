package reliable

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIsTerminal(t *testing.T) {
	assert.True(t, IsTerminal(StatusSucceeded))
	assert.True(t, IsTerminal(StatusDeadLetter))
	assert.True(t, IsTerminal(StatusDiscarded))
	assert.False(t, IsTerminal(StatusProcessing))
	assert.False(t, IsTerminal(StatusRetryScheduled))
}

func TestCanTransitionLegalTable(t *testing.T) {
	legal := []struct{ from, to Status }{
		{StatusProcessing, StatusSucceeded},
		{StatusProcessing, StatusRetryScheduled},
		{StatusProcessing, StatusDeadLetter},
		{StatusRetryScheduled, StatusProcessing},
		{StatusRetryScheduled, StatusDeadLetter},
		{StatusDeadLetter, StatusRetryScheduled},
		{StatusDeadLetter, StatusDiscarded},
		{StatusDeadLetter, StatusSucceeded},
	}
	for _, c := range legal {
		assert.True(t, CanTransition(c.from, c.to), "%s→%s must be legal", c.from, c.to)
	}
	illegal := []struct{ from, to Status }{
		{StatusSucceeded, StatusProcessing},
		{StatusDiscarded, StatusRetryScheduled},
		{StatusProcessing, StatusDiscarded},
		{StatusRetryScheduled, StatusSucceeded},
	}
	for _, c := range illegal {
		assert.False(t, CanTransition(c.from, c.to), "%s→%s must be illegal", c.from, c.to)
	}
}

func TestAdvanceAttempt(t *testing.T) {
	assert.Equal(t, 1, AdvanceAttempt(0))
	assert.Equal(t, 1, AdvanceAttempt(-3))
	assert.Equal(t, 2, AdvanceAttempt(1))
	assert.Equal(t, 6, AdvanceAttempt(5))
}

func TestShouldDeadLetter(t *testing.T) {
	assert.False(t, ShouldDeadLetter(1, 5))
	assert.False(t, ShouldDeadLetter(4, 5))
	assert.True(t, ShouldDeadLetter(5, 5))
	assert.True(t, ShouldDeadLetter(9, 5))
	assert.True(t, ShouldDeadLetter(1, 1), "maxAttempts=1 → 首次即上限")
}

func TestBackoffMonotonicAndCapped(t *testing.T) {
	base, capd := time.Second, time.Hour
	prev := time.Duration(0)
	for a := 1; a <= 30; a++ {
		got := Backoff(a, base, capd, 0.5)
		assert.LessOrEqual(t, got, capd, "must never exceed cap")
		assert.GreaterOrEqual(t, got, prev, "attempt=%d not monotonic", a)
		prev = got
	}
	assert.Equal(t, capd, Backoff(40, base, capd, 0.5))
}

func TestBackoffJitterBounds(t *testing.T) {
	const a = 3
	core := 4 * time.Second
	assert.Equal(t, time.Duration(0.8*float64(core)), Backoff(a, time.Second, time.Hour, 0.0))
	assert.Equal(t, time.Duration(1.0*float64(core)), Backoff(a, time.Second, time.Hour, 0.5))
	assert.Equal(t, time.Duration((0.8+0.4*0.99)*float64(core)), Backoff(a, time.Second, time.Hour, 0.99))
}

func TestBackoffPropertyNeverNegativeNeverOverCap(t *testing.T) {
	for _, a := range []int{1, 2, 3, 5, 10, 50} {
		for _, j := range []float64{0.0, 0.25, 0.5, 0.75, 0.999} {
			got := Backoff(a, 500*time.Millisecond, 2*time.Second, j)
			assert.Greater(t, got, time.Duration(0))
			assert.LessOrEqual(t, got, 2*time.Second+1)
		}
	}
}
