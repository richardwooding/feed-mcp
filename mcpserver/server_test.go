package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mmcdole/gofeed"
	"github.com/richardwooding/feed-mcp/model"
)

// Mock implementations for testing
type mockAllFeedsGetter struct {
	feeds []*model.FeedResult
	err   error
}

func (m *mockAllFeedsGetter) GetAllFeeds(ctx context.Context) ([]*model.FeedResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.feeds, nil
}

type mockFeedAndItemsGetter struct {
	feedMap map[string]*model.FeedAndItemsResult
	err     error
}

func (m *mockFeedAndItemsGetter) GetFeedAndItems(ctx context.Context, id string) (*model.FeedAndItemsResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	if result, exists := m.feedMap[id]; exists {
		return result, nil
	}
	return nil, errors.New("feed not found")
}

func TestNewServer(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: Config{
				Transport:          model.StdioTransport,
				AllFeedsGetter:     &mockAllFeedsGetter{},
				FeedAndItemsGetter: &mockFeedAndItemsGetter{},
			},
			wantErr: false,
		},
		{
			name: "undefined transport",
			config: Config{
				Transport:          model.UndefinedTransport,
				AllFeedsGetter:     &mockAllFeedsGetter{},
				FeedAndItemsGetter: &mockFeedAndItemsGetter{},
			},
			wantErr: true,
			errMsg:  "transport must be specified",
		},
		{
			name: "nil AllFeedsGetter",
			config: Config{
				Transport:          model.StdioTransport,
				AllFeedsGetter:     nil,
				FeedAndItemsGetter: &mockFeedAndItemsGetter{},
			},
			wantErr: true,
			errMsg:  "AllFeedsGetter is required",
		},
		{
			name: "nil FeedAndItemsGetter",
			config: Config{
				Transport:      model.StdioTransport,
				AllFeedsGetter: &mockAllFeedsGetter{},
			},
			wantErr: true,
			errMsg:  "FeedAndItemsGetter is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(tt.config)
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewServer() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if err.Error() != tt.errMsg {
					t.Errorf("NewServer() error = %v, want %v", err.Error(), tt.errMsg)
				}
				return
			}
			
			if err != nil {
				t.Errorf("NewServer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			if server == nil {
				t.Error("NewServer() returned nil server")
				return
			}
			
			if server.transport != tt.config.Transport {
				t.Errorf("NewServer() transport = %v, want %v", server.transport, tt.config.Transport)
			}
		})
	}
}

func TestServerToolsIntegration(t *testing.T) {
	// Create test data
	testFeeds := []*model.FeedResult{
		{
			ID:        "feed1",
			PublicURL: "https://example.com/feed1.xml",
			Title:     "Test Feed 1",
			Feed: &model.Feed{
				Title: "Test Feed 1",
				Link:  "https://example.com",
			},
		},
		{
			ID:        "feed2",
			PublicURL: "https://example.com/feed2.xml",
			Title:     "Test Feed 2",
			Feed: &model.Feed{
				Title: "Test Feed 2", 
				Link:  "https://example2.com",
			},
		},
	}

	testFeedAndItems := map[string]*model.FeedAndItemsResult{
		"feed1": {
			ID:        "feed1",
			PublicURL: "https://example.com/feed1.xml",
			Title:     "Test Feed 1",
			Feed: &model.Feed{
				Title: "Test Feed 1",
				Link:  "https://example.com",
			},
			Items: []*gofeed.Item{
				{
					Title: "Test Item 1",
					Link:  "https://example.com/item1",
				},
			},
		},
	}

	// Create mock server for fetch_link testing
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Test HTML content"))
	}))
	defer testServer.Close()

	// Create server with mocks
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

	t.Run("test all_syndication_feeds tool functionality", func(t *testing.T) {
		// Test the tool logic by calling the internal methods directly
		ctx := context.Background()
		result, err := server.allFeedsGetter.GetAllFeeds(ctx)
		if err != nil {
			t.Fatalf("GetAllFeeds() failed: %v", err)
		}

		if len(result) != 2 {
			t.Errorf("GetAllFeeds() returned %d feeds, want 2", len(result))
		}

		// Verify the result can be marshaled to JSON
		data, err := json.Marshal(result)
		if err != nil {
			t.Fatalf("Failed to marshal feeds to JSON: %v", err)
		}

		var unmarshaled []*model.FeedResult
		err = json.Unmarshal(data, &unmarshaled)
		if err != nil {
			t.Fatalf("Failed to unmarshal feeds from JSON: %v", err)
		}

		if len(unmarshaled) != 2 {
			t.Errorf("Unmarshaled %d feeds, want 2", len(unmarshaled))
		}
	})

	t.Run("test get_syndication_feed_items tool functionality", func(t *testing.T) {
		ctx := context.Background()
		result, err := server.feedAndItemsGetter.GetFeedAndItems(ctx, "feed1")
		if err != nil {
			t.Fatalf("GetFeedAndItems() failed: %v", err)
		}

		if result.ID != "feed1" {
			t.Errorf("GetFeedAndItems() returned ID %s, want feed1", result.ID)
		}

		if len(result.Items) != 1 {
			t.Errorf("GetFeedAndItems() returned %d items, want 1", len(result.Items))
		}

		// Verify the result can be marshaled to JSON
		data, err := json.Marshal(result)
		if err != nil {
			t.Fatalf("Failed to marshal feed and items to JSON: %v", err)
		}

		var unmarshaled model.FeedAndItemsResult
		err = json.Unmarshal(data, &unmarshaled)
		if err != nil {
			t.Fatalf("Failed to unmarshal feed and items from JSON: %v", err)
		}

		if unmarshaled.ID != "feed1" {
			t.Errorf("Unmarshaled ID %s, want feed1", unmarshaled.ID)
		}
	})

	t.Run("test get_syndication_feed_items with non-existent feed", func(t *testing.T) {
		ctx := context.Background()
		_, err := server.feedAndItemsGetter.GetFeedAndItems(ctx, "nonexistent")
		if err == nil {
			t.Error("GetFeedAndItems() should have failed for non-existent feed")
		}
	})
}

func TestServerWithErrors(t *testing.T) {
	t.Run("AllFeedsGetter returns error", func(t *testing.T) {
		mockAllFeeds := &mockAllFeedsGetter{err: errors.New("database connection failed")}
		mockFeedItems := &mockFeedAndItemsGetter{feedMap: make(map[string]*model.FeedAndItemsResult)}
		
		config := Config{
			Transport:          model.StdioTransport,
			AllFeedsGetter:     mockAllFeeds,
			FeedAndItemsGetter: mockFeedItems,
		}
		
		server, err := NewServer(config)
		if err != nil {
			t.Fatalf("NewServer() failed: %v", err)
		}

		ctx := context.Background()
		_, err = server.allFeedsGetter.GetAllFeeds(ctx)
		if err == nil {
			t.Error("GetAllFeeds() should have returned an error")
		}
		if err.Error() != "database connection failed" {
			t.Errorf("GetAllFeeds() error = %v, want 'database connection failed'", err)
		}
	})

	t.Run("FeedAndItemsGetter returns error", func(t *testing.T) {
		mockAllFeeds := &mockAllFeedsGetter{feeds: []*model.FeedResult{}}
		mockFeedItems := &mockFeedAndItemsGetter{err: errors.New("feed service unavailable")}
		
		config := Config{
			Transport:          model.StdioTransport,
			AllFeedsGetter:     mockAllFeeds,
			FeedAndItemsGetter: mockFeedItems,
		}
		
		server, err := NewServer(config)
		if err != nil {
			t.Fatalf("NewServer() failed: %v", err)
		}

		ctx := context.Background()
		_, err = server.feedAndItemsGetter.GetFeedAndItems(ctx, "feed1")
		if err == nil {
			t.Error("GetFeedAndItems() should have returned an error")
		}
		if err.Error() != "feed service unavailable" {
			t.Errorf("GetFeedAndItems() error = %v, want 'feed service unavailable'", err)
		}
	})
}

func TestFetchLinkParams(t *testing.T) {
	params := FetchLinkParams{URL: "https://example.com"}
	if params.URL != "https://example.com" {
		t.Errorf("FetchLinkParams.URL = %s, want https://example.com", params.URL)
	}
}

func TestGetSyndicationFeedParams(t *testing.T) {
	params := GetSyndicationFeedParams{ID: "feed123"}
	if params.ID != "feed123" {
		t.Errorf("GetSyndicationFeedParams.ID = %s, want feed123", params.ID)
	}
}

func TestServerTransportTypes(t *testing.T) {
	tests := []struct {
		name      string
		transport model.Transport
		valid     bool
	}{
		{
			name:      "stdio transport",
			transport: model.StdioTransport,
			valid:     true,
		},
		{
			name:      "http with sse transport",
			transport: model.HttpWithSSETransport,
			valid:     true,
		},
		{
			name:      "undefined transport",
			transport: model.UndefinedTransport,
			valid:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{
				Transport:          tt.transport,
				AllFeedsGetter:     &mockAllFeedsGetter{},
				FeedAndItemsGetter: &mockFeedAndItemsGetter{},
			}

			server, err := NewServer(config)
			
			if tt.valid {
				if err != nil {
					t.Errorf("NewServer() error = %v, want nil for valid transport", err)
				}
				if server == nil {
					t.Error("NewServer() returned nil server for valid transport")
				} else if server.transport != tt.transport {
					t.Errorf("Server transport = %v, want %v", server.transport, tt.transport)
				}
			} else {
				if err == nil {
					t.Error("NewServer() should have failed for invalid transport")
				}
			}
		})
	}
}

// Benchmark tests
func BenchmarkNewServer(b *testing.B) {
	config := Config{
		Transport:          model.StdioTransport,
		AllFeedsGetter:     &mockAllFeedsGetter{},
		FeedAndItemsGetter: &mockFeedAndItemsGetter{},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := NewServer(config)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetAllFeeds(b *testing.B) {
	feeds := make([]*model.FeedResult, 100)
	for i := 0; i < 100; i++ {
		feeds[i] = &model.FeedResult{
			ID:        "feed" + string(rune(i)),
			PublicURL: "https://example.com/feed" + string(rune(i)) + ".xml",
			Title:     "Test Feed " + string(rune(i)),
		}
	}

	mockAllFeeds := &mockAllFeedsGetter{feeds: feeds}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := mockAllFeeds.GetAllFeeds(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}
