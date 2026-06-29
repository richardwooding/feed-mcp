// Package store implements dynamic feed management functionality.
package store

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sony/gobreaker"

	"github.com/richardwooding/feed-mcp/mcpserver"
	"github.com/richardwooding/feed-mcp/model"
)

// Feed status constants
const (
	statusActive    = "active"
	statusError     = "error"
	statusRefreshed = "refreshed"
)

// feedCacheInfo holds the result of a feed cache check
type feedCacheInfo struct {
	ItemCount   int
	Title       string
	Status      string
	LastError   string
	LastFetched time.Time
	Found       bool
}

// DynamicFeedMetadata holds metadata for dynamically managed feeds
type DynamicFeedMetadata struct {
	Title       string               `json:"title,omitempty"`
	Category    string               `json:"category,omitempty"`
	Description string               `json:"description,omitempty"`
	AddedAt     time.Time            `json:"addedAt"`
	Source      mcpserver.FeedSource `json:"source"`
	Status      string               `json:"status"` // active, error, paused
	LastError   string               `json:"lastError,omitempty"`
	LastFetched time.Time            `json:"lastFetched"`
}

// DynamicStore extends Store with dynamic feed management capabilities
type DynamicStore struct {
	*Store
	config            Config
	dynamicFeeds      map[string]string               // feedID -> URL for runtime feeds
	feedMetadata      map[string]*DynamicFeedMetadata // feedID -> metadata
	dynamicMutex      sync.RWMutex
	allowRuntimeFeeds bool
}

// NewDynamicStore creates a new dynamic feed store
func NewDynamicStore(config *Config, allowRuntimeFeeds bool) (*DynamicStore, error) {
	// If runtime feeds are allowed and no initial feeds are provided, create an empty config
	if allowRuntimeFeeds && len(config.Feeds) == 0 {
		// Create a config with an empty feed list
		tempConfig := *config
		tempConfig.Feeds = []string{}
		tempConfig.AllowEmptyFeeds = true

		baseStore, err := NewStore(&tempConfig)
		if err != nil {
			return nil, err
		}

		ds := &DynamicStore{
			Store:             baseStore,
			config:            *config,
			dynamicFeeds:      make(map[string]string),
			feedMetadata:      make(map[string]*DynamicFeedMetadata),
			allowRuntimeFeeds: allowRuntimeFeeds,
		}

		return ds, nil
	}

	// Normal path with initial feeds
	baseStore, err := NewStore(config)
	if err != nil {
		return nil, err
	}

	ds := &DynamicStore{
		Store:             baseStore,
		config:            *config,
		dynamicFeeds:      make(map[string]string),
		feedMetadata:      make(map[string]*DynamicFeedMetadata),
		allowRuntimeFeeds: allowRuntimeFeeds,
	}

	// Initialize metadata for startup feeds
	ds.initializeStartupFeedMetadata()

	return ds, nil
}

// checkFeedCache retrieves feed information from cache and returns status information
func (ds *DynamicStore) checkFeedCache(ctx context.Context, url string) feedCacheInfo {
	info := feedCacheInfo{
		Status:      statusActive,
		LastFetched: time.Now(),
		Found:       true,
	}

	feed, err := ds.feedCacheManager.Get(ctx, url)
	if err == nil && feed != nil {
		info.ItemCount = len(feed.Items)
		info.Title = feed.Title
		info.LastFetched = time.Now()
	} else {
		info.Status = statusError
		info.Found = false
		if err != nil {
			info.LastError = err.Error()
		} else {
			info.LastError = "failed to fetch feed: unknown error"
		}
	}

	return info
}

// initializeStartupFeedMetadata creates metadata entries for feeds loaded at startup.
// Titles are deliberately left blank here; populating them required a synchronous
// fetch per feed which blocked NewDynamicStore for tens of seconds on large feed
// lists (issue #114). Title and LastFetched are populated on first read via
// list_managed_feeds or the regular feed access paths.
func (ds *DynamicStore) initializeStartupFeedMetadata() {
	ds.dynamicMutex.Lock()
	defer ds.dynamicMutex.Unlock()

	source := mcpserver.FeedSourceStartup
	if ds.config.OPML != "" {
		source = mcpserver.FeedSourceOPML
	}

	for _, entry := range ds.feedEntries() {
		ds.feedMetadata[entry.id] = &DynamicFeedMetadata{
			AddedAt: time.Now(), // Approximate startup time
			Source:  source,
			Status:  statusActive,
		}
	}
}

// alreadyExistsError builds the error returned when a feed URL is already
// registered.
func alreadyExistsError(url string) error {
	return model.NewFeedError(model.ErrorTypeValidation, fmt.Sprintf("feed with URL %s already exists", url)).
		WithOperation("add_feed").
		WithComponent("dynamic_store")
}

// AddFeed implements DynamicFeedManager.AddFeed
func (ds *DynamicStore) AddFeed(ctx context.Context, config mcpserver.FeedConfig) (*mcpserver.ManagedFeedInfo, error) {
	if !ds.allowRuntimeFeeds {
		return nil, model.NewFeedError(model.ErrorTypeConfiguration, "runtime feed management is not enabled").
			WithOperation("add_feed").
			WithComponent("dynamic_store")
	}

	// Validate the URL
	if ctx == nil {
		ctx = context.Background()
	}
	if err := model.ValidateFeedURLContext(ctx, config.URL, ds.config.AllowPrivateIPs); err != nil {
		return nil, err
	}

	// Fast duplicate check before the (potentially slow) initial fetch. The
	// feeds map contains all feeds, including dynamic ones.
	if ds.urlRegistered(config.URL) {
		return nil, alreadyExistsError(config.URL)
	}

	// Fetch the feed initially to get its title and validate reachability. This
	// is done WITHOUT holding dynamicMutex: a fetch can block for seconds doing
	// retries/backoff, and holding the lock across it would freeze every other
	// dynamic-store operation (list/remove/add) for the duration (#141). The
	// cache is keyed by URL and doesn't require the feed to be registered first.
	cacheInfo := ds.checkFeedCache(ctx, config.URL)
	itemCount := cacheInfo.ItemCount

	// If the caller aborted or timed out during the fetch, don't register the
	// feed in an error state — surface the context error instead.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	feedID := model.GenerateFeedID(config.URL)

	ds.dynamicMutex.Lock()
	defer ds.dynamicMutex.Unlock()

	// Re-check under the write lock: a concurrent AddFeed for the same URL may
	// have registered it while we were fetching.
	if ds.urlRegistered(config.URL) {
		return nil, alreadyExistsError(config.URL)
	}

	// Build a circuit breaker if circuit breaking is enabled.
	var cb *gobreaker.CircuitBreaker
	if ds.hasCircuitBreakers() {
		settings := gobreaker.Settings{
			Name:        fmt.Sprintf("feed-%s", config.URL),
			MaxRequests: ds.config.CircuitBreakerMaxRequests,
			Interval:    ds.config.CircuitBreakerInterval,
			Timeout:     ds.config.CircuitBreakerTimeout,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures >= ds.config.CircuitBreakerFailureThreshold
			},
		}
		cb = gobreaker.NewCircuitBreaker(settings)
	}

	// Register the feed (and its breaker) in the base store, and record it as a
	// dynamic feed.
	ds.putFeed(feedID, config.URL, cb)
	ds.dynamicFeeds[feedID] = config.URL

	// Create metadata from the fetch performed above.
	metadata := &DynamicFeedMetadata{
		Title:       config.Title,
		Category:    config.Category,
		Description: config.Description,
		AddedAt:     time.Now(),
		Source:      mcpserver.FeedSourceRuntime,
		Status:      statusActive,
	}
	if cacheInfo.Found {
		metadata.LastFetched = cacheInfo.LastFetched
		if metadata.Title == "" {
			metadata.Title = cacheInfo.Title
		}
	} else {
		metadata.LastError = cacheInfo.LastError
		metadata.Status = cacheInfo.Status
	}

	ds.feedMetadata[feedID] = metadata

	return &mcpserver.ManagedFeedInfo{
		FeedID:      feedID,
		URL:         config.URL,
		Title:       metadata.Title,
		Category:    metadata.Category,
		Description: metadata.Description,
		Status:      metadata.Status,
		LastFetched: metadata.LastFetched,
		LastError:   metadata.LastError,
		ItemCount:   itemCount,
		AddedAt:     metadata.AddedAt,
		Source:      string(metadata.Source),
	}, nil
}

// RemoveFeed implements DynamicFeedManager.RemoveFeed
func (ds *DynamicStore) RemoveFeed(ctx context.Context, feedID string) (*mcpserver.RemovedFeedInfo, error) {
	if !ds.allowRuntimeFeeds {
		return nil, model.NewFeedError(model.ErrorTypeConfiguration, "runtime feed management is not enabled").
			WithOperation("remove_feed").
			WithComponent("dynamic_store")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	// Phase 1: validate the removal and capture what we need, under the lock.
	url, title, err := ds.prepareRemoval(feedID)
	if err != nil {
		return nil, err
	}

	// Phase 2: read the item count for the response WITHOUT holding the lock —
	// a cache miss here triggers a network fetch, and holding dynamicMutex across
	// it would freeze every other dynamic-store operation (#141).
	itemCount := 0
	if feed, err := ds.feedCacheManager.Get(ctx, url); err == nil && feed != nil {
		itemCount = len(feed.Items)
	}

	// Phase 3: commit the removal under the lock, re-checking that the feed
	// wasn't already removed by a concurrent call while we read the count.
	if !ds.commitRemoval(feedID, url) {
		return nil, model.NewFeedError(model.ErrorTypeValidation, fmt.Sprintf("feed with ID %s not found", feedID)).
			WithOperation("remove_feed").
			WithComponent("dynamic_store")
	}

	// Clear from cache (no store lock needed; deletion errors are not critical).
	_ = ds.feedCacheManager.Delete(ctx, url)

	return &mcpserver.RemovedFeedInfo{
		FeedID:       feedID,
		URL:          url,
		Title:        title,
		ItemsRemoved: itemCount,
	}, nil
}

// prepareRemoval verifies that feedID exists and is a runtime feed (removable),
// returning its URL and title. It holds dynamicMutex only for the map reads.
func (ds *DynamicStore) prepareRemoval(feedID string) (url, title string, err error) {
	ds.dynamicMutex.RLock()
	defer ds.dynamicMutex.RUnlock()

	url, exists := ds.feedURL(feedID)
	if !exists {
		return "", "", model.NewFeedError(model.ErrorTypeValidation, fmt.Sprintf("feed with ID %s not found", feedID)).
			WithOperation("remove_feed").
			WithComponent("dynamic_store")
	}

	metadata := ds.feedMetadata[feedID]
	if metadata != nil && metadata.Source != mcpserver.FeedSourceRuntime {
		return "", "", model.NewFeedError(model.ErrorTypeValidation, fmt.Sprintf("cannot remove %s feed %s", metadata.Source, feedID)).
			WithOperation("remove_feed").
			WithComponent("dynamic_store")
	}

	if metadata != nil {
		title = metadata.Title
	}
	return url, title, nil
}

// commitRemoval deletes the feed from the base store and the dynamic maps under
// the write lock. It returns false if the feed no longer exists (already removed
// by a concurrent call).
func (ds *DynamicStore) commitRemoval(feedID, url string) bool {
	ds.dynamicMutex.Lock()
	defer ds.dynamicMutex.Unlock()

	if _, exists := ds.feedURL(feedID); !exists {
		return false
	}
	ds.deleteFeed(feedID, url)
	delete(ds.dynamicFeeds, feedID)
	delete(ds.feedMetadata, feedID)
	return true
}

// RemoveFeedByURL implements DynamicFeedManager.RemoveFeedByURL
func (ds *DynamicStore) RemoveFeedByURL(ctx context.Context, url string) (*mcpserver.RemovedFeedInfo, error) {
	var feedID string
	for _, entry := range ds.feedEntries() {
		if entry.url == url {
			feedID = entry.id
			break
		}
	}

	if feedID == "" {
		return nil, model.NewFeedError(model.ErrorTypeValidation, fmt.Sprintf("feed with URL %s not found", url)).
			WithOperation("remove_feed_by_url").
			WithComponent("dynamic_store")
	}

	return ds.RemoveFeed(ctx, feedID)
}

// ListManagedFeeds implements DynamicFeedManager.ListManagedFeeds
func (ds *DynamicStore) ListManagedFeeds(ctx context.Context) ([]mcpserver.ManagedFeedInfo, error) {
	// Snapshot the feeds and their metadata under the locks, then release them
	// before the per-feed network fetches below — holding dynamicMutex across
	// checkFeedCache would block AddFeed/RemoveFeed for the duration of every
	// cache load. Locks are taken dynamicMutex (outer) → feedsMu (inner, via
	// feedEntries), matching the ordering used elsewhere.
	type feedSnapshot struct {
		id   string
		url  string
		meta DynamicFeedMetadata
	}
	ds.dynamicMutex.RLock()
	entries := ds.feedEntries()
	snapshots := make([]feedSnapshot, 0, len(entries))
	for _, entry := range entries {
		meta := DynamicFeedMetadata{
			AddedAt: time.Now(),
			Source:  mcpserver.FeedSourceStartup,
			Status:  "active",
		}
		if m := ds.feedMetadata[entry.id]; m != nil {
			meta = *m
		}
		snapshots = append(snapshots, feedSnapshot{id: entry.id, url: entry.url, meta: meta})
	}
	ds.dynamicMutex.RUnlock()

	feeds := make([]mcpserver.ManagedFeedInfo, 0, len(snapshots))
	for i := range snapshots {
		snap := &snapshots[i]
		// Get current item count and status (network fetch; no lock held).
		cacheInfo := ds.checkFeedCache(ctx, snap.url)
		itemCount := cacheInfo.ItemCount
		status := cacheInfo.Status
		var lastError string
		var lastFetched time.Time

		if cacheInfo.Found {
			lastFetched = cacheInfo.LastFetched
		} else {
			lastError = cacheInfo.LastError
			lastFetched = snap.meta.LastFetched // Keep original if cache fetch failed
		}

		// Title falls back to the freshly-fetched cacheInfo.Title when metadata
		// is blank — startup/OPML feeds seed empty titles (see #114 lazy init)
		// and rely on the first list_managed_feeds call to surface the real title.
		title := snap.meta.Title
		if title == "" && cacheInfo.Found {
			title = cacheInfo.Title
		}

		feeds = append(feeds, mcpserver.ManagedFeedInfo{
			FeedID:      snap.id,
			URL:         snap.url,
			Title:       title,
			Category:    snap.meta.Category,
			Description: snap.meta.Description,
			Status:      status,
			LastFetched: lastFetched,
			LastError:   lastError,
			ItemCount:   itemCount,
			AddedAt:     snap.meta.AddedAt,
			Source:      string(snap.meta.Source),
		})
	}

	return feeds, nil
}

// RefreshFeed implements DynamicFeedManager.RefreshFeed
func (ds *DynamicStore) RefreshFeed(ctx context.Context, feedID string) (*mcpserver.RefreshFeedInfo, error) {
	url, exists := ds.feedURL(feedID)

	if !exists {
		return &mcpserver.RefreshFeedInfo{
			FeedID: feedID,
			Status: "not_found",
		}, nil
	}

	// Clear from cache to force refresh
	_ = ds.feedCacheManager.Delete(ctx, url) // Cache deletion errors are not critical

	// Get fresh content
	feed, err := ds.feedCacheManager.Get(ctx, url)

	refreshInfo := &mcpserver.RefreshFeedInfo{
		FeedID:      feedID,
		LastFetched: time.Now(),
	}

	if err != nil {
		refreshInfo.Status = statusError
		refreshInfo.Error = err.Error()

		// Update metadata
		ds.dynamicMutex.Lock()
		if metadata := ds.feedMetadata[feedID]; metadata != nil {
			metadata.Status = statusError
			metadata.LastError = err.Error()
		}
		ds.dynamicMutex.Unlock()
	} else {
		refreshInfo.Status = statusRefreshed
		refreshInfo.ItemsAdded = len(feed.Items)

		// Update metadata
		ds.dynamicMutex.Lock()
		if metadata := ds.feedMetadata[feedID]; metadata != nil {
			metadata.Status = statusActive
			metadata.LastError = ""
			metadata.LastFetched = time.Now()
			if metadata.Title == "" {
				metadata.Title = feed.Title
			}
		}
		ds.dynamicMutex.Unlock()
	}

	return refreshInfo, nil
}

// UpdateFeedMetadata implements DynamicFeedManager.UpdateFeedMetadata
func (ds *DynamicStore) UpdateFeedMetadata(ctx context.Context, feedID string, metadata mcpserver.FeedMetadata) error {
	ds.dynamicMutex.Lock()
	defer ds.dynamicMutex.Unlock()

	feedMeta := ds.feedMetadata[feedID]
	if feedMeta == nil {
		return model.NewFeedError(model.ErrorTypeValidation, fmt.Sprintf("feed with ID %s not found", feedID)).
			WithOperation("update_feed_metadata").
			WithComponent("dynamic_store")
	}

	// Update metadata fields
	if metadata.Title != "" {
		feedMeta.Title = metadata.Title
	}
	if metadata.Category != "" {
		feedMeta.Category = metadata.Category
	}
	if metadata.Description != "" {
		feedMeta.Description = metadata.Description
	}

	return nil
}

// PauseFeed implements DynamicFeedManager.PauseFeed
func (ds *DynamicStore) PauseFeed(ctx context.Context, feedID string) error {
	ds.dynamicMutex.Lock()
	defer ds.dynamicMutex.Unlock()

	metadata := ds.feedMetadata[feedID]
	if metadata == nil {
		return model.NewFeedError(model.ErrorTypeValidation, fmt.Sprintf("feed with ID %s not found", feedID)).
			WithOperation("pause_feed").
			WithComponent("dynamic_store")
	}

	metadata.Status = "paused"
	return nil
}

// ResumeFeed implements DynamicFeedManager.ResumeFeed
func (ds *DynamicStore) ResumeFeed(ctx context.Context, feedID string) error {
	ds.dynamicMutex.Lock()
	defer ds.dynamicMutex.Unlock()

	metadata := ds.feedMetadata[feedID]
	if metadata == nil {
		return model.NewFeedError(model.ErrorTypeValidation, fmt.Sprintf("feed with ID %s not found", feedID)).
			WithOperation("resume_feed").
			WithComponent("dynamic_store")
	}

	metadata.Status = statusActive
	return nil
}
