package model

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

// blockingResolver returns a resolver whose lookups block until their context is
// done, so a test can drive the resolve-timeout and cancellation paths
// deterministically without touching the network.
func blockingResolver() *net.Resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
}

// withResolver swaps the package resolver and resolve timeout for the duration of
// a test, restoring them afterwards. (TestMain installs a fail-fast resolver; the
// context tests need a blocking one.)
func withResolver(t *testing.T, r *net.Resolver, timeout time.Duration) {
	t.Helper()
	origR, origT := resolver, validateResolveTimeout
	resolver, validateResolveTimeout = r, timeout
	t.Cleanup(func() { resolver, validateResolveTimeout = origR, origT })
}

func TestValidateFeedURLContext_LenientOnInternalTimeout(t *testing.T) {
	withResolver(t, blockingResolver(), 50*time.Millisecond)

	// A named host needs resolution; the internal budget elapses while the parent
	// context stays live. The URL should be allowed (dial-time guard re-checks).
	if err := ValidateFeedURLContext(context.Background(), "http://named-host.example/feed", false); err != nil {
		t.Fatalf("ValidateFeedURLContext on internal resolve timeout = %v, want nil", err)
	}
}

func TestValidateFeedURLContext_PropagatesCancellation(t *testing.T) {
	withResolver(t, blockingResolver(), 5*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // canceled by the caller, not our internal budget

	err := ValidateFeedURLContext(ctx, "http://named-host.example/feed", false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ValidateFeedURLContext with canceled ctx = %v, want context.Canceled", err)
	}
}

func TestSanitizeFeedURLsContext_StopsOnCancellation(t *testing.T) {
	withResolver(t, blockingResolver(), 5*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := SanitizeFeedURLsContext(ctx, []string{"http://a.example/feed", "http://b.example/feed"}, false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SanitizeFeedURLsContext with canceled ctx = %v, want context.Canceled", err)
	}
}

func TestSanitizeFeedURLsContext_PropagatesDeadlineOnLastURL(t *testing.T) {
	// The deadline elapses while validating the only/last URL (so the loop-top
	// ctx.Err() check passes first). The context error must propagate, not be
	// folded into the formatted "invalid feed URLs" message.
	withResolver(t, blockingResolver(), 5*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	err := SanitizeFeedURLsContext(ctx, []string{"http://only-host.example/feed"}, false)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SanitizeFeedURLsContext = %v, want context.DeadlineExceeded", err)
	}
}
