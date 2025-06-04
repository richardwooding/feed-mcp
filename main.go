package main

import (
	"github.com/alecthomas/kong"
	"github.com/richardwooding/feed-mcp/cmd"
	"github.com/richardwooding/feed-mcp/model"
)

type CLI struct {
	model.Globals
	Run cmd.RunCmd `cmd:"" help:"Run MCP Server"`
}

func main() {
	cli := CLI{
		Globals: model.Globals{
			Version: model.VersionFlag("0.1.1"),
		},
	}

	ctx := kong.Parse(&cli,
		kong.Name("feed-mcp"),
		kong.Description("A MCP server for RSS and Atom feeds"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
		kong.Vars{
			"version": "0.1.11",
		})
	err := ctx.Run(&cli.Globals)
	ctx.FatalIfErrorf(err)
}
