package store

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

	// Wait for goroutines to finish (simulate, since feeds map is filled async)
	time.Sleep(200 * time.Millisecond)

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
	time.Sleep(200 * time.Millisecond)

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
	time.Sleep(200 * time.Millisecond)

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
