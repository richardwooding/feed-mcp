// Package cmd provides CLI commands for the feed-mcp server.
package cmd

import (
	"context"
	"time"

	"github.com/richardwooding/feed-mcp/mcpserver"
	"github.com/richardwooding/feed-mcp/model"
	"github.com/richardwooding/feed-mcp/store"
)

// RunCmd holds the command line arguments and flags for the run command
type RunCmd struct {
	Transport       string        `name:"transport" default:"stdio" enum:"stdio,http-with-sse" help:"Transport to use for the MCP server."`
	Feeds           []string      `arg:"" name:"feeds" optional:"" help:"Feeds to list (cannot be used with --opml)."`
	OPML            string        `name:"opml" help:"OPML file path or URL to load feed URLs from (cannot be used with feeds)."`
	ExpireAfter     time.Duration `name:"expire-after" default:"1h" help:"Expire feeds after this duration."`
	Timeout         time.Duration `name:"timeout" default:"30s" help:"Timeout for fetching feed."`
	ShutdownTimeout time.Duration `name:"shutdown-timeout" default:"30s" help:"Timeout for graceful shutdown."`
	// HTTP connection pooling settings
	MaxIdleConns        int           `name:"max-idle-conns" default:"100" help:"Maximum number of idle HTTP connections across all hosts."`
	MaxConnsPerHost     int           `name:"max-conns-per-host" default:"10" help:"Maximum number of connections per host."`
	MaxIdleConnsPerHost int           `name:"max-idle-conns-per-host" default:"5" help:"Maximum number of idle connections per host."`
	IdleConnTimeout     time.Duration `name:"idle-conn-timeout" default:"90s" help:"How long an idle connection remains idle before closing."`
	// Retry mechanism settings
	RetryMaxAttempts int           `name:"retry-max-attempts" default:"3" help:"Maximum number of retry attempts for failed feed fetches."`
	RetryBaseDelay   time.Duration `name:"retry-base-delay" default:"1s" help:"Base delay for exponential backoff between retry attempts."`
	RetryMaxDelay    time.Duration `name:"retry-max-delay" default:"30s" help:"Maximum delay between retry attempts."`
	RetryJitter      bool          `name:"retry-jitter" default:"true" help:"Enable jitter in retry delays to avoid thundering herd."`
	// Security settings
	AllowPrivateIPs bool `name:"allow-private-ips" default:"false" help:"Allow feed URLs that resolve to private IP ranges or localhost (disabled by default for security)."`
	// Runtime feed management settings
	AllowRuntimeFeeds bool `name:"allow-runtime-feeds" default:"false" help:"Enable runtime feed management tools (add_feed, remove_feed, list_managed_feeds)."`
}

// Run executes the feed MCP server with the given configuration
func (c *RunCmd) Run(globals *model.Globals, ctx context.Context) error {
	transport, err := model.ParseTransport(c.Transport)
	if err != nil {
		return err
	}

	// Determine the feed URLs to use
	var feedURLs []string

	// Check for mutually exclusive options
	if c.OPML != "" && len(c.Feeds) > 0 {
		return model.NewFeedError(model.ErrorTypeConfiguration, "cannot specify both --opml and feed URLs").
			WithOperation("run_command").
			WithComponent("cli")
	}

	if c.OPML != "" {
		// Load feed URLs from OPML
		feedURLs, err = model.LoadFeedURLsFromOPML(c.OPML)
		if err != nil {
			return err
		}
	} else if len(c.Feeds) > 0 {
		// Use directly specified feeds
		feedURLs = c.Feeds
	} else {
		return model.NewFeedError(model.ErrorTypeConfiguration, "no feeds specified - use either feed URLs or --opml").
			WithOperation("run_command").
			WithComponent("cli")
	}

	// Validate feed URLs for security
	if err := model.SanitizeFeedURLs(feedURLs, c.AllowPrivateIPs); err != nil {
		return err
	}

	storeConfig := store.Config{
		Feeds:               feedURLs,
		OPML:                c.OPML, // Pass OPML path for metadata source detection
		Timeout:             c.Timeout,
		ExpireAfter:         c.ExpireAfter,
		MaxIdleConns:        c.MaxIdleConns,
		MaxConnsPerHost:     c.MaxConnsPerHost,
		MaxIdleConnsPerHost: c.MaxIdleConnsPerHost,
		IdleConnTimeout:     c.IdleConnTimeout,
		RetryMaxAttempts:    c.RetryMaxAttempts,
		RetryBaseDelay:      c.RetryBaseDelay,
		RetryMaxDelay:       c.RetryMaxDelay,
		RetryJitter:         c.RetryJitter,
		AllowPrivateIPs:     c.AllowPrivateIPs,
	}

	serverConfig := mcpserver.Config{
		Transport: transport,
	}

	if c.AllowRuntimeFeeds {
		// Use DynamicStore for runtime feed management
		dynamicStore, err := store.NewDynamicStore(&storeConfig, true)
		if err != nil {
			return err
		}
		serverConfig.AllFeedsGetter = dynamicStore
		serverConfig.FeedAndItemsGetter = dynamicStore
		serverConfig.DynamicFeedManager = dynamicStore
	} else {
		// Use regular Store
		feedStore, err := store.NewStore(storeConfig)
		if err != nil {
			return err
		}
		serverConfig.AllFeedsGetter = feedStore
		serverConfig.FeedAndItemsGetter = feedStore
	}

	server, err := mcpserver.NewServer(serverConfig)
	if err != nil {
		return err
	}
	return server.Run(ctx)
}
