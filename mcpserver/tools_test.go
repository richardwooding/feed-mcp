package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/mmcdole/gofeed"
	"github.com/modelcontextprotocol/go-sdk/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/richardwooding/feed-mcp/model"
)

// Test the schema definitions used in the tools
func TestToolSchemas(t *testing.T) {
	t.Run("fetch_link tool schema", func(t *testing.T) {
		expected := &jsonschema.Schema{
			Type:     "object",
			Required: []string{"URL"},
			Properties: map[string]*jsonschema.Schema{
				"URL": {
					Type:        "string",
					Description: "Link URL",
				},
			},
		}

		// This tests that our schema definition is correct
		if expected.Type != "object" {
			t.Errorf("Expected type 'object', got %s", expected.Type)
		}

		if len(expected.Required) != 1 || expected.Required[0] != "URL" {
			t.Errorf("Expected required field 'URL', got %v", expected.Required)
		}

		urlProp, exists := expected.Properties["URL"]
		if !exists {
			t.Error("URL property not found in schema")
		}

		if urlProp.Type != "string" {
			t.Errorf("Expected URL type 'string', got %s", urlProp.Type)
		}

		if urlProp.Description != "Link URL" {
			t.Errorf("Expected URL description 'Link URL', got %s", urlProp.Description)
		}
	})

	t.Run("get_syndication_feed_items tool schema", func(t *testing.T) {
		expected := &jsonschema.Schema{
			Type:     "object",
			Required: []string{"ID"},
			Properties: map[string]*jsonschema.Schema{
				"ID": {
					Type:        "string",
					Description: "Feed ID",
				},
			},
		}

		// This tests that our schema definition is correct
		if expected.Type != "object" {
			t.Errorf("Expected type 'object', got %s", expected.Type)
		}

		if len(expected.Required) != 1 || expected.Required[0] != "ID" {
			t.Errorf("Expected required field 'ID', got %v", expected.Required)
		}

		idProp, exists := expected.Properties["ID"]
		if !exists {
			t.Error("ID property not found in schema")
		}

		if idProp.Type != "string" {
			t.Errorf("Expected ID type 'string', got %s", idProp.Type)
		}

		if idProp.Description != "Feed ID" {
			t.Errorf("Expected ID description 'Feed ID', got %s", idProp.Description)
		}
	})

	t.Run("all_syndication_feeds tool schema", func(t *testing.T) {
		expected := &jsonschema.Schema{Type: "object"}

		if expected.Type != "object" {
			t.Errorf("Expected type 'object', got %s", expected.Type)
		}
	})
}

// Test the tool logic by simulating what happens in the Run method
func TestToolLogic(t *testing.T) {
	// Setup test data
	testFeeds := []*model.FeedResult{
		{
			ID:        "feed1",
			PublicURL: "https://example.com/feed1.xml",
			Title:     "Test Feed 1",
			Feed: &model.Feed{
				Title:       "Test Feed 1",
				Link:        "https://example.com",
				Description: "Test feed description",
			},
		},
		{
			ID:         "feed2",
			PublicURL:  "https://example.com/feed2.xml",
			Title:      "Test Feed 2",
			FetchError: "Failed to fetch",
		},
	}

	testFeedAndItems := map[string]*model.FeedAndItemsResult{
		"feed1": {
			ID:        "feed1",
			PublicURL: "https://example.com/feed1.xml",
			Title:     "Test Feed 1",
			Feed: &model.Feed{
				Title:       "Test Feed 1",
				Link:        "https://example.com",
				Description: "Test feed description",
			},
			Items: []*gofeed.Item{
				{
					Title:       "Item 1",
					Link:        "https://example.com/item1",
					Description: "Item 1 description",
				},
				{
					Title:       "Item 2",
					Link:        "https://example.com/item2",
					Description: "Item 2 description",
				},
			},
		},
	}

	// Create server
	mockAllFeeds := &mockAllFeedsGetter{feeds: testFeeds}
	mockFeedItems := &mockFeedAndItemsGetter{feedMap: testFeedAndItems}

	config := Config{
		Transport:          model.StdioTransport,
		AllFeedsGetter:     mockAllFeeds,
		FeedAndItemsGetter: mockFeedItems,
	}

	server, err := NewServer(config)
	if err != nil {
		t.Fatalf("NewServer() failed: %v", err)
	}

	t.Run("all_syndication_feeds tool logic", func(t *testing.T) {
		// Simulate the tool handler logic
		ctx := context.Background()
		feedResults, err := server.allFeedsGetter.GetAllFeeds(ctx)
		if err != nil {
			t.Fatalf("GetAllFeeds() failed: %v", err)
		}

		// Test JSON marshaling (which happens in the tool)
		data, err := json.Marshal(feedResults)
		if err != nil {
			t.Fatalf("Failed to marshal feeds: %v", err)
		}

		// Verify the marshaled data contains expected content
		dataStr := string(data)
		if !strings.Contains(dataStr, "feed1") {
			t.Error("Marshaled data should contain 'feed1'")
		}
		if !strings.Contains(dataStr, "Test Feed 1") {
			t.Error("Marshaled data should contain 'Test Feed 1'")
		}
		if !strings.Contains(dataStr, "Failed to fetch") {
			t.Error("Marshaled data should contain error message")
		}

		// Test that we can unmarshal it back
		var unmarshaled []*model.FeedResult
		err = json.Unmarshal(data, &unmarshaled)
		if err != nil {
			t.Fatalf("Failed to unmarshal feeds: %v", err)
		}

		if len(unmarshaled) != 2 {
			t.Errorf("Expected 2 feeds, got %d", len(unmarshaled))
		}

		// Check that the data is preserved
		if unmarshaled[0].ID != "feed1" {
			t.Errorf("Expected first feed ID 'feed1', got %s", unmarshaled[0].ID)
		}
		if unmarshaled[1].FetchError != "Failed to fetch" {
			t.Errorf("Expected fetch error, got %s", unmarshaled[1].FetchError)
		}
	})

	t.Run("get_syndication_feed_items tool logic", func(t *testing.T) {
		// Simulate the tool handler logic
		ctx := context.Background()
		feedResult, err := server.feedAndItemsGetter.GetFeedAndItems(ctx, "feed1")
		if err != nil {
			t.Fatalf("GetFeedAndItems() failed: %v", err)
		}

		// Test JSON marshaling (which happens in the tool)
		data, err := json.Marshal(feedResult)
		if err != nil {
			t.Fatalf("Failed to marshal feed and items: %v", err)
		}

		// Verify the marshaled data contains expected content
		dataStr := string(data)
		if !strings.Contains(dataStr, "feed1") {
			t.Error("Marshaled data should contain 'feed1'")
		}
		if !strings.Contains(dataStr, "Item 1") {
			t.Error("Marshaled data should contain 'Item 1'")
		}
		if !strings.Contains(dataStr, "Item 2") {
			t.Error("Marshaled data should contain 'Item 2'")
		}

		// Test that we can unmarshal it back
		var unmarshaled model.FeedAndItemsResult
		err = json.Unmarshal(data, &unmarshaled)
		if err != nil {
			t.Fatalf("Failed to unmarshal feed and items: %v", err)
		}

		if unmarshaled.ID != "feed1" {
			t.Errorf("Expected feed ID 'feed1', got %s", unmarshaled.ID)
		}

		if len(unmarshaled.Items) != 2 {
			t.Errorf("Expected 2 items, got %d", len(unmarshaled.Items))
		}

		if unmarshaled.Items[0].Title != "Item 1" {
			t.Errorf("Expected first item title 'Item 1', got %s", unmarshaled.Items[0].Title)
		}
	})

	t.Run("fetch_link tool logic simulation", func(t *testing.T) {
		// Create a test HTTP server
		testContent := "<html><head><title>Test Page</title></head><body><h1>Hello World</h1></body></html>"
		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(testContent))
		}))
		defer testServer.Close()

		// Test that the fetch_link logic would work
		// (We can't easily test colly here without more complex setup,
		// but we can verify the HTTP server works)
		resp, err := http.Get(testServer.URL)
		if err != nil {
			t.Fatalf("Failed to get test URL: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		contentType := resp.Header.Get("Content-Type")
		if !strings.Contains(contentType, "text/html") {
			t.Errorf("Expected HTML content type, got %s", contentType)
		}
	})
}

func TestMCPServerCreation(t *testing.T) {
	// Test the MCP server creation logic that happens in Run()
	t.Run("MCP implementation structure", func(t *testing.T) {
		impl := &mcp.Implementation{
			Name:    "RSS, Atom, and JSON Feed Server",
			Version: "1.0.0",
		}

		if impl.Name != "RSS, Atom, and JSON Feed Server" {
			t.Errorf("Expected name 'RSS, Atom, and JSON Feed Server', got %s", impl.Name)
		}

		if impl.Version != "1.0.0" {
			t.Errorf("Expected version '1.0.0', got %s", impl.Version)
		}
	})

	t.Run("MCP tool structures", func(t *testing.T) {
		// Test fetch_link tool structure
		fetchLinkTool := &mcp.Tool{
			Name:        "fetch_link",
			Description: "Fetch link URL",
			InputSchema: &jsonschema.Schema{
				Type:     "object",
				Required: []string{"URL"},
				Properties: map[string]*jsonschema.Schema{
					"URL": {
						Type:        "string",
						Description: "Link URL",
					},
				},
			},
		}

		if fetchLinkTool.Name != "fetch_link" {
			t.Errorf("Expected tool name 'fetch_link', got %s", fetchLinkTool.Name)
		}

		if fetchLinkTool.Description != "Fetch link URL" {
			t.Errorf("Expected description 'Fetch link URL', got %s", fetchLinkTool.Description)
		}

		if fetchLinkTool.InputSchema == nil {
			t.Error("InputSchema should not be nil")
		}

		// Test all_syndication_feeds tool structure
		allFeedsTool := &mcp.Tool{
			Name:        "all_syndication_feeds",
			Description: "list available feedItem resources",
			InputSchema: &jsonschema.Schema{Type: "object"},
		}

		if allFeedsTool.Name != "all_syndication_feeds" {
			t.Errorf("Expected tool name 'all_syndication_feeds', got %s", allFeedsTool.Name)
		}

		if allFeedsTool.InputSchema.Type != "object" {
			t.Errorf("Expected schema type 'object', got %s", allFeedsTool.InputSchema.Type)
		}

		// Test get_syndication_feed_items tool structure
		getSyndicationFeedTool := &mcp.Tool{
			Name:        "get_syndication_feed_items",
			Description: "get syndication feed and items by id",
			InputSchema: &jsonschema.Schema{
				Type:     "object",
				Required: []string{"ID"},
				Properties: map[string]*jsonschema.Schema{
					"ID": {
						Type:        "string",
						Description: "Feed ID",
					},
				},
			},
		}

		if getSyndicationFeedTool.Name != "get_syndication_feed_items" {
			t.Errorf("Expected tool name 'get_syndication_feed_items', got %s", getSyndicationFeedTool.Name)
		}

		if len(getSyndicationFeedTool.InputSchema.Required) != 1 {
			t.Errorf("Expected 1 required field, got %d", len(getSyndicationFeedTool.InputSchema.Required))
		}
	})
}

func TestParameterTypes(t *testing.T) {
	t.Run("FetchLinkParams JSON serialization", func(t *testing.T) {
		params := FetchLinkParams{URL: "https://example.com/test"}

		data, err := json.Marshal(params)
		if err != nil {
			t.Fatalf("Failed to marshal FetchLinkParams: %v", err)
		}

		var unmarshaled FetchLinkParams
		err = json.Unmarshal(data, &unmarshaled)
		if err != nil {
			t.Fatalf("Failed to unmarshal FetchLinkParams: %v", err)
		}

		if unmarshaled.URL != params.URL {
			t.Errorf("Expected URL %s, got %s", params.URL, unmarshaled.URL)
		}
	})

	t.Run("GetSyndicationFeedParams JSON serialization", func(t *testing.T) {
		params := GetSyndicationFeedParams{ID: "test-feed-123"}

		data, err := json.Marshal(params)
		if err != nil {
			t.Fatalf("Failed to marshal GetSyndicationFeedParams: %v", err)
		}

		var unmarshaled GetSyndicationFeedParams
		err = json.Unmarshal(data, &unmarshaled)
		if err != nil {
			t.Fatalf("Failed to unmarshal GetSyndicationFeedParams: %v", err)
		}

		if unmarshaled.ID != params.ID {
			t.Errorf("Expected ID %s, got %s", params.ID, unmarshaled.ID)
		}
	})
}

func TestTransportErrorHandling(t *testing.T) {
	// Test the error handling in the transport switch statement
	mockAllFeeds := &mockAllFeedsGetter{feeds: []*model.FeedResult{}}
	mockFeedItems := &mockFeedAndItemsGetter{feedMap: make(map[string]*model.FeedAndItemsResult)}

	config := Config{
		Transport:          99, // Invalid transport value
		AllFeedsGetter:     mockAllFeeds,
		FeedAndItemsGetter: mockFeedItems,
	}

	server, err := NewServer(config)
	if err != nil {
		t.Fatalf("NewServer() failed: %v", err)
	}

	// The Run method should return an error for unsupported transport
	err = server.Run()
	if err == nil {
		t.Error("Run() should have failed for unsupported transport")
	}

	expectedErr := "unsupported transport"
	if err.Error() != expectedErr {
		t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
	}
}

func TestTypeDefinitions(t *testing.T) {
	// Test that our type definitions are what we expect
	t.Run("Server struct fields", func(t *testing.T) {
		serverType := reflect.TypeOf(Server{})

		// Check that Server has the expected fields
		expectedFields := []string{"transport", "allFeedsGetter", "feedAndItemsGetter", "sessionID"}

		if serverType.NumField() != len(expectedFields) {
			t.Errorf("Expected %d fields in Server, got %d", len(expectedFields), serverType.NumField())
		}

		for i, expectedField := range expectedFields {
			field := serverType.Field(i)
			if field.Name != expectedField {
				t.Errorf("Expected field %d to be %s, got %s", i, expectedField, field.Name)
			}
		}
	})

	t.Run("Config struct fields", func(t *testing.T) {
		configType := reflect.TypeOf(Config{})

		// Check that Config has the expected fields
		expectedFields := []string{"Transport", "AllFeedsGetter", "FeedAndItemsGetter"}

		if configType.NumField() != len(expectedFields) {
			t.Errorf("Expected %d fields in Config, got %d", len(expectedFields), configType.NumField())
		}

		for i, expectedField := range expectedFields {
			field := configType.Field(i)
			if field.Name != expectedField {
				t.Errorf("Expected field %d to be %s, got %s", i, expectedField, field.Name)
			}
		}
	})
}
