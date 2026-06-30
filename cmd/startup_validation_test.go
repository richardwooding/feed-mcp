package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidateStartupFeedURLs_NoURLs(t *testing.T) {
	if err := validateStartupFeedURLs(context.Background(), nil, false); err != nil {
		t.Fatalf("validateStartupFeedURLs(no URLs) = %v, want nil", err)
	}
}

func TestValidateStartupFeedURLs_RejectsInvalidURL(t *testing.T) {
	// A bad scheme is a genuine validation failure (no DNS needed) and must be
	// surfaced, not tolerated.
	err := validateStartupFeedURLs(context.Background(), []string{"file:///etc/passwd"}, false)
	if err == nil || !strings.Contains(err.Error(), "invalid feed URLs") {
		t.Fatalf("validateStartupFeedURLs(bad scheme) = %v, want an invalid-URL error", err)
	}
}

func TestValidateStartupFeedURLs_PropagatesCancellation(t *testing.T) {
	// A canceled startup context is fatal — only a DNS resolve-timeout is
	// tolerated, not real cancellation/shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := validateStartupFeedURLs(ctx, []string{"http://example.com/feed"}, false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("validateStartupFeedURLs(canceled) = %v, want context.Canceled", err)
	}
}

func TestStoreRateLimiterIdleTimeout(t *testing.T) {
	// 0 from the CLI means "disable eviction" → mapped to a negative duration
	// (the store treats 0 as "use default", so it must not stay 0).
	if got := storeRateLimiterIdleTimeout(0); got >= 0 {
		t.Errorf("storeRateLimiterIdleTimeout(0) = %v, want a negative (disabled) value", got)
	}
	// A positive value passes through unchanged.
	if got := storeRateLimiterIdleTimeout(time.Hour); got != time.Hour {
		t.Errorf("storeRateLimiterIdleTimeout(1h) = %v, want 1h", got)
	}
}
