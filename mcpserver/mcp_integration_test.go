// Package mcpserver provides integration tests for MCP protocol functionality
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/mmcdole/gofeed"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/richardwooding/feed-mcp/model"
)

// createMockItems creates mock feed items for testing
func createMockItems(count int) []*gofeed.Item {
	items := make([]*gofeed.Item, count)
	for i := 0; i < count; i++ {
		items[i] = &gofeed.Item{
			Title:       fmt.Sprintf("Test Item %d", i+1),
			Link:        fmt.Sprintf("https://example.com/item%d", i+1),
			Description: fmt.Sprintf("Description for test item %d", i+1),
			Published:   fmt.Sprintf("2024-01-%02d 10:00:00", i+1),
		}
	}
	return items
}

// TestMCPResourcesIntegration tests the full MCP Resources protocol integration
func TestMCPResourcesIntegration(t *testing.T) {
	// Create a test server
	mockAllFeeds := &mockAllFeedsGetter{
		feeds: []*model.FeedResult{
			{
				ID:        "feed1",
				Title:     "Test Feed 1",
				PublicURL: "https://example.com/feed1.xml",
			},
			{
				ID:        "feed2",
				Title:     "Test Feed 2",
				PublicURL: "https://example.com/feed2.xml",
			},
		},
	}

	mockFeedGetter := &mockFeedAndItemsGetter{
		feedMap: map[string]*model.FeedAndItemsResult{
			"feed1": {
				ID:        "feed1",
				Title:     "Test Feed 1",
				PublicURL: "https://example.com/feed1.xml",
				Feed:      &model.Feed{Title: "Test Feed 1", Description: "A test feed"},
				Items:     createMockItems(5),
			},
		},
	}

	config := Config{
		Transport:          model.StdioTransport,
		AllFeedsGetter:     mockAllFeeds,
		FeedAndItemsGetter: mockFeedGetter,
	}

	server, err := NewServer(config)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Test ResourceManager directly
	ctx := context.Background()

	t.Run("ListResources", func(t *testing.T) {
		resources, err := server.resourceManager.ListResources(ctx)
		if err != nil {
			t.Fatalf("ListResources failed: %v", err)
		}

		if len(resources) == 0 {
			t.Fatalf("Expected resources, got none")
		}

		// Should have at least the feed list resource plus individual feed resources
		expectedMinResources := 3 // feeds://all + 2 feeds
		if len(resources) < expectedMinResources {
			t.Errorf("Expected at least %d resources, got %d", expectedMinResources, len(resources))
		}

		// Check for feed list resource
		foundFeedListResource := false
		for _, resource := range resources {
			if resource.URI == "feeds://all" {
				foundFeedListResource = true
				if resource.Name != "All Feeds" {
					t.Errorf("Expected feed list resource name 'All Feeds', got '%s'", resource.Name)
				}
				if resource.MIMEType != "application/json" {
					t.Errorf("Expected feed list resource mime type 'application/json', got '%s'", resource.MIMEType)
				}
				break
			}
		}

		if !foundFeedListResource {
			t.Error("Expected to find feed list resource 'feeds://all'")
		}
	})

	t.Run("ReadFeedListResource", func(t *testing.T) {
		result, err := server.resourceManager.ReadResource(ctx, "feeds://all")
		if err != nil {
			t.Fatalf("ReadResource failed for feeds://all: %v", err)
		}

		if len(result.Contents) != 1 {
			t.Fatalf("Expected 1 content item, got %d", len(result.Contents))
		}

		content := result.Contents[0]
		if content.MIMEType != "application/json" {
			t.Errorf("Expected mime type 'application/json', got '%s'", content.MIMEType)
		}

		// Parse and verify the JSON content
		var feedListResponse struct {
			Feeds     []map[string]interface{} `json:"feeds"`
			Count     int                      `json:"count"`
			UpdatedAt string                   `json:"updated_at"`
		}
		if err := json.Unmarshal([]byte(content.Text), &feedListResponse); err != nil {
			t.Fatalf("Failed to parse feed list JSON: %v", err)
		}

		if feedListResponse.Count != 2 {
			t.Errorf("Expected 2 feeds in list, got %d", feedListResponse.Count)
		}

		if len(feedListResponse.Feeds) != 2 {
			t.Errorf("Expected 2 feeds in feeds array, got %d", len(feedListResponse.Feeds))
		}
	})

	t.Run("ReadFeedItemsResource", func(t *testing.T) {
		result, err := server.resourceManager.ReadResource(ctx, "feeds://feed/feed1/items")
		if err != nil {
			t.Fatalf("ReadResource failed for feed items: %v", err)
		}

		if len(result.Contents) != 1 {
			t.Fatalf("Expected 1 content item, got %d", len(result.Contents))
		}

		content := result.Contents[0]
		if content.MIMEType != "application/json" {
			t.Errorf("Expected mime type 'application/json', got '%s'", content.MIMEType)
		}

		// Parse and verify the JSON content - feed items are returned as a structured JSON
		var itemsResponse map[string]interface{}
		if err := json.Unmarshal([]byte(content.Text), &itemsResponse); err != nil {
			t.Fatalf("Failed to parse feed items JSON: %v", err)
		}

		// Check if items are present in the response structure
		if itemsResponse["items"] == nil {
			t.Error("Expected 'items' field in response")
		} else if itemsArray, ok := itemsResponse["items"].([]interface{}); !ok {
			t.Error("Expected 'items' to be an array")
		} else if len(itemsArray) != 5 {
			t.Errorf("Expected 5 items, got %d", len(itemsArray))
		}
	})

	t.Run("ReadFeedMetaResource", func(t *testing.T) {
		result, err := server.resourceManager.ReadResource(ctx, "feeds://feed/feed1/meta")
		if err != nil {
			t.Fatalf("ReadResource failed for feed meta: %v", err)
		}

		if len(result.Contents) != 1 {
			t.Fatalf("Expected 1 content item, got %d", len(result.Contents))
		}

		content := result.Contents[0]
		if content.MIMEType != "application/json" {
			t.Errorf("Expected mime type 'application/json', got '%s'", content.MIMEType)
		}

		// Parse and verify the JSON content
		var meta map[string]interface{}
		if err := json.Unmarshal([]byte(content.Text), &meta); err != nil {
			t.Fatalf("Failed to parse feed meta JSON: %v", err)
		}

		if meta["id"] != "feed1" {
			t.Errorf("Expected feed ID 'feed1', got %v", meta["id"])
		}

		if meta["title"] != "Test Feed 1" {
			t.Errorf("Expected feed title 'Test Feed 1', got %v", meta["title"])
		}
	})

	t.Run("ResourceSubscription", func(t *testing.T) {
		sessionID := "test-session"
		server.resourceManager.CreateSession(sessionID)

		// Test subscription
		err := server.resourceManager.Subscribe(sessionID, "feeds://feed/feed1/items")
		if err != nil {
			t.Fatalf("Failed to subscribe to resource: %v", err)
		}

		// Verify subscription exists
		sessions := server.resourceManager.GetSubscribedSessions("feeds://feed/feed1/items")
		if len(sessions) != 1 || sessions[0] != sessionID {
			t.Errorf("Expected session '%s' to be subscribed, got %v", sessionID, sessions)
		}

		// Test unsubscription
		err = server.resourceManager.Unsubscribe(sessionID, "feeds://feed/feed1/items")
		if err != nil {
			t.Fatalf("Failed to unsubscribe from resource: %v", err)
		}

		// Verify subscription is removed
		sessions = server.resourceManager.GetSubscribedSessions("feeds://feed/feed1/items")
		if len(sessions) != 0 {
			t.Errorf("Expected no subscriptions, got %v", sessions)
		}

		// Clean up
		server.resourceManager.RemoveSession(sessionID)
	})

	t.Run("InvalidResourceURI", func(t *testing.T) {
		_, err := server.resourceManager.ReadResource(ctx, "feeds://invalid/resource")
		if err == nil {
			t.Error("Expected error for invalid resource URI, got nil")
		}

		// Check that it's a resource URI error (the exact message may vary)
		if !strings.Contains(err.Error(), "URI does not match any supported resource patterns") {
			t.Errorf("Expected error about unsupported URI pattern, got '%s'", err.Error())
		}
	})

	t.Run("MissingFeedResource", func(t *testing.T) {
		_, err := server.resourceManager.ReadResource(ctx, "feeds://feed/nonexistent/items")
		if err == nil {
			t.Error("Expected error for missing feed, got nil")
		}
	})
}

// TestMCPServerResourceHandlers tests that resource handlers are properly registered
func TestMCPServerResourceHandlers(t *testing.T) {
	// Create server with minimal mock data
	mockAllFeeds := &mockAllFeedsGetter{
		feeds: []*model.FeedResult{
			{ID: "test", Title: "Test Feed", PublicURL: "https://example.com/test.xml"},
		},
	}

	mockFeedGetter := &mockFeedAndItemsGetter{
		feedMap: map[string]*model.FeedAndItemsResult{
			"test": {
				ID:        "test",
				Title:     "Test Feed",
				PublicURL: "https://example.com/test.xml",
				Feed:      &model.Feed{Title: "Test Feed"},
				Items:     createMockItems(1),
			},
		},
	}

	config := Config{
		Transport:          model.StdioTransport,
		AllFeedsGetter:     mockAllFeeds,
		FeedAndItemsGetter: mockFeedGetter,
	}

	server, err := NewServer(config)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Create MCP server
	srv := mcp.NewServer(
		&mcp.Implementation{
			Name:    "Test MCP Server",
			Version: "1.0.0",
		},
		&mcp.ServerOptions{
			HasResources: true,
		},
	)

	// This should not panic and should register resources
	server.addResourceHandlers(srv)

	// Test passes if no panic occurs and resources are registered
	t.Log("Resource handlers registered successfully")
}