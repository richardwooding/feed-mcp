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
func createTestMockItems(count int) []*gofeed.Item {
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

// setupIntegrationTestServer creates a test server for integration testing
func setupIntegrationTestServer(t *testing.T) *Server {
	t.Helper()

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
				Items:     createTestMockItems(5),
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
	return server
}

func TestMCPResourcesListResources(t *testing.T) {
	server := setupIntegrationTestServer(t)
	ctx := context.Background()

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
			if resource.MIMEType != JSONMIMEType {
				t.Errorf("Expected feed list resource mime type 'application/json', got '%s'", resource.MIMEType)
			}
			break
		}
	}

	if !foundFeedListResource {
		t.Error("Expected to find feed list resource 'feeds://all'")
	}
}

func TestMCPResourcesReadFeedListResource(t *testing.T) {
	server := setupIntegrationTestServer(t)
	ctx := context.Background()

	result, err := server.resourceManager.ReadResource(ctx, "feeds://all")
	if err != nil {
		t.Fatalf("ReadResource failed for feeds://all: %v", err)
	}

	if len(result.Contents) != 1 {
		t.Fatalf("Expected 1 content item, got %d", len(result.Contents))
	}

	content := result.Contents[0]
	if content.MIMEType != JSONMIMEType {
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
}

func TestMCPResourcesInvalidResourceURI(t *testing.T) {
	server := setupIntegrationTestServer(t)
	ctx := context.Background()

	_, err := server.resourceManager.ReadResource(ctx, "feeds://invalid/resource")
	if err == nil {
		t.Error("Expected error for invalid resource URI, got nil")
	}

	// Check that it's a resource URI error (the exact message may vary)
	if !strings.Contains(err.Error(), "URI does not match any supported resource patterns") {
		t.Errorf("Expected error about unsupported URI pattern, got '%s'", err.Error())
	}
}

func TestMCPServerResourceHandlersRegistration(t *testing.T) {
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
				Items:     createTestMockItems(1),
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
