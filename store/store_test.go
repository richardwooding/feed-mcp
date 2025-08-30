package store

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func mockFeedServer(t *testing.T, title string) *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, err := w.Write([]byte(`
			<rss version="2.0">
				<channel>
					<title>` + title + `</title>
					<item>
						<title>Item 1</title>
						<link>http://example.com/1</link>
					</item>
				</channel>
			</rss>
		`))
		if err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	})
	return httptest.NewServer(handler)
}

func TestNewStore_NoFeeds(t *testing.T) {
	_, err := NewStore(Config{Feeds: []string{}})
	if err == nil {
		t.Fatal("expected error when no feeds are provided")
	}
}

func TestNewStore_AndGetAllFeeds(t *testing.T) {
	srv := mockFeedServer(t, "FeedTitle")
	defer srv.Close()

	store, err := NewStore(Config{Feeds: []string{srv.URL}})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	ctx := context.Background()
	results, err := store.GetAllFeeds(ctx)
	if err != nil {
		t.Fatalf("GetAllFeeds failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Title != "FeedTitle" {
		t.Errorf("expected Title 'FeedTitle', got %q", results[0].Title)
	}
	if results[0].Feed == nil {
		t.Error("expected Feed to be non-nil")
	}
}

func TestGetFeedAndItems_Success(t *testing.T) {
	srv := mockFeedServer(t, "FeedTitle2")
	defer srv.Close()

	store, err := NewStore(Config{Feeds: []string{srv.URL}})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// Find the ID for the feed
	var id string
	for k := range store.feeds {
		id = k
		break
	}
	if id == "" {
		t.Fatal("no feed ID found")
	}

	ctx := context.Background()
	result, err := store.GetFeedAndItems(ctx, id)
	if err != nil {
		t.Fatalf("GetFeedAndItems failed: %v", err)
	}
	if result.Title != "FeedTitle2" {
		t.Errorf("expected Title 'FeedTitle2', got %q", result.Title)
	}
	if result.Feed == nil {
		t.Error("expected Feed to be non-nil")
	}
	if len(result.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(result.Items))
	}
}

func TestGetFeedAndItems_NotFound(t *testing.T) {
	srv := mockFeedServer(t, "FeedTitle3")
	defer srv.Close()

	store, err := NewStore(Config{Feeds: []string{srv.URL}})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	ctx := context.Background()
	_, err = store.GetFeedAndItems(ctx, "nonexistent")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got %v", err)
	}
}

func TestGetAllFeeds_FetchError(t *testing.T) {
	// Use an invalid URL to simulate fetch error
	badURL := "http://127.0.0.1:0/doesnotexist"
	store, err := NewStore(Config{Feeds: []string{badURL}})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	ctx := context.Background()
	results, err := store.GetAllFeeds(ctx)
	if err != nil {
		t.Fatalf("GetAllFeeds failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].FetchError == "" {
		t.Error("expected FetchError to be set")
	}
}

func TestGetFeedAndItems_FetchError(t *testing.T) {
	badURL := "http://127.0.0.1:0/doesnotexist"
	store, err := NewStore(Config{Feeds: []string{badURL}})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	var id string
	for k := range store.feeds {
		id = k
		break
	}
	if id == "" {
		t.Fatal("no feed ID found")
	}

	ctx := context.Background()
	result, err := store.GetFeedAndItems(ctx, id)
	if err != nil {
		t.Fatalf("GetFeedAndItems failed: %v", err)
	}
	if result.FetchError == "" {
		t.Error("expected FetchError to be set")
	}
}

func TestNewRateLimitedHTTPClient(t *testing.T) {
	poolConfig := HTTPPoolConfig{
		MaxIdleConns:        100,
		MaxConnsPerHost:     10,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     90 * time.Second,
	}
	client := NewRateLimitedHTTPClient(1.0, 2, poolConfig)

	if client == nil {
		t.Fatal("expected client to be non-nil")
	}

	if client.Timeout != 30*time.Second {
		t.Errorf("expected timeout to be 30s, got %v", client.Timeout)
	}

	// Verify it's our custom transport
	if _, ok := client.Transport.(*RateLimitedTransport); !ok {
		t.Error("expected RateLimitedTransport")
	}
}

func TestRateLimitedTransport_RateLimit(t *testing.T) {
	// Track number of requests
	var requestCount int64

	// Create a test server that tracks requests
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Create a very restrictive rate limiter: 1 request per second with burst of 1
	poolConfig := HTTPPoolConfig{
		MaxIdleConns:        100,
		MaxConnsPerHost:     10,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     90 * time.Second,
	}
	client := NewRateLimitedHTTPClient(1.0, 1, poolConfig)

	start := time.Now()

	// Make 3 requests - should take at least 2 seconds due to rate limiting
	for i := 0; i < 3; i++ {
		resp, err := client.Get(srv.URL)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		resp.Body.Close()
	}

	duration := time.Since(start)

	// Should have taken at least 2 seconds (first request immediate, next two rate-limited)
	if duration < 2*time.Second {
		t.Errorf("expected at least 2s delay, got %v", duration)
	}

	// Verify all 3 requests were made
	if atomic.LoadInt64(&requestCount) != 3 {
		t.Errorf("expected 3 requests, got %d", atomic.LoadInt64(&requestCount))
	}
}

func TestStore_DefaultRateLimiting(t *testing.T) {
	srv := mockFeedServer(t, "RateLimitTest")
	defer srv.Close()

	// Create store without custom HTTP client - should use default rate limiting
	store, err := NewStore(Config{
		Feeds: []string{srv.URL},
		// Don't set RequestsPerSecond or BurstCapacity - should use defaults
	})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// Verify rate limiting is enabled by checking all feeds can be fetched
	ctx := context.Background()
	results, err := store.GetAllFeeds(ctx)
	if err != nil {
		t.Fatalf("GetAllFeeds failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Title != "RateLimitTest" {
		t.Errorf("expected Title 'RateLimitTest', got %q", results[0].Title)
	}
}

func TestStore_CustomRateLimiting(t *testing.T) {
	srv := mockFeedServer(t, "CustomRateTest")
	defer srv.Close()

	// Create store with custom rate limiting settings
	store, err := NewStore(Config{
		Feeds:             []string{srv.URL},
		RequestsPerSecond: 0.5, // Very slow: 1 request every 2 seconds
		BurstCapacity:     1,   // No burst
	})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// The feed should still work, just rate-limited
	ctx := context.Background()
	results, err := store.GetAllFeeds(ctx)
	if err != nil {
		t.Fatalf("GetAllFeeds failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Title != "CustomRateTest" {
		t.Errorf("expected Title 'CustomRateTest', got %q", results[0].Title)
	}
}

func TestStore_CustomHTTPClientPreserved(t *testing.T) {
	srv := mockFeedServer(t, "CustomClientTest")
	defer srv.Close()

	// Create custom HTTP client
	customClient := &http.Client{Timeout: 5 * time.Second}

	// Create store with custom HTTP client - rate limiting should be skipped
	store, err := NewStore(Config{
		Feeds:             []string{srv.URL},
		HTTPClient:        customClient,
		RequestsPerSecond: 10.0, // These should be ignored since HTTPClient is provided
		BurstCapacity:     20,
	})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// Should still work with custom client
	ctx := context.Background()
	results, err := store.GetAllFeeds(ctx)
	if err != nil {
		t.Fatalf("GetAllFeeds failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Title != "CustomClientTest" {
		t.Errorf("expected Title 'CustomClientTest', got %q", results[0].Title)
	}
}

func TestStore_CircuitBreakerDisabled(t *testing.T) {
	srv := mockFeedServer(t, "CircuitBreakerTest")
	defer srv.Close()

	// Create store with circuit breaker explicitly disabled
	disabled := false
	store, err := NewStore(Config{
		Feeds:                 []string{srv.URL},
		CircuitBreakerEnabled: &disabled,
	})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// Verify circuit breaker is not initialized
	if store.circuitBreakers != nil {
		t.Error("expected circuitBreakers to be nil when disabled")
	}

	ctx := context.Background()
	results, err := store.GetAllFeeds(ctx)
	if err != nil {
		t.Fatalf("GetAllFeeds failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Circuit breaker should not be open since it's disabled
	if results[0].CircuitBreakerOpen {
		t.Error("expected CircuitBreakerOpen to be false when circuit breaker is disabled")
	}
}

func TestStore_CircuitBreakerEnabledByDefault(t *testing.T) {
	srv := mockFeedServer(t, "CircuitBreakerTest")
	defer srv.Close()

	// Create store without specifying circuit breaker setting - should be enabled by default
	store, err := NewStore(Config{
		Feeds:                 []string{srv.URL},
		CircuitBreakerTimeout: 1 * time.Second, // Short timeout for testing
	})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// Verify circuit breaker is initialized by default
	if store.circuitBreakers == nil {
		t.Fatal("expected circuitBreakers to be initialized by default")
	}

	if cb, exists := store.circuitBreakers[srv.URL]; !exists {
		t.Fatal("expected circuit breaker to exist for feed URL")
	} else if cb == nil {
		t.Fatal("expected circuit breaker to be non-nil")
	}

	ctx := context.Background()
	results, err := store.GetAllFeeds(ctx)
	if err != nil {
		t.Fatalf("GetAllFeeds failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Circuit breaker should be closed initially
	if results[0].CircuitBreakerOpen {
		t.Error("expected CircuitBreakerOpen to be false initially")
	}
}

func TestStore_CircuitBreakerExplicitlyEnabled(t *testing.T) {
	srv := mockFeedServer(t, "CircuitBreakerTest")
	defer srv.Close()

	// Create store with circuit breaker explicitly enabled
	enabled := true
	store, err := NewStore(Config{
		Feeds:                 []string{srv.URL},
		CircuitBreakerEnabled: &enabled,
		CircuitBreakerTimeout: 1 * time.Second, // Short timeout for testing
	})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// Verify circuit breaker is initialized
	if store.circuitBreakers == nil {
		t.Fatal("expected circuitBreakers to be initialized when enabled")
	}

	if cb, exists := store.circuitBreakers[srv.URL]; !exists {
		t.Fatal("expected circuit breaker to exist for feed URL")
	} else if cb == nil {
		t.Fatal("expected circuit breaker to be non-nil")
	}

	ctx := context.Background()
	results, err := store.GetAllFeeds(ctx)
	if err != nil {
		t.Fatalf("GetAllFeeds failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Circuit breaker should be closed initially
	if results[0].CircuitBreakerOpen {
		t.Error("expected CircuitBreakerOpen to be false initially")
	}
}

func TestStore_CircuitBreakerFailures(t *testing.T) {
	// Create a server that fails consistently
	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failingServer.Close()

	// Create store with circuit breaker enabled and aggressive settings
	enabled := true
	store, err := NewStore(Config{
		Feeds:                 []string{failingServer.URL},
		CircuitBreakerEnabled: &enabled,
		CircuitBreakerTimeout: 1 * time.Second,
		ExpireAfter:           1 * time.Millisecond, // Force cache expiry
	})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	ctx := context.Background()

	// Make multiple requests to trigger circuit breaker
	for i := 0; i < 5; i++ {
		results, err := store.GetAllFeeds(ctx)
		if err != nil {
			t.Fatalf("GetAllFeeds failed on attempt %d: %v", i+1, err)
		}

		if len(results) != 1 {
			t.Fatalf("expected 1 result on attempt %d, got %d", i+1, len(results))
		}

		// Should have fetch error
		if results[0].FetchError == "" {
			t.Errorf("expected FetchError on attempt %d", i+1)
		}

		// Clear cache to force new requests
		time.Sleep(2 * time.Millisecond)
	}

	// After multiple failures, circuit breaker should be open
	results, err := store.GetAllFeeds(ctx)
	if err != nil {
		t.Fatalf("GetAllFeeds failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Circuit breaker should now be open
	if !results[0].CircuitBreakerOpen {
		t.Error("expected CircuitBreakerOpen to be true after multiple failures")
	}
}

func TestStore_CircuitBreakerRecovery(t *testing.T) {
	// Create a server that initially fails then recovers
	var requestCount int64
	recoveringServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt64(&requestCount, 1)
		if count <= 3 {
			// Fail first 3 requests
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Succeed after that
		w.Header().Set("Content-Type", "application/rss+xml")
		_, err := w.Write([]byte(`
			<rss version="2.0">
				<channel>
					<title>Recovered Feed</title>
					<item>
						<title>Recovery Item</title>
						<link>http://example.com/recovery</link>
					</item>
				</channel>
			</rss>
		`))
		if err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer recoveringServer.Close()

	// Create store with circuit breaker enabled and retries limited to 1 attempt for predictable circuit breaker testing
	enabled := true
	store, err := NewStore(Config{
		Feeds:                 []string{recoveringServer.URL},
		CircuitBreakerEnabled: &enabled,
		CircuitBreakerTimeout: 1 * time.Second,      // Short timeout for quick recovery
		ExpireAfter:           1 * time.Millisecond, // Force cache expiry
		RetryMaxAttempts:      1,                    // Limit retries to 1 for circuit breaker testing
	})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	ctx := context.Background()

	// First 3 requests should fail and open the circuit
	for i := 0; i < 3; i++ {
		results, _ := store.GetAllFeeds(ctx)
		if len(results) > 0 && results[0].FetchError == "" {
			t.Errorf("expected failure on request %d", i+1)
		}
		time.Sleep(2 * time.Millisecond) // Clear cache
	}

	// Wait for circuit breaker timeout
	time.Sleep(1100 * time.Millisecond)

	// Next request should succeed and close the circuit
	results, err := store.GetAllFeeds(ctx)
	if err != nil {
		t.Fatalf("GetAllFeeds failed after recovery: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result after recovery, got %d", len(results))
	}

	// Should eventually succeed
	maxAttempts := 3
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if results[0].FetchError == "" && results[0].Title == "Recovered Feed" {
			break
		}
		if attempt == maxAttempts-1 {
			t.Errorf("expected recovery after %d attempts, got FetchError: %q, Title: %q",
				maxAttempts, results[0].FetchError, results[0].Title)
		}
		time.Sleep(100 * time.Millisecond)
		results, _ = store.GetAllFeeds(ctx)
		if len(results) == 0 {
			continue
		}
	}
}

func TestStore_CircuitBreakerCustomSettings(t *testing.T) {
	srv := mockFeedServer(t, "CustomSettingsTest")
	defer srv.Close()

	// Create store with custom circuit breaker settings
	enabled := true
	store, err := NewStore(Config{
		Feeds:                          []string{srv.URL},
		CircuitBreakerEnabled:          &enabled,
		CircuitBreakerMaxRequests:      5,
		CircuitBreakerInterval:         2 * time.Second,
		CircuitBreakerTimeout:          3 * time.Second,
		CircuitBreakerFailureThreshold: 2, // Custom failure threshold
	})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// Verify settings are applied
	if _, exists := store.circuitBreakers[srv.URL]; !exists {
		t.Fatal("expected circuit breaker to exist")
	}

	// We can't directly access the settings, but we can verify the circuit breaker works
	ctx := context.Background()
	results, err := store.GetAllFeeds(ctx)
	if err != nil {
		t.Fatalf("GetAllFeeds failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Title != "CustomSettingsTest" {
		t.Errorf("expected Title 'CustomSettingsTest', got %q", results[0].Title)
	}
}

func TestStore_CircuitBreakerCustomFailureThreshold(t *testing.T) {
	// Create a server that fails consistently
	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failingServer.Close()

	// Create store with custom failure threshold of 2 failures
	enabled := true
	store, err := NewStore(Config{
		Feeds:                          []string{failingServer.URL},
		CircuitBreakerEnabled:          &enabled,
		CircuitBreakerTimeout:          1 * time.Second,
		CircuitBreakerFailureThreshold: 2,                    // Should open after 2 failures instead of default 3
		ExpireAfter:                    1 * time.Millisecond, // Force cache expiry
	})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	ctx := context.Background()

	// Make 2 requests - should be enough to trigger circuit breaker with threshold of 2
	for i := 0; i < 2; i++ {
		results, err := store.GetAllFeeds(ctx)
		if err != nil {
			t.Fatalf("GetAllFeeds failed on attempt %d: %v", i+1, err)
		}

		if len(results) != 1 {
			t.Fatalf("expected 1 result on attempt %d, got %d", i+1, len(results))
		}

		// Should have fetch error
		if results[0].FetchError == "" {
			t.Errorf("expected FetchError on attempt %d", i+1)
		}

		// Clear cache to force new requests
		time.Sleep(2 * time.Millisecond)
	}

	// After 2 failures with threshold of 2, circuit breaker should be open
	results, err := store.GetAllFeeds(ctx)
	if err != nil {
		t.Fatalf("GetAllFeeds failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Circuit breaker should now be open with only 2 failures
	if !results[0].CircuitBreakerOpen {
		t.Error("expected CircuitBreakerOpen to be true after 2 failures with threshold of 2")
	}
}

func TestGetFeedAndItems_CircuitBreakerState(t *testing.T) {
	srv := mockFeedServer(t, "FeedAndItemsCircuitTest")
	defer srv.Close()

	// Create store with circuit breaker enabled (should be default, but let's be explicit for this test)
	enabled := true
	store, err := NewStore(Config{
		Feeds:                 []string{srv.URL},
		CircuitBreakerEnabled: &enabled,
	})
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// Find the ID for the feed
	var id string
	for k := range store.feeds {
		id = k
		break
	}
	if id == "" {
		t.Fatal("no feed ID found")
	}

	ctx := context.Background()
	result, err := store.GetFeedAndItems(ctx, id)
	if err != nil {
		t.Fatalf("GetFeedAndItems failed: %v", err)
	}

	// Circuit breaker should be closed initially
	if result.CircuitBreakerOpen {
		t.Error("expected CircuitBreakerOpen to be false initially")
	}

	if result.Title != "FeedAndItemsCircuitTest" {
		t.Errorf("expected Title 'FeedAndItemsCircuitTest', got %q", result.Title)
	}
}

func TestStore_ConnectionPooling(t *testing.T) {
	srv := mockFeedServer(t, "ConnectionPoolTest")
	defer srv.Close()

	// Test with custom connection pool settings
	store, err := NewStore(Config{
		Feeds:               []string{srv.URL},
		ExpireAfter:         1 * time.Hour,
		MaxIdleConns:        50,
		MaxConnsPerHost:     20,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     60 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify that a store was created successfully
	if store == nil {
		t.Fatal("expected store to be created successfully with connection pool settings")
	}

	// Get feeds to ensure HTTP client works with pooling settings
	results, err := store.GetAllFeeds(context.Background())
	if err != nil {
		t.Fatalf("GetAllFeeds() failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 feed result, got %d", len(results))
	}

	// Verify feed was fetched successfully
	if results[0].FetchError != "" {
		t.Errorf("expected no fetch error, got: %s", results[0].FetchError)
	}

	if results[0].Title != "ConnectionPoolTest" {
		t.Errorf("expected feed title 'ConnectionPoolTest', got %q", results[0].Title)
	}
}

func TestHTTPPoolConfig_DefaultValues(t *testing.T) {
	srv := mockFeedServer(t, "DefaultPoolTest")
	defer srv.Close()

	// Test with default values (should be set automatically)
	store, err := NewStore(Config{
		Feeds:       []string{srv.URL},
		ExpireAfter: 1 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify store creation succeeds with defaults
	if store == nil {
		t.Fatal("expected store to be created with default connection pool settings")
	}

	// Fetch feeds to ensure defaults work
	results, err := store.GetAllFeeds(context.Background())
	if err != nil {
		t.Fatalf("GetAllFeeds() failed with defaults: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 feed result, got %d", len(results))
	}

	if results[0].Title != "DefaultPoolTest" {
		t.Errorf("expected feed title 'DefaultPoolTest', got %q", results[0].Title)
	}
}

func TestNewRateLimitedHTTPClient_ConnectionPoolSettings(t *testing.T) {
	poolConfig := HTTPPoolConfig{
		MaxIdleConns:        75,
		MaxConnsPerHost:     15,
		MaxIdleConnsPerHost: 8,
		IdleConnTimeout:     120 * time.Second,
	}

	client := NewRateLimitedHTTPClient(2.0, 3, poolConfig)

	if client == nil {
		t.Fatal("expected client to be non-nil")
	}

	// Verify it's our custom transport
	rateLimitedTransport, ok := client.Transport.(*RateLimitedTransport)
	if !ok {
		t.Fatal("expected RateLimitedTransport")
	}

	// Verify the underlying transport is our custom HTTP transport
	httpTransport, ok := rateLimitedTransport.transport.(*http.Transport)
	if !ok {
		t.Fatal("expected underlying transport to be *http.Transport")
	}

	// Verify connection pool settings
	if httpTransport.MaxIdleConns != 75 {
		t.Errorf("expected MaxIdleConns to be 75, got %d", httpTransport.MaxIdleConns)
	}

	if httpTransport.MaxConnsPerHost != 15 {
		t.Errorf("expected MaxConnsPerHost to be 15, got %d", httpTransport.MaxConnsPerHost)
	}

	if httpTransport.MaxIdleConnsPerHost != 8 {
		t.Errorf("expected MaxIdleConnsPerHost to be 8, got %d", httpTransport.MaxIdleConnsPerHost)
	}

	if httpTransport.IdleConnTimeout != 120*time.Second {
		t.Errorf("expected IdleConnTimeout to be 120s, got %v", httpTransport.IdleConnTimeout)
	}
}

// Retry mechanism tests

func TestRetryMechanism_SuccessfulFetch(t *testing.T) {
	server := mockFeedServer(t, "Test Feed")
	defer server.Close()

	config := Config{
		Feeds:            []string{server.URL},
		Timeout:          5 * time.Second,
		ExpireAfter:      1 * time.Millisecond, // Force cache miss
		RetryMaxAttempts: 3,
		RetryBaseDelay:   100 * time.Millisecond,
		RetryMaxDelay:    1 * time.Second,
		RetryJitter:      false, // Disable jitter for predictable testing
	}

	store, err := NewStore(config)
	if err != nil {
		t.Fatal(err)
	}

	feeds, err := store.GetAllFeeds(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(feeds) != 1 {
		t.Fatalf("expected 1 feed, got %d", len(feeds))
	}

	if feeds[0].Title != "Test Feed" {
		t.Errorf("expected feed title 'Test Feed', got %q", feeds[0].Title)
	}
}

func TestRetryMechanism_RetriesOnFailure(t *testing.T) {
	var requestCount int64

	// Server that fails first 2 requests, succeeds on 3rd
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt64(&requestCount, 1)
		if count <= 2 {
			w.WriteHeader(http.StatusInternalServerError) // 5xx error - retryable
			return
		}

		// Success on 3rd attempt
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(`
			<rss version="2.0">
				<channel>
					<title>Retry Test Feed</title>
					<item>
						<title>Item 1</title>
						<link>http://example.com/1</link>
					</item>
				</channel>
			</rss>
		`))
	}))
	defer server.Close()

	// Disable circuit breaker to test retry mechanism in isolation
	disabled := false
	config := Config{
		Feeds:                 []string{server.URL},
		Timeout:               5 * time.Second,
		ExpireAfter:           1 * time.Hour, // Long cache to prevent double requests
		RetryMaxAttempts:      3,
		RetryBaseDelay:        50 * time.Millisecond,
		RetryMaxDelay:         1 * time.Second,
		RetryJitter:           false, // Disable jitter for predictable testing
		CircuitBreakerEnabled: &disabled,
	}

	store, err := NewStore(config)
	if err != nil {
		t.Fatal(err)
	}

	// NewStore automatically fetches feeds during initialization.
	// Reset counter to isolate GetAllFeeds behavior
	atomic.StoreInt64(&requestCount, 0)

	feeds, err := store.GetAllFeeds(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Should succeed after 3 attempts
	if len(feeds) != 1 {
		t.Fatalf("expected 1 feed, got %d", len(feeds))
	}

	if feeds[0].Title != "Retry Test Feed" {
		t.Errorf("expected feed title 'Retry Test Feed', got %q", feeds[0].Title)
	}

	// Wait for Ristretto cache to process async writes
	// Ristretto is asynchronous, so we need to give it time to actually cache the data
	time.Sleep(200 * time.Millisecond)

	// Reset counter before testing cache hit behavior
	atomic.StoreInt64(&requestCount, 0)

	// Second call to GetAllFeeds should hit cache and make 0 additional requests
	_, err = store.GetAllFeeds(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Additional delay to ensure cache operations complete
	time.Sleep(50 * time.Millisecond)

	finalCount := atomic.LoadInt64(&requestCount)
	if finalCount != 0 {
		t.Errorf("expected 0 additional requests from second GetAllFeeds (cache hit), got %d", finalCount)
	}
}

func TestRetryMechanism_ExhaustsRetries(t *testing.T) {
	var requestCount int64

	// Server that always fails with 5xx error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// Disable circuit breaker to test retry mechanism in isolation
	disabled := false
	config := Config{
		Feeds:                 []string{server.URL},
		Timeout:               5 * time.Second,
		ExpireAfter:           1 * time.Millisecond, // Force cache miss
		RetryMaxAttempts:      3,
		RetryBaseDelay:        50 * time.Millisecond,
		RetryMaxDelay:         1 * time.Second,
		RetryJitter:           false,
		CircuitBreakerEnabled: &disabled,
	}

	// Reset counter before NewStore call since it will trigger initial fetch
	atomic.StoreInt64(&requestCount, 0)

	store, err := NewStore(config)
	if err != nil {
		t.Fatal(err)
	}

	// NewStore should have made exactly 3 attempts (retries during initialization)
	initCount := atomic.LoadInt64(&requestCount)
	if initCount != 3 {
		t.Errorf("expected 3 requests during NewStore, got %d", initCount)
	}

	// Reset counter to test fresh fetch
	atomic.StoreInt64(&requestCount, 0)

	feeds, err := store.GetAllFeeds(context.Background())
	if err != nil {
		t.Fatal("expected success even with failed feeds")
	}

	// Should have a feed with error
	if len(feeds) != 1 {
		t.Fatalf("expected 1 feed, got %d", len(feeds))
	}

	if feeds[0].FetchError == "" {
		t.Error("expected fetch error, got none")
	}

	// Should have made exactly 3 more attempts
	finalCount := atomic.LoadInt64(&requestCount)
	if finalCount != 3 {
		t.Errorf("expected 3 requests during GetAllFeeds, got %d", finalCount)
	}
}

func TestRetryMechanism_NonRetryableError(t *testing.T) {
	var requestCount int64

	// Server that returns 404 (non-retryable)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		w.WriteHeader(http.StatusNotFound) // 4xx error - not retryable
	}))
	defer server.Close()

	// Disable circuit breaker to test retry mechanism in isolation
	disabled := false
	config := Config{
		Feeds:                 []string{server.URL},
		Timeout:               5 * time.Second,
		ExpireAfter:           1 * time.Millisecond, // Force cache miss
		RetryMaxAttempts:      3,
		RetryBaseDelay:        50 * time.Millisecond,
		RetryMaxDelay:         1 * time.Second,
		RetryJitter:           false,
		CircuitBreakerEnabled: &disabled,
	}

	// Reset counter before NewStore call since it will trigger initial fetch
	atomic.StoreInt64(&requestCount, 0)

	store, err := NewStore(config)
	if err != nil {
		t.Fatal(err)
	}

	// NewStore should have made only 1 attempt (4xx errors are not retryable)
	initCount := atomic.LoadInt64(&requestCount)
	if initCount != 1 {
		t.Errorf("expected 1 request during NewStore, got %d", initCount)
	}

	// Reset counter to test fresh fetch
	atomic.StoreInt64(&requestCount, 0)

	feeds, err := store.GetAllFeeds(context.Background())
	if err != nil {
		t.Fatal("expected success even with failed feeds")
	}

	// Should have a feed with error
	if len(feeds) != 1 {
		t.Fatalf("expected 1 feed, got %d", len(feeds))
	}

	if feeds[0].FetchError == "" {
		t.Error("expected fetch error, got none")
	}

	// Should have made only 1 more request (no retries for 4xx errors)
	finalCount := atomic.LoadInt64(&requestCount)
	if finalCount != 1 {
		t.Errorf("expected 1 request during GetAllFeeds, got %d", finalCount)
	}
}

func TestRetryMechanism_ExponentialBackoff(t *testing.T) {
	var requestCount int64
	var timestamps []time.Time

	// Server that always fails to test timing
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		timestamps = append(timestamps, time.Now())
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// Disable circuit breaker to test retry mechanism in isolation
	disabled := false
	config := Config{
		Feeds:                 []string{server.URL},
		Timeout:               5 * time.Second,
		ExpireAfter:           1 * time.Millisecond, // Force cache miss
		RetryMaxAttempts:      3,
		RetryBaseDelay:        100 * time.Millisecond,
		RetryMaxDelay:         10 * time.Second,
		RetryJitter:           false, // Disable jitter for timing tests
		CircuitBreakerEnabled: &disabled,
	}

	// Reset counters before NewStore call since it will trigger initial fetch
	atomic.StoreInt64(&requestCount, 0)
	timestamps = nil

	store, err := NewStore(config)
	if err != nil {
		t.Fatal(err)
	}

	// Verify NewStore made 3 attempts with proper timing
	if len(timestamps) != 3 {
		t.Fatalf("expected 3 timestamps during NewStore, got %d", len(timestamps))
	}

	// Reset for GetAllFeeds test
	atomic.StoreInt64(&requestCount, 0)
	timestamps = nil

	startTime := time.Now()
	feeds, err := store.GetAllFeeds(context.Background())
	if err != nil {
		t.Fatal("expected success even with failed feeds")
	}

	// Should have a feed with error
	if len(feeds) != 1 {
		t.Fatalf("expected 1 feed, got %d", len(feeds))
	}

	// Verify timing - should have delays of ~100ms, ~200ms between attempts
	if len(timestamps) != 3 {
		t.Fatalf("expected 3 timestamps during GetAllFeeds, got %d", len(timestamps))
	}

	// First delay should be ~100ms (base delay)
	firstDelay := timestamps[1].Sub(timestamps[0])
	if firstDelay < 90*time.Millisecond || firstDelay > 150*time.Millisecond {
		t.Errorf("expected first delay ~100ms, got %v", firstDelay)
	}

	// Second delay should be ~200ms (base delay * 2)
	secondDelay := timestamps[2].Sub(timestamps[1])
	if secondDelay < 180*time.Millisecond || secondDelay > 250*time.Millisecond {
		t.Errorf("expected second delay ~200ms, got %v", secondDelay)
	}

	// Total time should be at least 300ms
	totalTime := time.Since(startTime)
	if totalTime < 300*time.Millisecond {
		t.Errorf("expected total time >= 300ms, got %v", totalTime)
	}
}

func TestRetryMechanism_MaxDelayRespected(t *testing.T) {
	var requestCount int64
	var timestamps []time.Time

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		timestamps = append(timestamps, time.Now())
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := Config{
		Feeds:            []string{server.URL},
		Timeout:          5 * time.Second,
		ExpireAfter:      1 * time.Millisecond,
		RetryMaxAttempts: 4, // More attempts to test max delay
		RetryBaseDelay:   100 * time.Millisecond,
		RetryMaxDelay:    200 * time.Millisecond, // Small max delay
		RetryJitter:      false,
	}

	store, err := NewStore(config)
	if err != nil {
		t.Fatal(err)
	}

	store.GetAllFeeds(context.Background())

	// Verify that delays are capped at max delay
	if len(timestamps) < 3 {
		t.Fatalf("expected at least 3 timestamps, got %d", len(timestamps))
	}

	// Third delay should be capped at max delay (~200ms), not 400ms
	if len(timestamps) >= 4 {
		thirdDelay := timestamps[3].Sub(timestamps[2])
		if thirdDelay > 250*time.Millisecond {
			t.Errorf("expected third delay <= 250ms (max delay + tolerance), got %v", thirdDelay)
		}
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name      string
		error     string
		retryable bool
	}{
		{"nil error", "", false},
		{"5xx server error", "HTTP error: status code 500", true},
		{"502 bad gateway", "status code 502", true},
		{"gofeed 5xx error", "http error: 500 Internal Server Error", true},
		{"4xx client error", "status code 404", false},
		{"401 unauthorized", "status code 401", false},
		{"gofeed 4xx error", "http error: 404 Not Found", false},
		{"DNS error", "no such host", true},
		{"connection refused", "connection refused", true},
		{"connection reset", "connection reset", true},
		{"network unreachable", "network unreachable", true},
		{"timeout", "timeout", true},
		{"i/o timeout", "i/o timeout", true},
		{"unknown error", "some other error", true}, // Default to retryable
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.error != "" {
				err = &testError{msg: tt.error}
			}

			result := isRetryableError(err)
			if result != tt.retryable {
				t.Errorf("expected %v, got %v for error: %q", tt.retryable, result, tt.error)
			}
		})
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestCalculateRetryDelay(t *testing.T) {
	tests := []struct {
		name        string
		attempt     int
		baseDelay   time.Duration
		maxDelay    time.Duration
		useJitter   bool
		minExpected time.Duration
		maxExpected time.Duration
	}{
		{"first attempt", 1, 100 * time.Millisecond, 10 * time.Second, false, 100 * time.Millisecond, 100 * time.Millisecond},
		{"second attempt", 2, 100 * time.Millisecond, 10 * time.Second, false, 200 * time.Millisecond, 200 * time.Millisecond},
		{"third attempt", 3, 100 * time.Millisecond, 10 * time.Second, false, 400 * time.Millisecond, 400 * time.Millisecond},
		{"capped by max delay", 10, 100 * time.Millisecond, 1 * time.Second, false, 1 * time.Second, 1 * time.Second},
		{"zero attempt", 0, 100 * time.Millisecond, 10 * time.Second, false, 100 * time.Millisecond, 100 * time.Millisecond},
		{"with jitter", 2, 100 * time.Millisecond, 10 * time.Second, true, 100 * time.Millisecond, 300 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := calculateRetryDelay(tt.attempt, tt.baseDelay, tt.maxDelay, tt.useJitter)

			if delay < tt.minExpected || delay > tt.maxExpected {
				t.Errorf("expected delay between %v and %v, got %v", tt.minExpected, tt.maxExpected, delay)
			}
		})
	}
}

func TestRetryMechanism_DefaultConfiguration(t *testing.T) {
	var requestCount int64

	// Server that fails first 2 requests in each batch, succeeds on 3rd to test default retry count
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt64(&requestCount, 1)
		// Fail first 2 requests in each "batch" of 3, succeed on 3rd
		if (count-1)%3 < 2 {
			w.WriteHeader(http.StatusInternalServerError) // 5xx error - retryable
			return
		}

		// Success on every 3rd attempt (default retry count)
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(`
			<rss version="2.0">
				<channel>
					<title>Default Config Feed</title>
					<item>
						<title>Item 1</title>
						<link>http://example.com/1</link>
					</item>
				</channel>
			</rss>
		`))
	}))
	defer server.Close()

	// Test that defaults work by creating a store with minimal config
	// Disable circuit breaker to test retry mechanism in isolation
	disabled := false
	config := Config{
		Feeds:                 []string{server.URL},
		ExpireAfter:           1 * time.Millisecond, // Force cache miss
		CircuitBreakerEnabled: &disabled,
		// Don't set retry values to test defaults (should be 3 attempts, 1s base delay)
	}

	// Reset counter
	atomic.StoreInt64(&requestCount, 0)

	store, err := NewStore(config)
	if err != nil {
		t.Fatal(err)
	}

	// NewStore should succeed after 3 attempts (default retry max attempts)
	initCount := atomic.LoadInt64(&requestCount)
	if initCount != 3 {
		t.Errorf("expected 3 requests during NewStore with defaults, got %d", initCount)
	}

	// Verify the store was created successfully and defaults worked
	feeds, err := store.GetAllFeeds(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(feeds) != 1 {
		t.Fatalf("expected 1 feed, got %d", len(feeds))
	}

	if feeds[0].Title != "Default Config Feed" {
		t.Errorf("expected feed title 'Default Config Feed', got %q", feeds[0].Title)
	}

	// This test primarily verifies that the default retry configuration
	// allows the store to succeed after 3 attempts during initialization
}

// Retry metrics tests

func TestRetryMetrics_SuccessfulFeeds(t *testing.T) {
	server := mockFeedServer(t, "Metrics Test Feed")
	defer server.Close()

	config := Config{
		Feeds:            []string{server.URL},
		Timeout:          5 * time.Second,
		ExpireAfter:      1 * time.Millisecond, // Force cache miss
		RetryMaxAttempts: 3,
		RetryBaseDelay:   50 * time.Millisecond,
		RetryMaxDelay:    1 * time.Second,
		RetryJitter:      false,
	}

	store, err := NewStore(config)
	if err != nil {
		t.Fatal(err)
	}

	// Get initial metrics
	initialMetrics := store.GetRetryMetrics()

	// Should have 1 successful feed from initialization
	if initialMetrics.SuccessfulFeeds != 1 {
		t.Errorf("expected 1 successful feed in initial metrics, got %d", initialMetrics.SuccessfulFeeds)
	}
	if initialMetrics.TotalAttempts != 1 {
		t.Errorf("expected 1 total attempt in initial metrics, got %d", initialMetrics.TotalAttempts)
	}
	if initialMetrics.TotalRetries != 0 {
		t.Errorf("expected 0 retries in initial metrics, got %d", initialMetrics.TotalRetries)
	}
	if initialMetrics.RetrySuccessRate != 100.0 {
		t.Errorf("expected 100%% success rate in initial metrics, got %f", initialMetrics.RetrySuccessRate)
	}

	// Wait for cache to expire then fetch again to trigger cache miss
	time.Sleep(10 * time.Millisecond) // Wait for cache expiration
	feeds, err := store.GetAllFeeds(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(feeds) != 1 {
		t.Fatalf("expected 1 feed, got %d", len(feeds))
	}

	// Get final metrics
	finalMetrics := store.GetRetryMetrics()

	// Should have 2 successful feeds total
	if finalMetrics.SuccessfulFeeds != 2 {
		t.Errorf("expected 2 successful feeds in final metrics, got %d", finalMetrics.SuccessfulFeeds)
	}
	if finalMetrics.TotalAttempts != 2 {
		t.Errorf("expected 2 total attempts in final metrics, got %d", finalMetrics.TotalAttempts)
	}
	if finalMetrics.TotalRetries != 0 {
		t.Errorf("expected 0 retries in final metrics, got %d", finalMetrics.TotalRetries)
	}
	if finalMetrics.RetrySuccessRate != 100.0 {
		t.Errorf("expected 100%% success rate in final metrics, got %f", finalMetrics.RetrySuccessRate)
	}
}

func TestRetryMetrics_WithRetries(t *testing.T) {
	var requestCount int64

	// Server that fails first 2 requests, succeeds on 3rd
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt64(&requestCount, 1)
		if count <= 2 {
			w.WriteHeader(http.StatusInternalServerError) // 5xx error - retryable
			return
		}

		// Success on 3rd attempt
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(`
			<rss version="2.0">
				<channel>
					<title>Retry Metrics Test Feed</title>
					<item>
						<title>Item 1</title>
						<link>http://example.com/1</link>
					</item>
				</channel>
			</rss>
		`))
	}))
	defer server.Close()

	// Disable circuit breaker to test retry mechanism in isolation
	disabled := false
	config := Config{
		Feeds:                 []string{server.URL},
		Timeout:               5 * time.Second,
		ExpireAfter:           1 * time.Millisecond, // Force cache miss
		RetryMaxAttempts:      3,
		RetryBaseDelay:        50 * time.Millisecond,
		RetryMaxDelay:         1 * time.Second,
		RetryJitter:           false,
		CircuitBreakerEnabled: &disabled,
	}

	// Reset counter
	atomic.StoreInt64(&requestCount, 0)

	store, err := NewStore(config)
	if err != nil {
		t.Fatal(err)
	}

	// Get metrics after initialization
	metrics := store.GetRetryMetrics()

	// Should have 1 successful feed with retries
	if metrics.SuccessfulFeeds != 1 {
		t.Errorf("expected 1 successful feed, got %d", metrics.SuccessfulFeeds)
	}
	if metrics.TotalAttempts != 3 {
		t.Errorf("expected 3 total attempts, got %d", metrics.TotalAttempts)
	}
	if metrics.TotalRetries != 2 {
		t.Errorf("expected 2 retries, got %d", metrics.TotalRetries)
	}
	if metrics.FailedFeeds != 0 {
		t.Errorf("expected 0 failed feeds, got %d", metrics.FailedFeeds)
	}
	if metrics.RetrySuccessRate != 100.0 {
		t.Errorf("expected 100%% success rate, got %f", metrics.RetrySuccessRate)
	}
}

func TestRetryMetrics_FailedFeeds(t *testing.T) {
	var requestCount int64

	// Server that always fails with 5xx error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// Disable circuit breaker to test retry mechanism in isolation
	disabled := false
	config := Config{
		Feeds:                 []string{server.URL},
		Timeout:               5 * time.Second,
		ExpireAfter:           1 * time.Millisecond, // Force cache miss
		RetryMaxAttempts:      3,
		RetryBaseDelay:        50 * time.Millisecond,
		RetryMaxDelay:         1 * time.Second,
		RetryJitter:           false,
		CircuitBreakerEnabled: &disabled,
	}

	// Reset counter
	atomic.StoreInt64(&requestCount, 0)

	store, err := NewStore(config)
	if err != nil {
		t.Fatal(err)
	}

	// Get metrics after initialization
	metrics := store.GetRetryMetrics()

	// Should have 1 failed feed with retries
	if metrics.SuccessfulFeeds != 0 {
		t.Errorf("expected 0 successful feeds, got %d", metrics.SuccessfulFeeds)
	}
	if metrics.TotalAttempts != 3 {
		t.Errorf("expected 3 total attempts, got %d", metrics.TotalAttempts)
	}
	if metrics.TotalRetries != 2 {
		t.Errorf("expected 2 retries, got %d", metrics.TotalRetries)
	}
	if metrics.FailedFeeds != 1 {
		t.Errorf("expected 1 failed feed, got %d", metrics.FailedFeeds)
	}
	if metrics.RetrySuccessRate != 0.0 {
		t.Errorf("expected 0%% success rate, got %f", metrics.RetrySuccessRate)
	}
}

func TestRetryMetrics_MixedResults(t *testing.T) {
	// Create one working server and one failing server
	workingServer := mockFeedServer(t, "Working Feed")
	defer workingServer.Close()

	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failingServer.Close()

	// Disable circuit breaker to test retry mechanism in isolation
	disabled := false
	config := Config{
		Feeds:                 []string{workingServer.URL, failingServer.URL},
		Timeout:               5 * time.Second,
		ExpireAfter:           1 * time.Millisecond, // Force cache miss
		RetryMaxAttempts:      3,
		RetryBaseDelay:        50 * time.Millisecond,
		RetryMaxDelay:         1 * time.Second,
		RetryJitter:           false,
		CircuitBreakerEnabled: &disabled,
	}

	store, err := NewStore(config)
	if err != nil {
		t.Fatal(err)
	}

	// Get metrics after initialization
	metrics := store.GetRetryMetrics()

	// Should have mixed results
	if metrics.SuccessfulFeeds != 1 {
		t.Errorf("expected 1 successful feed, got %d", metrics.SuccessfulFeeds)
	}
	if metrics.FailedFeeds != 1 {
		t.Errorf("expected 1 failed feed, got %d", metrics.FailedFeeds)
	}
	// Working server: 1 attempt, failing server: 3 attempts
	if metrics.TotalAttempts != 4 {
		t.Errorf("expected 4 total attempts, got %d", metrics.TotalAttempts)
	}
	// Failing server: 2 retries
	if metrics.TotalRetries != 2 {
		t.Errorf("expected 2 retries, got %d", metrics.TotalRetries)
	}
	// 1 success out of 2 feeds = 50%
	expectedSuccessRate := 50.0
	if metrics.RetrySuccessRate != expectedSuccessRate {
		t.Errorf("expected %.1f%% success rate, got %f", expectedSuccessRate, metrics.RetrySuccessRate)
	}
}
