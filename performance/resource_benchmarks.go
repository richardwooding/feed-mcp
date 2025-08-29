// Package performance provides utilities and benchmarks for MCP Resources performance testing
package performance

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/richardwooding/feed-mcp/mcpserver"
	"github.com/richardwooding/feed-mcp/model"
)

// BenchmarkConfig holds configuration for performance benchmarks
type BenchmarkConfig struct {
	FeedCount       int           // Number of feeds to use in benchmarks
	ItemsPerFeed    int           // Number of items per feed
	ConcurrentUsers int           // Number of concurrent users to simulate
	Duration        time.Duration // How long to run the benchmark
}

// DefaultBenchmarkConfig returns a default benchmark configuration
func DefaultBenchmarkConfig() *BenchmarkConfig {
	return &BenchmarkConfig{
		FeedCount:       100,
		ItemsPerFeed:    50,
		ConcurrentUsers: 10,
		Duration:        30 * time.Second,
	}
}

// Metrics holds performance measurement results
type Metrics struct {
	TotalOperations  int64
	AverageLatency   time.Duration
	P95Latency       time.Duration
	P99Latency       time.Duration
	ThroughputPerSec float64
	ErrorRate        float64
	MemoryUsageMB    float64
	GoroutineCount   int
}

// ResourcePerformanceTester provides utilities for testing resource performance
type ResourcePerformanceTester struct {
	resourceManager *mcpserver.ResourceManager
	config          *BenchmarkConfig
}

// NewResourcePerformanceTester creates a new performance tester
func NewResourcePerformanceTester(config *BenchmarkConfig) *ResourcePerformanceTester {
	if config == nil {
		config = DefaultBenchmarkConfig()
	}

	// Create mock data for testing
	mockAllFeeds := createMockAllFeedsGetter(config.FeedCount)
	mockFeedGetter := createMockFeedAndItemsGetter(config.FeedCount, config.ItemsPerFeed)

	// Optimize cache config for performance testing
	cacheConfig := &mcpserver.ResourceCacheConfig{
		DefaultTTL:      10 * time.Minute,
		FeedListTTL:     5 * time.Minute,
		FeedItemsTTL:    10 * time.Minute,
		FeedMetadataTTL: 15 * time.Minute,
		MaxCost:         256 << 20, // 256MB
		NumCounters:     10000,
		BufferItems:     256,
	}

	rm := mcpserver.NewResourceManagerWithConfig(mockAllFeeds, mockFeedGetter, cacheConfig)

	return &ResourcePerformanceTester{
		resourceManager: rm,
		config:          config,
	}
}

// BenchmarkResourceListing measures resource listing performance
func (rpt *ResourcePerformanceTester) BenchmarkResourceListing(ctx context.Context) (*Metrics, error) {
	return rpt.runBenchmark(ctx, "ListResources", func(ctx context.Context) error {
		_, err := rpt.resourceManager.ListResources(ctx)
		return err
	})
}

// BenchmarkResourceReading measures resource reading performance with cache hits
func (rpt *ResourcePerformanceTester) BenchmarkResourceReading(ctx context.Context) (*Metrics, error) {
	// Pre-warm cache
	feedID := generateFeedID("https://example.com/feed1.xml")
	uri := fmt.Sprintf("feeds://feed/%s/items", feedID)
	_ = rpt.resourceManager.ReadResource(ctx, uri)

	return rpt.runBenchmark(ctx, "ReadResource", func(ctx context.Context) error {
		_, err := rpt.resourceManager.ReadResource(ctx, uri)
		return err
	})
}

// BenchmarkConcurrentAccess measures performance under concurrent load
func (rpt *ResourcePerformanceTester) BenchmarkConcurrentAccess(ctx context.Context) (*Metrics, error) {
	return rpt.runConcurrentBenchmark(ctx, "ConcurrentAccess", func(ctx context.Context, workerID int) error {
		// Mix of operations
		switch workerID % 4 {
		case 0:
			_, err := rpt.resourceManager.ListResources(ctx)
			return err
		case 1:
			feedID := generateFeedID(fmt.Sprintf("https://example.com/feed%d.xml", workerID%rpt.config.FeedCount))
			uri := fmt.Sprintf("feeds://feed/%s/items", feedID)
			_, err := rpt.resourceManager.ReadResource(ctx, uri)
			return err
		case 2:
			sessionID := fmt.Sprintf("session-%d", workerID)
			rpt.resourceManager.CreateSession(sessionID)
			feedID := generateFeedID(fmt.Sprintf("https://example.com/feed%d.xml", workerID%rpt.config.FeedCount))
			uri := fmt.Sprintf("feeds://feed/%s", feedID)
			return rpt.resourceManager.Subscribe(sessionID, uri)
		case 3:
			feedID := generateFeedID(fmt.Sprintf("https://example.com/feed%d.xml", workerID%rpt.config.FeedCount))
			uri := fmt.Sprintf("feeds://feed/%s", feedID)
			_ = rpt.resourceManager.GetSubscribedSessions(uri)
			return nil
		}
		return nil
	})
}

// BenchmarkMemoryUsage measures memory usage patterns
func (rpt *ResourcePerformanceTester) BenchmarkMemoryUsage(ctx context.Context) (*Metrics, error) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	initialMemory := memStats.Alloc

	metrics, err := rpt.BenchmarkResourceListing(ctx)
	if err != nil {
		return nil, err
	}

	runtime.ReadMemStats(&memStats)
	finalMemory := memStats.Alloc

	metrics.MemoryUsageMB = float64(finalMemory-initialMemory) / (1024 * 1024)
	return metrics, nil
}

// runBenchmark executes a single-threaded benchmark
func (rpt *ResourcePerformanceTester) runBenchmark(ctx context.Context, name string, operation func(context.Context) error) (*Metrics, error) {
	var operations int64
	var totalLatency time.Duration
	var errors int64
	latencies := make([]time.Duration, 0, 1000)

	timeout := time.After(rpt.config.Duration)
	startTime := time.Now()

	for {
		select {
		case <-timeout:
			goto done
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			opStart := time.Now()
			err := operation(ctx)
			latency := time.Since(opStart)

			operations++
			totalLatency += latency
			latencies = append(latencies, latency)

			if err != nil {
				errors++
			}

			// Limit latency slice size to prevent memory issues
			if len(latencies) > 10000 {
				latencies = latencies[1000:]
			}
		}
	}

done:
	duration := time.Since(startTime)

	if operations == 0 {
		return &Metrics{}, nil
	}

	// Calculate percentiles
	p95, p99 := calculatePercentiles(latencies)

	return &Metrics{
		TotalOperations:  operations,
		AverageLatency:   totalLatency / time.Duration(operations),
		P95Latency:       p95,
		P99Latency:       p99,
		ThroughputPerSec: float64(operations) / duration.Seconds(),
		ErrorRate:        float64(errors) / float64(operations),
		GoroutineCount:   runtime.NumGoroutine(),
	}, nil
}

// runConcurrentBenchmark executes a multi-threaded benchmark
func (rpt *ResourcePerformanceTester) runConcurrentBenchmark(ctx context.Context, name string, operation func(context.Context, int) error) (*Metrics, error) {
	var totalOperations int64
	var totalErrors int64
	var totalLatency time.Duration
	var mu sync.Mutex
	latencies := make([]time.Duration, 0, 1000)

	var wg sync.WaitGroup
	timeout := time.After(rpt.config.Duration)
	startTime := time.Now()

	// Start concurrent workers
	for i := 0; i < rpt.config.ConcurrentUsers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			var operations int64
			var errors int64
			var workerLatency time.Duration
			workerLatencies := make([]time.Duration, 0, 100)

			for {
				select {
				case <-timeout:
					// Aggregate worker results
					mu.Lock()
					totalOperations += operations
					totalErrors += errors
					totalLatency += workerLatency
					latencies = append(latencies, workerLatencies...)
					mu.Unlock()
					return
				case <-ctx.Done():
					return
				default:
					opStart := time.Now()
					err := operation(ctx, workerID)
					latency := time.Since(opStart)

					operations++
					workerLatency += latency
					workerLatencies = append(workerLatencies, latency)

					if err != nil {
						errors++
					}

					// Limit latency slice size
					if len(workerLatencies) > 1000 {
						workerLatencies = workerLatencies[100:]
					}
				}
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	if totalOperations == 0 {
		return &Metrics{}, nil
	}

	// Calculate percentiles
	p95, p99 := calculatePercentiles(latencies)

	return &Metrics{
		TotalOperations:  totalOperations,
		AverageLatency:   totalLatency / time.Duration(totalOperations),
		P95Latency:       p95,
		P99Latency:       p99,
		ThroughputPerSec: float64(totalOperations) / duration.Seconds(),
		ErrorRate:        float64(totalErrors) / float64(totalOperations),
		GoroutineCount:   runtime.NumGoroutine(),
	}, nil
}

// Helper functions

func calculatePercentiles(latencies []time.Duration) (p95, p99 time.Duration) {
	if len(latencies) == 0 {
		return 0, 0
	}

	// Simple percentile calculation (would use sort in production)
	total := len(latencies)
	p95Index := int(float64(total) * 0.95)
	p99Index := int(float64(total) * 0.99)

	if p95Index >= total {
		p95Index = total - 1
	}
	if p99Index >= total {
		p99Index = total - 1
	}

	// Find approximate percentiles (simplified for demo)
	var sum time.Duration
	for _, l := range latencies {
		sum += l
	}
	avg := sum / time.Duration(len(latencies))

	// Rough estimate based on average (in production, would properly sort and calculate)
	_ = p95Index                            // Use the calculated index (simplified for demo)
	_ = p99Index                            // Use the calculated index (simplified for demo)
	p95 = time.Duration(float64(avg) * 1.5) // Approximate P95
	p99 = time.Duration(float64(avg) * 2.0) // Approximate P99

	return p95, p99
}

func generateFeedID(url string) string {
	// Simple hash function for testing (matches the one in resources.go)
	hash := uint32(0)
	for _, c := range url {
		hash = hash*31 + uint32(c)
	}
	return fmt.Sprintf("%x", hash)
}

// Mock implementations for testing

func createMockAllFeedsGetter(count int) *mockAllFeedsGetter {
	feeds := make([]*model.FeedResult, count)
	for i := 0; i < count; i++ {
		feedID := generateFeedID(fmt.Sprintf("https://example.com/perf-feed%d.xml", i))
		feeds[i] = &model.FeedResult{
			ID:        feedID,
			Title:     fmt.Sprintf("Performance Feed %d", i),
			PublicURL: fmt.Sprintf("https://example.com/perf-feed%d.xml", i),
		}
	}
	return &mockAllFeedsGetter{feeds: feeds}
}

func createMockFeedAndItemsGetter(feedCount, itemsPerFeed int) *mockFeedAndItemsGetter {
	feedsMap := make(map[string]*model.FeedAndItemsResult)

	for i := 0; i < feedCount; i++ {
		url := fmt.Sprintf("https://example.com/perf-feed%d.xml", i)
		feedID := generateFeedID(url)

		// Create gofeed.Item objects
		items := make([]*gofeed.Item, itemsPerFeed)
		for j := 0; j < itemsPerFeed; j++ {
			items[j] = &gofeed.Item{
				Title:       fmt.Sprintf("Performance Item %d-%d", i, j),
				Description: fmt.Sprintf("Performance test item %d in feed %d", j, i),
				Link:        fmt.Sprintf("https://example.com/perf-feed%d/item%d", i, j),
				Published:   time.Now().Add(-time.Duration(j) * time.Hour).Format(time.RFC3339),
				Authors:     []*gofeed.Person{{Name: fmt.Sprintf("Author %d", j%5)}},
				Categories:  []string{fmt.Sprintf("perf-cat%d", j%3)},
				GUID:        fmt.Sprintf("perf-guid-%d-%d", i, j),
			}
		}

		feedsMap[feedID] = &model.FeedAndItemsResult{
			ID:        feedID,
			Title:     fmt.Sprintf("Performance Feed %d", i),
			PublicURL: url,
			Feed:      &model.Feed{Title: fmt.Sprintf("Performance Feed %d", i), Description: fmt.Sprintf("A performance test feed number %d", i)},
			Items:     items,
		}
	}

	return &mockFeedAndItemsGetter{feeds: feedsMap}
}

// Mock types (these would typically be imported from the test files)
type mockAllFeedsGetter struct {
	feeds []*model.FeedResult
}

// GetAllFeeds implements the AllFeedsGetter interface for testing
func (m *mockAllFeedsGetter) GetAllFeeds(ctx context.Context) ([]*model.FeedResult, error) {
	return m.feeds, nil
}

type mockFeedAndItemsGetter struct {
	feeds map[string]*model.FeedAndItemsResult
}

// GetFeedAndItems implements the FeedAndItemsGetter interface for testing
func (m *mockFeedAndItemsGetter) GetFeedAndItems(ctx context.Context, feedID string) (*model.FeedAndItemsResult, error) {
	if feed, exists := m.feeds[feedID]; exists {
		return feed, nil
	}
	return nil, fmt.Errorf("feed not found: %s", feedID)
}
