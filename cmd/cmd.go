// Package cmd provides CLI commands for the feed-mcp server.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/richardwooding/feed-mcp/mcpserver"
	"github.com/richardwooding/feed-mcp/model"
	"github.com/richardwooding/feed-mcp/store"
)

// RunCmd holds the command line arguments and flags for the run command
type RunCmd struct {
	Transport       string        `name:"transport" default:"stdio" enum:"stdio,http-with-sse,streamable-http" help:"Transport to use for the MCP server (streamable-http is recommended for HTTP)."`
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
	// Rate limiting settings (applied per host)
	RequestsPerSecond      float64       `name:"requests-per-second" default:"2" help:"Per-host rate limit for outbound feed requests (requests per second)."`
	BurstCapacity          int           `name:"burst-capacity" default:"5" help:"Per-host rate-limit burst capacity (max immediate requests before throttling)."`
	RateLimiterIdleTimeout time.Duration `name:"rate-limiter-idle-timeout" default:"1h" help:"Evict a host's rate limiter after this idle period, bounding memory under runtime feed churn (0 disables eviction)."`
	// Retry mechanism settings
	RetryMaxAttempts int           `name:"retry-max-attempts" default:"3" help:"Maximum number of retry attempts for failed feed fetches."`
	RetryBaseDelay   time.Duration `name:"retry-base-delay" default:"1s" help:"Base delay for exponential backoff between retry attempts."`
	RetryMaxDelay    time.Duration `name:"retry-max-delay" default:"30s" help:"Maximum delay between retry attempts."`
	RetryJitter      bool          `name:"retry-jitter" default:"true" help:"Enable jitter in retry delays to avoid thundering herd."`
	// Security settings
	AllowPrivateIPs bool `name:"allow-private-ips" default:"false" help:"Allow feed URLs that resolve to private IP ranges or localhost (disabled by default for security)."`
	// Runtime feed management settings
	AllowRuntimeFeeds bool `name:"allow-runtime-feeds" default:"false" help:"Enable runtime feed management tools (add_feed, remove_feed, list_managed_feeds)."`
	// HTTP server settings (for streamable-http transport)
	HTTPPort           string        `name:"http-port" default:"8080" env:"PORT" help:"Port for HTTP server (streamable-http transport)."`
	HTTPStateless      bool          `name:"http-stateless" default:"false" help:"Run HTTP server in stateless mode (no session tracking)."`
	HTTPSessionTimeout time.Duration `name:"http-session-timeout" default:"30m" help:"Timeout for idle HTTP sessions."`
}

// validateStartupFeedURLs runs up-front SSRF validation over the configured feed
// URLs. Each URL is validated independently (and concurrently, so a large feed
// list with several slow or dead hosts doesn't serialize one DNS resolve-timeout
// after another and delay startup). A per-URL resolve-timeout is tolerated — the
// dial-time guard re-checks every destination at fetch time, so slow DNS must not
// block startup — but genuine validation errors are aggregated and returned, and
// a real cancellation/shutdown of the parent context aborts startup.
func validateStartupFeedURLs(ctx context.Context, feedURLs []string, allowPrivateIPs bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(feedURLs) == 0 {
		return nil
	}

	// Each goroutine writes only its own index, so the slice needs no locking.
	// A bounded semaphore caps in-flight DNS resolutions so a very large feed
	// list (e.g. a big OPML) can't exhaust file descriptors or overload the
	// resolver.
	type urlResult struct {
		url string
		err error
	}
	const maxConcurrentValidations = 16
	results := make([]urlResult, len(feedURLs))
	sem := make(chan struct{}, maxConcurrentValidations)
	var wg sync.WaitGroup
	for i, url := range feedURLs {
		wg.Add(1)
		go func(i int, url string) {
			defer wg.Done()
			// Don't block on the semaphore if the caller has already given up;
			// record the context error and return so shutdown isn't delayed.
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i] = urlResult{url: url, err: ctx.Err()}
				return
			}
			defer func() { <-sem }()
			results[i] = urlResult{url: url, err: model.ValidateFeedURLContext(ctx, url, allowPrivateIPs)}
		}(i, url)
	}
	wg.Wait()

	// A real parent-context error (cancellation, or ctx's own deadline) aborts
	// startup; checking it here means a resolve-timeout below is only our
	// internal budget.
	if err := ctx.Err(); err != nil {
		return err
	}

	var invalidURLs []string
	for _, r := range results {
		if r.err == nil {
			continue
		}
		if errors.Is(r.err, context.DeadlineExceeded) {
			log.Printf("warning: feed URL validation timed out resolving DNS for %s; continuing (re-checked at fetch time): %v", r.url, r.err)
			continue
		}
		invalidURLs = append(invalidURLs, fmt.Sprintf("%s: %v", r.url, r.err))
	}

	if len(invalidURLs) > 0 {
		return fmt.Errorf("invalid feed URLs:\n%s", strings.Join(invalidURLs, "\n"))
	}
	return nil
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
	} else if !c.AllowRuntimeFeeds {
		// Only require feeds if runtime feed management is disabled
		return model.NewFeedError(model.ErrorTypeConfiguration, "no feeds specified - use either feed URLs or --opml").
			WithOperation("run_command").
			WithComponent("cli")
	} else {
		// Allow starting with no feeds when runtime feed management is enabled
		feedURLs = []string{}
	}

	// Validate feed URLs for security (skip validation if no URLs and runtime feeds are allowed)
	if err := validateStartupFeedURLs(ctx, feedURLs, c.AllowPrivateIPs); err != nil {
		return err
	}

	storeConfig := store.Config{
		Feeds:                  feedURLs,
		OPML:                   c.OPML, // Pass OPML path for metadata source detection
		Timeout:                c.Timeout,
		ExpireAfter:            c.ExpireAfter,
		RequestsPerSecond:      c.RequestsPerSecond,
		BurstCapacity:          c.BurstCapacity,
		RateLimiterIdleTimeout: c.RateLimiterIdleTimeout,
		MaxIdleConns:           c.MaxIdleConns,
		MaxConnsPerHost:        c.MaxConnsPerHost,
		MaxIdleConnsPerHost:    c.MaxIdleConnsPerHost,
		IdleConnTimeout:        c.IdleConnTimeout,
		RetryMaxAttempts:       c.RetryMaxAttempts,
		RetryBaseDelay:         c.RetryBaseDelay,
		RetryMaxDelay:          c.RetryMaxDelay,
		RetryJitter:            c.RetryJitter,
		AllowPrivateIPs:        c.AllowPrivateIPs,
	}

	serverConfig := mcpserver.Config{
		Transport:          transport,
		HTTPPort:           c.HTTPPort,
		HTTPStateless:      c.HTTPStateless,
		HTTPSessionTimeout: c.HTTPSessionTimeout,
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
		feedStore, err := store.NewStore(&storeConfig)
		if err != nil {
			return err
		}
		serverConfig.AllFeedsGetter = feedStore
		serverConfig.FeedAndItemsGetter = feedStore
	}

	server, err := mcpserver.NewServer(&serverConfig)
	if err != nil {
		return err
	}
	return server.Run(ctx)
}
