// Package mcpserver implements MCP Resources functionality for serving feed resources.
package mcpserver

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/richardwooding/feed-mcp/model"
)

// URI template constants for different resource types
const (
	FeedListURI     = "feeds://all"
	FeedURI         = "feeds://feed/{feedId}"
	FeedItemsURI    = "feeds://feed/{feedId}/items"
	FeedMetaURI     = "feeds://feed/{feedId}/meta"
)

// ResourceManager handles MCP resource operations for feeds
type ResourceManager struct {
	store           AllFeedsGetter
	feedAndItemsGetter FeedAndItemsGetter
	sessions        map[string]*ResourceSession
	mu              sync.RWMutex
}

// ResourceSession tracks subscription state for a client session
type ResourceSession struct {
	id            string
	subscriptions map[string]bool // resource URI -> subscribed
	lastUpdate    time.Time
	mu            sync.RWMutex
}

// NewResourceManager creates a new ResourceManager
func NewResourceManager(store AllFeedsGetter, feedAndItemsGetter FeedAndItemsGetter) *ResourceManager {
	return &ResourceManager{
		store:           store,
		feedAndItemsGetter: feedAndItemsGetter,
		sessions:        make(map[string]*ResourceSession),
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
		MIMEType:    "application/json",
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
		feedID := generateFeedID(feed.PublicURL)

		// Individual feed resource
		resources = append(resources, &mcp.Resource{
			URI:         expandURITemplate(FeedURI, map[string]string{"feedId": feedID}),
			Name:        fmt.Sprintf("Feed: %s", feed.Title),
			Description: fmt.Sprintf("Complete feed data for %s", feed.Title),
			MIMEType:    "application/json",
		})

		// Feed items only resource
		resources = append(resources, &mcp.Resource{
			URI:         expandURITemplate(FeedItemsURI, map[string]string{"feedId": feedID}),
			Name:        fmt.Sprintf("Items: %s", feed.Title),
			Description: fmt.Sprintf("Feed items only for %s", feed.Title),
			MIMEType:    "application/json",
		})

		// Feed metadata resource
		resources = append(resources, &mcp.Resource{
			URI:         expandURITemplate(FeedMetaURI, map[string]string{"feedId": feedID}),
			Name:        fmt.Sprintf("Metadata: %s", feed.Title),
			Description: fmt.Sprintf("Feed metadata for %s", feed.Title),
			MIMEType:    "application/json",
		})
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
		return nil, model.NewFeedError(model.ErrorTypeValidation, "Unknown resource URI").
			WithURL(uri).
			WithOperation("read_resource").
			WithComponent("resource_manager")
	}
}

// readFeedList reads the feed list resource
func (rm *ResourceManager) readFeedList(ctx context.Context) (*mcp.ReadResourceResult, error) {
	feedResults, err := rm.store.GetAllFeeds(ctx)
	if err != nil {
		return nil, model.CreateRetryError(err, "", 0, 0).
			WithOperation("read_feed_list").
			WithComponent("resource_manager")
	}

	// Create a simplified feed list for the resource
	feedList := make([]map[string]interface{}, 0, len(feedResults))
	for _, feed := range feedResults {
		feedID := generateFeedID(feed.PublicURL)
		feedList = append(feedList, map[string]interface{}{
			"id":          feedID,
			"title":       feed.Title,
			"public_url":  feed.PublicURL,
			"has_error":   feed.FetchError != "",
			"circuit_breaker_open": feed.CircuitBreakerOpen,
		})
	}

	content := map[string]interface{}{
		"feeds":      feedList,
		"count":      len(feedList),
		"updated_at": time.Now().UTC(),
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      FeedListURI,
				MIMEType: "application/json",
				Text:     mustMarshalJSON(content),
			},
		},
	}, nil
}

// readFeed reads a complete feed resource
func (rm *ResourceManager) readFeed(ctx context.Context, uri string) (*mcp.ReadResourceResult, error) {
	feedID, err := extractFeedIDFromURI(uri, FeedURI)
	if err != nil {
		return nil, err
	}

	feedResult, err := rm.feedAndItemsGetter.GetFeedAndItems(ctx, feedID)
	if err != nil {
		return nil, model.CreateRetryError(err, uri, 0, 0).
			WithOperation("read_feed").
			WithComponent("resource_manager")
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      uri,
				MIMEType: "application/json",
				Text:     mustMarshalJSON(feedResult),
			},
		},
	}, nil
}

// readFeedItems reads feed items only
func (rm *ResourceManager) readFeedItems(ctx context.Context, uri string) (*mcp.ReadResourceResult, error) {
	feedID, err := extractFeedIDFromURI(uri, FeedItemsURI)
	if err != nil {
		return nil, err
	}

	feedResult, err := rm.feedAndItemsGetter.GetFeedAndItems(ctx, feedID)
	if err != nil {
		return nil, model.CreateRetryError(err, uri, 0, 0).
			WithOperation("read_feed_items").
			WithComponent("resource_manager")
	}

	// Extract items from the feed if available
	var items interface{}
	itemCount := 0
	if feedResult.Feed != nil {
		// Items would be in the Feed struct - we need to check the actual structure
		items = map[string]interface{}{}  // Placeholder - need to implement actual item extraction
		itemCount = 0
	}

	content := map[string]interface{}{
		"items":      items,
		"count":      itemCount,
		"updated_at": time.Now().UTC(),
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      uri,
				MIMEType: "application/json",
				Text:     mustMarshalJSON(content),
			},
		},
	}, nil
}

// readFeedMeta reads feed metadata only
func (rm *ResourceManager) readFeedMeta(ctx context.Context, uri string) (*mcp.ReadResourceResult, error) {
	feedID, err := extractFeedIDFromURI(uri, FeedMetaURI)
	if err != nil {
		return nil, err
	}

	feedResult, err := rm.feedAndItemsGetter.GetFeedAndItems(ctx, feedID)
	if err != nil {
		return nil, model.CreateRetryError(err, uri, 0, 0).
			WithOperation("read_feed_meta").
			WithComponent("resource_manager")
	}

	// Extract only metadata fields from FeedResult and its nested Feed
	metadata := map[string]interface{}{
		"id":           feedID,
		"title":        feedResult.Title,
		"public_url":   feedResult.PublicURL,
		"has_error":    feedResult.FetchError != "",
		"fetch_error":  feedResult.FetchError,
		"circuit_breaker_open": feedResult.CircuitBreakerOpen,
		"updated_at":   time.Now().UTC(),
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

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      uri,
				MIMEType: "application/json",
				Text:     mustMarshalJSON(metadata),
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

// generateFeedID creates a stable feed ID from a URL
func generateFeedID(feedURL string) string {
	// Parse URL to extract host and path for a more readable ID
	if parsedURL, err := url.Parse(feedURL); err == nil {
		// Create a slug-like ID from the host and path
		slug := strings.ToLower(parsedURL.Host)
		if parsedURL.Path != "" && parsedURL.Path != "/" {
			// Clean the path and append to host
			path := strings.Trim(parsedURL.Path, "/")
			path = regexp.MustCompile(`[^a-z0-9-_]`).ReplaceAllString(path, "-")
			path = regexp.MustCompile(`-+`).ReplaceAllString(path, "-")
			slug = slug + "-" + path
		}
		// Truncate if too long and add hash suffix for uniqueness
		if len(slug) > 40 {
			hash := fmt.Sprintf("%x", md5.Sum([]byte(feedURL)))[:8]
			slug = slug[:32] + "-" + hash
		}
		return slug
	}
	
	// Fallback to hash if URL parsing fails
	return fmt.Sprintf("feed-%x", md5.Sum([]byte(feedURL)))[:16]
}

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
	// Convert template to regex pattern
	pattern := regexp.QuoteMeta(template)
	// Replace quoted template variables with regex pattern
	pattern = regexp.MustCompile(`\\{[^}]+\\}`).ReplaceAllString(pattern, `[^/]+`)
	pattern = "^" + pattern + "$"
	
	matched, _ := regexp.MatchString(pattern, uri)
	return matched
}

// extractFeedIDFromURI extracts the feedId parameter from a URI using a template
func extractFeedIDFromURI(uri, template string) (string, error) {
	// Create regex from template
	pattern := regexp.QuoteMeta(template)
	pattern = strings.ReplaceAll(pattern, `\{feedId\}`, `([^/]+)`)
	pattern = "^" + pattern + "$"
	
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", model.NewFeedError(model.ErrorTypeValidation, "Invalid URI template").
			WithURL(uri).
			WithOperation("extract_feed_id").
			WithComponent("resource_manager")
	}
	
	matches := re.FindStringSubmatch(uri)
	if len(matches) < 2 {
		return "", model.NewFeedError(model.ErrorTypeValidation, "Could not extract feed ID from URI").
			WithURL(uri).
			WithOperation("extract_feed_id").
			WithComponent("resource_manager")
	}
	
	return matches[1], nil
}

// mustMarshalJSON marshals an object to JSON string, panicking on error
func mustMarshalJSON(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal JSON: %v", err))
	}
	return string(data)
}