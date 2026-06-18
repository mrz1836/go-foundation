package models_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mrz1836/go-foundation/models"
)

func TestClockFrom_RealClockFallback(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	got := models.ClockFrom(ctx).Now(ctx)

	assert.WithinDuration(t, time.Now(), got, time.Second,
		"a clock-less context must yield the real clock")
}

func TestClockFrom_ReturnsAttachedClock(t *testing.T) {
	t.Parallel()

	anchor := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	ctx := models.WithClock(context.Background(), models.NewFixedClock(anchor))

	assert.Equal(t, anchor, models.ClockFrom(ctx).Now(ctx))
}

func TestFixedClock_NowIgnoresContext(t *testing.T) {
	t.Parallel()

	anchor := time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)
	clk := models.NewFixedClock(anchor)

	assert.Equal(t, anchor, clk.Now(context.Background()))
	assert.Equal(t, anchor, clk.Now(context.TODO()))
}
