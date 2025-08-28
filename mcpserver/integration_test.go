package mcpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/richardwooding/feed-mcp/model"
)

func TestServerRunMethod(t *testing.T) {
	// Create a mock HTTP server for testing the fetch_link functionality
	mockHTTPServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html><body>Test Content</body></html>"))
	}))
	defer mockHTTPServer.Close()

	// Test data
	testFeeds := []*model.FeedResult{
		{
			ID:        "test-feed-1",
			PublicURL: "https://example.com/feed.xml",
			Title:     "Test Feed",
			Feed: &model.Feed{
				Title:       "Test Feed",
				Link:        "https://example.com",
				Description: "A test feed",
			},
		},
	}

	testFeedAndItems := map[string]*model.FeedAndItemsResult{
		"test-feed-1": {
			ID:        "test-feed-1",
			PublicURL: "https://example.com/feed.xml",
			Title:     "Test Feed",
			Feed: &model.Feed{
				Title:       "Test Feed",
				Link:        "https://example.com",
				Description: "A test feed",
			},
			Items: []*gofeed.Item{
				{
					Title: "Test Item 1",
					Link:  "https://example.com/item1",
				},
			},
		},
	}

	// Create mocks
	mockAllFeeds := &mockAllFeedsGetter{feeds: testFeeds}
	mockFeedItems := &mockFeedAndItemsGetter{feedMap: testFeedAndItems}

	// Test different transport configurations
	tests := []struct {
		name      string
		transport model.Transport
		expectErr bool
	}{
		{
			name:      "undefined transport should error",
			transport: model.UndefinedTransport,
			expectErr: true,
		},
		// Note: We can't easily test StdioTransport and HttpWithSSETransport in unit tests
		// because they require actual transport connections. Those would be integration tests.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{
				Transport:          tt.transport,
				AllFeedsGetter:     mockAllFeeds,
				FeedAndItemsGetter: mockFeedItems,
			}

			server, err := NewServer(config)
			if err != nil && !tt.expectErr {
				t.Fatalf("NewServer() failed: %v", err)
			}
			if err == nil && tt.expectErr {
				t.Fatal("NewServer() should have failed but didn't")
			}

			if tt.expectErr {
				return // Skip the Run test for invalid configurations
			}

			// Test the Run method in a goroutine with timeout
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			done := make(chan error, 1)
			go func() {
				done <- server.Run()
			}()

			select {
			case err := <-done:
				if tt.transport == model.UndefinedTransport {
					if err == nil {
						t.Error("Run() should have failed for undefined transport")
					}
				}
			case <-ctx.Done():
				// Expected for valid transports that would run indefinitely
			}
		})
	}
}

func TestServerMockDataConsistency(t *testing.T) {
	// Test that our mock data structures are consistent with the expected types
	t.Run("mock feeds data structure", func(t *testing.T) {
		feeds := []*model.FeedResult{
			{
				ID:        "test",
				PublicURL: "https://example.com",
				Title:     "Test",
				Feed: &model.Feed{
					Title: "Test Feed",
				},
			},
		}

		mock := &mockAllFeedsGetter{feeds: feeds}
		result, err := mock.GetAllFeeds(context.Background())
		if err != nil {
			t.Fatalf("GetAllFeeds() failed: %v", err)
		}

		if len(result) != 1 {
			t.Errorf("Expected 1 feed, got %d", len(result))
		}

		if result[0].ID != "test" {
			t.Errorf("Expected ID 'test', got %s", result[0].ID)
		}
	})

	t.Run("mock feed and items data structure", func(t *testing.T) {
		feedMap := map[string]*model.FeedAndItemsResult{
			"test": {
				ID:        "test",
				PublicURL: "https://example.com",
				Title:     "Test",
				Items:     []*gofeed.Item{},
			},
		}

		mock := &mockFeedAndItemsGetter{feedMap: feedMap}
		result, err := mock.GetFeedAndItems(context.Background(), "test")
		if err != nil {
			t.Fatalf("GetFeedAndItems() failed: %v", err)
		}

		if result.ID != "test" {
			t.Errorf("Expected ID 'test', got %s", result.ID)
		}
	})
}

func TestServerConfigValidation(t *testing.T) {
	// Test various configuration edge cases
	t.Run("all nil configuration", func(t *testing.T) {
		config := Config{}
		_, err := NewServer(config)
		if err == nil {
			t.Error("NewServer() should fail with empty config")
		}
	})

	t.Run("partial configuration", func(t *testing.T) {
		config := Config{
			Transport: model.StdioTransport,
		}
		_, err := NewServer(config)
		if err == nil {
			t.Error("NewServer() should fail with partial config")
		}
	})
}

// Test for proper interface compliance
var (
	_ AllFeedsGetter     = (*mockAllFeedsGetter)(nil)
	_ FeedAndItemsGetter = (*mockFeedAndItemsGetter)(nil)
)

func TestInterfaceCompliance(t *testing.T) {
	// This test ensures our mocks properly implement the required interfaces
	var allFeeds AllFeedsGetter = &mockAllFeedsGetter{}
	var feedItems FeedAndItemsGetter = &mockFeedAndItemsGetter{}

	// Test that we can call the interface methods
	ctx := context.Background()

	_, err := allFeeds.GetAllFeeds(ctx)
	if err != nil {
		t.Errorf("AllFeedsGetter interface not properly implemented: %v", err)
	}

	_, err = feedItems.GetFeedAndItems(ctx, "test")
	if err == nil {
		t.Error("Expected error for empty feedMap, but got nil")
	}
}

func TestMockErrorScenarios(t *testing.T) {
	t.Run("mock all feeds getter with error", func(t *testing.T) {
		mock := &mockAllFeedsGetter{
			err: context.DeadlineExceeded,
		}

		_, err := mock.GetAllFeeds(context.Background())
		if err != context.DeadlineExceeded {
			t.Errorf("Expected DeadlineExceeded error, got %v", err)
		}
	})

	t.Run("mock feed items getter with error", func(t *testing.T) {
		mock := &mockFeedAndItemsGetter{
			err: context.Canceled,
		}

		_, err := mock.GetFeedAndItems(context.Background(), "test")
		if err != context.Canceled {
			t.Errorf("Expected Canceled error, got %v", err)
		}
	})
}
