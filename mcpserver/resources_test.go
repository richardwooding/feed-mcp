package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/richardwooding/feed-mcp/model"
)

// mockResourceAllFeedsGetter is a mock implementation of AllFeedsGetter for resource testing
type mockResourceAllFeedsGetter struct {
	feeds []*model.FeedResult
	err   error
}

func (m *mockResourceAllFeedsGetter) GetAllFeeds(ctx context.Context) ([]*model.FeedResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.feeds, nil
}

// mockResourceFeedAndItemsGetter is a mock implementation of FeedAndItemsGetter for resource testing  
type mockResourceFeedAndItemsGetter struct {
	feeds map[string]*model.FeedAndItemsResult
	err   error
}

func (m *mockResourceFeedAndItemsGetter) GetFeedAndItems(ctx context.Context, feedID string) (*model.FeedAndItemsResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	if feed, exists := m.feeds[feedID]; exists {
		return feed, nil
	}
	return nil, model.NewFeedError(model.ErrorTypeValidation, "Feed not found")
}

// TestResourceManager tests the core ResourceManager functionality
func TestResourceManager(t *testing.T) {
	// Create mock feeds
	mockFeeds := []*model.FeedResult{
		{
			ID:        "test-feed-1",
			Title:     "Test Feed 1",
			PublicURL: "https://example.com/feed1.xml",
			Feed: &model.Feed{
				Title:       "Test Feed 1",
				Description: "A test feed",
				Link:        "https://example.com",
				FeedType:    "rss",
			},
		},
		{
			ID:        "test-feed-2", 
			Title:     "Test Feed 2",
			PublicURL: "https://example.com/feed2.xml",
			Feed: &model.Feed{
				Title:       "Test Feed 2",
				Description: "Another test feed",
				Link:        "https://example2.com",
				FeedType:    "atom",
			},
		},
	}

	mockAllFeeds := &mockResourceAllFeedsGetter{feeds: mockFeeds}
	feedsMap := make(map[string]*model.FeedAndItemsResult)
	for _, feed := range mockFeeds {
		feedID := generateFeedID(feed.PublicURL)
		// Convert FeedResult to FeedAndItemsResult
		feedAndItems := &model.FeedAndItemsResult{
			ID:                 feed.ID,
			PublicURL:          feed.PublicURL,
			Title:              feed.Title,
			FetchError:         feed.FetchError,
			Feed:               feed.Feed,
			Items:              nil, // No items for now
			CircuitBreakerOpen: feed.CircuitBreakerOpen,
		}
		feedsMap[feedID] = feedAndItems
	}
	mockFeedGetter := &mockResourceFeedAndItemsGetter{feeds: feedsMap}

	rm := NewResourceManager(mockAllFeeds, mockFeedGetter)

	t.Run("ListResources", func(t *testing.T) {
		ctx := context.Background()
		resources, err := rm.ListResources(ctx)

		if err != nil {
			t.Fatalf("ListResources failed: %v", err)
		}

		// Should have 1 feed list resource + 3 resources per feed (feed, items, meta) * 2 feeds = 7 total
		expectedCount := 1 + (3 * 2)
		if len(resources) != expectedCount {
			t.Errorf("Expected %d resources, got %d", expectedCount, len(resources))
		}

		// Check feed list resource
		foundFeedList := false
		for _, resource := range resources {
			if resource.URI == FeedListURI {
				foundFeedList = true
				if resource.Name != "All Feeds" {
					t.Errorf("Expected feed list name 'All Feeds', got '%s'", resource.Name)
				}
				if resource.MIMEType != "application/json" {
					t.Errorf("Expected MIME type 'application/json', got '%s'", resource.MIMEType)
				}
				break
			}
		}
		if !foundFeedList {
			t.Error("Feed list resource not found")
		}
	})

	t.Run("ReadFeedListResource", func(t *testing.T) {
		ctx := context.Background()
		result, err := rm.ReadResource(ctx, FeedListURI)

		if err != nil {
			t.Fatalf("ReadResource for feed list failed: %v", err)
		}

		if len(result.Contents) != 1 {
			t.Fatalf("Expected 1 content item, got %d", len(result.Contents))
		}

		content := result.Contents[0]
		if content.URI != FeedListURI {
			t.Errorf("Expected URI %s, got %s", FeedListURI, content.URI)
		}

		if content.MIMEType != "application/json" {
			t.Errorf("Expected MIME type 'application/json', got %s", content.MIMEType)
		}

		// Check that the JSON content contains feed data
		if !strings.Contains(content.Text, "Test Feed 1") {
			t.Error("Feed list content should contain 'Test Feed 1'")
		}
		if !strings.Contains(content.Text, "feeds") {
			t.Error("Feed list content should contain 'feeds' field")
		}
	})

	t.Run("ReadIndividualFeedResource", func(t *testing.T) {
		ctx := context.Background()
		feedID := generateFeedID("https://example.com/feed1.xml")
		uri := expandURITemplate(FeedURI, map[string]string{"feedId": feedID})
		
		result, err := rm.ReadResource(ctx, uri)

		if err != nil {
			t.Fatalf("ReadResource for individual feed failed: %v", err)
		}

		if len(result.Contents) != 1 {
			t.Fatalf("Expected 1 content item, got %d", len(result.Contents))
		}

		content := result.Contents[0]
		if content.URI != uri {
			t.Errorf("Expected URI %s, got %s", uri, content.URI)
		}

		// Check that the JSON content contains the full feed data
		if !strings.Contains(content.Text, "Test Feed 1") {
			t.Error("Feed content should contain 'Test Feed 1'")
		}
	})

	t.Run("ReadFeedMetadataResource", func(t *testing.T) {
		ctx := context.Background()
		feedID := generateFeedID("https://example.com/feed1.xml")
		uri := expandURITemplate(FeedMetaURI, map[string]string{"feedId": feedID})
		
		result, err := rm.ReadResource(ctx, uri)

		if err != nil {
			t.Fatalf("ReadResource for feed metadata failed: %v", err)
		}

		if len(result.Contents) != 1 {
			t.Fatalf("Expected 1 content item, got %d", len(result.Contents))
		}

		content := result.Contents[0]
		// Check that metadata contains expected fields
		if !strings.Contains(content.Text, "description") {
			t.Error("Metadata should contain 'description' field")
		}
		if !strings.Contains(content.Text, "public_url") {
			t.Error("Metadata should contain 'public_url' field")
		}
	})

	t.Run("InvalidResourceURI", func(t *testing.T) {
		ctx := context.Background()
		_, err := rm.ReadResource(ctx, "invalid://uri")

		if err == nil {
			t.Error("Expected error for invalid URI, but got none")
		}

		// Check that it contains the expected error message
		if !strings.Contains(err.Error(), "Unknown resource URI") {
			t.Error("Expected 'Unknown resource URI' error message")
		}
	})
}

// TestURITemplates tests the URI template helper functions
func TestURITemplates(t *testing.T) {
	t.Run("expandURITemplate", func(t *testing.T) {
		template := "feeds://feed/{feedId}/items"
		params := map[string]string{"feedId": "test-feed"}
		expected := "feeds://feed/test-feed/items"
		
		result := expandURITemplate(template, params)
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("matchesTemplate", func(t *testing.T) {
		template := "feeds://feed/{feedId}"
		
		// Test matching URI
		uri := "feeds://feed/test-feed"
		if !matchesTemplate(uri, template) {
			t.Error("URI should match template")
		}

		// Test non-matching URI
		uri = "feeds://invalid/test-feed"
		if matchesTemplate(uri, template) {
			t.Error("URI should not match template")
		}
	})

	t.Run("extractFeedIDFromURI", func(t *testing.T) {
		template := "feeds://feed/{feedId}/items"
		uri := "feeds://feed/test-feed/items"
		
		feedID, err := extractFeedIDFromURI(uri, template)
		if err != nil {
			t.Fatalf("extractFeedIDFromURI failed: %v", err)
		}

		if feedID != "test-feed" {
			t.Errorf("Expected feedID 'test-feed', got '%s'", feedID)
		}
	})
}

// TestGenerateFeedID tests the feed ID generation
func TestGenerateFeedID(t *testing.T) {
	testCases := []struct {
		name        string
		url         string
		expectSlug  bool
		expectHash  bool
	}{
		{
			name:       "Simple domain",
			url:        "https://example.com/feed.xml",
			expectSlug: true,
		},
		{
			name:       "Domain with path",
			url:        "https://techcrunch.com/feed/",
			expectSlug: true,
		},
		{
			name:       "Complex path",
			url:        "https://www.reddit.com/r/golang/.rss",
			expectSlug: true,
		},
		{
			name:       "Very long URL",
			url:        "https://very-long-domain-name-that-exceeds-reasonable-limits.com/very/long/path/that/should/trigger/truncation/feed.xml",
			expectHash: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			feedID := generateFeedID(tc.url)
			
			if feedID == "" {
				t.Error("Feed ID should not be empty")
			}

			// Feed ID should be reasonable length
			if len(feedID) > 50 {
				t.Errorf("Feed ID too long: %d characters", len(feedID))
			}

			// Should not contain unsafe characters for URIs
			if strings.ContainsAny(feedID, " /?#[]@!$&'()*+,;=") {
				t.Errorf("Feed ID contains unsafe characters: %s", feedID)
			}

			// Multiple calls should return the same ID (stable)
			feedID2 := generateFeedID(tc.url)
			if feedID != feedID2 {
				t.Error("Feed ID generation should be stable")
			}
		})
	}
}

// TestResourceSession tests the ResourceSession functionality  
func TestResourceSession(t *testing.T) {
	session := &ResourceSession{
		id:            "test-session",
		subscriptions: make(map[string]bool),
	}

	t.Run("Subscribe", func(t *testing.T) {
		uri := "feeds://feed/test-feed"
		session.Subscribe(uri)

		if !session.IsSubscribed(uri) {
			t.Error("Session should be subscribed to URI")
		}

		subs := session.GetSubscriptions()
		if len(subs) != 1 || subs[0] != uri {
			t.Error("Subscription list should contain the URI")
		}
	})

	t.Run("Unsubscribe", func(t *testing.T) {
		uri := "feeds://feed/test-feed"
		session.Subscribe(uri)
		session.Unsubscribe(uri)

		if session.IsSubscribed(uri) {
			t.Error("Session should not be subscribed after unsubscribe")
		}

		subs := session.GetSubscriptions()
		if len(subs) != 0 {
			t.Error("Subscription list should be empty after unsubscribe")
		}
	})
}