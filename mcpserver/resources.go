// Package mcpserver implements MCP Resources functionality for serving feed resources.
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/store"
	ristretto_store "github.com/eko/gocache/store/ristretto/v4"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/richardwooding/feed-mcp/model"
)

// URI template constants for different resource types
const (
	FeedListURI  = "feeds://all"
	FeedURI      = "feeds://feed/{feedId}"
	FeedItemsURI = "feeds://feed/{feedId}/items"
	FeedMetaURI  = "feeds://feed/{feedId}/meta"
)

// MIME type constants
const (
	JSONMIMEType = "application/json"
)

// ResourceManager handles MCP resource operations for feeds
type ResourceManager struct {
	store                AllFeedsGetter
	feedAndItemsGetter   FeedAndItemsGetter
	sessions             map[string]*ResourceSession
	resourceCache        *cache.Cache[string]  // Cache for serialized resource content
	cacheConfig          *ResourceCacheConfig  // Cache configuration
	cacheMetrics         *ResourceCacheMetrics // Cache performance metrics
	invalidationHooks    []func(uri string)    // Cache invalidation hooks for notifications
	pendingNotifications map[string]time.Time  // URIs needing notification -> timestamp
	mu                   sync.RWMutex
}

// ResourceSession tracks subscription state for a client session
type ResourceSession struct {
	id            string
	subscriptions map[string]bool // resource URI -> subscribed
	lastUpdate    time.Time
	mu            sync.RWMutex
}

// ResourceCacheMetrics tracks cache performance metrics
type ResourceCacheMetrics struct {
	Hits             uint64
	Misses           uint64
	Evictions        uint64
	InvalidationHits uint64 // Cache invalidations triggered
	mu               sync.RWMutex
}

// ResourceCacheConfig holds resource-specific cache configuration
type ResourceCacheConfig struct {
	DefaultTTL      time.Duration // Default TTL for resource content
	FeedListTTL     time.Duration // TTL for feed list resources
	FeedItemsTTL    time.Duration // TTL for feed items resources
	FeedMetadataTTL time.Duration // TTL for feed metadata resources
	MaxCost         int64         // Maximum cache size in bytes
	NumCounters     int64         // Number of keys to track frequency
	BufferItems     int64         // Number of keys per Get buffer
}

// NewResourceManager creates a new ResourceManager with configurable cache settings
func NewResourceManager(feedStore AllFeedsGetter, feedAndItemsGetter FeedAndItemsGetter) *ResourceManager {
	return NewResourceManagerWithConfig(feedStore, feedAndItemsGetter, nil)
}

// NewResourceManagerWithConfig creates a ResourceManager with custom cache configuration
func NewResourceManagerWithConfig(feedStore AllFeedsGetter, feedAndItemsGetter FeedAndItemsGetter, config *ResourceCacheConfig) *ResourceManager {
	// Set default cache configuration if not provided
	if config == nil {
		config = &ResourceCacheConfig{
			DefaultTTL:      10 * time.Minute, // Default 10 minutes TTL
			FeedListTTL:     5 * time.Minute,  // Feed list changes less frequently
			FeedItemsTTL:    10 * time.Minute, // Feed items change regularly
			FeedMetadataTTL: 15 * time.Minute, // Metadata changes less frequently
			MaxCost:         1 << 30,          // 1GB max size
			NumCounters:     1000,             // Track frequency of 1000 keys
			BufferItems:     64,               // Buffer 64 keys per Get
		}
	}

	// Validate and set defaults for zero values
	if config.DefaultTTL <= 0 {
		config.DefaultTTL = 10 * time.Minute
	}
	if config.FeedListTTL <= 0 {
		config.FeedListTTL = config.DefaultTTL
	}
	if config.FeedItemsTTL <= 0 {
		config.FeedItemsTTL = config.DefaultTTL
	}
	if config.FeedMetadataTTL <= 0 {
		config.FeedMetadataTTL = config.DefaultTTL
	}
	if config.MaxCost <= 0 {
		config.MaxCost = 1 << 30 // 1GB default
	}
	if config.NumCounters <= 0 {
		config.NumCounters = 1000
	}
	if config.BufferItems <= 0 {
		config.BufferItems = 64
	}

	// Create Ristretto cache for resource content
	ristrettoCache, _ := ristretto.NewCache(&ristretto.Config{
		NumCounters: config.NumCounters,
		MaxCost:     config.MaxCost,
		BufferItems: config.BufferItems,
	})

	ristrettoStore := ristretto_store.NewRistretto(ristrettoCache)
	resourceCache := cache.New[string](ristrettoStore)

	return &ResourceManager{
		store:                feedStore,
		feedAndItemsGetter:   feedAndItemsGetter,
		sessions:             make(map[string]*ResourceSession),
		resourceCache:        resourceCache,
		cacheConfig:          config,
		cacheMetrics:         &ResourceCacheMetrics{},
		invalidationHooks:    make([]func(string), 0),
		pendingNotifications: make(map[string]time.Time),
	}
}

// CreateSession creates a new resource session
func (rm *ResourceManager) CreateSession(sessionID string) *ResourceSession {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	session := &ResourceSession{
		id:            sessionID,
		subscriptions: make(map[string]bool),
		lastUpdate:    time.Now(),
	}
	rm.sessions[sessionID] = session
	return session
}

// RemoveSession removes a resource session
func (rm *ResourceManager) RemoveSession(sessionID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	delete(rm.sessions, sessionID)
}

// AddCacheInvalidationHook adds a hook function that gets called when cache is invalidated
func (rm *ResourceManager) AddCacheInvalidationHook(hook func(uri string)) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.invalidationHooks = append(rm.invalidationHooks, hook)
}

// triggerInvalidationHooks calls all registered invalidation hooks
func (rm *ResourceManager) triggerInvalidationHooks(uri string) {
	rm.mu.RLock()
	hooks := make([]func(string), len(rm.invalidationHooks))
	copy(hooks, rm.invalidationHooks)
	rm.mu.RUnlock()

	// Call hooks without holding the mutex to avoid deadlocks
	for _, hook := range hooks {
		hook(uri)
	}
}

// GetSession retrieves a resource session
func (rm *ResourceManager) GetSession(sessionID string) (*ResourceSession, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	session, exists := rm.sessions[sessionID]
	return session, exists
}

// ListResources returns all available resources
func (rm *ResourceManager) ListResources(ctx context.Context) ([]*mcp.Resource, error) {
	resources := []*mcp.Resource{}

	// Add the feed list resource
	resources = append(resources, &mcp.Resource{
		URI:         FeedListURI,
		Name:        "All Feeds",
		Description: "List of all available syndication feeds",
		MIMEType:    JSONMIMEType,
	})

	// Get all feeds to create individual feed resources
	feedResults, err := rm.store.GetAllFeeds(ctx)
	if err != nil {
		return nil, model.CreateRetryError(err, "", 0, 0).
			WithOperation("list_resources").
			WithComponent("resource_manager")
	}

	// Create resources for each feed
	for _, feed := range feedResults {
		feedID := model.GenerateFeedID(feed.PublicURL)

		// Add all three feed resources at once
		resources = append(resources,
			&mcp.Resource{
				URI:         expandURITemplate(FeedURI, map[string]string{"feedId": feedID}),
				Name:        fmt.Sprintf("Feed: %s", feed.Title),
				Description: fmt.Sprintf("Complete feed data for %s", feed.Title),
				MIMEType:    JSONMIMEType,
			},
			&mcp.Resource{
				URI:         expandURITemplate(FeedItemsURI, map[string]string{"feedId": feedID}),
				Name:        fmt.Sprintf("Items: %s", feed.Title),
				Description: fmt.Sprintf("Feed items only for %s", feed.Title),
				MIMEType:    JSONMIMEType,
			},
			&mcp.Resource{
				URI:         expandURITemplate(FeedMetaURI, map[string]string{"feedId": feedID}),
				Name:        fmt.Sprintf("Metadata: %s", feed.Title),
				Description: fmt.Sprintf("Feed metadata for %s", feed.Title),
				MIMEType:    JSONMIMEType,
			},
		)
	}

	return resources, nil
}

// ReadResource reads content for a specific resource
func (rm *ResourceManager) ReadResource(ctx context.Context, uri string) (*mcp.ReadResourceResult, error) {
	switch {
	case uri == FeedListURI:
		return rm.readFeedList(ctx)
	case matchesTemplate(uri, FeedURI):
		return rm.readFeed(ctx, uri)
	case matchesTemplate(uri, FeedItemsURI):
		return rm.readFeedItems(ctx, uri)
	case matchesTemplate(uri, FeedMetaURI):
		return rm.readFeedMeta(ctx, uri)
	default:
		return nil, model.CreateInvalidResourceURIError(uri, "URI does not match any supported resource patterns")
	}
}

// readFeedList reads the feed list resource
func (rm *ResourceManager) readFeedList(ctx context.Context) (*mcp.ReadResourceResult, error) {
	cacheKey := rm.generateCacheKey(FeedListURI)

	// Try to get from cache first
	if cachedContent, err := rm.resourceCache.Get(ctx, cacheKey); err == nil && cachedContent != "" {
		rm.recordCacheHit()
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{
					URI:      FeedListURI,
					MIMEType: JSONMIMEType,
					Text:     cachedContent,
				},
			},
		}, nil
	}

	rm.recordCacheMiss()

	feedResults, err := rm.store.GetAllFeeds(ctx)
	if err != nil {
		return nil, model.CreateRetryError(err, "", 0, 0).
			WithOperation("read_feed_list").
			WithComponent("resource_manager")
	}

	// Create a simplified feed list for the resource
	feedList := make([]map[string]interface{}, 0, len(feedResults))
	for _, feed := range feedResults {
		feedID := model.GenerateFeedID(feed.PublicURL)
		feedList = append(feedList, map[string]interface{}{
			"id":                   feedID,
			"title":                feed.Title,
			"public_url":           feed.PublicURL,
			"has_error":            feed.FetchError != "",
			"circuit_breaker_open": feed.CircuitBreakerOpen,
		})
	}

	content := map[string]interface{}{
		"feeds":      feedList,
		"count":      len(feedList),
		"updated_at": time.Now().UTC(),
	}

	contentJSON, err := marshalJSONContent(content, FeedListURI)
	if err != nil {
		return nil, err
	}

	// Cache the result with appropriate TTL for this resource type
	ttl := rm.getTTLForResourceType(FeedListURI)
	_ = rm.resourceCache.Set(ctx, cacheKey, contentJSON, store.WithExpiration(ttl))

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      FeedListURI,
				MIMEType: JSONMIMEType,
				Text:     contentJSON,
			},
		},
	}, nil
}

// readFeed reads a complete feed resource with optional filtering
func (rm *ResourceManager) readFeed(ctx context.Context, uri string) (*mcp.ReadResourceResult, error) {
	// Try to get from cache first
	cacheKey := rm.generateCacheKey(uri)
	if cachedContent, err := rm.resourceCache.Get(ctx, cacheKey); err == nil && cachedContent != "" {
		rm.recordCacheHit()
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{
					URI:      uri,
					MIMEType: JSONMIMEType,
					Text:     cachedContent,
				},
			},
		}, nil
	}

	rm.recordCacheMiss()

	feedID, err := extractFeedIDFromURI(uri, FeedURI)
	if err != nil {
		return nil, err
	}

	// Parse URI parameters for filtering
	filters, err := ParseURIParameters(uri)
	if err != nil {
		return nil, err
	}

	feedResult, err := rm.feedAndItemsGetter.GetFeedAndItems(ctx, feedID)
	if err != nil {
		// Check if this is a specific resource error
		var feedErr *model.FeedError
		if errors.As(err, &feedErr) {
			// Enhance the existing FeedError with resource context
			return nil, feedErr.WithOperation("read_feed").WithURL(uri)
		}
		// For generic errors, check if feed exists
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil, model.CreateResourceNotFoundError(uri, feedID).WithOperation("read_feed")
		}
		// Default to resource unavailable for other errors
		return nil, model.CreateResourceUnavailableError(uri, err.Error()).WithOperation("read_feed")
	}

	// If filters are applied, filter the items
	if filters != nil && feedResult.Items != nil {
		originalCount := len(feedResult.Items)
		filteredItems := ApplyFilters(feedResult.Items, filters)

		// Create a copy of the result with filtered items
		filteredResult := *feedResult
		filteredResult.Items = filteredItems

		// Add filter summary as custom field
		filterSummary := CreateFilterSummary(originalCount, len(filteredItems), filters)

		content := map[string]interface{}{
			"feed_result": &filteredResult,
			"filter_info": filterSummary,
			"updated_at":  time.Now().UTC(),
		}

		contentJSON, err := marshalJSONContent(content, uri)
		if err != nil {
			return nil, err
		}

		// Cache the result with appropriate TTL
		ttl := rm.getTTLForResourceType(uri)
		_ = rm.resourceCache.Set(ctx, cacheKey, contentJSON, store.WithExpiration(ttl))

		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{
					URI:      uri,
					MIMEType: JSONMIMEType,
					Text:     contentJSON,
				},
			},
		}, nil
	}

	// Cache and return the unfiltered result
	contentJSON, err := marshalJSONContent(feedResult, uri)
	if err != nil {
		return nil, err
	}

	// Cache the result with appropriate TTL
	ttl := rm.getTTLForResourceType(uri)
	_ = rm.resourceCache.Set(ctx, cacheKey, contentJSON, store.WithExpiration(ttl))

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      uri,
				MIMEType: JSONMIMEType,
				Text:     contentJSON,
			},
		},
	}, nil
}

// readFeedItems reads feed items with optional filtering
func (rm *ResourceManager) readFeedItems(ctx context.Context, uri string) (*mcp.ReadResourceResult, error) {
	// Try to get from cache first
	cacheKey := rm.generateCacheKey(uri)
	if cachedContent, err := rm.resourceCache.Get(ctx, cacheKey); err == nil && cachedContent != "" {
		rm.recordCacheHit()
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{
					URI:      uri,
					MIMEType: JSONMIMEType,
					Text:     cachedContent,
				},
			},
		}, nil
	}

	rm.recordCacheMiss()

	feedID, err := extractFeedIDFromURI(uri, FeedItemsURI)
	if err != nil {
		return nil, err
	}

	// Parse URI parameters for filtering
	filters, err := ParseURIParameters(uri)
	if err != nil {
		return nil, err
	}

	feedResult, err := rm.feedAndItemsGetter.GetFeedAndItems(ctx, feedID)
	if err != nil {
		// Check if this is a specific resource error
		var feedErr *model.FeedError
		if errors.As(err, &feedErr) {
			// Enhance the existing FeedError with resource context
			return nil, feedErr.WithOperation("read_feed_items").WithURL(uri)
		}
		// For generic errors, check if feed exists
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil, model.CreateResourceNotFoundError(uri, feedID).WithOperation("read_feed_items")
		}
		// Default to resource unavailable for other errors
		return nil, model.CreateResourceUnavailableError(uri, err.Error()).WithOperation("read_feed_items")
	}

	// Extract and filter items from the feed
	originalItems := feedResult.Items
	originalCount := len(originalItems)

	// Apply filters
	filteredItems := ApplyFilters(originalItems, filters)
	filteredCount := len(filteredItems)

	// Create filter summary
	filterSummary := CreateFilterSummary(originalCount, filteredCount, filters)

	content := map[string]interface{}{
		"items":       filteredItems,
		"count":       filteredCount,
		"filter_info": filterSummary,
		"updated_at":  time.Now().UTC(),
	}

	contentJSON, err := marshalJSONContent(content, uri)
	if err != nil {
		return nil, err
	}

	// Cache the result with appropriate TTL
	ttl := rm.getTTLForResourceType(uri)
	_ = rm.resourceCache.Set(ctx, cacheKey, contentJSON, store.WithExpiration(ttl))

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      uri,
				MIMEType: JSONMIMEType,
				Text:     contentJSON,
			},
		},
	}, nil
}

// readFeedMeta reads feed metadata only
func (rm *ResourceManager) readFeedMeta(ctx context.Context, uri string) (*mcp.ReadResourceResult, error) {
	// Try to get from cache first
	cacheKey := rm.generateCacheKey(uri)
	if cachedContent, err := rm.resourceCache.Get(ctx, cacheKey); err == nil && cachedContent != "" {
		rm.recordCacheHit()
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{
					URI:      uri,
					MIMEType: JSONMIMEType,
					Text:     cachedContent,
				},
			},
		}, nil
	}

	rm.recordCacheMiss()

	feedID, err := extractFeedIDFromURI(uri, FeedMetaURI)
	if err != nil {
		return nil, err
	}

	feedResult, err := rm.feedAndItemsGetter.GetFeedAndItems(ctx, feedID)
	if err != nil {
		// Check if this is a specific resource error
		var feedErr *model.FeedError
		if errors.As(err, &feedErr) {
			// Enhance the existing FeedError with resource context
			return nil, feedErr.WithOperation("read_feed_meta").WithURL(uri)
		}
		// For generic errors, check if feed exists
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil, model.CreateResourceNotFoundError(uri, feedID).WithOperation("read_feed_meta")
		}
		// Default to resource unavailable for other errors
		return nil, model.CreateResourceUnavailableError(uri, err.Error()).WithOperation("read_feed_meta")
	}

	// Extract only metadata fields from FeedResult and its nested Feed
	metadata := map[string]interface{}{
		"id":                   feedID,
		"title":                feedResult.Title,
		"public_url":           feedResult.PublicURL,
		"has_error":            feedResult.FetchError != "",
		"fetch_error":          feedResult.FetchError,
		"circuit_breaker_open": feedResult.CircuitBreakerOpen,
		"updated_at":           time.Now().UTC(),
	}

	// Add Feed-specific metadata if available
	if feedResult.Feed != nil {
		metadata["description"] = feedResult.Feed.Description
		metadata["link"] = feedResult.Feed.Link
		metadata["feed_link"] = feedResult.Feed.FeedLink
		metadata["language"] = feedResult.Feed.Language
		metadata["copyright"] = feedResult.Feed.Copyright
		metadata["updated"] = feedResult.Feed.Updated
		metadata["published"] = feedResult.Feed.Published
		metadata["feed_type"] = feedResult.Feed.FeedType
		metadata["feed_version"] = feedResult.Feed.FeedVersion
		metadata["generator"] = feedResult.Feed.Generator
		metadata["categories"] = feedResult.Feed.Categories
		metadata["links"] = feedResult.Feed.Links
		metadata["authors"] = feedResult.Feed.Authors
		metadata["image"] = feedResult.Feed.Image
	}

	contentJSON, err := marshalJSONContent(metadata, uri)
	if err != nil {
		return nil, err
	}

	// Cache the result with appropriate TTL
	ttl := rm.getTTLForResourceType(uri)
	_ = rm.resourceCache.Set(ctx, cacheKey, contentJSON, store.WithExpiration(ttl))

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      uri,
				MIMEType: JSONMIMEType,
				Text:     contentJSON,
			},
		},
	}, nil
}

// Subscribe adds a resource subscription for a session
func (rs *ResourceSession) Subscribe(uri string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.subscriptions[uri] = true
	rs.lastUpdate = time.Now()
}

// Unsubscribe removes a resource subscription for a session
func (rs *ResourceSession) Unsubscribe(uri string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	delete(rs.subscriptions, uri)
	rs.lastUpdate = time.Now()
}

// IsSubscribed checks if a session is subscribed to a resource
func (rs *ResourceSession) IsSubscribed(uri string) bool {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.subscriptions[uri]
}

// GetSubscriptions returns all active subscriptions
func (rs *ResourceSession) GetSubscriptions() []string {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	uris := make([]string, 0, len(rs.subscriptions))
	for uri := range rs.subscriptions {
		uris = append(uris, uri)
	}
	return uris
}

// Helper functions

// expandURITemplate expands a URI template with the given parameters
func expandURITemplate(template string, params map[string]string) string {
	result := template
	for key, value := range params {
		placeholder := "{" + key + "}"
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

// matchesTemplate checks if a URI matches a template pattern
func matchesTemplate(uri, template string) bool {
	// Parse the URI to remove query parameters for pattern matching
	parsedURL, err := url.Parse(uri)
	if err != nil {
		return false
	}

	// Use the path without query parameters for pattern matching
	cleanURI := parsedURL.Scheme + "://" + parsedURL.Host + parsedURL.Path

	// Convert template to regex pattern
	pattern := regexp.QuoteMeta(template)
	// Replace quoted template variables with regex pattern
	pattern = regexp.MustCompile(`\\{[^}]+\\}`).ReplaceAllString(pattern, `[^/]+`)
	pattern = "^" + pattern + "$"

	matched, _ := regexp.MatchString(pattern, cleanURI)
	return matched
}

// extractFeedIDFromURI extracts the feedId parameter from a URI using a template
func extractFeedIDFromURI(uri, template string) (string, error) {
	// Parse the URI to remove query parameters for pattern matching
	parsedURL, err := url.Parse(uri)
	if err != nil {
		return "", model.CreateInvalidResourceURIError(uri, "URI parsing failed")
	}

	// Use the path without query parameters for pattern matching
	cleanURI := parsedURL.Scheme + "://" + parsedURL.Host + parsedURL.Path

	// Create regex from template
	pattern := regexp.QuoteMeta(template)
	pattern = strings.ReplaceAll(pattern, `\{feedId\}`, `([^/]+)`)
	pattern = "^" + pattern + "$"

	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", model.CreateInvalidResourceURIError(uri, "Invalid URI template pattern")
	}

	matches := re.FindStringSubmatch(cleanURI)
	if len(matches) < 2 {
		return "", model.CreateInvalidResourceURIError(uri, "Could not extract feed ID from URI path")
	}

	return matches[1], nil
}

// marshalJSONContent marshals an object to JSON string with proper error handling
func marshalJSONContent(v interface{}, uri string) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", model.CreateResourceContentError(err, uri, "marshal_json")
	}
	return string(data), nil
}

// Cache helper methods

// generateCacheKey generates a cache key for a resource URI
// Includes URI parameters to ensure proper cache segmentation for filtered requests
func (rm *ResourceManager) generateCacheKey(uri string) string {
	// Parse the URI to extract the base URI and parameters
	parsedURL, err := url.Parse(uri)
	if err != nil {
		// Fallback to simple key if parsing fails
		return "resource:" + uri
	}

	// Create a consistent cache key that includes parameters
	baseKey := "resource:" + parsedURL.Scheme + "://" + parsedURL.Host + parsedURL.Path

	// If there are query parameters, include them in a consistent order
	if parsedURL.RawQuery != "" {
		// Use a hash of the query parameters for consistent key generation
		h := fnv.New64a()
		_, _ = h.Write([]byte(parsedURL.RawQuery)) // FNV hash Write never returns an error
		queryHash := h.Sum64()
		baseKey = fmt.Sprintf("%s?hash=%x", baseKey, queryHash)
	}

	return baseKey
}

// recordCacheHit increments the cache hit counter
func (rm *ResourceManager) recordCacheHit() {
	rm.cacheMetrics.mu.Lock()
	defer rm.cacheMetrics.mu.Unlock()
	rm.cacheMetrics.Hits++
}

// recordCacheMiss increments the cache miss counter
func (rm *ResourceManager) recordCacheMiss() {
	rm.cacheMetrics.mu.Lock()
	defer rm.cacheMetrics.mu.Unlock()
	rm.cacheMetrics.Misses++
}

// recordCacheInvalidation increments the cache invalidation counter
func (rm *ResourceManager) recordCacheInvalidation() {
	rm.cacheMetrics.mu.Lock()
	defer rm.cacheMetrics.mu.Unlock()
	rm.cacheMetrics.InvalidationHits++
}

// getTTLForResourceType returns the appropriate TTL for a resource type
func (rm *ResourceManager) getTTLForResourceType(uri string) time.Duration {
	if strings.Contains(uri, "/items") {
		return rm.cacheConfig.FeedItemsTTL
	}
	if strings.Contains(uri, "/meta") {
		return rm.cacheConfig.FeedMetadataTTL
	}
	if strings.Contains(uri, "feeds://all") || strings.Contains(uri, "feeds://list") {
		return rm.cacheConfig.FeedListTTL
	}
	// Default for other resource types (individual feeds)
	return rm.cacheConfig.DefaultTTL
}

// GetCacheMetrics returns current cache metrics
func (rm *ResourceManager) GetCacheMetrics() ResourceCacheMetrics {
	rm.cacheMetrics.mu.RLock()
	defer rm.cacheMetrics.mu.RUnlock()
	return ResourceCacheMetrics{
		Hits:             rm.cacheMetrics.Hits,
		Misses:           rm.cacheMetrics.Misses,
		Evictions:        rm.cacheMetrics.Evictions,
		InvalidationHits: rm.cacheMetrics.InvalidationHits,
	}
}

// InvalidateCache invalidates all cached resources and triggers notification hooks
func (rm *ResourceManager) InvalidateCache(ctx context.Context) error {
	err := rm.resourceCache.Clear(ctx)
	if err == nil {
		rm.recordCacheInvalidation()
		// Trigger invalidation hooks for all resources
		rm.triggerInvalidationHooks("*") // "*" indicates all resources
	}
	return err
}

// InvalidateResourceCache invalidates cache for a specific resource URI
func (rm *ResourceManager) InvalidateResourceCache(ctx context.Context, uri string) error {
	cacheKey := rm.generateCacheKey(uri)
	err := rm.resourceCache.Delete(ctx, cacheKey)
	if err == nil {
		rm.recordCacheInvalidation()
		rm.triggerInvalidationHooks(uri)
	}
	return err
}

// InvalidateFeedCache invalidates all cache entries for a specific feed
func (rm *ResourceManager) InvalidateFeedCache(ctx context.Context, feedID string) error {
	// Invalidate all resource types for this feed
	feedURI := strings.Replace(FeedURI, "{feedId}", feedID, 1)
	itemsURI := strings.Replace(FeedItemsURI, "{feedId}", feedID, 1)
	metaURI := strings.Replace(FeedMetaURI, "{feedId}", feedID, 1)

	var lastErr error
	uris := []string{feedURI, itemsURI, metaURI}

	for _, uri := range uris {
		if err := rm.InvalidateResourceCache(ctx, uri); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// Subscription management methods

// Subscribe adds a subscription for the given session and resource URI
func (rm *ResourceManager) Subscribe(sessionID, uri string) error {
	rm.mu.RLock()
	session, exists := rm.sessions[sessionID]
	rm.mu.RUnlock()

	if !exists {
		return model.CreateSessionError(nil, sessionID, "subscribe")
	}

	session.mu.Lock()
	defer session.mu.Unlock()
	session.subscriptions[uri] = true
	session.lastUpdate = time.Now()

	return nil
}

// Unsubscribe removes a subscription for the given session and resource URI
func (rm *ResourceManager) Unsubscribe(sessionID, uri string) error {
	rm.mu.RLock()
	session, exists := rm.sessions[sessionID]
	rm.mu.RUnlock()

	if !exists {
		return model.CreateSessionError(nil, sessionID, "unsubscribe")
	}

	session.mu.Lock()
	defer session.mu.Unlock()
	delete(session.subscriptions, uri)
	session.lastUpdate = time.Now()

	return nil
}

// GetSubscribedSessions returns all sessions subscribed to a given resource URI
func (rm *ResourceManager) GetSubscribedSessions(uri string) []string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	var subscribedSessions []string
	for sessionID, session := range rm.sessions {
		session.mu.RLock()
		if session.subscriptions[uri] {
			subscribedSessions = append(subscribedSessions, sessionID)
		}
		session.mu.RUnlock()
	}
	return subscribedSessions
}

// GetAllSubscribedURIs returns all URIs that have at least one subscription
func (rm *ResourceManager) GetAllSubscribedURIs() []string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	uriSet := make(map[string]bool)
	for _, session := range rm.sessions {
		session.mu.RLock()
		for uri := range session.subscriptions {
			uriSet[uri] = true
		}
		session.mu.RUnlock()
	}

	uris := make([]string, 0, len(uriSet))
	for uri := range uriSet {
		uris = append(uris, uri)
	}
	return uris
}

// GetSubscriptionCount returns the number of active subscriptions for this session
func (rs *ResourceSession) GetSubscriptionCount() int {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return len(rs.subscriptions)
}

// MarkPendingNotification marks a resource URI as needing notification
func (rm *ResourceManager) MarkPendingNotification(uri string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.pendingNotifications[uri] = time.Now()
}

// GetPendingNotifications returns and clears all pending notification URIs
func (rm *ResourceManager) GetPendingNotifications() []string {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	uris := make([]string, 0, len(rm.pendingNotifications))
	for uri := range rm.pendingNotifications {
		uris = append(uris, uri)
	}

	// Clear the pending notifications
	rm.pendingNotifications = make(map[string]time.Time)

	return uris
}

// DetectResourceChanges checks for changes in feed content and returns URIs that have changed
// This is a placeholder implementation - in a production system, this would use timestamps,
// content hashes, or other change detection mechanisms
func (rm *ResourceManager) DetectResourceChanges(ctx context.Context) ([]string, error) {
	// Get any pending notifications from cache invalidation events first
	pendingURIs := rm.GetPendingNotifications()
	changedURIs := make([]string, len(pendingURIs))
	copy(changedURIs, pendingURIs)

	// For now, we'll also implement a basic change detection by checking if feeds have updated
	// In the future, this could be enhanced with:
	// - Content hash comparison
	// - Last-modified timestamp checking
	// - ETag support
	// - Database-based change tracking

	// Check if the feed list has changed by comparing current feeds with cached state
	feedResults, err := rm.store.GetAllFeeds(ctx)
	if err != nil {
		return nil, model.CreateRetryError(err, "", 0, 0).
			WithOperation("detect_changes").
			WithComponent("resource_manager")
	}

	// Always assume the feed list might have changed for now
	// In practice, you'd compare with a stored state
	changedURIs = append(changedURIs, FeedListURI)

	// Check individual feeds for changes
	for _, feed := range feedResults {
		feedID := model.GenerateFeedID(feed.PublicURL)

		// For each feed, assume all its resources might have changed
		// In a real implementation, you'd check timestamps, content hashes, etc.
		changedURIs = append(changedURIs,
			expandURITemplate(FeedURI, map[string]string{"feedId": feedID}),
			expandURITemplate(FeedItemsURI, map[string]string{"feedId": feedID}),
			expandURITemplate(FeedMetaURI, map[string]string{"feedId": feedID}),
		)
	}

	return changedURIs, nil
}
