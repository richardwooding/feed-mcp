package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/richardwooding/feed-mcp/model"
)

// createTestServer creates a test server with mock data
func createTestServer(t *testing.T) *Server {
	mockAllFeeds := &mockAllFeedsGetter{
		feeds: []*model.FeedResult{
			{
				ID:        "test1",
				Title:     "Test Feed 1",
				PublicURL: "https://example.com/feed1.xml",
				Feed: &model.Feed{
					Title:       "Test Feed 1",
					Link:        "https://example.com",
					Description: "Test feed description",
				},
			},
		},
	}

	mockFeedItems := &mockFeedAndItemsGetter{
		feedMap: make(map[string]*model.FeedAndItemsResult),
	}

	config := Config{
		Transport:          model.StdioTransport,
		AllFeedsGetter:     mockAllFeeds,
		FeedAndItemsGetter: mockFeedItems,
	}

	server, err := NewServer(config)
	if err != nil {
		t.Fatalf("NewServer() failed: %v", err)
	}
	return server
}

// validatePromptResult validates basic prompt result structure
func validatePromptResult(t *testing.T, result *mcp.GetPromptResult) {
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if result.Description == "" {
		t.Error("Expected non-empty description")
	}
	if len(result.Messages) == 0 {
		t.Error("Expected at least one message")
	}
	if len(result.Messages) > 0 && result.Messages[0].Role != "user" {
		t.Errorf("Expected role 'user', got %s", result.Messages[0].Role)
	}
}

// TestPromptImplementation verifies that prompts are properly implemented and registered
func TestPromptImplementation(t *testing.T) {
	server := createTestServer(t)

	// Test prompt handlers directly
	t.Run("analyze_feed_trends prompt handler", func(t *testing.T) {
		req := &mcp.GetPromptRequest{
			Params: &mcp.GetPromptParams{
				Arguments: map[string]string{
					"timeframe":  "24h",
					"categories": "tech",
				},
			},
		}

		result, err := server.handleAnalyzeFeedTrends(context.Background(), req)
		if err != nil {
			t.Fatalf("handleAnalyzeFeedTrends() failed: %v", err)
		}
		validatePromptResult(t, result)
	})

	t.Run("summarize_feeds prompt handler", func(t *testing.T) {
		req := &mcp.GetPromptRequest{
			Params: &mcp.GetPromptParams{
				Arguments: map[string]string{
					"summary_type": "brief",
				},
			},
		}

		result, err := server.handleSummarizeFeeds(context.Background(), req)
		if err != nil {
			t.Fatalf("handleSummarizeFeeds() failed: %v", err)
		}
		validatePromptResult(t, result)
	})

	t.Run("monitor_keywords prompt handler", func(t *testing.T) {
		req := &mcp.GetPromptRequest{
			Params: &mcp.GetPromptParams{
				Arguments: map[string]string{
					"keywords":        "golang,mcp",
					"timeframe":       "24h",
					"alert_threshold": "2",
				},
			},
		}

		result, err := server.handleMonitorKeywords(context.Background(), req)
		if err != nil {
			t.Fatalf("handleMonitorKeywords() failed: %v", err)
		}
		validatePromptResult(t, result)
	})

	t.Run("compare_sources prompt handler", func(t *testing.T) {
		req := &mcp.GetPromptRequest{
			Params: &mcp.GetPromptParams{
				Arguments: map[string]string{
					"topic": "technology",
				},
			},
		}

		result, err := server.handleCompareSources(context.Background(), req)
		if err != nil {
			t.Fatalf("handleCompareSources() failed: %v", err)
		}
		validatePromptResult(t, result)
	})

	t.Run("generate_feed_report prompt handler", func(t *testing.T) {
		req := &mcp.GetPromptRequest{
			Params: &mcp.GetPromptParams{
				Arguments: map[string]string{
					"report_type": "comprehensive",
					"timeframe":   "7d",
				},
			},
		}

		result, err := server.handleGenerateFeedReport(context.Background(), req)
		if err != nil {
			t.Fatalf("handleGenerateFeedReport() failed: %v", err)
		}
		validatePromptResult(t, result)
	})
}

// validateErrorResult checks if result contains expected error message
func validateErrorResult(t *testing.T, result *mcp.GetPromptResult, expectedError string) {
	if result == nil || len(result.Messages) == 0 {
		t.Fatal("Expected error result")
	}
	message := result.Messages[0].Content.(*mcp.TextContent).Text
	if !strings.Contains(message, expectedError) {
		t.Errorf("Expected error about %s, got: %s", expectedError, message)
	}
}

// TestPromptParameterValidation tests parameter validation in prompt handlers
func TestPromptParameterValidation(t *testing.T) {
	server := createTestServer(t)

	t.Run("monitor_keywords requires keywords parameter", func(t *testing.T) {
		req := &mcp.GetPromptRequest{
			Params: &mcp.GetPromptParams{
				Arguments: map[string]string{
					"timeframe": "24h",
					// Missing required "keywords" parameter
				},
			},
		}

		result, err := server.handleMonitorKeywords(context.Background(), req)
		if err != nil {
			t.Fatalf("handleMonitorKeywords() failed: %v", err)
		}
		validateErrorResult(t, result, "Keywords parameter is required")
	})

	t.Run("compare_sources requires topic parameter", func(t *testing.T) {
		req := &mcp.GetPromptRequest{
			Params: &mcp.GetPromptParams{
				Arguments: map[string]string{
					// Missing required "topic" parameter
				},
			},
		}

		result, err := server.handleCompareSources(context.Background(), req)
		if err != nil {
			t.Fatalf("handleCompareSources() failed: %v", err)
		}
		validateErrorResult(t, result, "Topic parameter is required")
	})

	t.Run("invalid timeframe handling", func(t *testing.T) {
		req := &mcp.GetPromptRequest{
			Params: &mcp.GetPromptParams{
				Arguments: map[string]string{
					"timeframe": "invalid-duration",
				},
			},
		}

		result, err := server.handleAnalyzeFeedTrends(context.Background(), req)
		if err != nil {
			t.Fatalf("handleAnalyzeFeedTrends() failed: %v", err)
		}
		validateErrorResult(t, result, "Invalid timeframe")
	})
}

// TestPromptHelperFunctions tests the helper functions used by prompt handlers
func TestPromptHelperFunctions(t *testing.T) {
	t.Run("getStringArg with valid argument", func(t *testing.T) {
		args := map[string]string{
			"test_key": "test_value",
		}

		result := getStringArg(args, "test_key", "default")
		if result != "test_value" {
			t.Errorf("Expected 'test_value', got %s", result)
		}
	})

	t.Run("getStringArg with missing argument returns default", func(t *testing.T) {
		args := map[string]string{}

		result := getStringArg(args, "missing_key", "default_value")
		if result != "default_value" {
			t.Errorf("Expected 'default_value', got %s", result)
		}
	})

	t.Run("getIntArg with valid argument", func(t *testing.T) {
		args := map[string]string{
			"num_key": "42",
		}

		result := getIntArg(args, "num_key", 0)
		if result != 42 {
			t.Errorf("Expected 42, got %d", result)
		}
	})

	t.Run("getIntArg with invalid argument returns default", func(t *testing.T) {
		args := map[string]string{
			"num_key": "not-a-number",
		}

		result := getIntArg(args, "num_key", 10)
		if result != 10 {
			t.Errorf("Expected 10, got %d", result)
		}
	})

	t.Run("parseDuration with common formats", func(t *testing.T) {
		testCases := []struct {
			input    string
			expected string
		}{
			{"24h", "24h0m0s"},
			{"1d", "24h0m0s"},
			{"7d", "168h0m0s"},
			{"1w", "168h0m0s"},
			{"30d", "720h0m0s"},
		}

		for _, tc := range testCases {
			duration, err := parseDuration(tc.input)
			if err != nil {
				t.Errorf("parseDuration(%s) failed: %v", tc.input, err)
				continue
			}

			if duration.String() != tc.expected {
				t.Errorf("parseDuration(%s) = %s, expected %s", tc.input, duration, tc.expected)
			}
		}
	})
}
