package store

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
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

// raceWriter adds, reads, and removes feeds, mutating the shared maps. It counts
// successful adds so the test can confirm it actually exercised concurrent
// writes (rather than silently no-op'ing if every AddFeed failed).
func raceWriter(ctx context.Context, ds *DynamicStore, baseURL string, worker int, wg *sync.WaitGroup, added *atomic.Int64) {
	defer wg.Done()
	for j := range 30 {
		url := fmt.Sprintf("%s/feed-%d-%d", baseURL, worker, j)
		info, err := ds.AddFeed(ctx, mcpserver.FeedConfig{URL: url})
		if err != nil || info == nil {
			continue
		}
		added.Add(1)
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

	// A bounded context so a regression that hangs a fetch fails the test
	// instead of stalling the whole suite.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stop := make(chan struct{})
	var readers, writers sync.WaitGroup
	var added atomic.Int64

	for range 4 {
		readers.Add(1)
		go raceReader(ctx, ds, stop, &readers)
	}
	for w := range 4 {
		writers.Add(1)
		go raceWriter(ctx, ds, srv.URL, w, &writers, &added)
	}

	writers.Wait()
	close(stop)
	readers.Wait()

	if added.Load() == 0 {
		t.Fatal("no AddFeed succeeded; the test did not exercise concurrent map writes")
	}
}
