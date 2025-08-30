package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/richardwooding/feed-mcp/mcpserver"
)

// TestDynamicStore_NewDynamicStore tests the creation of a new dynamic store
func TestDynamicStore_NewDynamicStore(t *testing.T) {
	config := Config{
		Feeds:       []string{"https://example.com/feed.xml"},
		Timeout:     30 * time.Second,
		ExpireAfter: 1 * time.Hour,
	}

	// Test creation with runtime feeds allowed
	ds, err := NewDynamicStore(&config, true)
	if err != nil {
		t.Fatalf("Failed to create dynamic store: %v", err)
	}

	if ds.allowRuntimeFeeds != true {
		t.Error("Expected allowRuntimeFeeds to be true")
	}

	if len(ds.feedMetadata) == 0 {
		t.Error("Expected feed metadata to be initialized for startup feeds")
	}
}

// TestDynamicStore_AddFeed tests adding a feed at runtime
func TestDynamicStore_AddFeed(t *testing.T) {
	config := Config{
		Feeds:       []string{},
		Timeout:     30 * time.Second,
		ExpireAfter: 1 * time.Hour,
	}

	ds, err := NewDynamicStore(&config, true)
	if err != nil {
		t.Fatalf("Failed to create dynamic store: %v", err)
	}

	ctx := context.Background()
	feedConfig := mcpserver.FeedConfig{
		URL:         "https://example.com/new-feed.xml",
		Title:       "Test Feed",
		Category:    "test",
		Description: "A test feed",
	}

	// Test adding a feed - this will fail since we can't actually fetch it
	// but we can test the validation logic
	_, err = ds.AddFeed(ctx, feedConfig)
	if err != nil {
		// Expected to fail due to network fetch, but error should be from fetching, not validation
		if !strings.Contains(err.Error(), "URL") && !strings.Contains(err.Error(), "fetch") && !strings.Contains(err.Error(), "network") {
			t.Errorf("Unexpected error type: %v", err)
		}
	}
}

// TestDynamicStore_AddFeed_RuntimeDisabled tests that add feed fails when runtime feeds are disabled
func TestDynamicStore_AddFeed_RuntimeDisabled(t *testing.T) {
	config := Config{
		Feeds:       []string{"https://example.com/initial-feed.xml"}, // Need at least one feed when runtime disabled
		Timeout:     30 * time.Second,
		ExpireAfter: 1 * time.Hour,
	}

	ds, err := NewDynamicStore(&config, false)
	if err != nil {
		t.Fatalf("Failed to create dynamic store: %v", err)
	}

	ctx := context.Background()
	feedConfig := mcpserver.FeedConfig{
		URL:         "https://example.com/new-feed.xml",
		Title:       "Test Feed",
		Category:    "test",
		Description: "A test feed",
	}

	_, err = ds.AddFeed(ctx, feedConfig)
	if err == nil {
		t.Error("Expected AddFeed to fail when runtime feeds are disabled")
	}

	if !strings.Contains(err.Error(), "runtime feed management is not enabled") {
		t.Errorf("Expected runtime management disabled error, got: %v", err)
	}
}

// TestDynamicStore_ListManagedFeeds tests listing managed feeds
func TestDynamicStore_ListManagedFeeds(t *testing.T) {
	config := Config{
		Feeds:       []string{"https://example.com/startup-feed.xml"},
		Timeout:     30 * time.Second,
		ExpireAfter: 1 * time.Hour,
	}

	ds, err := NewDynamicStore(&config, true)
	if err != nil {
		t.Fatalf("Failed to create dynamic store: %v", err)
	}

	ctx := context.Background()
	feeds, err := ds.ListManagedFeeds(ctx)
	if err != nil {
		t.Fatalf("Failed to list managed feeds: %v", err)
	}

	if len(feeds) != 1 {
		t.Errorf("Expected 1 startup feed, got %d", len(feeds))
	}

	feed := feeds[0]
	if feed.URL != "https://example.com/startup-feed.xml" {
		t.Errorf("Expected startup feed URL, got %s", feed.URL)
	}

	if feed.Source != "startup" {
		t.Errorf("Expected source 'startup', got %s", feed.Source)
	}
}
