package mcpserver

import (
	"context"
	"time"
)

// DynamicFeedManager provides methods for runtime feed management
type DynamicFeedManager interface {
	// AddFeed adds a new feed at runtime and returns its information
	AddFeed(ctx context.Context, config FeedConfig) (*ManagedFeedInfo, error)

	// RemoveFeed removes a feed by ID
	RemoveFeed(ctx context.Context, feedID string) (*RemovedFeedInfo, error)

	// RemoveFeedByURL removes a feed by URL
	RemoveFeedByURL(ctx context.Context, url string) (*RemovedFeedInfo, error)

	// ListManagedFeeds returns all managed feeds with their metadata and status
	ListManagedFeeds(ctx context.Context) ([]ManagedFeedInfo, error)

	// RefreshFeed forces a refresh of a specific feed
	RefreshFeed(ctx context.Context, feedID string) (*RefreshFeedInfo, error)

	// UpdateFeedMetadata updates feed metadata (title, category, description)
	UpdateFeedMetadata(ctx context.Context, feedID string, metadata FeedMetadata) error

	// PauseFeed pauses fetching for a specific feed
	PauseFeed(ctx context.Context, feedID string) error

	// ResumeFeed resumes fetching for a paused feed
	ResumeFeed(ctx context.Context, feedID string) error
}

// FeedConfig holds configuration for a new feed
type FeedConfig struct {
	URL         string `json:"url" description:"RSS/Atom/JSON feed URL"`
	Title       string `json:"title,omitempty" description:"Optional human-readable title"`
	Category    string `json:"category,omitempty" description:"Optional category for organization"`
	Description string `json:"description,omitempty" description:"Optional description"`
}

// FeedMetadata holds updatable metadata for a feed
type FeedMetadata struct {
	Title       string `json:"title,omitempty" description:"Feed title"`
	Category    string `json:"category,omitempty" description:"Feed category"`
	Description string `json:"description,omitempty" description:"Feed description"`
}

// ManagedFeedInfo contains comprehensive information about a managed feed
type ManagedFeedInfo struct {
	FeedID      string    `json:"feedId" description:"Unique feed identifier"`
	URL         string    `json:"url" description:"Feed URL"`
	Title       string    `json:"title" description:"Feed title"`
	Category    string    `json:"category,omitempty" description:"Feed category"`
	Description string    `json:"description,omitempty" description:"Feed description"`
	Status      string    `json:"status" description:"'active', 'error', 'paused'"`
	LastFetched time.Time `json:"lastFetched" description:"Last successful fetch time"`
	LastError   string    `json:"lastError,omitempty" description:"Most recent error message"`
	ItemCount   int       `json:"itemCount" description:"Current number of cached items"`
	AddedAt     time.Time `json:"addedAt" description:"When feed was added"`
	Source      string    `json:"source" description:"'runtime', 'startup', 'opml'"`
}

// RemovedFeedInfo contains information about a removed feed
type RemovedFeedInfo struct {
	FeedID       string `json:"feedId" description:"ID of removed feed"`
	URL          string `json:"url" description:"URL of removed feed"`
	Title        string `json:"title" description:"Title of removed feed"`
	ItemsRemoved int    `json:"itemsRemoved" description:"Number of cached items removed"`
}

// RefreshFeedInfo contains information about a feed refresh operation
type RefreshFeedInfo struct {
	FeedID      string    `json:"feedId" description:"Feed ID that was refreshed"`
	Status      string    `json:"status" description:"'refreshed', 'error', 'not_found'"`
	ItemsAdded  int       `json:"itemsAdded" description:"Number of new items fetched"`
	LastFetched time.Time `json:"lastFetched" description:"Timestamp of refresh"`
	Error       string    `json:"error,omitempty" description:"Error message if refresh failed"`
}

// FeedSource indicates how a feed was added to the system
type FeedSource string

// Feed source constants indicate how a feed was added to the system
const (
	FeedSourceStartup FeedSource = "startup" // From CLI args
	FeedSourceOPML    FeedSource = "opml"    // From OPML file
	FeedSourceRuntime FeedSource = "runtime" // Added via tools
)
