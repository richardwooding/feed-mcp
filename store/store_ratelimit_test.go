package store

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/richardwooding/hostrate"
)

func TestApplyConfigDefaults_RateLimiterIdleTimeout(t *testing.T) {
	// Zero means "unset" → default to 1h so the per-host limiter map is bounded.
	c := Config{}
	applyConfigDefaults(&c)
	if c.RateLimiterIdleTimeout != time.Hour {
		t.Errorf("default RateLimiterIdleTimeout = %v, want 1h", c.RateLimiterIdleTimeout)
	}

	// A negative value is an explicit opt-out (eviction disabled) and must be
	// preserved, not overwritten by the default.
	c2 := Config{RateLimiterIdleTimeout: -1}
	applyConfigDefaults(&c2)
	if c2.RateLimiterIdleTimeout != -1 {
		t.Errorf("negative RateLimiterIdleTimeout = %v, want it preserved (-1)", c2.RateLimiterIdleTimeout)
	}
}

func TestNewRateLimitedHTTPClient_EvictsIdleLimiters(t *testing.T) {
	poolConfig := HTTPPoolConfig{MaxIdleConns: 1, MaxConnsPerHost: 1, MaxIdleConnsPerHost: 1, IdleConnTimeout: time.Second}
	// Short idle timeout so eviction is observable; allow loopback so the dial is
	// attempted (the connection is refused, but a limiter entry is still created
	// before the dial — which is all this test needs).
	client := NewRateLimitedHTTPClient(1000, 1000, poolConfig, true, 50*time.Millisecond)

	tr, ok := client.Transport.(*hostrate.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *hostrate.Transport", client.Transport)
	}

	// touch issues a request whose only purpose is to register a per-host limiter
	// (created before the dial in RoundTrip). The dial itself fails — nothing is
	// listening — and a short context bounds it so a slow/unroutable dial can't
	// hang the test regardless of the platform's loopback/IPv6 setup.
	touch := func(url string) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		if resp, err := client.Do(req); err == nil {
			_ = resp.Body.Close()
		}
	}

	// Host A: key "127.0.0.1".
	touch("http://127.0.0.1:1/a")
	if tr.Len() == 0 {
		t.Fatal("expected a per-host limiter after the first request")
	}

	// Let A go idle past the eviction window.
	time.Sleep(80 * time.Millisecond)

	// Host B: key "localhost" (distinct key, resolves via the hosts file — no
	// network DNS). This request triggers a sweep that evicts the idle A.
	touch("http://localhost:1/b")
	if got := tr.Len(); got != 1 {
		t.Fatalf("idle limiter not evicted: Len = %d, want 1 (only the just-used host)", got)
	}
}
