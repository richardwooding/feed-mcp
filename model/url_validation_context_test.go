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

func TestValidator_FailsClosedOnResolveTimeout(t *testing.T) {
	t.Parallel()
	v := newValidator(blockingResolver(), 50*time.Millisecond)

	// A named host needs resolution; the resolve budget elapses. The validator
	// fails closed, surfacing the deadline error rather than allowing the URL.
	err := v.validateURL(context.Background(), "http://named-host.example/feed", false)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("validateURL on resolve timeout = %v, want context.DeadlineExceeded", err)
	}
}

func TestValidator_PropagatesCancellation(t *testing.T) {
	t.Parallel()
	v := newValidator(blockingResolver(), 5*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := v.validateURL(ctx, "http://named-host.example/feed", false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("validateURL with canceled ctx = %v, want context.Canceled", err)
	}
}

func TestValidator_SanitizeURLsPropagatesContextError(t *testing.T) {
	t.Parallel()
	v := newValidator(blockingResolver(), 5*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := v.sanitizeURLs(ctx, []string{"http://a.example/feed", "http://b.example/feed"}, false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("sanitizeURLs with canceled ctx = %v, want context.Canceled", err)
	}
}

func TestValidator_SanitizeURLsPropagatesDeadlineOnLastURL(t *testing.T) {
	t.Parallel()
	// The deadline elapses while validating the only/last URL (so the loop-top
	// ctx.Err() check passes first). The context error must propagate, not be
	// folded into the formatted "invalid feed URLs" message.
	v := newValidator(blockingResolver(), 5*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	err := v.sanitizeURLs(ctx, []string{"http://only-host.example/feed"}, false)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("sanitizeURLs = %v, want context.DeadlineExceeded", err)
	}
}
