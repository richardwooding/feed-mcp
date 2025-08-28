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

func TestStore_CustomHttpClientPreserved(t *testing.T) {
	srv := mockFeedServer(t, "CustomClientTest")
	defer srv.Close()

	// Create custom HTTP client
	customClient := &http.Client{Timeout: 5 * time.Second}

	// Create store with custom HTTP client - rate limiting should be skipped
	store, err := NewStore(Config{
		Feeds:             []string{srv.URL},
		HttpClient:        customClient,
		RequestsPerSecond: 10.0, // These should be ignored since HttpClient is provided
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

	// Create store with circuit breaker enabled
	enabled := true
	store, err := NewStore(Config{
		Feeds:                 []string{recoveringServer.URL},
		CircuitBreakerEnabled: &enabled,
		CircuitBreakerTimeout: 1 * time.Second, // Short timeout for quick recovery
		ExpireAfter:           1 * time.Millisecond, // Force cache expiry
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
		CircuitBreakerFailureThreshold: 2, // Should open after 2 failures instead of default 3
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
		Feeds:                []string{srv.URL},
		ExpireAfter:          1 * time.Hour,
		MaxIdleConns:         50,
		MaxConnsPerHost:      20,
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
