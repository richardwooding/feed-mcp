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

// waitOrFail fails the test if ch doesn't fire within d.
func waitOrFail(t *testing.T, ch <-chan struct{}, d time.Duration, msg string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(d):
		t.Fatal(msg)
	}
}

// TestDynamicStore_AddFeedDoesNotHoldLockAcrossFetch guards the #141 fix: while
// AddFeed is blocked on its initial (slow) feed fetch, other dynamic-store
// operations must still complete — the store-wide lock must not be held across
// the fetch.
func TestDynamicStore_AddFeedDoesNotHoldLockAcrossFetch(t *testing.T) {
	reached := make(chan struct{}) // closed when the slow fetch handler is entered
	release := make(chan struct{}) // closed to let the slow fetch return
	var reachedOnce, releaseOnce sync.Once
	releaseFetch := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseFetch()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			reachedOnce.Do(func() { close(reached) })
			<-release
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<rss version="2.0"><channel><title>t</title></channel></rss>`))
	}))
	defer srv.Close()

	cfg := Config{
		Feeds:             []string{srv.URL + "/seed"},
		AllowPrivateIPs:   true,
		RequestsPerSecond: 1000,
		BurstCapacity:     1000,
		ExpireAfter:       time.Hour,
	}
	ds, err := NewDynamicStore(&cfg, true)
	if err != nil {
		t.Fatalf("NewDynamicStore failed: %v", err)
	}
	ctx := context.Background()

	addDone := make(chan struct{})
	go func() {
		defer close(addDone)
		_, _ = ds.AddFeed(ctx, mcpserver.FeedConfig{URL: srv.URL + "/slow"})
	}()

	waitOrFail(t, reached, 5*time.Second, "AddFeed never reached the slow fetch")

	// AddFeed is now blocked inside the fetch. A concurrent operation must not be
	// blocked behind it — if the lock were held across the fetch, this would hang.
	opDone := make(chan struct{})
	go func() {
		defer close(opDone)
		_, _ = ds.ListManagedFeeds(ctx)
	}()
	waitOrFail(t, opDone, 3*time.Second, "ListManagedFeeds blocked while AddFeed was fetching (lock held across fetch?)")

	releaseFetch()
	waitOrFail(t, addDone, 5*time.Second, "AddFeed did not finish after the fetch was released")
}

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
