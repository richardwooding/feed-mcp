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

// raceReader hammers the inherited base-Store read paths until stop is closed.
func raceReader(ctx context.Context, ds *DynamicStore, stop <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-stop:
			return
		default:
		}
		_, _ = ds.GetAllFeeds(ctx)
		_, _ = ds.ListManagedFeeds(ctx)
	}
}

// raceWriter adds, reads, and removes feeds, mutating the shared maps.
func raceWriter(ctx context.Context, ds *DynamicStore, baseURL string, worker int, wg *sync.WaitGroup) {
	defer wg.Done()
	for j := range 30 {
		url := fmt.Sprintf("%s/feed-%d-%d", baseURL, worker, j)
		info, err := ds.AddFeed(ctx, mcpserver.FeedConfig{URL: url})
		if err != nil || info == nil {
			continue
		}
		_, _ = ds.GetFeedAndItems(ctx, info.FeedID)
		_, _ = ds.RemoveFeed(ctx, info.FeedID)
	}
}

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

	for range 4 {
		readers.Add(1)
		go raceReader(ctx, ds, stop, &readers)
	}
	for w := range 4 {
		writers.Add(1)
		go raceWriter(ctx, ds, srv.URL, w, &writers)
	}

	writers.Wait()
	close(stop)
	readers.Wait()
}
