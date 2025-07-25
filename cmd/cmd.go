package cmd

import (
	"errors"
	"time"

	"github.com/richardwooding/feed-mcp/mcpserver"
	"github.com/richardwooding/feed-mcp/model"
	"github.com/richardwooding/feed-mcp/store"
)

type RunCmd struct {
	Transport   string        `name:"transport" default:"stdio" enum:"stdio,http-with-sse" help:"Transport to use for the MCP server."`
	Feeds       []string      `arg:"" name:"feeds" help:"Feeds to list."`
	ExpireAfter time.Duration `name:"expire-after" default:"1h" help:"Expire feeds after this duration."`
	Timeout     time.Duration `name:"timeout" default:"30s" help:"Timeout for fetching feed."`
}

func (c *RunCmd) Run(globals *model.Globals) error {
	transport, err := model.ParseTransport(c.Transport)
	if err != nil {
		return err
	}
	if len(c.Feeds) == 0 {
		return errors.New("no feeds specified")
	}
	feedStore, err := store.NewStore(store.Config{
		Feeds:       c.Feeds,
		Timeout:     c.Timeout,
		ExpireAfter: c.ExpireAfter,
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
	return server.Run()
}
