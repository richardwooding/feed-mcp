package main

import (
	"github.com/alecthomas/kong"
	"github.com/richardwooding/feed-mcp/mcpserver"
)

var CLI struct {
	Run struct {
		Transport string   `name:"transport" default:"stdio" enum:"stdio,http-with-sse" help:"Transport to use for the MCP server."`
		Feeds     []string `arg:"" name:"feeds" help:"Feeds to list."`
	} `cmd:"" help:"Run MCP Server"`
}

func main() {
	ctx := kong.Parse(&CLI)
	switch ctx.Command() {
	case "run <feeds>":
		transport, err := mcpserver.ParseTransport(CLI.Run.Transport)
		if err != nil {
			panic(err)
		}
		if len(CLI.Run.Feeds) == 0 {
			panic("at least one feed must be specified")
		}
		server, err := mcpserver.NewServer(mcpserver.Config{
			Transport: transport,
			Feeds:     CLI.Run.Feeds,
		})
		if err != nil {
			panic(err)
		}
		if err := server.Run(); err != nil {
			panic(err)
		}
	default:
		panic(ctx.Command())
	}
}
