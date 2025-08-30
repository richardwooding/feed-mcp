package mcpserver

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/richardwooding/feed-mcp/model"
)

// BenchmarkResourceListing tests the performance of listing resources
func BenchmarkResourceListing(b *testing.B) {
	rm := createBenchmarkResourceManager(100) // 100 feeds for benchmark
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := rm.ListResources(ctx)
		if err != nil {
			b.Fatalf("ListResources failed: %v", err)
		}
	}
}

// BenchmarkResourceListingConcurrent tests concurrent resource listing performance
func BenchmarkResourceListingConcurrent(b *testing.B) {
	rm := createBenchmarkResourceManager(100)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := rm.ListResources(ctx)
			if err != nil {
				b.Errorf("Concurrent ListResources failed: %v", err)
			}
		}
	})
}

// BenchmarkResourceReading tests reading different resource types
func BenchmarkResourceReading(b *testing.B) {
	rm := createBenchmarkResourceManager(10)
	ctx := context.Background()
	feedID := model.GenerateFeedID("https://example.com/feed1.xml")

	testCases := []struct {
		name string
		uri  string
	}{
		{"FeedList", FeedListURI},
		{"Feed", strings.Replace(FeedURI, "{feedId}", feedID, 1)},
		{"FeedItems", strings.Replace(FeedItemsURI, "{feedId}", feedID, 1)},
		{"FeedMeta", strings.Replace(FeedMetaURI, "{feedId}", feedID, 1)},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			// Warm up cache
			_, _ = rm.ReadResource(ctx, tc.uri)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, err := rm.ReadResource(ctx, tc.uri)
				if err != nil {
					b.Fatalf("ReadResource failed for %s: %v", tc.name, err)
				}
			}
		})
	}
}

// BenchmarkResourceReadingColdCache tests performance without cache
func BenchmarkResourceReadingColdCache(b *testing.B) {
	rm := createBenchmarkResourceManager(10)
	ctx := context.Background()
	feedID := model.GenerateFeedID("https://example.com/feed1.xml")

	testCases := []struct {
		name string
		uri  string
	}{
		{"FeedList", FeedListURI},
		{"Feed", strings.Replace(FeedURI, "{feedId}", feedID, 1)},
		{"FeedItems", strings.Replace(FeedItemsURI, "{feedId}", feedID, 1)},
		{"FeedMeta", strings.Replace(FeedMetaURI, "{feedId}", feedID, 1)},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Clear cache before each iteration
				_ = rm.InvalidateCache(ctx)

				_, err := rm.ReadResource(ctx, tc.uri)
				if err != nil {
					b.Fatalf("ReadResource failed for %s: %v", tc.name, err)
				}
			}
		})
	}
}

// BenchmarkResourceReadingWithFilters tests performance with URI parameters
func BenchmarkResourceReadingWithFilters(b *testing.B) {
	rm := createBenchmarkResourceManager(5)
	ctx := context.Background()
	feedID := model.GenerateFeedID("https://example.com/feed1.xml")

	testCases := []struct {
		name string
		uri  string
	}{
		{"NoFilters", strings.Replace(FeedItemsURI, "{feedId}", feedID, 1)},
		{"WithLimit", strings.Replace(FeedItemsURI, "{feedId}", feedID, 1) + "?limit=10"},
		{"WithDateRange", strings.Replace(FeedItemsURI, "{feedId}", feedID, 1) + "?since=2023-01-01T00:00:00Z&until=2023-12-31T23:59:59Z"},
		{"WithSearch", strings.Replace(FeedItemsURI, "{feedId}", feedID, 1) + "?search=test"},
		{"WithMultipleFilters", strings.Replace(FeedItemsURI, "{feedId}", feedID, 1) + "?limit=5&author=test&category=news"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			// Warm up cache
			_, _ = rm.ReadResource(ctx, tc.uri)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, err := rm.ReadResource(ctx, tc.uri)
				if err != nil {
					b.Fatalf("ReadResource with filters failed for %s: %v", tc.name, err)
				}
			}
		})
	}
}

// BenchmarkCacheOperations tests cache performance
func BenchmarkCacheOperations(b *testing.B) {
	rm := createBenchmarkResourceManager(10)
	ctx := context.Background()

	testCases := []struct {
		name string
		op   func() error
	}{
		{"CacheSet", func() error {
			key := fmt.Sprintf("test-key-%d", time.Now().UnixNano())
			return rm.resourceCache.Set(ctx, key, "test-value")
		}},
		{"CacheGet", func() error {
			// Pre-populate some cache entries
			key := "benchmark-key"
			_ = rm.resourceCache.Set(ctx, key, "benchmark-value")
			time.Sleep(1 * time.Millisecond) // Allow cache to process
			_, err := rm.resourceCache.Get(ctx, key)
			return err
		}},
		{"CacheInvalidate", func() error {
			return rm.InvalidateCache(ctx)
		}},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				err := tc.op()
				if err != nil && !strings.Contains(err.Error(), "not found") {
					b.Fatalf("Cache operation failed for %s: %v", tc.name, err)
				}
			}
		})
	}
}

// BenchmarkSubscriptionOperations tests subscription performance
func BenchmarkSubscriptionOperations(b *testing.B) {
	rm := createBenchmarkResourceManager(10)
	feedID := model.GenerateFeedID("https://example.com/feed1.xml")
	uri := strings.Replace(FeedItemsURI, "{feedId}", feedID, 1)

	testCases := []struct {
		name string
		op   func(sessionID string) error
	}{
		{"Subscribe", func(sessionID string) error {
			return rm.Subscribe(sessionID, uri)
		}},
		{"Unsubscribe", func(sessionID string) error {
			// Subscribe first
			_ = rm.Subscribe(sessionID, uri)
			return rm.Unsubscribe(sessionID, uri)
		}},
		{"GetSubscribedSessions", func(sessionID string) error {
			_ = rm.Subscribe(sessionID, uri)
			_ = rm.GetSubscribedSessions(uri)
			return nil
		}},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				sessionID := fmt.Sprintf("session-%d", i)
				rm.CreateSession(sessionID)

				err := tc.op(sessionID)
				if err != nil {
					b.Fatalf("Subscription operation failed for %s: %v", tc.name, err)
				}

				rm.RemoveSession(sessionID)
			}
		})
	}
}

// BenchmarkConcurrentSubscriptions tests concurrent subscription handling
func BenchmarkConcurrentSubscriptions(b *testing.B) {
	rm := createBenchmarkResourceManager(10)
	feedID := model.GenerateFeedID("https://example.com/feed1.xml")
	uri := strings.Replace(FeedItemsURI, "{feedId}", feedID, 1)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		sessionID := fmt.Sprintf("session-%d", time.Now().UnixNano())
		rm.CreateSession(sessionID)
		defer rm.RemoveSession(sessionID)

		for pb.Next() {
			_ = rm.Subscribe(sessionID, uri)
			_ = rm.Unsubscribe(sessionID, uri)
		}
	})
}

// BenchmarkMemoryUsage measures memory usage patterns
func BenchmarkMemoryUsage(b *testing.B) {
	feedCounts := []int{10, 50, 100, 500}

	for _, count := range feedCounts {
		b.Run(fmt.Sprintf("Feeds_%d", count), func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				rm := createBenchmarkResourceManager(count)
				ctx := context.Background()

				// Exercise the resource manager
				_, _ = rm.ListResources(ctx)
				for j := 0; j < 10; j++ {
					feedID := model.GenerateFeedID(fmt.Sprintf("https://example.com/feed%d.xml", j%count))
					uri := strings.Replace(FeedItemsURI, "{feedId}", feedID, 1)
					_, _ = rm.ReadResource(ctx, uri)
				}

				// Force garbage collection to measure retained memory
				runtime.GC()
			}
		})
	}
}

// BenchmarkLargeScale tests performance with many concurrent operations
func BenchmarkLargeScale(b *testing.B) {
	rm := createBenchmarkResourceManager(100)
	ctx := context.Background()

	// Pre-warm cache and create sessions
	sessionCount := 50
	sessions := make([]string, sessionCount)
	for i := 0; i < sessionCount; i++ {
		sessions[i] = fmt.Sprintf("session-%d", i)
		rm.CreateSession(sessions[i])
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			sessionID := sessions[time.Now().Nanosecond()%sessionCount]
			feedID := model.GenerateFeedID(fmt.Sprintf("https://example.com/feed%d.xml", time.Now().Nanosecond()%100))

			// Mix of operations
			switch time.Now().Nanosecond() % 4 {
			case 0:
				_, _ = rm.ListResources(ctx)
			case 1:
				uri := strings.Replace(FeedItemsURI, "{feedId}", feedID, 1)
				_, _ = rm.ReadResource(ctx, uri)
			case 2:
				uri := strings.Replace(FeedURI, "{feedId}", feedID, 1)
				_ = rm.Subscribe(sessionID, uri)
			case 3:
				_ = rm.GetSubscribedSessions(fmt.Sprintf("feeds://feed/%s/items", feedID))
			}
		}
	})

	// Clean up sessions
	for _, sessionID := range sessions {
		rm.RemoveSession(sessionID)
	}
}

// createBenchmarkResourceManager creates a ResourceManager with test data optimized for benchmarks
func createBenchmarkResourceManager(feedCount int) *ResourceManager {
	feeds := make([]*model.FeedResult, feedCount)
	feedsMap := make(map[string]*model.FeedAndItemsResult)

	for i := 0; i < feedCount; i++ {
		url := fmt.Sprintf("https://example.com/feed%d.xml", i)
		feedID := model.GenerateFeedID(url)

		// Create feed with realistic data size
		feed := &model.FeedResult{
			ID:        feedID,
			Title:     fmt.Sprintf("Benchmark Feed %d", i),
			PublicURL: url,
		}
		feeds[i] = feed

		// Create items with realistic content
		items := make([]*gofeed.Item, 20) // 20 items per feed
		for j := 0; j < 20; j++ {
			items[j] = &gofeed.Item{
				Title:       fmt.Sprintf("Item %d-%d", i, j),
				Description: fmt.Sprintf("Description for item %d in feed %d with some content", j, i),
				Link:        fmt.Sprintf("https://example.com/feed%d/item%d", i, j),
				Published:   time.Now().Add(-time.Duration(j) * time.Hour).Format(time.RFC3339),
				Authors: []*gofeed.Person{
					{Name: fmt.Sprintf("Author %d", j%3), Email: fmt.Sprintf("author%d@example.com", j%3)},
				},
				Categories: []string{fmt.Sprintf("category%d", j%5), "benchmark"},
				GUID:       fmt.Sprintf("guid-%d-%d", i, j),
			}
		}

		feedAndItems := &model.FeedAndItemsResult{
			ID:        feedID,
			Title:     fmt.Sprintf("Benchmark Feed %d", i),
			PublicURL: url,
			Feed:      &model.Feed{Title: fmt.Sprintf("Benchmark Feed %d", i), Description: fmt.Sprintf("A benchmark test feed number %d", i)},
			Items:     items,
		}
		feedsMap[feedID] = feedAndItems
	}

	mockAllFeeds := &mockAllFeedsGetter{feeds: feeds}
	mockFeedGetter := &mockFeedAndItemsGetter{feedMap: feedsMap}

	// Use custom cache config optimized for benchmarks
	config := &ResourceCacheConfig{
		DefaultTTL:      5 * time.Minute,
		FeedListTTL:     2 * time.Minute,
		FeedItemsTTL:    5 * time.Minute,
		FeedMetadataTTL: 10 * time.Minute,
		MaxCost:         64 << 20, // 64MB for benchmarks
		NumCounters:     10000,    // Track more keys for benchmarks
		BufferItems:     128,      // Larger buffer for benchmarks
	}

	return NewResourceManagerWithConfig(mockAllFeeds, mockFeedGetter, config)
}
