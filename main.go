package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"

	"github.com/richardwooding/feed-mcp/cmd"
	"github.com/richardwooding/feed-mcp/model"
	"github.com/richardwooding/feed-mcp/version"
)

type CLI struct {
	model.Globals
	Run cmd.RunCmd `cmd:"" help:"Run MCP Server"`
}

func main() {
	versionStr := version.GetVersion()
	cli := CLI{
		Globals: model.Globals{
			Version: model.VersionFlag(versionStr),
		},
	}

	kongCtx := kong.Parse(&cli,
		kong.Name("feed-mcp"),
		kong.Description("A MCP server for RSS and Atom feeds"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
		kong.Vars{
			"version": versionStr,
		})

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle SIGINT (Ctrl+C) and SIGTERM
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		cancel() // Cancel context on signal
	}()

	// Pass the context to the command
	err := kongCtx.Run(&cli.Globals, ctx)
	kongCtx.FatalIfErrorf(err)
}
