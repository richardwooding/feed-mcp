package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/gocolly/colly"
	"github.com/modelcontextprotocol/go-sdk/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/richardwooding/feed-mcp/model"
)

var sessionCounter int64

type Config struct {
	Transport          model.Transport
	AllFeedsGetter     AllFeedsGetter
	FeedAndItemsGetter FeedAndItemsGetter
}

type Server struct {
	transport          model.Transport
	allFeedsGetter     AllFeedsGetter
	feedAndItemsGetter FeedAndItemsGetter
	sessionID          string
}

// generateSessionID creates a unique session ID for this server instance
func generateSessionID() string {
	counter := atomic.AddInt64(&sessionCounter, 1)
	return fmt.Sprintf("feed-mcp-session-%d-%d", time.Now().UnixNano(), counter)
}

func NewServer(config Config) (*Server, error) {
	if config.Transport == model.UndefinedTransport {
		return nil, errors.New("transport must be specified")
	}
	if config.AllFeedsGetter == nil {
		return nil, errors.New("AllFeedsGetter is required")
	}
	if config.FeedAndItemsGetter == nil {
		return nil, errors.New("FeedAndItemsGetter is required")
	}
	return &Server{
		transport:          config.Transport,
		allFeedsGetter:     config.AllFeedsGetter,
		feedAndItemsGetter: config.FeedAndItemsGetter,
		sessionID:          generateSessionID(),
	}, nil
}

type FetchLinkParams struct {
	URL string
}

type GetSyndicationFeedParams struct {
	ID string
}

func (s *Server) Run(ctx context.Context) (err error) {

	// Create a new MCP server
	srv := mcp.NewServer(
		&mcp.Implementation{
			Name:    "RSS, Atom, and JSON Feed Server",
			Version: "1.0.0",
		},
		nil,
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
	mcp.AddTool(srv, fetchLinkTool, func(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[FetchLinkParams]) (*mcp.CallToolResultFor[any], error) {
		c := colly.NewCollector()

		var data []byte

		c.OnResponse(func(response *colly.Response) {
			data = response.Body
		})

		err = c.Visit(params.Arguments.URL)
		if err != nil {
			return nil, err
		}
		return &mcp.CallToolResultFor[any]{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil
	})

	// Add all_syndication_feeds tool
	allFeedsTool := &mcp.Tool{
		Name:        "all_syndication_feeds",
		Description: "list available feedItem resources",
		InputSchema: &jsonschema.Schema{Type: "object"}, // No parameters needed
	}
	mcp.AddTool(srv, allFeedsTool, func(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[any]) (*mcp.CallToolResultFor[any], error) {
		feedResults, err := s.allFeedsGetter.GetAllFeeds(ctx)
		if err != nil {
			return nil, err
		}
		data, err := json.Marshal(feedResults)
		if err != nil {
			return nil, err
		}
		return &mcp.CallToolResultFor[any]{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil
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
	mcp.AddTool(srv, getSyndicationFeedTool, func(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[GetSyndicationFeedParams]) (*mcp.CallToolResultFor[any], error) {
		feedResult, err := s.feedAndItemsGetter.GetFeedAndItems(ctx, params.Arguments.ID)
		if err != nil {
			return nil, err
		}
		data, err := json.Marshal(feedResult)
		if err != nil {
			return nil, err
		}
		return &mcp.CallToolResultFor[any]{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil
	})

	switch s.transport {
	case model.StdioTransport:
		err = srv.Run(ctx, mcp.NewStdioTransport())
	case model.HttpWithSSETransport:
		err = srv.Run(ctx, mcp.NewStreamableServerTransport(s.sessionID))
	default:
		return errors.New("unsupported transport")
	}

	return
}
