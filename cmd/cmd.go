package cmd

import (
	"context"
	"errors"
	"time"

	"github.com/richardwooding/feed-mcp/mcpserver"
	"github.com/richardwooding/feed-mcp/model"
	"github.com/richardwooding/feed-mcp/store"
)

type RunCmd struct {
	Transport            string        `name:"transport" default:"stdio" enum:"stdio,http-with-sse" help:"Transport to use for the MCP server."`
	Feeds                []string      `arg:"" name:"feeds" help:"Feeds to list."`
	ExpireAfter          time.Duration `name:"expire-after" default:"1h" help:"Expire feeds after this duration."`
	Timeout              time.Duration `name:"timeout" default:"30s" help:"Timeout for fetching feed."`
	ShutdownTimeout      time.Duration `name:"shutdown-timeout" default:"30s" help:"Timeout for graceful shutdown."`
	// HTTP connection pooling settings
	MaxIdleConns         int           `name:"max-idle-conns" default:"100" help:"Maximum number of idle HTTP connections across all hosts."`
	MaxConnsPerHost      int           `name:"max-conns-per-host" default:"10" help:"Maximum number of connections per host."`
	MaxIdleConnsPerHost  int           `name:"max-idle-conns-per-host" default:"5" help:"Maximum number of idle connections per host."`
	IdleConnTimeout      time.Duration `name:"idle-conn-timeout" default:"90s" help:"How long an idle connection remains idle before closing."`
}

func (c *RunCmd) Run(globals *model.Globals, ctx context.Context) error {
	transport, err := model.ParseTransport(c.Transport)
	if err != nil {
		return err
	}
	if len(c.Feeds) == 0 {
		return errors.New("no feeds specified")
	}
	feedStore, err := store.NewStore(store.Config{
		Feeds:                c.Feeds,
		Timeout:              c.Timeout,
		ExpireAfter:          c.ExpireAfter,
		MaxIdleConns:         c.MaxIdleConns,
		MaxConnsPerHost:      c.MaxConnsPerHost,
		MaxIdleConnsPerHost:  c.MaxIdleConnsPerHost,
		IdleConnTimeout:      c.IdleConnTimeout,
	})
	if err != nil {
		return err
	}
	server, err := mcpserver.NewServer(mcpserver.Config{
		Transport:          transport,
		AllFeedsGetter:     feedStore,
		FeedAndItemsGetter: feedStore,
	})
	if err != nil {
		return err
	}
	return server.Run(ctx)
}
