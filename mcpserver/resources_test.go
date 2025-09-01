package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/richardwooding/feed-mcp/model"
)

// Test constants
const (
	testFeedURI  = "feeds://feed/test-feed"
	testFeedURL1 = "https://example.com/feed1.xml"
	testFeedURL2 = "https://example.com/feed2.xml"
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

// createTestResourceManager creates a ResourceManager with mock data for testing
func createTestResourceManager() *ResourceManager {
	mockFeeds := []*model.FeedResult{
		{
			ID:        "test-feed-1",
			Title:     "Test Feed 1",
			PublicURL: testFeedURL1,
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
			PublicURL: testFeedURL2,
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
		feedID := model.GenerateFeedID(feed.PublicURL)
		feedAndItems := &model.FeedAndItemsResult{
			ID:                 feed.ID,
			PublicURL:          feed.PublicURL,
			Title:              feed.Title,
			FetchError:         feed.FetchError,
			Feed:               feed.Feed,
			Items:              nil,
			CircuitBreakerOpen: feed.CircuitBreakerOpen,
		}
		feedsMap[feedID] = feedAndItems
	}
	mockFeedGetter := &mockResourceFeedAndItemsGetter{feeds: feedsMap}
	return NewResourceManager(mockAllFeeds, mockFeedGetter)
}

// TestListResources tests the ListResources functionality
func TestListResources(t *testing.T) {
	rm := createTestResourceManager()
	ctx := context.Background()
	resources, err := rm.ListResources(ctx)
	if err != nil {
		t.Fatalf("ListResources failed: %v", err)
	}

	validateResourceCount(t, resources)
	validateCoreResources(t, resources)
}

// validateResourceCount validates the expected number of resources
func validateResourceCount(t *testing.T, resources []*mcp.Resource) {
	// Should have 1 feed list resource + 1 parameter docs resource + 3 resources per feed * 2 feeds = 8 total
	expectedCount := 2 + (3 * 2)
	if len(resources) != expectedCount {
		t.Errorf("Expected %d resources, got %d", expectedCount, len(resources))
	}
}

// validateCoreResources validates the presence and properties of core resources
func validateCoreResources(t *testing.T, resources []*mcp.Resource) {
	foundFeedList := false
	foundParameterDocs := false

	for _, resource := range resources {
		if resource.URI == FeedListURI {
			foundFeedList = true
			validateFeedListResource(t, resource)
		}
		if resource.URI == ParameterDocsURI {
			foundParameterDocs = true
			validateParameterDocsResource(t, resource)
		}
	}

	if !foundFeedList {
		t.Error("Feed list resource not found")
	}
	if !foundParameterDocs {
		t.Error("Parameter documentation resource not found")
	}
}

// validateFeedListResource validates the feed list resource properties
func validateFeedListResource(t *testing.T, resource *mcp.Resource) {
	if resource.Name != "All Feeds" {
		t.Errorf("Expected feed list name 'All Feeds', got '%s'", resource.Name)
	}
	if resource.MIMEType != JSONMIMEType {
		t.Errorf("Expected MIME type 'application/json', got '%s'", resource.MIMEType)
	}
}

// validateParameterDocsResource validates the parameter docs resource properties
func validateParameterDocsResource(t *testing.T, resource *mcp.Resource) {
	if resource.Name != "URI Parameter Documentation" {
		t.Errorf("Expected parameter docs name 'URI Parameter Documentation', got '%s'", resource.Name)
	}
	if resource.MIMEType != JSONMIMEType {
		t.Errorf("Expected MIME type 'application/json', got '%s'", resource.MIMEType)
	}
}

// TestReadParameterDocsResource tests reading the parameter documentation resource
func TestReadParameterDocsResource(t *testing.T) {
	rm := createTestResourceManager()
	ctx := context.Background()
	result, err := rm.ReadResource(ctx, ParameterDocsURI)
	if err != nil {
		t.Fatalf("ReadResource failed: %v", err)
	}

	if len(result.Contents) != 1 {
		t.Fatalf("Expected 1 content item, got %d", len(result.Contents))
	}

	content := result.Contents[0]
	if content.URI != ParameterDocsURI {
		t.Errorf("Expected URI '%s', got '%s'", ParameterDocsURI, content.URI)
	}
	if content.MIMEType != JSONMIMEType {
		t.Errorf("Expected MIME type '%s', got '%s'", JSONMIMEType, content.MIMEType)
	}

	// Verify it's valid JSON and contains expected structure
	if content.Text == "" {
		t.Error("Expected non-empty content text")
	}

	// Parse JSON to verify structure
	var paramDocs map[string]interface{}
	if err := json.Unmarshal([]byte(content.Text), &paramDocs); err != nil {
		t.Errorf("Content is not valid JSON: %v", err)
	}

	// Check for expected top-level structure
	if uriParams, ok := paramDocs["uri_parameters"]; !ok {
		t.Error("Expected 'uri_parameters' key in parameter docs")
	} else if uriParamsMap, ok := uriParams.(map[string]interface{}); !ok {
		t.Error("Expected 'uri_parameters' to be an object")
	} else {
		// Check for expected sections
		expectedSections := []string{"description", "base_parameters", "enhanced_parameters", "usage_examples", "combination_notes"}
		for _, section := range expectedSections {
			if _, exists := uriParamsMap[section]; !exists {
				t.Errorf("Expected section '%s' not found in parameter docs", section)
			}
		}
	}
}

// TestReadFeedListResource tests reading the feed list resource
func TestReadFeedListResource(t *testing.T) {
	rm := createTestResourceManager()
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

	if content.MIMEType != JSONMIMEType {
		t.Errorf("Expected MIME type 'application/json', got %s", content.MIMEType)
	}

	if !strings.Contains(content.Text, "Test Feed 1") {
		t.Error("Feed list content should contain 'Test Feed 1'")
	}
	if !strings.Contains(content.Text, "feeds") {
		t.Error("Feed list content should contain 'feeds' field")
	}
}

// TestReadIndividualFeedResource tests reading individual feed resources
func TestReadIndividualFeedResource(t *testing.T) {
	rm := createTestResourceManager()
	ctx := context.Background()
	feedID := model.GenerateFeedID(testFeedURL1)
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

	if !strings.Contains(content.Text, "Test Feed 1") {
		t.Error("Feed content should contain 'Test Feed 1'")
	}
}

// TestReadFeedItemsResource tests reading feed items resources
func TestReadFeedItemsResource(t *testing.T) {
	// Create test data with items
	mockFeeds := []*model.FeedResult{
		{
			ID:        "test-feed-with-items",
			Title:     "Test Feed With Items",
			PublicURL: testFeedURL1,
			Feed: &model.Feed{
				Title:       "Test Feed With Items",
				Description: "A test feed with items",
				Link:        "https://example.com",
				FeedType:    "rss",
			},
		},
	}

	// Create mock items using gofeed.Item
	mockItems := []*gofeed.Item{
		{
			Title:       "Test Item 1",
			Description: "First test item",
			Link:        "https://example.com/item1",
			Published:   "2024-01-01T10:00:00Z",
		},
		{
			Title:       "Test Item 2",
			Description: "Second test item",
			Link:        "https://example.com/item2",
			Published:   "2024-01-02T10:00:00Z",
		},
	}

	mockAllFeeds := &mockResourceAllFeedsGetter{feeds: mockFeeds}
	feedsMap := make(map[string]*model.FeedAndItemsResult)
	for _, feed := range mockFeeds {
		feedID := model.GenerateFeedID(feed.PublicURL)
		feedAndItems := &model.FeedAndItemsResult{
			ID:                 feed.ID,
			PublicURL:          feed.PublicURL,
			Title:              feed.Title,
			FetchError:         feed.FetchError,
			Feed:               feed.Feed,
			Items:              mockItems, // Add the mock items
			CircuitBreakerOpen: feed.CircuitBreakerOpen,
		}
		feedsMap[feedID] = feedAndItems
	}
	mockFeedGetter := &mockResourceFeedAndItemsGetter{feeds: feedsMap}
	rm := NewResourceManager(mockAllFeeds, mockFeedGetter)

	ctx := context.Background()
	feedID := model.GenerateFeedID(testFeedURL1)
	uri := expandURITemplate(FeedItemsURI, map[string]string{"feedId": feedID})

	result, err := rm.ReadResource(ctx, uri)
	if err != nil {
		t.Fatalf("ReadResource for feed items failed: %v", err)
	}

	if len(result.Contents) != 1 {
		t.Fatalf("Expected 1 content item, got %d", len(result.Contents))
	}

	content := result.Contents[0]
	if content.URI != uri {
		t.Errorf("Expected URI %s, got %s", uri, content.URI)
	}

	// Verify items are in the content
	if !strings.Contains(content.Text, "Test Item 1") {
		t.Error("Items content should contain 'Test Item 1'")
	}
	if !strings.Contains(content.Text, "Test Item 2") {
		t.Error("Items content should contain 'Test Item 2'")
	}
	if !strings.Contains(content.Text, `"count":2`) {
		t.Error("Items content should show count of 2")
	}
}

// TestReadFeedMetadataResource tests reading feed metadata resources
func TestReadFeedMetadataResource(t *testing.T) {
	rm := createTestResourceManager()
	ctx := context.Background()
	feedID := model.GenerateFeedID(testFeedURL1)
	uri := expandURITemplate(FeedMetaURI, map[string]string{"feedId": feedID})

	result, err := rm.ReadResource(ctx, uri)
	if err != nil {
		t.Fatalf("ReadResource for feed metadata failed: %v", err)
	}

	if len(result.Contents) != 1 {
		t.Fatalf("Expected 1 content item, got %d", len(result.Contents))
	}

	content := result.Contents[0]
	if !strings.Contains(content.Text, "description") {
		t.Error("Metadata should contain 'description' field")
	}
	if !strings.Contains(content.Text, "public_url") {
		t.Error("Metadata should contain 'public_url' field")
	}
}

// TestInvalidResourceURI tests handling of invalid resource URIs
func TestInvalidResourceURI(t *testing.T) {
	rm := createTestResourceManager()
	ctx := context.Background()
	_, err := rm.ReadResource(ctx, "invalid://uri")

	if err == nil {
		t.Error("Expected error for invalid URI, but got none")
	}

	if !strings.Contains(err.Error(), "URI does not match any supported resource patterns") {
		t.Error("Expected 'URI does not match any supported resource patterns' error message")
	}
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
		uri := testFeedURI
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
		name       string
		url        string
		expectSlug bool
		expectHash bool
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
			feedID := model.GenerateFeedID(tc.url)

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
			feedID2 := model.GenerateFeedID(tc.url)
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

// TestResourceCaching tests the resource caching functionality
func TestResourceCaching(t *testing.T) {
	// Create mock feeds
	mockFeeds := []*model.FeedResult{
		{
			ID:        "test-feed-cache",
			Title:     "Test Feed Cache",
			PublicURL: testFeedURL1,
			Feed: &model.Feed{
				Title:       "Test Feed Cache",
				Description: "A test feed for cache testing",
				Link:        "https://example.com",
				FeedType:    "rss",
			},
		},
	}

	mockAllFeeds := &mockResourceAllFeedsGetter{feeds: mockFeeds}
	feedsMap := make(map[string]*model.FeedAndItemsResult)
	for _, feed := range mockFeeds {
		feedID := model.GenerateFeedID(feed.PublicURL)
		feedAndItems := &model.FeedAndItemsResult{
			ID:                 feed.ID,
			PublicURL:          feed.PublicURL,
			Title:              feed.Title,
			FetchError:         feed.FetchError,
			Feed:               feed.Feed,
			Items:              nil,
			CircuitBreakerOpen: feed.CircuitBreakerOpen,
		}
		feedsMap[feedID] = feedAndItems
	}
	mockFeedGetter := &mockResourceFeedAndItemsGetter{feeds: feedsMap}
	rm := NewResourceManager(mockAllFeeds, mockFeedGetter)

	ctx := context.Background()

	t.Run("Feed list caching", func(t *testing.T) {
		// First call should be a cache miss
		initialMetrics := rm.GetCacheMetrics()

		result1, err := rm.ReadResource(ctx, FeedListURI)
		if err != nil {
			t.Fatalf("First ReadResource failed: %v", err)
		}

		// Check cache miss was recorded
		metrics1 := rm.GetCacheMetrics()
		if metrics1.Misses != initialMetrics.Misses+1 {
			t.Errorf("Expected cache miss count to increase by 1, got %d->%d", initialMetrics.Misses, metrics1.Misses)
		}

		// Wait a moment for Ristretto cache to process Set operation
		time.Sleep(100 * time.Millisecond)

		// Second call should be a cache hit
		result2, err := rm.ReadResource(ctx, FeedListURI)
		if err != nil {
			t.Fatalf("Second ReadResource failed: %v", err)
		}

		// Check cache hit was recorded
		metrics2 := rm.GetCacheMetrics()
		if metrics2.Hits != metrics1.Hits+1 {
			t.Errorf("Expected cache hit count to increase by 1, got %d->%d", metrics1.Hits, metrics2.Hits)
		}

		// Results should be identical
		if result1.Contents[0].Text != result2.Contents[0].Text {
			t.Error("Cached result should be identical to original")
		}
	})

	t.Run("Cache invalidation", func(t *testing.T) {
		// Make a request to populate cache
		_, err := rm.ReadResource(ctx, FeedListURI)
		if err != nil {
			t.Fatalf("ReadResource failed: %v", err)
		}

		// Invalidate cache
		err = rm.InvalidateCache(ctx)
		if err != nil {
			t.Fatalf("InvalidateCache failed: %v", err)
		}

		// Next request should be a cache miss
		initialMetrics := rm.GetCacheMetrics()
		_, err = rm.ReadResource(ctx, FeedListURI)
		if err != nil {
			t.Fatalf("ReadResource after cache invalidation failed: %v", err)
		}

		// Should have recorded a cache miss
		finalMetrics := rm.GetCacheMetrics()
		if finalMetrics.Misses != initialMetrics.Misses+1 {
			t.Errorf("Expected cache miss after invalidation, got %d->%d", initialMetrics.Misses, finalMetrics.Misses)
		}
	})
}
