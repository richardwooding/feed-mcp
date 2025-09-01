// Package mcpserver implements the Model Context Protocol server for serving RSS/Atom/JSON feeds.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/gocolly/colly"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mmcdole/gofeed"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/richardwooding/feed-mcp/model"
	"github.com/richardwooding/feed-mcp/version"
)

// FeedAndItemsResult represents a feed along with its items
type FeedAndItemsResult = model.FeedAndItemsResult

var sessionCounter int64

// Config holds the configuration for creating a new MCP server
type Config struct {
	AllFeedsGetter     AllFeedsGetter
	FeedAndItemsGetter FeedAndItemsGetter
	DynamicFeedManager DynamicFeedManager // Optional: for runtime feed management
	Transport          model.Transport
}

// Server implements an MCP server for serving syndication feeds
type Server struct {
	allFeedsGetter     AllFeedsGetter
	feedAndItemsGetter FeedAndItemsGetter
	dynamicFeedManager DynamicFeedManager // Optional: for runtime feed management
	resourceManager    *ResourceManager
	sessionID          string
	transport          model.Transport
}

// generateSessionID creates a unique session ID for this server instance
func generateSessionID() string {
	counter := atomic.AddInt64(&sessionCounter, 1)
	return fmt.Sprintf("feed-mcp-session-%d-%d", time.Now().UnixNano(), counter)
}

// NewServer creates a new MCP server with the given configuration
func NewServer(config Config) (*Server, error) {
	if config.Transport == model.UndefinedTransport {
		return nil, model.NewFeedError(model.ErrorTypeTransport, "transport must be specified").
			WithOperation("create_server").
			WithComponent("mcp_server")
	}
	if config.AllFeedsGetter == nil {
		return nil, model.NewFeedError(model.ErrorTypeConfiguration, "AllFeedsGetter is required").
			WithOperation("create_server").
			WithComponent("mcp_server")
	}
	if config.FeedAndItemsGetter == nil {
		return nil, model.NewFeedError(model.ErrorTypeConfiguration, "FeedAndItemsGetter is required").
			WithOperation("create_server").
			WithComponent("mcp_server")
	}
	server := &Server{
		transport:          config.Transport,
		allFeedsGetter:     config.AllFeedsGetter,
		feedAndItemsGetter: config.FeedAndItemsGetter,
		dynamicFeedManager: config.DynamicFeedManager,
		sessionID:          generateSessionID(),
	}
	server.resourceManager = NewResourceManager(config.AllFeedsGetter, config.FeedAndItemsGetter)

	// Set up cache invalidation hook to trigger resource change notifications
	server.setupCacheInvalidationHooks()

	return server, nil
}

// FetchLinkParams contains parameters for the fetch_link tool.
type FetchLinkParams struct {
	URL string
}

// GetSyndicationFeedParams contains parameters for the get_syndication_feed_items tool.
type GetSyndicationFeedParams struct {
	ID string
}

// AddFeedParams contains parameters for the add_feed tool.
type AddFeedParams struct {
	URL         string `json:"url"`
	Title       string `json:"title,omitempty"`
	Category    string `json:"category,omitempty"`
	Description string `json:"description,omitempty"`
}

// RemoveFeedParams contains parameters for the remove_feed tool.
type RemoveFeedParams struct {
	FeedID string `json:"feedId,omitempty"`
	URL    string `json:"url,omitempty"`
}

// MergeFeedsParams contains parameters for the merge_feeds tool.
type MergeFeedsParams struct {
	FeedIDs     []string `json:"feedIds"`
	Title       string   `json:"title,omitempty"`
	MaxItems    int      `json:"maxItems,omitempty"`
	SortBy      string   `json:"sortBy,omitempty"`      // date, title, source
	Deduplicate bool     `json:"deduplicate,omitempty"` // Remove duplicate items
}

// ExportFeedDataParams contains parameters for the export_feed_data tool.
type ExportFeedDataParams struct {
	FeedIDs    []string `json:"feedIds,omitempty"`    // Specific feeds to export (empty = all)
	Format     string   `json:"format"`               // json, csv, opml, rss, atom
	Since      string   `json:"since,omitempty"`      // ISO 8601 date
	Until      string   `json:"until,omitempty"`      // ISO 8601 date
	MaxItems   int      `json:"maxItems,omitempty"`   // Limit exported items
	IncludeAll bool     `json:"includeAll,omitempty"` // Include feed metadata
}

// MergedFeedResult represents the result of merging multiple feeds.
type MergedFeedResult struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Items       []*gofeed.Item `json:"items"`
	SourceFeeds []string       `json:"source_feeds"`
	TotalItems  int            `json:"total_items"`
	CreatedAt   time.Time      `json:"created_at"`
}

// Run starts the MCP server and handles client connections until context is canceled
func (s *Server) Run(ctx context.Context) (err error) {
	// Create a new MCP server with resource subscription support
	srv := mcp.NewServer(
		&mcp.Implementation{
			Name:    "RSS, Atom, and JSON Feed Server",
			Version: version.GetVersion(),
		},
		&mcp.ServerOptions{
			// Enable resource subscription support
			SubscribeHandler:   s.handleSubscribeResource,
			UnsubscribeHandler: s.handleUnsubscribeResource,
			HasResources:       true, // Advertise resource capabilities
		},
	)

	// Add fetch_link tool
	fetchLinkTool := &mcp.Tool{
		Name:        "fetch_link",
		Description: "Fetch link URL",
		InputSchema: &jsonschema.Schema{
			Type:     "object",
			Required: []string{"URL"},
			Properties: map[string]*jsonschema.Schema{
				"URL": {
					Type:        "string",
					Description: "Link URL",
				},
			},
		},
	}
	// The MCP SDK v0.3.0 AddTool function signature includes three return values:
	// (*mcp.CallToolResult, any, error) where the middle 'any' value is for
	// additional metadata that can be returned to the client.
	mcp.AddTool(srv, fetchLinkTool, func(ctx context.Context, req *mcp.CallToolRequest, args FetchLinkParams) (*mcp.CallToolResult, any, error) {
		c := colly.NewCollector()

		var data []byte

		c.OnResponse(func(response *colly.Response) {
			data = response.Body
		})

		err = c.Visit(args.URL)
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	// Add all_syndication_feeds tool
	allFeedsTool := &mcp.Tool{
		Name:        "all_syndication_feeds",
		Description: "list available feedItem resources",
		InputSchema: &jsonschema.Schema{Type: "object"}, // No parameters needed
	}
	mcp.AddTool(srv, allFeedsTool, func(ctx context.Context, req *mcp.CallToolRequest, args any) (*mcp.CallToolResult, any, error) {
		feedResults, err := s.allFeedsGetter.GetAllFeeds(ctx)
		if err != nil {
			return nil, nil, err
		}
		data, err := json.Marshal(feedResults)
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	// Add get_syndication_feed_items tool
	getSyndicationFeedTool := &mcp.Tool{
		Name:        "get_syndication_feed_items",
		Description: "get syndication feed and items by id",
		InputSchema: &jsonschema.Schema{
			Type:     "object",
			Required: []string{"ID"},
			Properties: map[string]*jsonschema.Schema{
				"ID": {
					Type:        "string",
					Description: "Feed ID",
				},
			},
		},
	}
	mcp.AddTool(srv, getSyndicationFeedTool, func(ctx context.Context, req *mcp.CallToolRequest, args GetSyndicationFeedParams) (*mcp.CallToolResult, any, error) {
		feedResult, err := s.feedAndItemsGetter.GetFeedAndItems(ctx, args.ID)
		if err != nil {
			return nil, nil, err
		}
		data, err := json.Marshal(feedResult)
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	// Add feed aggregation tools (Phase 2)
	s.addAggregationTools(srv)

	// Add dynamic feed management tools if DynamicFeedManager is available
	s.addDynamicFeedTools(srv)

	// Add resource handlers for MCP Resources support
	s.addResourceHandlers(srv)

	// Add MCP prompts for feed intelligence features
	s.addPrompts(srv)

	switch s.transport {
	case model.StdioTransport:
		err = srv.Run(ctx, &mcp.StdioTransport{})
	case model.HTTPWithSSETransport:
		err = srv.Run(ctx, &mcp.StreamableServerTransport{SessionID: s.sessionID})
	default:
		return model.NewFeedError(model.ErrorTypeTransport, "unsupported transport").
			WithOperation("run_server").
			WithComponent("mcp_server")
	}

	return
}

// addAggregationTools adds feed aggregation tools to the server
func (s *Server) addAggregationTools(srv *mcp.Server) {
	// Add merge_feeds tool
	mergeFeedsTool := &mcp.Tool{
		Name:        "merge_feeds",
		Description: "Merge multiple feeds into a single aggregated feed with deduplication and sorting",
		InputSchema: &jsonschema.Schema{
			Type:     "object",
			Required: []string{"feedIds"},
			Properties: map[string]*jsonschema.Schema{
				"feedIds": {
					Type:        "array",
					Description: "Array of feed IDs to merge",
					Items: &jsonschema.Schema{
						Type: "string",
					},
				},
				"title": {
					Type:        "string",
					Description: "Title for the merged feed",
				},
				"maxItems": {
					Type:        "integer",
					Description: "Maximum number of items to include (0 for no limit)",
					Minimum:     &[]float64{0}[0],
				},
				"sortBy": {
					Type:        "string",
					Description: "Sort order: date (default), title, source",
					Enum:        []interface{}{"date", "title", "source"},
				},
				"deduplicate": {
					Type:        "boolean",
					Description: "Remove duplicate items based on title and link",
				},
			},
		},
	}
	mcp.AddTool(srv, mergeFeedsTool, func(ctx context.Context, req *mcp.CallToolRequest, args MergeFeedsParams) (*mcp.CallToolResult, any, error) {
		mergedFeed, err := s.mergeFeeds(ctx, args)
		if err != nil {
			return nil, nil, err
		}

		data, err := json.Marshal(mergedFeed)
		if err != nil {
			return nil, nil, err
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	// Add export_feed_data tool
	exportFeedDataTool := &mcp.Tool{
		Name:        "export_feed_data",
		Description: "Export feed data in various formats (JSON, CSV, OPML, RSS, Atom)",
		InputSchema: &jsonschema.Schema{
			Type:     "object",
			Required: []string{"format"},
			Properties: map[string]*jsonschema.Schema{
				"feedIds": {
					Type:        "array",
					Description: "Feed IDs to export (empty for all feeds)",
					Items: &jsonschema.Schema{
						Type: "string",
					},
				},
				"format": {
					Type:        "string",
					Description: "Export format",
					Enum:        []interface{}{"json", "csv", "opml", "rss", "atom"},
				},
				"since": {
					Type:        "string",
					Description: "Include items published after this date (ISO 8601)",
				},
				"until": {
					Type:        "string",
					Description: "Include items published before this date (ISO 8601)",
				},
				"maxItems": {
					Type:        "integer",
					Description: "Maximum number of items per feed (0 for no limit)",
					Minimum:     &[]float64{0}[0],
				},
				"includeAll": {
					Type:        "boolean",
					Description: "Include all feed metadata and statistics",
				},
			},
		},
	}
	mcp.AddTool(srv, exportFeedDataTool, func(ctx context.Context, req *mcp.CallToolRequest, args ExportFeedDataParams) (*mcp.CallToolResult, any, error) {
		exportedData, err := s.exportFeedData(ctx, &args)
		if err != nil {
			return nil, nil, err
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: exportedData}},
		}, nil, nil
	})
}

// addDynamicFeedTools adds dynamic feed management tools to the server
func (s *Server) addDynamicFeedTools(srv *mcp.Server) {
	// Only add tools if DynamicFeedManager is available
	if s.dynamicFeedManager == nil {
		return
	}

	// Add add_feed tool
	addFeedTool := &mcp.Tool{
		Name:        "add_feed",
		Description: "Add a new RSS/Atom/JSON feed at runtime",
		InputSchema: &jsonschema.Schema{
			Type:     "object",
			Required: []string{"url"},
			Properties: map[string]*jsonschema.Schema{
				"url": {
					Type:        "string",
					Description: "RSS/Atom/JSON feed URL",
				},
				"title": {
					Type:        "string",
					Description: "Optional human-readable title",
				},
				"category": {
					Type:        "string",
					Description: "Optional category for organization",
				},
				"description": {
					Type:        "string",
					Description: "Optional description",
				},
			},
		},
	}
	mcp.AddTool(srv, addFeedTool, func(ctx context.Context, req *mcp.CallToolRequest, args AddFeedParams) (*mcp.CallToolResult, any, error) {
		config := FeedConfig(args)

		feedInfo, err := s.dynamicFeedManager.AddFeed(ctx, config)
		if err != nil {
			return nil, nil, err
		}

		data, err := json.Marshal(feedInfo)
		if err != nil {
			return nil, nil, err
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	// Add remove_feed tool
	removeFeedTool := &mcp.Tool{
		Name:        "remove_feed",
		Description: "Remove a feed by ID or URL",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"feedId": {
					Type:        "string",
					Description: "Feed ID to remove",
				},
				"url": {
					Type:        "string",
					Description: "Feed URL to remove",
				},
			},
			OneOf: []*jsonschema.Schema{
				{Required: []string{"feedId"}},
				{Required: []string{"url"}},
			},
		},
	}
	mcp.AddTool(srv, removeFeedTool, func(ctx context.Context, req *mcp.CallToolRequest, args RemoveFeedParams) (*mcp.CallToolResult, any, error) {
		var feedInfo *RemovedFeedInfo
		var err error

		if args.FeedID != "" {
			feedInfo, err = s.dynamicFeedManager.RemoveFeed(ctx, args.FeedID)
		} else if args.URL != "" {
			feedInfo, err = s.dynamicFeedManager.RemoveFeedByURL(ctx, args.URL)
		} else {
			return nil, nil, model.NewFeedError(model.ErrorTypeValidation, "either feedId or url must be provided").
				WithOperation("remove_feed").
				WithComponent("mcp_server")
		}

		if err != nil {
			return nil, nil, err
		}

		data, err := json.Marshal(feedInfo)
		if err != nil {
			return nil, nil, err
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	// Add list_managed_feeds tool
	listManagedFeedsTool := &mcp.Tool{
		Name:        "list_managed_feeds",
		Description: "List all managed feeds with metadata and status",
		InputSchema: &jsonschema.Schema{Type: "object"}, // No parameters needed
	}
	mcp.AddTool(srv, listManagedFeedsTool, func(ctx context.Context, req *mcp.CallToolRequest, args any) (*mcp.CallToolResult, any, error) {
		feeds, err := s.dynamicFeedManager.ListManagedFeeds(ctx)
		if err != nil {
			return nil, nil, err
		}

		data, err := json.Marshal(feeds)
		if err != nil {
			return nil, nil, err
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})
}

// addResourceHandlers adds MCP Resource handlers to the server
func (s *Server) addResourceHandlers(srv *mcp.Server) {
	// Get all resources from ResourceManager and add them
	ctx := context.Background()
	resources, err := s.resourceManager.ListResources(ctx)
	if err != nil {
		// Log error but continue - resources will be empty
		return
	}

	// Add each resource with its handler
	for _, resource := range resources {
		srv.AddResource(resource, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return s.resourceManager.ReadResource(ctx, req.Params.URI)
		})
	}
}

// Resource operations are handled automatically by the MCP SDK v0.3.0
// when a ResourceManager is provided to the server configuration.
// All resource protocol methods are implemented in mcpserver/resources.go
func (s *Server) handleSubscribeResource(ctx context.Context, req *mcp.SubscribeRequest) error {
	// Create or get session for this connection
	sessionID := s.sessionID // Use server session ID for now
	_, exists := s.resourceManager.GetSession(sessionID)
	if !exists {
		s.resourceManager.CreateSession(sessionID)
	}

	// Subscribe to the resource
	return s.resourceManager.Subscribe(sessionID, req.Params.URI)
}

// handleUnsubscribeResource handles resource unsubscription requests using v0.3.0 SDK
func (s *Server) handleUnsubscribeResource(ctx context.Context, req *mcp.UnsubscribeRequest) error {
	// Get session for this connection
	sessionID := s.sessionID // Use server session ID for now

	// Unsubscribe from the resource
	return s.resourceManager.Unsubscribe(sessionID, req.Params.URI)
}

// setupCacheInvalidationHooks sets up hooks to trigger resource change notifications
// when cache invalidation occurs
func (s *Server) setupCacheInvalidationHooks() {
	// Store reference to server for use in closure
	server := s

	// Add hook that triggers resource update notifications when cache is invalidated
	s.resourceManager.AddCacheInvalidationHook(func(uri string) {
		// Skip notification processing if uri is "*" (global invalidation)
		// Global cache clears don't map to specific resource changes
		if uri == "*" {
			return
		}

		// Check if there are any subscriptions for this resource
		subscribedSessions := server.resourceManager.GetSubscribedSessions(uri)
		if len(subscribedSessions) == 0 {
			return // No subscriptions, no need to notify
		}

		// Mark this resource as needing notification
		server.resourceManager.MarkPendingNotification(uri)
	})
}

// NotifyResourceUpdated sends resource update notifications to subscribed clients using v0.3.0 SDK
// This method would be called when resource content changes are detected
func (s *Server) NotifyResourceUpdated(ctx context.Context, uri string, mcpServer *mcp.Server) error {
	// Get all sessions subscribed to this resource
	subscribedSessions := s.resourceManager.GetSubscribedSessions(uri)

	if len(subscribedSessions) == 0 {
		return nil // No subscriptions, nothing to notify
	}

	// Invalidate the cache to ensure fresh content on next request
	if err := s.resourceManager.InvalidateCache(ctx); err != nil {
		return model.NewFeedError(model.ErrorTypeInternal, "Failed to invalidate cache").
			WithOperation("notify_resource_updated").
			WithComponent("mcp_server")
	}

	// Use the v0.3.0 SDK's built-in notification system
	return mcpServer.ResourceUpdated(ctx, &mcp.ResourceUpdatedNotificationParams{
		URI: uri,
	})
}

// CheckForResourceChanges periodically checks for resource changes and sends notifications
// This is a background process that should be started when the server runs
func (s *Server) CheckForResourceChanges(ctx context.Context, interval time.Duration, mcpServer *mcp.Server) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Detect changes in resources
			changedURIs, err := s.resourceManager.DetectResourceChanges(ctx)
			if err != nil {
				// Log error but continue checking
				continue
			}

			// Notify subscribers of changes
			for _, uri := range changedURIs {
				if err := s.NotifyResourceUpdated(ctx, uri, mcpServer); err != nil {
					// Log error but continue with other URIs
					continue
				}
			}
		}
	}
}

// addPrompts adds MCP prompts for feed intelligence features
func (s *Server) addPrompts(srv *mcp.Server) {
	// Feed Analysis Prompts
	srv.AddPrompt(
		&mcp.Prompt{
			Name:        "analyze_feed_trends",
			Description: "Analyze trends and patterns across multiple feeds over time",
			Arguments: []*mcp.PromptArgument{
				{
					Name:        "timeframe",
					Description: "Time period to analyze (e.g., '24h', '7d', '30d')",
					Required:    false,
				},
				{
					Name:        "categories",
					Description: "Comma-separated list of categories to filter by",
					Required:    false,
				},
			},
		},
		s.handleAnalyzeFeedTrends,
	)

	srv.AddPrompt(
		&mcp.Prompt{
			Name:        "summarize_feeds",
			Description: "Generate comprehensive summaries of feed content with key insights",
			Arguments: []*mcp.PromptArgument{
				{
					Name:        "feed_ids",
					Description: "Comma-separated list of specific feed IDs to summarize (optional - defaults to all feeds)",
					Required:    false,
				},
				{
					Name:        "summary_type",
					Description: "Type of summary: 'brief', 'detailed', or 'executive' (default: 'brief')",
					Required:    false,
				},
			},
		},
		s.handleSummarizeFeeds,
	)

	srv.AddPrompt(
		&mcp.Prompt{
			Name:        "monitor_keywords",
			Description: "Track specific keywords or topics across all feeds with alerts and insights",
			Arguments: []*mcp.PromptArgument{
				{
					Name:        "keywords",
					Description: "Comma-separated list of keywords or phrases to monitor",
					Required:    true,
				},
				{
					Name:        "timeframe",
					Description: "Time period to monitor (e.g., '24h', '7d') - defaults to '24h'",
					Required:    false,
				},
				{
					Name:        "alert_threshold",
					Description: "Minimum number of mentions to trigger alert (default: 1)",
					Required:    false,
				},
			},
		},
		s.handleMonitorKeywords,
	)

	srv.AddPrompt(
		&mcp.Prompt{
			Name:        "compare_sources",
			Description: "Compare coverage and perspectives across different feed sources",
			Arguments: []*mcp.PromptArgument{
				{
					Name:        "topic",
					Description: "Topic or keyword to compare across sources",
					Required:    true,
				},
				{
					Name:        "feed_ids",
					Description: "Specific feed IDs to compare (optional - defaults to all feeds)",
					Required:    false,
				},
			},
		},
		s.handleCompareSources,
	)

	srv.AddPrompt(
		&mcp.Prompt{
			Name:        "generate_feed_report",
			Description: "Generate detailed reports on feed performance, content quality, and engagement metrics",
			Arguments: []*mcp.PromptArgument{
				{
					Name:        "report_type",
					Description: "Type of report: 'performance', 'content', 'engagement', or 'comprehensive'",
					Required:    false,
				},
				{
					Name:        "timeframe",
					Description: "Time period for the report (e.g., '7d', '30d', '90d')",
					Required:    false,
				},
			},
		},
		s.handleGenerateFeedReport,
	)
}

// mergeFeeds implements the feed merging logic
func (s *Server) mergeFeeds(ctx context.Context, args MergeFeedsParams) (*MergedFeedResult, error) {
	var allItems []*gofeed.Item
	var feedTitles []string

	// Default values
	if args.SortBy == "" {
		args.SortBy = "date"
	}

	// Fetch all specified feeds
	for _, feedID := range args.FeedIDs {
		feedResult, err := s.feedAndItemsGetter.GetFeedAndItems(ctx, feedID)
		if err != nil {
			// Continue with other feeds if one fails
			continue
		}

		if feedResult.Feed != nil {
			feedTitles = append(feedTitles, feedResult.Feed.Title)
			allItems = append(allItems, feedResult.Items...)
		}
	}

	// Deduplicate if requested
	if args.Deduplicate {
		allItems = deduplicateItems(allItems)
	}

	// Sort items based on sortBy parameter
	switch args.SortBy {
	case "title":
		sortItemsByTitle(allItems)
	case "source":
		sortItemsBySource(allItems)
	default: // "date"
		sortItemsByDate(allItems)
	}

	// Limit items if maxItems is specified
	if args.MaxItems > 0 && len(allItems) > args.MaxItems {
		allItems = allItems[:args.MaxItems]
	}

	// Create merged feed title
	title := args.Title
	if title == "" {
		title = fmt.Sprintf("Merged Feed (%d sources)", len(args.FeedIDs))
	}

	// Create merged feed result
	mergedFeed := &MergedFeedResult{
		ID:          fmt.Sprintf("merged-%d", time.Now().Unix()),
		Title:       title,
		Description: fmt.Sprintf("Merged feed containing %d items from %d sources", len(allItems), len(feedTitles)),
		Items:       allItems,
		SourceFeeds: feedTitles,
		TotalItems:  len(allItems),
		CreatedAt:   time.Now(),
	}

	return mergedFeed, nil
}

// exportFeedData implements the feed export logic
func (s *Server) exportFeedData(ctx context.Context, args *ExportFeedDataParams) (string, error) {
	// Get feeds to export
	feedResults, err := s.getFeedsForExport(ctx, args.FeedIDs)
	if err != nil {
		return "", err
	}

	// Apply filters
	feedResults = s.applyExportFilters(feedResults, args)

	// Export in requested format
	return s.exportInFormat(feedResults, args)
}

// getFeedsForExport retrieves the feeds that need to be exported
func (s *Server) getFeedsForExport(ctx context.Context, feedIDs []string) ([]*FeedAndItemsResult, error) {
	if len(feedIDs) == 0 {
		return s.getAllFeedsForExport(ctx)
	}

	return s.getSpecificFeedsForExport(ctx, feedIDs)
}

// getAllFeedsForExport gets all feeds for export
func (s *Server) getAllFeedsForExport(ctx context.Context) ([]*FeedAndItemsResult, error) {
	allFeeds, err := s.allFeedsGetter.GetAllFeeds(ctx)
	if err != nil {
		return nil, err
	}

	feedResults := make([]*FeedAndItemsResult, 0, len(allFeeds))
	for _, feed := range allFeeds {
		feedResult, err := s.feedAndItemsGetter.GetFeedAndItems(ctx, feed.ID)
		if err != nil {
			continue
		}
		feedResults = append(feedResults, feedResult)
	}
	return feedResults, nil
}

// getSpecificFeedsForExport gets specific feeds for export
func (s *Server) getSpecificFeedsForExport(ctx context.Context, feedIDs []string) ([]*FeedAndItemsResult, error) {
	feedResults := make([]*FeedAndItemsResult, 0, len(feedIDs))
	for _, feedID := range feedIDs {
		feedResult, err := s.feedAndItemsGetter.GetFeedAndItems(ctx, feedID)
		if err != nil {
			continue
		}
		feedResults = append(feedResults, feedResult)
	}
	return feedResults, nil
}

// applyExportFilters applies date and item limit filters
func (s *Server) applyExportFilters(feedResults []*FeedAndItemsResult, args *ExportFeedDataParams) []*FeedAndItemsResult {
	// Apply date filters if specified
	if args.Since != "" || args.Until != "" {
		feedResults = filterFeedResultsByDate(feedResults, args.Since, args.Until)
	}

	// Apply maxItems limit per feed
	if args.MaxItems > 0 {
		for _, feedResult := range feedResults {
			if len(feedResult.Items) > args.MaxItems {
				feedResult.Items = feedResult.Items[:args.MaxItems]
			}
		}
	}

	return feedResults
}

// exportInFormat exports the feed results in the requested format
func (s *Server) exportInFormat(feedResults []*FeedAndItemsResult, args *ExportFeedDataParams) (string, error) {
	switch args.Format {
	case "json":
		return exportAsJSON(feedResults, args.IncludeAll)
	case "csv":
		return exportAsCSV(feedResults)
	case "opml":
		return exportAsOPML(feedResults)
	case "rss":
		return exportAsRSS(feedResults)
	case "atom":
		return exportAsAtom(feedResults)
	default:
		return "", model.NewFeedError(model.ErrorTypeValidation, fmt.Sprintf("unsupported export format: %s", args.Format)).
			WithOperation("export_feed_data").
			WithComponent("mcp_server")
	}
}

// Helper functions for feed merging and export

// deduplicateItems removes duplicate items based on title and link
func deduplicateItems(items []*gofeed.Item) []*gofeed.Item {
	seen := make(map[string]bool)
	var unique []*gofeed.Item

	for _, item := range items {
		// Create a unique key based on title and link
		key := fmt.Sprintf("%s|%s", item.Title, item.Link)
		if !seen[key] {
			seen[key] = true
			unique = append(unique, item)
		}
	}

	return unique
}

// sortItemsByDate sorts items by published date (newest first)
func sortItemsByDate(items []*gofeed.Item) {
	// Implementation will use sort.Slice
	for i := 0; i < len(items)-1; i++ {
		for j := i + 1; j < len(items); j++ {
			if items[i].PublishedParsed != nil && items[j].PublishedParsed != nil {
				if items[i].PublishedParsed.Before(*items[j].PublishedParsed) {
					items[i], items[j] = items[j], items[i]
				}
			}
		}
	}
}

// sortItemsByTitle sorts items alphabetically by title
func sortItemsByTitle(items []*gofeed.Item) {
	for i := 0; i < len(items)-1; i++ {
		for j := i + 1; j < len(items); j++ {
			if items[i].Title > items[j].Title {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
}

// sortItemsBySource sorts items by source feed title
func sortItemsBySource(items []*gofeed.Item) {
	for i := 0; i < len(items)-1; i++ {
		for j := i + 1; j < len(items); j++ {
			// gofeed.Item doesn't have a Source field, so we'll skip this for now
			// or we could use the Custom map if available
			sourceI := ""
			sourceJ := ""
			if items[i].Custom != nil && items[i].Custom["source"] != "" {
				sourceI = items[i].Custom["source"]
			}
			if items[j].Custom != nil && items[j].Custom["source"] != "" {
				sourceJ = items[j].Custom["source"]
			}
			if sourceI > sourceJ {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
}

// filterFeedResultsByDate filters feed result items by publication date range
func filterFeedResultsByDate(feedResults []*FeedAndItemsResult, since, until string) []*FeedAndItemsResult {
	sinceTime, untilTime, err := parseTimeRange(since, until)
	if err != nil {
		return feedResults // Skip filtering if parsing fails
	}

	for _, feedResult := range feedResults {
		feedResult.Items = filterItemsByDateRange(feedResult.Items, sinceTime, untilTime)
	}

	return feedResults
}

// parseTimeRange parses since and until time strings
func parseTimeRange(since, until string) (sinceTime, untilTime time.Time, err error) {
	if since != "" {
		sinceTime, err = time.Parse(time.RFC3339, since)
		if err != nil {
			return
		}
	}

	if until != "" {
		untilTime, err = time.Parse(time.RFC3339, until)
		if err != nil {
			return
		}
	}

	return
}

// filterItemsByDateRange filters items within the given date range
func filterItemsByDateRange(items []*gofeed.Item, sinceTime, untilTime time.Time) []*gofeed.Item {
	var filteredItems []*gofeed.Item

	for _, item := range items {
		if itemInDateRange(item, sinceTime, untilTime) {
			filteredItems = append(filteredItems, item)
		}
	}

	return filteredItems
}

// itemInDateRange checks if an item falls within the date range
func itemInDateRange(item *gofeed.Item, sinceTime, untilTime time.Time) bool {
	if item.PublishedParsed == nil {
		return true
	}

	if !sinceTime.IsZero() && item.PublishedParsed.Before(sinceTime) {
		return false
	}

	if !untilTime.IsZero() && item.PublishedParsed.After(untilTime) {
		return false
	}

	return true
}

// Export format implementations

// exportAsJSON exports feed results as JSON
func exportAsJSON(feedResults []*FeedAndItemsResult, includeAll bool) (string, error) {
	data := struct {
		FeedResults []*FeedAndItemsResult `json:"feed_results"`
		ExportedAt  time.Time             `json:"exported_at"`
		Count       int                   `json:"count"`
	}{
		FeedResults: feedResults,
		ExportedAt:  time.Now(),
		Count:       len(feedResults),
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}

	return string(jsonData), nil
}

// exportAsCSV exports feed results as CSV
func exportAsCSV(feedResults []*FeedAndItemsResult) (string, error) {
	var result string

	// CSV header
	result += "Feed Title,Feed URL,Item Title,Item Link,Published Date,Description\n"

	// CSV rows
	for _, feedResult := range feedResults {
		for _, item := range feedResult.Items {
			// Escape commas and quotes in CSV fields
			feedTitle := escapeCSVField(feedResult.Title)
			feedURL := escapeCSVField(feedResult.PublicURL)
			itemTitle := escapeCSVField(item.Title)
			itemLink := escapeCSVField(item.Link)
			publishedDate := ""
			if item.PublishedParsed != nil {
				publishedDate = item.PublishedParsed.Format(time.RFC3339)
			}
			description := escapeCSVField(item.Description)

			result += fmt.Sprintf("%s,%s,%s,%s,%s,%s\n",
				feedTitle, feedURL, itemTitle, itemLink, publishedDate, description)
		}
	}

	return result, nil
}

// exportAsOPML exports feed results as OPML
func exportAsOPML(feedResults []*FeedAndItemsResult) (string, error) {
	result := `<?xml version="1.0" encoding="UTF-8"?>
<opml version="1.0">
<head>
<title>Feed Export</title>
<dateCreated>` + time.Now().Format(time.RFC1123Z) + `</dateCreated>
</head>
<body>
`

	for _, feedResult := range feedResults {
		result += fmt.Sprintf(`<outline text=%q title=%q type="rss" xmlUrl=%q htmlUrl=%q/>`,
			escapeXML(feedResult.Title), escapeXML(feedResult.Title), escapeXML(feedResult.PublicURL), escapeXML(feedResult.PublicURL))
		result += "\n"
	}

	result += `</body>
</opml>`

	return result, nil
}

// exportAsRSS exports feed results as RSS 2.0
func exportAsRSS(feedResults []*FeedAndItemsResult) (string, error) {
	result := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
<channel>
<title>Combined Feed Export</title>
<description>Combined feed containing items from multiple sources</description>
<lastBuildDate>` + time.Now().Format(time.RFC1123Z) + `</lastBuildDate>
`

	for _, feedResult := range feedResults {
		for _, item := range feedResult.Items {
			pubDate := ""
			if item.PublishedParsed != nil {
				pubDate = item.PublishedParsed.Format(time.RFC1123Z)
			}
			result += `<item>
<title>` + escapeXML(item.Title) + `</title>
<link>` + escapeXML(item.Link) + `</link>
<description>` + escapeXML(item.Description) + `</description>
<pubDate>` + pubDate + `</pubDate>
<guid>` + escapeXML(item.Link) + `</guid>
</item>
`
		}
	}

	result += `</channel>
</rss>`

	return result, nil
}

// exportAsAtom exports feed results as Atom 1.0
func exportAsAtom(feedResults []*FeedAndItemsResult) (string, error) {
	result := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
<title>Combined Feed Export</title>
<subtitle>Combined feed containing items from multiple sources</subtitle>
<updated>` + time.Now().Format(time.RFC3339) + `</updated>
<id>urn:feed-mcp:export:` + fmt.Sprintf("%d", time.Now().Unix()) + `</id>
`

	for _, feedResult := range feedResults {
		for _, item := range feedResult.Items {
			updatedDate := time.Now().Format(time.RFC3339)
			if item.PublishedParsed != nil {
				updatedDate = item.PublishedParsed.Format(time.RFC3339)
			}
			result += `<entry>
<title>` + escapeXML(item.Title) + `</title>
<link href="` + escapeXML(item.Link) + `"/>
<summary>` + escapeXML(item.Description) + `</summary>
<updated>` + updatedDate + `</updated>
<id>` + escapeXML(item.Link) + `</id>
</entry>
`
		}
	}

	result += `</feed>`

	return result, nil
}

// Utility functions for escaping

// escapeCSVField escapes a field for CSV format
func escapeCSVField(field string) string {
	// If field contains comma, quote, or newline, wrap in quotes and escape quotes
	if containsAny(field, ",\"\\n\\r") {
		field = `"` + replaceAll(field, `"`, `""`) + `"`
	}
	return field
}

// escapeXML escapes a string for XML format
func escapeXML(s string) string {
	s = replaceAll(s, "&", "&amp;")
	s = replaceAll(s, "<", "&lt;")
	s = replaceAll(s, ">", "&gt;")
	s = replaceAll(s, `"`, "&quot;")
	s = replaceAll(s, "'", "&#39;")
	return s
}

// containsAny checks if string contains any of the specified characters
func containsAny(s, chars string) bool {
	for _, char := range chars {
		for _, sChar := range s {
			if char == sChar {
				return true
			}
		}
	}
	return false
}

// replaceAll replaces all occurrences of old with replacement in string
func replaceAll(s, old, replacement string) string {
	result := ""
	for i := 0; i < len(s); {
		if i+len(old) <= len(s) && s[i:i+len(old)] == old {
			result += replacement
			i += len(old)
		} else {
			result += string(s[i])
			i++
		}
	}
	return result
}
