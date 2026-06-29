package store

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/richardwooding/feed-mcp/mcpserver"
)

// TestDynamicStore_ConcurrentAccessNoRace exercises the data race fixed in #142:
// DynamicStore.AddFeed/RemoveFeed mutate the base Store's feeds and
// circuitBreakers maps while the (inherited) GetAllFeeds/GetFeedAndItems read
// them. Run with -race; before the fix this triggered "concurrent map read and
// map write" and could fatally crash the server.
func TestDynamicStore_ConcurrentAccessNoRace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<rss version="2.0"><channel><title>race</title></channel></rss>`))
	}))
	defer srv.Close()

	cfg := Config{
		Feeds:             []string{srv.URL + "/seed"},
		AllowPrivateIPs:   true, // dial-time guard must allow the loopback test server
		RequestsPerSecond: 1000, // avoid rate-limit throttling during the test
		BurstCapacity:     1000,
		ExpireAfter:       time.Hour,
	}
	ds, err := NewDynamicStore(&cfg, true)
	if err != nil {
		t.Fatalf("NewDynamicStore failed: %v", err)
	}

	ctx := context.Background()
	stop := make(chan struct{})
	var readers, writers sync.WaitGroup

	// Readers: hammer the inherited base-Store read paths.
	for range 4 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_, _ = ds.GetAllFeeds(ctx)
				_, _ = ds.ListManagedFeeds(ctx)
			}
		}()
	}

	// Writers: add, read, and remove feeds, mutating the shared maps.
	for w := range 4 {
		writers.Add(1)
		go func(w int) {
			defer writers.Done()
			for j := range 30 {
				url := fmt.Sprintf("%s/feed-%d-%d", srv.URL, w, j)
				info, err := ds.AddFeed(ctx, mcpserver.FeedConfig{URL: url})
				if err != nil || info == nil {
					continue
				}
				_, _ = ds.GetFeedAndItems(ctx, info.FeedID)
				_, _ = ds.RemoveFeed(ctx, info.FeedID)
			}
		}(w)
	}

	writers.Wait()
	close(stop)
	readers.Wait()
}
