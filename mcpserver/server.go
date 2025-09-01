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
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/richardwooding/feed-mcp/model"
	"github.com/richardwooding/feed-mcp/version"
)

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

	// Add dynamic feed management tools if DynamicFeedManager is available
	s.addDynamicFeedTools(srv)

	// Add resource handlers for MCP Resources support
	s.addResourceHandlers(srv)

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
