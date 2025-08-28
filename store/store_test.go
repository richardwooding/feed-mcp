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
	client := NewRateLimitedHTTPClient(1.0, 2)

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
	client := NewRateLimitedHTTPClient(1.0, 1)

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
