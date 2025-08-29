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
	Transport          model.Transport
}

// Server implements an MCP server for serving syndication feeds
type Server struct {
	allFeedsGetter     AllFeedsGetter
	feedAndItemsGetter FeedAndItemsGetter
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

// addResourceHandlers adds MCP Resource handlers to the server
func (s *Server) addResourceHandlers(srv *mcp.Server) {
	// Note: The MCP Go SDK resource handlers are still being designed
	// For now, we prepare the infrastructure but cannot add handlers yet
	// This will be implemented once the SDK provides resource handler support

	// When resource handlers are available, we would add them like this:
	// srv.AddResourceHandler("resources/list", s.handleListResources)
	// srv.AddResourceHandler("resources/read", s.handleReadResource)
	// srv.AddResourceHandler("resources/subscribe", s.handleSubscribeResource)
	// srv.AddResourceHandler("resources/unsubscribe", s.handleUnsubscribeResource)
}

// MCP v0.3.0 SDK now has built-in resource subscription support

// handleSubscribeResource handles resource subscription requests using v0.3.0 SDK
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
