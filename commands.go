package main

import "github.com/richardwooding/feed-mcp/mcpserver"

type RunCmd struct {
	Transport string   `name:"transport" default:"stdio" enum:"stdio,http-with-sse" help:"Transport to use for the MCP server."`
	Feeds     []string `arg:"" name:"feeds" help:"Feeds to list."`
}

func (c *RunCmd) Run(globals *Globals) error {
	transport, err := mcpserver.ParseTransport(c.Transport)
	if err != nil {
		return err
	}
	if len(c.Feeds) == 0 {
		panic("at least one feed must be specified")
	}
	server, err := mcpserver.NewServer(mcpserver.Config{
		Transport: transport,
		Feeds:     c.Feeds,
	})
	if err != nil {
		return err
	}
	return server.Run()
}
