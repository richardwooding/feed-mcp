package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gocolly/colly"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/richardwooding/feed-mcp/model"
)

type Config struct {
	Transport          model.Transport
	AllFeedsGetter     AllFeedsGetter
	FeedAndItemsGetter FeedAndItemsGetter
}

type Server struct {
	transport          model.Transport
	allFeedsGetter     AllFeedsGetter
	feedAndItemsGetter FeedAndItemsGetter
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
	}, nil
}

func (s *Server) Run() (err error) {

	// Create a new MCP server
	srv := server.NewMCPServer(
		"RSS, Atom, and JSON Feed Server",
		"1.0.0",
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)

	fetchLinkTool := mcp.NewTool("fetch_link",
		mcp.WithDescription("Fetch link URL"),
		mcp.WithString("link",
			mcp.Required(),
			mcp.Description("Link URL"),
		),
	)

	srv.AddTool(fetchLinkTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		link, err := request.RequireString("link")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		c := colly.NewCollector()

		var data []byte

		c.OnResponse(func(response *colly.Response) {
			data = response.Body
		})

		err = c.Visit(link)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	})

	allFeedsTool := mcp.NewTool("all_syndication_feeds",
		mcp.WithDescription("list available feedItem resources"),
	)

	srv.AddTool(allFeedsTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		feedResults, err := s.allFeedsGetter.GetAllFeeds(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, err := json.Marshal(feedResults)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	})

	getSyndicationFeedTool := mcp.NewTool("get_syndication_feed_items",
		mcp.WithDescription("get syndication feed and items by id"),
		mcp.WithString("id",
			mcp.Required(),
			mcp.Description("Feed ID"),
		),
	)

	srv.AddTool(getSyndicationFeedTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := request.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		feedResult, err := s.feedAndItemsGetter.GetFeedAndItems(ctx, id)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if data, err := json.Marshal(feedResult); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		} else {
			return mcp.NewToolResultText(string(data)), nil
		}
	})

	switch s.transport {
	case model.StdioTransport:
		err = server.ServeStdio(srv)
	case model.HttpWithSSETransport:
		httpServer := server.NewStreamableHTTPServer(srv)
		err = httpServer.Start(":8080")
	default:
		return errors.New("unsupported transport")
	}

	return
}
