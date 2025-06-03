package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dgraph-io/ristretto"
	"github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/store"
	ristretto_store "github.com/eko/gocache/store/ristretto/v4"
	"github.com/gocolly/colly"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/mmcdole/gofeed"
	"github.com/richardwooding/feed-mcp/model"
	"sync"
	"time"
)

type Config struct {
	Transport Transport `json:"transport" jsonschema:"required,description=The transport to use for the server,enum=stdio,http-with-sse"`
	Feeds     []string  `json:"feeds" jsonschema:"required,description=Feeds to list."`
}

type feedItem struct {
	title string
	url   string
}

type Server struct {
	transport           Transport
	feeds               map[string]feedItem
	feedCacheManager    *cache.LoadableCache[*gofeed.Feed]
	articleCacheManager *cache.LoadableCache[string]
}

func NewServer(config Config) (*Server, error) {
	if config.Transport == UndefinedTransport {
		return nil, errors.New("transport must be specified")
	}

	if len(config.Feeds) == 0 {
		return nil, errors.New("at least one feedItem must be specified")
	}

	ristrettoCache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1000,
		MaxCost:     100,
		BufferItems: 64,
	})
	if err != nil {
		return nil, err
	}

	ristrettoStore := ristretto_store.NewRistretto(ristrettoCache)

	loadFunction := func(ctx context.Context, key any) (*gofeed.Feed, []store.Option, error) {
		if url, ok := key.(string); ok {
			fp := gofeed.NewParser()
			feed, err := fp.ParseURL(url)
			if err != nil {
				return nil, nil, err
			}
			return feed, []store.Option{store.WithExpiration(5 * time.Minute)}, nil
		} else {
			return nil, nil, errors.New("invalid key type")
		}
	}

	cacheManager := cache.NewLoadable[*gofeed.Feed](
		loadFunction,
		cache.New[*gofeed.Feed](ristrettoStore),
	)

	wg := &sync.WaitGroup{}
	feeds := make(map[string]feedItem, len(config.Feeds))
	for _, feedURL := range config.Feeds {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			id, err := gonanoid.New()
			if err != nil {
				fmt.Printf("error generating id: %s\n", err)
				return
			}
			feed, err := cacheManager.Get(context.Background(), url)
			if err != nil {
				fmt.Printf("Failed to load feedItem %s: %v\n", url, err)
				return
			}
			feeds[id] = feedItem{feed.Title, url}
		}(feedURL)
	}

	return &Server{
		transport:        config.Transport,
		feeds:            feeds,
		feedCacheManager: cacheManager,
	}, nil
}

type FeedResult struct {
	ID         string      `json:"id"`
	PublicURL  string      `json:"public_url"`
	Title      string      `json:"title,omitempty"`
	FetchError string      `json:"fetch_error,omitempty"`
	Feed       *model.Feed `json:"feed,omitempty"`
}

type FeedAndItemsResult struct {
	ID         string         `json:"id"`
	PublicURL  string         `json:"public_url"`
	Title      string         `json:"title,omitempty"`
	FetchError string         `json:"fetch_error,omitempty"`
	Feed       *model.Feed    `json:"feed_result,omitempty"`
	Items      []*gofeed.Item `json:"items,omitempty"`
}

func (s *Server) Run() (err error) {

	// Create a new MCP server
	srv := server.NewMCPServer(
		"Calculator Demo",
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

	srv.AddTool(allFeedsTool, func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		feedResults := make([]FeedResult, len(s.feeds))
		var wg = &sync.WaitGroup{}
		idx := 0
		for id, item := range s.feeds {
			wg.Add(1)
			go func(idx int, item feedItem) {
				defer wg.Done()
				if feed, err := s.feedCacheManager.Get(context.Background(), item.url); err == nil {
					feedResults[idx] = FeedResult{
						ID:        id,
						PublicURL: item.url,
						Title:     item.title,
						Feed:      model.FromGoFeed(feed),
					}
				} else {
					feedResults[idx] = FeedResult{
						PublicURL:  item.url,
						Title:      item.title,
						FetchError: err.Error(),
					}
				}
			}(idx, item)
			idx++
		}
		wg.Wait()

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

	srv.AddTool(getSyndicationFeedTool, func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := request.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		var feedResult FeedAndItemsResult
		if item, ok := s.feeds[id]; !ok {
			return mcp.NewToolResultError("id not found"), nil
		} else {
			if feed, err := s.feedCacheManager.Get(context.Background(), item.url); err == nil {
				feedResult = FeedAndItemsResult{
					PublicURL: item.url,
					Title:     item.title,
					Feed:      model.FromGoFeed(feed),
					Items:     feed.Items,
				}
			} else {
				return mcp.NewToolResultError(err.Error()), nil
			}
		}
		if data, err := json.Marshal(feedResult); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		} else {
			return mcp.NewToolResultText(string(data)), nil
		}
	})

	switch s.transport {
	case StdioTransport:
		err = server.ServeStdio(srv)
	case HttpWithSSETransport:
		httpServer := server.NewStreamableHTTPServer(srv)
		err = httpServer.Start(":8080")
	default:
		return errors.New("unsupported transport")
	}

	return
}
