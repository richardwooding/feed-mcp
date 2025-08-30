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
	statusActive = "active"
	statusError  = "error"
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
	LastFetched time.Time            `json:"lastFetched,omitempty"`
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

		baseStore, err := NewStoreWithEmptyFeeds(&tempConfig, true)
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
	baseStore, err := NewStoreWithEmptyFeeds(config, false)
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

// initializeStartupFeedMetadata creates metadata entries for feeds loaded at startup
func (ds *DynamicStore) initializeStartupFeedMetadata() {
	ds.dynamicMutex.Lock()
	defer ds.dynamicMutex.Unlock()

	for feedID, url := range ds.feeds {
		// Determine source based on how feeds were loaded
		source := mcpserver.FeedSourceStartup
		if ds.config.OPML != "" {
			source = mcpserver.FeedSourceOPML
		}

		ds.feedMetadata[feedID] = &DynamicFeedMetadata{
			AddedAt: time.Now(), // Approximate startup time
			Source:  source,
			Status:  statusActive,
		}

		// Try to get feed title from cache
		cacheInfo := ds.checkFeedCache(context.Background(), url)
		if cacheInfo.Found {
			ds.feedMetadata[feedID].Title = cacheInfo.Title
			ds.feedMetadata[feedID].LastFetched = cacheInfo.LastFetched
		}
	}
}

// AddFeed implements DynamicFeedManager.AddFeed
func (ds *DynamicStore) AddFeed(ctx context.Context, config mcpserver.FeedConfig) (*mcpserver.ManagedFeedInfo, error) {
	if !ds.allowRuntimeFeeds {
		return nil, model.NewFeedError(model.ErrorTypeConfiguration, "runtime feed management is not enabled").
			WithOperation("add_feed").
			WithComponent("dynamic_store")
	}

	// Validate the URL
	if err := model.SanitizeFeedURLs([]string{config.URL}, ds.config.AllowPrivateIPs); err != nil {
		return nil, err
	}

	ds.dynamicMutex.Lock()
	defer ds.dynamicMutex.Unlock()

	// Check if feed already exists (ds.feeds contains all feeds including dynamic ones)
	for _, url := range ds.feeds {
		if url == config.URL {
			return nil, model.NewFeedError(model.ErrorTypeValidation, fmt.Sprintf("feed with URL %s already exists", config.URL)).
				WithOperation("add_feed").
				WithComponent("dynamic_store")
		}
	}

	// Generate feed ID
	feedID := model.GenerateFeedID(config.URL)

	// Add circuit breaker if enabled
	if ds.circuitBreakers != nil {
		settings := gobreaker.Settings{
			Name:        fmt.Sprintf("feed-%s", config.URL),
			MaxRequests: ds.config.CircuitBreakerMaxRequests,
			Interval:    ds.config.CircuitBreakerInterval,
			Timeout:     ds.config.CircuitBreakerTimeout,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures >= ds.config.CircuitBreakerFailureThreshold
			},
		}
		ds.circuitBreakers[config.URL] = gobreaker.NewCircuitBreaker(settings)
	}

	// Add to dynamic feeds
	ds.dynamicFeeds[feedID] = config.URL
	ds.feeds[feedID] = config.URL

	// Create metadata
	metadata := &DynamicFeedMetadata{
		Title:       config.Title,
		Category:    config.Category,
		Description: config.Description,
		AddedAt:     time.Now(),
		Source:      mcpserver.FeedSourceRuntime,
		Status:      statusActive,
	}

	// Try to fetch feed initially to get title and validate
	cacheInfo := ds.checkFeedCache(ctx, config.URL)
	itemCount := cacheInfo.ItemCount
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

	ds.dynamicMutex.Lock()
	defer ds.dynamicMutex.Unlock()

	url, exists := ds.feeds[feedID]
	if !exists {
		return nil, model.NewFeedError(model.ErrorTypeValidation, fmt.Sprintf("feed with ID %s not found", feedID)).
			WithOperation("remove_feed").
			WithComponent("dynamic_store")
	}

	metadata := ds.feedMetadata[feedID]

	// Don't allow removal of startup or OPML feeds
	if metadata != nil && metadata.Source != mcpserver.FeedSourceRuntime {
		return nil, model.NewFeedError(model.ErrorTypeValidation, fmt.Sprintf("cannot remove %s feed %s", metadata.Source, feedID)).
			WithOperation("remove_feed").
			WithComponent("dynamic_store")
	}

	// Get item count before removal
	itemCount := 0
	if feed, err := ds.feedCacheManager.Get(ctx, url); err == nil && feed != nil {
		itemCount = len(feed.Items)
	}

	// Remove from maps
	delete(ds.feeds, feedID)
	delete(ds.dynamicFeeds, feedID)
	delete(ds.feedMetadata, feedID)

	// Remove circuit breaker
	if ds.circuitBreakers != nil {
		delete(ds.circuitBreakers, url)
	}

	// Clear from cache
	_ = ds.feedCacheManager.Delete(ctx, url) // Cache deletion errors are not critical

	title := ""
	if metadata != nil {
		title = metadata.Title
	}

	return &mcpserver.RemovedFeedInfo{
		FeedID:       feedID,
		URL:          url,
		Title:        title,
		ItemsRemoved: itemCount,
	}, nil
}

// RemoveFeedByURL implements DynamicFeedManager.RemoveFeedByURL
func (ds *DynamicStore) RemoveFeedByURL(ctx context.Context, url string) (*mcpserver.RemovedFeedInfo, error) {
	ds.dynamicMutex.RLock()
	var feedID string
	for id, feedURL := range ds.feeds {
		if feedURL == url {
			feedID = id
			break
		}
	}
	ds.dynamicMutex.RUnlock()

	if feedID == "" {
		return nil, model.NewFeedError(model.ErrorTypeValidation, fmt.Sprintf("feed with URL %s not found", url)).
			WithOperation("remove_feed_by_url").
			WithComponent("dynamic_store")
	}

	return ds.RemoveFeed(ctx, feedID)
}

// ListManagedFeeds implements DynamicFeedManager.ListManagedFeeds
func (ds *DynamicStore) ListManagedFeeds(ctx context.Context) ([]mcpserver.ManagedFeedInfo, error) {
	ds.dynamicMutex.RLock()
	defer ds.dynamicMutex.RUnlock()

	feeds := make([]mcpserver.ManagedFeedInfo, 0, len(ds.feeds))

	for feedID, url := range ds.feeds {
		metadata := ds.feedMetadata[feedID]
		if metadata == nil {
			// Fallback metadata for missing entries
			metadata = &DynamicFeedMetadata{
				AddedAt: time.Now(),
				Source:  mcpserver.FeedSourceStartup,
				Status:  "active",
			}
		}

		// Get current item count and update status
		cacheInfo := ds.checkFeedCache(ctx, url)
		itemCount := cacheInfo.ItemCount
		var status string
		var lastError string
		var lastFetched time.Time

		if cacheInfo.Found {
			status = cacheInfo.Status
			lastError = ""
			lastFetched = cacheInfo.LastFetched
		} else {
			status = cacheInfo.Status
			lastError = cacheInfo.LastError
			lastFetched = metadata.LastFetched // Keep original if cache fetch failed
		}

		feeds = append(feeds, mcpserver.ManagedFeedInfo{
			FeedID:      feedID,
			URL:         url,
			Title:       metadata.Title,
			Category:    metadata.Category,
			Description: metadata.Description,
			Status:      status,
			LastFetched: lastFetched,
			LastError:   lastError,
			ItemCount:   itemCount,
			AddedAt:     metadata.AddedAt,
			Source:      string(metadata.Source),
		})
	}

	return feeds, nil
}

// RefreshFeed implements DynamicFeedManager.RefreshFeed
func (ds *DynamicStore) RefreshFeed(ctx context.Context, feedID string) (*mcpserver.RefreshFeedInfo, error) {
	ds.dynamicMutex.RLock()
	url, exists := ds.feeds[feedID]
	ds.dynamicMutex.RUnlock()

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
		refreshInfo.Status = "error"
		refreshInfo.Error = err.Error()

		// Update metadata
		ds.dynamicMutex.Lock()
		if metadata := ds.feedMetadata[feedID]; metadata != nil {
			metadata.Status = statusError
			metadata.LastError = err.Error()
		}
		ds.dynamicMutex.Unlock()
	} else {
		refreshInfo.Status = "refreshed"
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
