package mcpserver

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/richardwooding/feed-mcp/model"
)

func TestResourceCacheConfig(t *testing.T) {
	t.Run("Default configuration", func(t *testing.T) {
		rm := NewResourceManager(&mockAllFeedsGetter{}, &mockFeedAndItemsGetter{})

		// Test default values
		if rm.cacheConfig.DefaultTTL != 10*time.Minute {
			t.Errorf("Expected DefaultTTL to be 10m, got %v", rm.cacheConfig.DefaultTTL)
		}
		if rm.cacheConfig.FeedListTTL != 5*time.Minute {
			t.Errorf("Expected FeedListTTL to be 5m, got %v", rm.cacheConfig.FeedListTTL)
		}
		if rm.cacheConfig.FeedItemsTTL != 10*time.Minute {
			t.Errorf("Expected FeedItemsTTL to be 10m, got %v", rm.cacheConfig.FeedItemsTTL)
		}
		if rm.cacheConfig.FeedMetadataTTL != 15*time.Minute {
			t.Errorf("Expected FeedMetadataTTL to be 15m, got %v", rm.cacheConfig.FeedMetadataTTL)
		}
	})

	t.Run("Custom configuration", func(t *testing.T) {
		config := &ResourceCacheConfig{
			DefaultTTL:      20 * time.Minute,
			FeedListTTL:     2 * time.Minute,
			FeedItemsTTL:    15 * time.Minute,
			FeedMetadataTTL: 30 * time.Minute,
			MaxCost:         1 << 20, // 1MB
			NumCounters:     500,
			BufferItems:     32,
		}

		rm := NewResourceManagerWithConfig(&mockAllFeedsGetter{}, &mockFeedAndItemsGetter{}, config)

		if rm.cacheConfig.DefaultTTL != 20*time.Minute {
			t.Errorf("Expected custom DefaultTTL to be 20m, got %v", rm.cacheConfig.DefaultTTL)
		}
		if rm.cacheConfig.FeedListTTL != 2*time.Minute {
			t.Errorf("Expected custom FeedListTTL to be 2m, got %v", rm.cacheConfig.FeedListTTL)
		}
		if rm.cacheConfig.MaxCost != 1<<20 {
			t.Errorf("Expected custom MaxCost to be 1MB, got %d", rm.cacheConfig.MaxCost)
		}
	})

	t.Run("Zero values use defaults", func(t *testing.T) {
		config := &ResourceCacheConfig{
			DefaultTTL:      0,
			FeedListTTL:     0,
			FeedItemsTTL:    0,
			FeedMetadataTTL: 0,
			MaxCost:         0,
			NumCounters:     0,
			BufferItems:     0,
		}

		rm := NewResourceManagerWithConfig(&mockAllFeedsGetter{}, &mockFeedAndItemsGetter{}, config)

		// Should fall back to defaults
		if rm.cacheConfig.DefaultTTL != 10*time.Minute {
			t.Errorf("Expected zero DefaultTTL to use default 10m, got %v", rm.cacheConfig.DefaultTTL)
		}
		if rm.cacheConfig.MaxCost != 1<<30 {
			t.Errorf("Expected zero MaxCost to use default 1GB, got %d", rm.cacheConfig.MaxCost)
		}
	})
}

func TestCacheKeyGeneration(t *testing.T) {
	rm := NewResourceManager(&mockAllFeedsGetter{}, &mockFeedAndItemsGetter{})

	tests := []struct {
		name     string
		uri      string
		expected string
	}{
		{
			name:     "Simple URI without parameters",
			uri:      "feeds://feed/abc123",
			expected: "resource:feeds://feed/abc123",
		},
		{
			name:     "URI with query parameters",
			uri:      "feeds://feed/abc123/items?limit=10&since=2023-01-01T00:00:00Z",
			expected: "resource:feeds://feed/abc123/items?hash=", // Hash will be appended
		},
		{
			name:     "Feed list URI",
			uri:      "feeds://all",
			expected: "resource:feeds://all",
		},
		{
			name:     "Invalid URI falls back",
			uri:      "not-a-valid://uri[",
			expected: "resource:not-a-valid://uri[",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rm.generateCacheKey(tt.uri)

			if strings.Contains(tt.expected, "?hash=") {
				// For URIs with parameters, just check the prefix
				prefix := strings.Split(tt.expected, "?hash=")[0]
				if !strings.HasPrefix(result, prefix) {
					t.Errorf("Expected cache key to start with %s, got %s", prefix, result)
				}
				if !strings.Contains(result, "?hash=") {
					t.Errorf("Expected cache key to contain hash parameter, got %s", result)
				}
			} else {
				if result != tt.expected {
					t.Errorf("Expected cache key %s, got %s", tt.expected, result)
				}
			}
		})
	}
}

func TestGetTTLForResourceType(t *testing.T) {
	config := &ResourceCacheConfig{
		DefaultTTL:      10 * time.Minute,
		FeedListTTL:     5 * time.Minute,
		FeedItemsTTL:    15 * time.Minute,
		FeedMetadataTTL: 20 * time.Minute,
	}

	rm := NewResourceManagerWithConfig(&mockAllFeedsGetter{}, &mockFeedAndItemsGetter{}, config)

	tests := []struct {
		name     string
		uri      string
		expected time.Duration
	}{
		{
			name:     "Feed list URI",
			uri:      "feeds://all",
			expected: 5 * time.Minute,
		},
		{
			name:     "Feed items URI",
			uri:      "feeds://feed/abc123/items",
			expected: 15 * time.Minute,
		},
		{
			name:     "Feed items URI with parameters",
			uri:      "feeds://feed/abc123/items?limit=10",
			expected: 15 * time.Minute,
		},
		{
			name:     "Feed metadata URI",
			uri:      "feeds://feed/abc123/meta",
			expected: 20 * time.Minute,
		},
		{
			name:     "Individual feed URI",
			uri:      "feeds://feed/abc123",
			expected: 10 * time.Minute,
		},
		{
			name:     "Other resource type",
			uri:      "feeds://other/resource",
			expected: 10 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rm.getTTLForResourceType(tt.uri)
			if result != tt.expected {
				t.Errorf("Expected TTL %v for URI %s, got %v", tt.expected, tt.uri, result)
			}
		})
	}
}

func TestCacheMetrics(t *testing.T) {
	rm := NewResourceManager(&mockAllFeedsGetter{}, &mockFeedAndItemsGetter{})

	// Initial metrics should be zero
	metrics := rm.GetCacheMetrics()
	if metrics.Hits != 0 || metrics.Misses != 0 || metrics.InvalidationHits != 0 {
		t.Errorf("Expected initial metrics to be zero, got Hits=%d, Misses=%d, InvalidationHits=%d",
			metrics.Hits, metrics.Misses, metrics.InvalidationHits)
	}

	// Record some metrics
	rm.recordCacheHit()
	rm.recordCacheHit()
	rm.recordCacheMiss()
	rm.recordCacheInvalidation()

	// Check updated metrics
	metrics = rm.GetCacheMetrics()
	if metrics.Hits != 2 {
		t.Errorf("Expected 2 hits, got %d", metrics.Hits)
	}
	if metrics.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", metrics.Misses)
	}
	if metrics.InvalidationHits != 1 {
		t.Errorf("Expected 1 invalidation, got %d", metrics.InvalidationHits)
	}
}

func TestCacheInvalidationHooks(t *testing.T) {
	rm := NewResourceManager(&mockAllFeedsGetter{}, &mockFeedAndItemsGetter{})

	// Track hook calls
	var hookCalls []string

	// Add hooks
	rm.AddCacheInvalidationHook(func(uri string) {
		hookCalls = append(hookCalls, "hook1:"+uri)
	})
	rm.AddCacheInvalidationHook(func(uri string) {
		hookCalls = append(hookCalls, "hook2:"+uri)
	})

	// Trigger hooks
	ctx := context.Background()
	_ = rm.InvalidateResourceCache(ctx, "feeds://test/uri")

	// Check hooks were called
	if len(hookCalls) != 2 {
		t.Errorf("Expected 2 hook calls, got %d", len(hookCalls))
	}

	expectedCalls := []string{"hook1:feeds://test/uri", "hook2:feeds://test/uri"}
	for i, expected := range expectedCalls {
		if i >= len(hookCalls) || hookCalls[i] != expected {
			t.Errorf("Expected hook call %d to be '%s', got '%s'", i, expected, hookCalls[i])
		}
	}
}

func TestCacheInvalidationMethods(t *testing.T) {
	rm := NewResourceManager(&mockAllFeedsGetter{}, &mockFeedAndItemsGetter{})
	ctx := context.Background()

	// Track invalidation hooks
	var invalidatedURIs []string
	rm.AddCacheInvalidationHook(func(uri string) {
		invalidatedURIs = append(invalidatedURIs, uri)
	})

	t.Run("InvalidateResourceCache", func(t *testing.T) {
		testResourceCacheInvalidation(t, rm, ctx, &invalidatedURIs)
	})

	t.Run("InvalidateCache (all resources)", func(t *testing.T) {
		testAllResourcesInvalidation(t, rm, ctx, &invalidatedURIs)
	})

	t.Run("InvalidateFeedCache", func(t *testing.T) {
		testFeedCacheInvalidation(t, rm, ctx, &invalidatedURIs)
	})
}

func testResourceCacheInvalidation(t *testing.T, rm *ResourceManager, ctx context.Context, invalidatedURIs *[]string) {
	*invalidatedURIs = nil // Reset

	err := rm.InvalidateResourceCache(ctx, "feeds://feed/test123/items")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if len(*invalidatedURIs) != 1 || (*invalidatedURIs)[0] != "feeds://feed/test123/items" {
		t.Errorf("Expected hook to be called with specific URI, got %v", *invalidatedURIs)
	}

	// Check metrics
	metrics := rm.GetCacheMetrics()
	if metrics.InvalidationHits == 0 {
		t.Errorf("Expected invalidation hit to be recorded")
	}
}

func testAllResourcesInvalidation(t *testing.T, rm *ResourceManager, ctx context.Context, invalidatedURIs *[]string) {
	*invalidatedURIs = nil // Reset

	err := rm.InvalidateCache(ctx)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if len(*invalidatedURIs) != 1 || (*invalidatedURIs)[0] != "*" {
		t.Errorf("Expected hook to be called with '*', got %v", *invalidatedURIs)
	}
}

func testFeedCacheInvalidation(t *testing.T, rm *ResourceManager, ctx context.Context, invalidatedURIs *[]string) {
	*invalidatedURIs = nil // Reset

	err := rm.InvalidateFeedCache(ctx, "test123")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Should invalidate all resource types for the feed
	expectedURIs := []string{
		"feeds://feed/test123",
		"feeds://feed/test123/items",
		"feeds://feed/test123/meta",
	}

	if len(*invalidatedURIs) != len(expectedURIs) {
		t.Errorf("Expected %d invalidated URIs, got %d: %v", len(expectedURIs), len(*invalidatedURIs), *invalidatedURIs)
		return
	}

	for _, expectedURI := range expectedURIs {
		found := false
		for _, actualURI := range *invalidatedURIs {
			if actualURI == expectedURI {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected URI %s to be invalidated, but it wasn't", expectedURI)
		}
	}
}

func TestResourceCacheIntegrationWithReading(t *testing.T) {
	mockStore, mockGetter := createTestMockObjects()
	rm := NewResourceManager(mockStore, mockGetter)
	ctx := context.Background()

	t.Run("Feed list caching", func(t *testing.T) {
		testFeedListCaching(t, rm, ctx)
	})

	t.Run("Feed items caching with parameters", func(t *testing.T) {
		testFeedItemsCachingWithParams(t, rm, ctx)
	})
}

func createTestMockObjects() (*mockAllFeedsGetter, *mockFeedAndItemsGetter) {
	mockStore := &mockAllFeedsGetter{
		feeds: []*model.FeedResult{
			{
				ID:        "test-feed",
				Title:     "Test Feed",
				PublicURL: "https://example.com/feed.xml",
				Feed:      &model.Feed{Title: "Test Feed"},
			},
		},
	}

	mockGetter := &mockFeedAndItemsGetter{
		feedMap: map[string]*model.FeedAndItemsResult{
			"test-feed": {
				ID:        "test-feed",
				Title:     "Test Feed",
				PublicURL: "https://example.com/feed.xml",
				Feed:      &model.Feed{Title: "Test Feed"},
				Items: []*gofeed.Item{
					{Title: "Test Item 1"},
					{Title: "Test Item 2"},
				},
			},
		},
	}

	return mockStore, mockGetter
}

func testFeedListCaching(t *testing.T, rm *ResourceManager, ctx context.Context) {
	// Check initial metrics
	initialMetrics := rm.GetCacheMetrics()
	t.Logf("Initial metrics: Hits=%d, Misses=%d", initialMetrics.Hits, initialMetrics.Misses)

	// First request should be a cache miss
	result1, err := rm.ReadResource(ctx, "feeds://all")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	metrics1 := rm.GetCacheMetrics()
	t.Logf("After first request: Hits=%d, Misses=%d", metrics1.Hits, metrics1.Misses)
	if metrics1.Misses != initialMetrics.Misses+1 {
		t.Errorf("Expected %d cache miss, got %d", initialMetrics.Misses+1, metrics1.Misses)
	}

	// Ristretto cache is async, give it time to process the Set operation
	time.Sleep(10 * time.Millisecond)

	// Second request should be a cache hit
	result2, err := rm.ReadResource(ctx, "feeds://all")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	metrics2 := rm.GetCacheMetrics()
	t.Logf("After second request: Hits=%d, Misses=%d", metrics2.Hits, metrics2.Misses)
	if metrics2.Misses != metrics1.Misses {
		t.Errorf("Expected still %d cache misses, got %d", metrics1.Misses, metrics2.Misses)
	}
	if metrics2.Hits != metrics1.Hits+1 {
		t.Errorf("Expected %d cache hits, got %d", metrics1.Hits+1, metrics2.Hits)
	}

	// Results should be identical
	if result1.Contents[0].Text != result2.Contents[0].Text {
		t.Errorf("Cache hit should return same content as cache miss")
	}
}

func testFeedItemsCachingWithParams(t *testing.T, rm *ResourceManager, ctx context.Context) {
	// Reset metrics
	rm.cacheMetrics.Hits = 0
	rm.cacheMetrics.Misses = 0

	// Different URIs with different parameters should have different cache keys
	uri1 := "feeds://feed/test-feed/items?limit=5"
	uri2 := "feeds://feed/test-feed/items?limit=10"

	// First request to uri1 - cache miss
	_, err := rm.ReadResource(ctx, uri1)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// First request to uri2 - cache miss (different parameters)
	_, err = rm.ReadResource(ctx, uri2)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Ristretto cache is async, give it time to process Set operations
	time.Sleep(10 * time.Millisecond)

	// Second request to uri1 - cache hit
	_, err = rm.ReadResource(ctx, uri1)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	metrics := rm.GetCacheMetrics()
	if metrics.Misses != 2 {
		t.Errorf("Expected 2 cache misses (different URIs), got %d", metrics.Misses)
	}
	if metrics.Hits != 1 {
		t.Errorf("Expected 1 cache hit (repeated URI), got %d", metrics.Hits)
	}
}
