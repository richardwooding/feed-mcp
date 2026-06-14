package store

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/richardwooding/feed-mcp/mcpserver"
)

// rssFeedServer starts a test server serving a minimal valid RSS feed with the
// given channel title. The server is closed automatically when the test ends.
func rssFeedServer(t *testing.T, title string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<rss version="2.0"><channel><title>` + title +
			`</title><item><title>item1</title><link>http://example.com/1</link></item></channel></rss>`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newRuntimeStore creates a DynamicStore with runtime feed management enabled
// and no startup feeds.
func newRuntimeStore(t *testing.T) *DynamicStore {
	t.Helper()
	ds, err := NewDynamicStore(&Config{Feeds: []string{}, AllowPrivateIPs: true}, true)
	if err != nil {
		t.Fatalf("NewDynamicStore: %v", err)
	}
	return ds
}

// addRuntimeFeed adds a runtime feed and returns its generated feed ID.
func addRuntimeFeed(t *testing.T, ds *DynamicStore, url string) string {
	t.Helper()
	info, err := ds.AddFeed(context.Background(), mcpserver.FeedConfig{URL: url})
	if err != nil {
		t.Fatalf("AddFeed(%s): %v", url, err)
	}
	return info.FeedID
}

func TestDynamicStore_RemoveFeed_Success(t *testing.T) {
	srv := rssFeedServer(t, "Removable Feed")
	ds := newRuntimeStore(t)
	ctx := context.Background()
	feedID := addRuntimeFeed(t, ds, srv.URL)

	removed, err := ds.RemoveFeed(ctx, feedID)
	if err != nil {
		t.Fatalf("RemoveFeed: %v", err)
	}
	if removed.FeedID != feedID {
		t.Errorf("FeedID = %q, want %q", removed.FeedID, feedID)
	}
	if removed.URL != srv.URL {
		t.Errorf("URL = %q, want %q", removed.URL, srv.URL)
	}

	// The feed must be gone from every internal map.
	if _, ok := ds.feeds[feedID]; ok {
		t.Error("feed still present in ds.feeds after removal")
	}
	if _, ok := ds.dynamicFeeds[feedID]; ok {
		t.Error("feed still present in ds.dynamicFeeds after removal")
	}
	if _, ok := ds.feedMetadata[feedID]; ok {
		t.Error("metadata still present after removal")
	}
}

func TestDynamicStore_RemoveFeed_RuntimeDisabled(t *testing.T) {
	ds, err := NewDynamicStore(&Config{Feeds: []string{"https://example.com/feed.xml"}}, false)
	if err != nil {
		t.Fatalf("NewDynamicStore: %v", err)
	}

	_, err = ds.RemoveFeed(context.Background(), "any-id")
	if err == nil {
		t.Fatal("expected error when runtime feeds are disabled")
	}
	if !strings.Contains(err.Error(), "runtime feed management is not enabled") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDynamicStore_RemoveFeed_NotFound(t *testing.T) {
	ds := newRuntimeStore(t)

	_, err := ds.RemoveFeed(context.Background(), "missing-id")
	if err == nil {
		t.Fatal("expected error for unknown feed ID")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDynamicStore_RemoveFeed_StartupFeedRejected(t *testing.T) {
	srv := rssFeedServer(t, "Startup Feed")
	// Runtime management enabled, but the feed is a startup feed (not runtime).
	ds, err := NewDynamicStore(&Config{Feeds: []string{srv.URL}, AllowPrivateIPs: true}, true)
	if err != nil {
		t.Fatalf("NewDynamicStore: %v", err)
	}

	var feedID string
	for id := range ds.feeds {
		feedID = id
	}

	_, err = ds.RemoveFeed(context.Background(), feedID)
	if err == nil {
		t.Fatal("expected error removing a startup feed")
	}
	if !strings.Contains(err.Error(), "cannot remove") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDynamicStore_RemoveFeedByURL_Success(t *testing.T) {
	srv := rssFeedServer(t, "By URL")
	ds := newRuntimeStore(t)
	ctx := context.Background()
	feedID := addRuntimeFeed(t, ds, srv.URL)

	removed, err := ds.RemoveFeedByURL(ctx, srv.URL)
	if err != nil {
		t.Fatalf("RemoveFeedByURL: %v", err)
	}
	if removed.FeedID != feedID {
		t.Errorf("FeedID = %q, want %q", removed.FeedID, feedID)
	}
	if _, ok := ds.feeds[feedID]; ok {
		t.Error("feed still present after RemoveFeedByURL")
	}
}

func TestDynamicStore_RemoveFeedByURL_NotFound(t *testing.T) {
	ds := newRuntimeStore(t)

	_, err := ds.RemoveFeedByURL(context.Background(), "https://example.com/never-added.xml")
	if err == nil {
		t.Fatal("expected error for unknown URL")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDynamicStore_UpdateFeedMetadata_Success(t *testing.T) {
	srv := rssFeedServer(t, "Original Title")
	ds := newRuntimeStore(t)
	ctx := context.Background()
	feedID := addRuntimeFeed(t, ds, srv.URL)

	err := ds.UpdateFeedMetadata(ctx, feedID, mcpserver.FeedMetadata{
		Title:       "Updated Title",
		Category:    "news",
		Description: "Updated description",
	})
	if err != nil {
		t.Fatalf("UpdateFeedMetadata: %v", err)
	}

	md := ds.feedMetadata[feedID]
	if md.Title != "Updated Title" {
		t.Errorf("Title = %q, want %q", md.Title, "Updated Title")
	}
	if md.Category != "news" {
		t.Errorf("Category = %q, want %q", md.Category, "news")
	}
	if md.Description != "Updated description" {
		t.Errorf("Description = %q, want %q", md.Description, "Updated description")
	}
}

func TestDynamicStore_UpdateFeedMetadata_EmptyFieldsPreserved(t *testing.T) {
	srv := rssFeedServer(t, "Keep Me")
	ds := newRuntimeStore(t)
	ctx := context.Background()
	feedID := addRuntimeFeed(t, ds, srv.URL)

	// Seed a known title/category, then update with empty fields.
	ds.feedMetadata[feedID].Title = "Keep Title"
	ds.feedMetadata[feedID].Category = "keep-cat"

	if err := ds.UpdateFeedMetadata(ctx, feedID, mcpserver.FeedMetadata{Description: "only desc"}); err != nil {
		t.Fatalf("UpdateFeedMetadata: %v", err)
	}

	md := ds.feedMetadata[feedID]
	if md.Title != "Keep Title" {
		t.Errorf("Title overwritten by empty value: %q", md.Title)
	}
	if md.Category != "keep-cat" {
		t.Errorf("Category overwritten by empty value: %q", md.Category)
	}
	if md.Description != "only desc" {
		t.Errorf("Description = %q, want %q", md.Description, "only desc")
	}
}

func TestDynamicStore_UpdateFeedMetadata_NotFound(t *testing.T) {
	ds := newRuntimeStore(t)

	err := ds.UpdateFeedMetadata(context.Background(), "missing-id", mcpserver.FeedMetadata{Title: "x"})
	if err == nil {
		t.Fatal("expected error for unknown feed ID")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDynamicStore_RefreshFeed_Success(t *testing.T) {
	srv := rssFeedServer(t, "Refreshable Feed")
	ds := newRuntimeStore(t)
	ctx := context.Background()
	feedID := addRuntimeFeed(t, ds, srv.URL)

	info, err := ds.RefreshFeed(ctx, feedID)
	if err != nil {
		t.Fatalf("RefreshFeed: %v", err)
	}
	if info.Status != statusRefreshed {
		t.Fatalf("Status = %q, want %q", info.Status, statusRefreshed)
	}
	if info.ItemsAdded != 1 {
		t.Errorf("ItemsAdded = %d, want 1", info.ItemsAdded)
	}

	// Metadata should reflect a healthy refresh.
	md := ds.feedMetadata[feedID]
	if md.Status != statusActive {
		t.Errorf("metadata status = %q, want %q", md.Status, statusActive)
	}
	if md.LastError != "" {
		t.Errorf("metadata LastError = %q, want empty", md.LastError)
	}
	if md.Title != "Refreshable Feed" {
		t.Errorf("metadata Title = %q, want %q", md.Title, "Refreshable Feed")
	}
}

func TestDynamicStore_PauseResumeFeed(t *testing.T) {
	srv := rssFeedServer(t, "Pausable")
	ds := newRuntimeStore(t)
	ctx := context.Background()
	feedID := addRuntimeFeed(t, ds, srv.URL)

	if err := ds.PauseFeed(ctx, feedID); err != nil {
		t.Fatalf("PauseFeed: %v", err)
	}
	if got := ds.feedMetadata[feedID].Status; got != "paused" {
		t.Errorf("status after pause = %q, want %q", got, "paused")
	}

	if err := ds.ResumeFeed(ctx, feedID); err != nil {
		t.Fatalf("ResumeFeed: %v", err)
	}
	if got := ds.feedMetadata[feedID].Status; got != statusActive {
		t.Errorf("status after resume = %q, want %q", got, statusActive)
	}
}

func TestDynamicStore_PauseFeed_NotFound(t *testing.T) {
	ds := newRuntimeStore(t)

	if err := ds.PauseFeed(context.Background(), "missing-id"); err == nil {
		t.Fatal("expected error for unknown feed ID")
	}
}

func TestDynamicStore_ResumeFeed_NotFound(t *testing.T) {
	ds := newRuntimeStore(t)

	if err := ds.ResumeFeed(context.Background(), "missing-id"); err == nil {
		t.Fatal("expected error for unknown feed ID")
	}
}
