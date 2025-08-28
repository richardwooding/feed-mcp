package store

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// RSS template constants for benchmarks
const (
	rssTemplate = `
		<rss version="2.0">
			<channel>
				<title>%s</title>
				<description>%s</description>
				<item>
					<title>%s</title>
					<link>%s</link>
					<description>%s</description>
				</item>
			</channel>
		</rss>
	`
)

// Helper function to create RSS feed content
func createRSSFeedContent(title, description, itemTitle, itemLink, itemDescription string) string {
	return fmt.Sprintf(rssTemplate, title, description, itemTitle, itemLink, itemDescription)
}

// Helper function to create a test server with RSS content
func createTestFeedServer(title, description, itemTitle, itemLink, itemDescription string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		feedContent := createRSSFeedContent(title, description, itemTitle, itemLink, itemDescription)
		w.Write([]byte(feedContent))
	}))
}

// BenchmarkStore_WithoutConnectionPooling benchmarks feed fetching without optimized connection pooling
func BenchmarkStore_WithoutConnectionPooling(b *testing.B) {
	// Create multiple test servers to simulate multiple feeds
	servers := make([]*httptest.Server, 10)
	urls := make([]string, 10)
	
	for i := 0; i < 10; i++ {
		server := createTestFeedServer(
			"Benchmark Feed",
			"A test feed for benchmarking", 
			"Test Item",
			"http://example.com/item",
			"Test content",
		)
		servers[i] = server
		urls[i] = server.URL
	}
	
	// Clean up servers after benchmark
	defer func() {
		for _, server := range servers {
			server.Close()
		}
	}()

	// Create store with minimal connection pool settings (simulating poor pooling)
	store, err := NewStore(Config{
		Feeds:                urls,
		ExpireAfter:          1 * time.Millisecond, // Force cache misses
		MaxIdleConns:         1,  // Minimal pooling
		MaxConnsPerHost:      1,  // Only 1 connection per host
		MaxIdleConnsPerHost: 1,  // Minimal idle connections
		IdleConnTimeout:     1 * time.Second, // Short idle timeout
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_, err := store.GetAllFeeds(context.Background())
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStore_WithConnectionPooling benchmarks feed fetching with optimized connection pooling
func BenchmarkStore_WithConnectionPooling(b *testing.B) {
	// Create multiple test servers to simulate multiple feeds
	servers := make([]*httptest.Server, 10)
	urls := make([]string, 10)
	
	for i := 0; i < 10; i++ {
		server := createTestFeedServer(
			"Benchmark Feed",
			"A test feed for benchmarking", 
			"Test Item",
			"http://example.com/item",
			"Test content",
		)
		servers[i] = server
		urls[i] = server.URL
	}
	
	// Clean up servers after benchmark
	defer func() {
		for _, server := range servers {
			server.Close()
		}
	}()

	// Create store with optimized connection pool settings
	store, err := NewStore(Config{
		Feeds:                urls,
		ExpireAfter:          1 * time.Millisecond, // Force cache misses
		MaxIdleConns:         100, // Generous connection pool
		MaxConnsPerHost:      20,  // Multiple connections per host
		MaxIdleConnsPerHost: 10,  // Keep connections alive
		IdleConnTimeout:     90 * time.Second, // Long idle timeout
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_, err := store.GetAllFeeds(context.Background())
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStore_ConcurrentAccess benchmarks concurrent access to feeds with connection pooling
func BenchmarkStore_ConcurrentAccess(b *testing.B) {
	// Create multiple test servers
	servers := make([]*httptest.Server, 5)
	urls := make([]string, 5)
	
	for i := 0; i < 5; i++ {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Simulate some processing time
			time.Sleep(10 * time.Millisecond)
			w.Header().Set("Content-Type", "application/rss+xml")
			feedContent := createRSSFeedContent(
				"Concurrent Feed",
				"A test feed for concurrent benchmarking",
				"Concurrent Item",
				"http://example.com/concurrent",
				"Concurrent test content",
			)
			w.Write([]byte(feedContent))
		}))
		servers[i] = server
		urls[i] = server.URL
	}
	
	defer func() {
		for _, server := range servers {
			server.Close()
		}
	}()

	// Create store with good connection pooling for concurrent access
	store, err := NewStore(Config{
		Feeds:                urls,
		ExpireAfter:          1 * time.Millisecond, // Force cache misses
		MaxIdleConns:         50,
		MaxConnsPerHost:      15,
		MaxIdleConnsPerHost: 8,
		IdleConnTimeout:     60 * time.Second,
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := store.GetAllFeeds(context.Background())
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkHTTPClient_ConnectionReuse benchmarks raw HTTP client connection reuse
func BenchmarkHTTPClient_ConnectionReuse(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	// Test with connection pooling
	b.Run("WithPooling", func(b *testing.B) {
		poolConfig := HTTPPoolConfig{
			MaxIdleConns:        100,
			MaxConnsPerHost:     20,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		}
		client := NewRateLimitedHTTPClient(1000.0, 1000, poolConfig) // High limits to avoid rate limiting
		
		b.ResetTimer()
		
		for i := 0; i < b.N; i++ {
			resp, err := client.Get(server.URL)
			if err != nil {
				b.Fatal(err)
			}
			resp.Body.Close()
		}
	})

	// Test without optimized pooling
	b.Run("WithoutPooling", func(b *testing.B) {
		poolConfig := HTTPPoolConfig{
			MaxIdleConns:        1,
			MaxConnsPerHost:     1,
			MaxIdleConnsPerHost: 1,
			IdleConnTimeout:     1 * time.Second,
		}
		client := NewRateLimitedHTTPClient(1000.0, 1000, poolConfig) // High limits to avoid rate limiting
		
		b.ResetTimer()
		
		for i := 0; i < b.N; i++ {
			resp, err := client.Get(server.URL)
			if err != nil {
				b.Fatal(err)
			}
			resp.Body.Close()
		}
	})
}

// BenchmarkStore_ScalabilityTest tests performance with varying numbers of feeds
func BenchmarkStore_ScalabilityTest(b *testing.B) {
	feedCounts := []int{1, 5, 10, 25, 50}
	
	for _, feedCount := range feedCounts {
		b.Run(fmt.Sprintf("Feeds_%d", feedCount), func(b *testing.B) {
			// Create test servers
			servers := make([]*httptest.Server, feedCount)
			urls := make([]string, feedCount)
			
			for i := 0; i < feedCount; i++ {
				server := createTestFeedServer(
					"Scalability Feed",
					"Feed for scalability testing",
					"Scale Test Item",
					"http://example.com/scale",
					"Scalability test content",
				)
				servers[i] = server
				urls[i] = server.URL
			}
			
			defer func() {
				for _, server := range servers {
					server.Close()
				}
			}()

			// Create store with optimized settings for the feed count
			store, err := NewStore(Config{
				Feeds:                urls,
				ExpireAfter:          1 * time.Millisecond, // Force cache misses
				MaxIdleConns:         feedCount * 2,        // Scale with feed count
				MaxConnsPerHost:      10,                   // Fixed per host
				MaxIdleConnsPerHost: 5,                    // Fixed idle per host
				IdleConnTimeout:     60 * time.Second,
			})
			if err != nil {
				b.Fatal(err)
			}

			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				_, err := store.GetAllFeeds(context.Background())
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}