package main

import (
	"github.com/richardwooding/feed-mcp/mcpserver"
	"github.com/richardwooding/feed-mcp/model"
	"github.com/richardwooding/feed-mcp/store"
)

type RunCmd struct {
	Transport string   `name:"transport" default:"stdio" enum:"stdio,http-with-sse" help:"Transport to use for the MCP server."`
	Feeds     []string `arg:"" name:"feeds" help:"Feeds to list."`
}

func (c *RunCmd) Run(globals *Globals) error {
	transport, err := model.ParseTransport(c.Transport)
	if err != nil {
		return err
	}
	if len(c.Feeds) == 0 {
		panic("at least one feed must be specified")
	}
	feedStore, err := store.NewStore(store.Config{
		Feeds: c.Feeds,
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
