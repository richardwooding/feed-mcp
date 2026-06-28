package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"
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
