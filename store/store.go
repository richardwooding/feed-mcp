// Package store implements feed management with caching, circuit breaking, and retry logic.
package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/store"
	ristretto_store "github.com/eko/gocache/store/ristretto/v4"
	"github.com/mmcdole/gofeed"
	"github.com/richardwooding/hostrate"
	"github.com/richardwooding/ssrfguard"
	"github.com/sony/gobreaker"
	"golang.org/x/time/rate"

	"github.com/richardwooding/feed-mcp/model"
)

// keyAttempt is the structured-log field key for the current retry attempt.
const keyAttempt = "attempt"

// HTTPPoolConfig holds HTTP connection pool configuration
type HTTPPoolConfig struct {
	MaxIdleConns        int
	MaxConnsPerHost     int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration
}

// Config holds configuration settings for the feed store
type Config struct {
	HTTPClient                     *http.Client
	CircuitBreakerEnabled          *bool
	Feeds                          []string
	CircuitBreakerInterval         time.Duration
	RetryBaseDelay                 time.Duration
	BurstCapacity                  int
	ExpireAfter                    time.Duration
	RequestsPerSecond              float64
	RateLimiterIdleTimeout         time.Duration // Evict a host's rate limiter after this idle period. Zero means "use the default" (1h); a negative value disables eviction.
	Timeout                        time.Duration
	CircuitBreakerTimeout          time.Duration
	RetryMaxDelay                  time.Duration
	MaxIdleConns                   int
	MaxConnsPerHost                int
	MaxIdleConnsPerHost            int
	IdleConnTimeout                time.Duration
	RetryMaxAttempts               int
	CircuitBreakerMaxRequests      uint32
	CircuitBreakerFailureThreshold uint32
	RetryJitter                    bool
	OPML                           string // OPML file path for metadata source detection
	AllowPrivateIPs                bool   // Allow private IP addresses in URLs
	AllowEmptyFeeds                bool   // Allow creating store with no initial feeds (used by DynamicStore)
}

// RetryMetrics holds metrics for retry operations
type RetryMetrics struct {
	TotalAttempts    int64   // Total number of HTTP attempts made
	TotalRetries     int64   // Total number of retries (excluding initial attempts)
	SuccessfulFeeds  int64   // Number of feeds successfully fetched
	FailedFeeds      int64   // Number of feeds that failed after all retries
	RetrySuccessRate float64 // Percentage of feeds that succeeded after retrying
}

// Store manages feed fetching, caching, and retrieval with retry logic
type Store struct {
	feeds            map[string]string
	feedCacheManager *cache.LoadableCache[*gofeed.Feed]
	feedCache        *cache.Cache[*gofeed.Feed]
	circuitBreakers  map[string]*gobreaker.CircuitBreaker
	retryMetrics     *RetryMetrics
	metricsMutex     sync.RWMutex
	// feedsMu guards the feeds and circuitBreakers maps. The base Store only
	// reads them after construction, but DynamicStore mutates them at runtime
	// (add_feed / remove_feed) concurrently with reads here, so every access to
	// either map — base or dynamic — must hold this lock. It is held only around
	// the map operations themselves, never across a network fetch.
	feedsMu sync.RWMutex
}

// feedEntry pairs a feed's ID with its URL for snapshotting the feeds map.
type feedEntry struct {
	id  string
	url string
}

// feedEntries returns a snapshot of the configured feeds, taken under the read
// lock so iteration can proceed without holding the lock across network fetches.
func (s *Store) feedEntries() []feedEntry {
	s.feedsMu.RLock()
	defer s.feedsMu.RUnlock()
	entries := make([]feedEntry, 0, len(s.feeds))
	for id, url := range s.feeds {
		entries = append(entries, feedEntry{id: id, url: url})
	}
	return entries
}

// feedURL returns the URL for a feed ID under the read lock.
func (s *Store) feedURL(id string) (string, bool) {
	s.feedsMu.RLock()
	defer s.feedsMu.RUnlock()
	url, ok := s.feeds[id]
	return url, ok
}

// cachedItemCount returns the item count for a feed if it is already in the
// cache, without triggering the loadable cache's network fetch. It returns 0 on
// a cache miss — used where a fresh fetch would be wasteful, e.g. removing a
// feed (which may be offline) just to report how many items it had.
func (s *Store) cachedItemCount(ctx context.Context, url string) int {
	if s.feedCache == nil {
		return 0
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if feed, err := s.feedCache.Get(ctx, url); err == nil && feed != nil {
		return len(feed.Items)
	}
	return 0
}

// urlRegistered reports whether a feed already uses the given URL, under the
// read lock. Feed IDs are GenerateFeedID(url), so this is an O(1) lookup; the
// value comparison guards against a hash collision mapping a different URL to
// the same ID.
func (s *Store) urlRegistered(url string) bool {
	id := model.GenerateFeedID(url)
	s.feedsMu.RLock()
	defer s.feedsMu.RUnlock()
	existing, ok := s.feeds[id]
	return ok && existing == url
}

// hasCircuitBreakers reports whether circuit breakers are configured.
func (s *Store) hasCircuitBreakers() bool {
	s.feedsMu.RLock()
	defer s.feedsMu.RUnlock()
	return s.circuitBreakers != nil
}

// circuitBreaker returns the circuit breaker for a URL under the read lock.
func (s *Store) circuitBreaker(url string) (*gobreaker.CircuitBreaker, bool) {
	s.feedsMu.RLock()
	defer s.feedsMu.RUnlock()
	if s.circuitBreakers == nil {
		return nil, false
	}
	cb, ok := s.circuitBreakers[url]
	return cb, ok
}

// putFeed registers a feed (and, when configured, its circuit breaker) under the
// write lock.
func (s *Store) putFeed(id, url string, cb *gobreaker.CircuitBreaker) {
	s.feedsMu.Lock()
	defer s.feedsMu.Unlock()
	s.feeds[id] = url
	if cb != nil && s.circuitBreakers != nil {
		s.circuitBreakers[url] = cb
	}
}

// deleteFeed removes a feed and its circuit breaker under the write lock.
func (s *Store) deleteFeed(id, url string) {
	s.feedsMu.Lock()
	defer s.feedsMu.Unlock()
	delete(s.feeds, id)
	if s.circuitBreakers != nil {
		delete(s.circuitBreakers, url)
	}
}

// newPooledTransport builds an *http.Transport with the given connection pool
// settings, otherwise mirroring http.DefaultTransport's defaults.
//
// The dialer carries an ssrfguard Control hook so SSRF protection runs at dial
// time, after DNS resolution: it inspects the IP actually being connected to and
// blocks internal addresses. This is the backstop against DNS rebinding, where a
// host passes up-front model.ValidateFeedURL as public but later resolves to an
// internal address. When allowPrivateIPs is set, internal ranges are permitted.
func newPooledTransport(poolConfig HTTPPoolConfig, allowPrivateIPs bool) *http.Transport {
	guard := ssrfguard.New(ssrfguard.WithAllowPrivate(allowPrivateIPs))
	return &http.Transport{
		MaxIdleConns:        poolConfig.MaxIdleConns,
		MaxConnsPerHost:     poolConfig.MaxConnsPerHost,
		MaxIdleConnsPerHost: poolConfig.MaxIdleConnsPerHost,
		IdleConnTimeout:     poolConfig.IdleConnTimeout,
		// Copy other default settings from http.DefaultTransport
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			Control:   guard.Control,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

// NewRateLimitedHTTPClient creates an HTTP client with per-host rate limiting and connection pooling.
// The requestsPerSecond and burstCapacity arguments configure each host's token bucket independently.
// Per-host rate limiting is provided by github.com/richardwooding/hostrate. When allowPrivateIPs is
// false, the transport blocks connections to internal addresses at dial time (see newPooledTransport).
//
// The optional idleTimeout bounds the per-host limiter map: a host's limiter is
// evicted after it has been idle for that long, so runtime feed churn can't grow
// the map without bound (#117). Only the first value is used; a non-positive
// value (or omitting it) disables eviction, retaining one limiter per host for
// the client's lifetime — fine when the host set is small and fixed. It is
// variadic so existing callers that don't configure eviction keep compiling.
func NewRateLimitedHTTPClient(requestsPerSecond float64, burstCapacity int, poolConfig HTTPPoolConfig, allowPrivateIPs bool, idleTimeout ...time.Duration) *http.Client {
	var opts []hostrate.Option
	if len(idleTimeout) > 0 && idleTimeout[0] > 0 {
		// Only enable eviction for a positive timeout; a non-positive value means
		// "no eviction", which is hostrate's default when the option is absent.
		opts = append(opts, hostrate.WithIdleTimeout(idleTimeout[0]))
	}
	transport := hostrate.New(
		newPooledTransport(poolConfig, allowPrivateIPs),
		rate.Limit(requestsPerSecond),
		burstCapacity,
		opts...,
	)

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second, // Default timeout
	}
}

// isRetryableError determines if an error should trigger a retry attempt.
// Returns true for network errors (DNS, connection, timeout) and 5xx HTTP status codes.
// Returns false for context cancellation, 4xx client errors, and other non-transient failures.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Context cancellation and timeout errors are not retryable
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// A dial-time SSRF block is deterministic: the destination resolves to a
	// blocked address and will on every retry. Retrying only adds backoff delay
	// and can trip the circuit breaker, so treat it as non-retryable.
	if errors.Is(err, ssrfguard.ErrBlockedAddress) {
		return false
	}

	// DNS and network errors are retryable
	if strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "network unreachable") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "i/o timeout") {
		return true
	}

	// HTTP status errors (gofeed uses "http error: XXX" format)
	if strings.Contains(errStr, "http error: 5") || strings.Contains(errStr, "status code 5") {
		return true // 5xx server errors are retryable
	}

	if strings.Contains(errStr, "http error: 4") || strings.Contains(errStr, "status code 4") {
		return false // 4xx client errors are not retryable
	}

	// Default to retryable for unknown network-related errors
	return true
}

// calculateRetryDelay calculates the delay for the next retry using exponential backoff.
// Uses formula: baseDelay * 2^(attempt-1), capped at maxDelay.
// Applies jitter (±50% random variance) when useJitter is true to prevent thundering herd.
func calculateRetryDelay(attempt int, baseDelay, maxDelay time.Duration, useJitter bool) time.Duration {
	if attempt <= 0 {
		return baseDelay
	}

	// Exponential backoff: baseDelay * 2^(attempt-1)
	delay := min(
		// Cap at maxDelay
		time.Duration(float64(baseDelay)*math.Pow(2, float64(attempt-1))), maxDelay)

	// Add jitter to avoid thundering herd
	if useJitter && delay > 0 {
		jitterRange := delay / 2
		var jitter time.Duration
		if jitterRange > 0 {
			jitter = time.Duration(rand.Int63n(int64(jitterRange)))
		} else {
			jitter = 0
		}
		delay = max(
			// Ensure delay is never negative
			delay-jitterRange/2+jitter, 0)
	}

	return delay
}

// retryableFeedFetch performs feed fetching with retry logic and comprehensive metrics tracking.
// Attempts up to maxAttempts times for retryable errors, with exponential backoff delays.
// Updates retry metrics and integrates with circuit breaker patterns for fault tolerance.
//
//nolint:gocognit,gocyclo,gocritic // Function complexity is necessary for comprehensive retry logic with metrics and error handling
func retryableFeedFetch(ctx context.Context, url string, parser *gofeed.Parser, config Config, metrics *RetryMetrics, metricsMutex *sync.RWMutex) (*gofeed.Feed, error) {
	var lastErr error
	maxAttempts := config.RetryMaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1 // At least one attempt
	}

	attemptCount := 0

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		attemptCount++

		// Track total attempts
		if metrics != nil && metricsMutex != nil {
			metricsMutex.Lock()
			metrics.TotalAttempts++
			if attempt > 1 {
				metrics.TotalRetries++
			}
			metricsMutex.Unlock()
		}

		// Create timeout context for this attempt
		attemptCtx, cancel := context.WithTimeout(ctx, config.Timeout)

		feed, err := parser.ParseURLWithContext(url, attemptCtx)
		cancel()

		// Success case
		if err == nil {
			// Track successful feed
			if metrics != nil && metricsMutex != nil {
				metricsMutex.Lock()
				metrics.SuccessfulFeeds++
				// Update success rate
				totalFeeds := metrics.SuccessfulFeeds + metrics.FailedFeeds
				if totalFeeds > 0 {
					metrics.RetrySuccessRate = float64(metrics.SuccessfulFeeds) / float64(totalFeeds) * 100
				}
				metricsMutex.Unlock()
			}

			// Debug log successful fetch
			extra := map[string]any{
				"items_count": len(feed.Items),
			}
			msg := "Successfully fetched feed"
			if attempt > 1 {
				extra[keyAttempt] = attempt
				extra["max_attempts"] = maxAttempts
				msg = fmt.Sprintf("Successfully fetched feed after %d attempts", attempt)
			}
			model.DebugLogWithContext(
				msg,
				"feed_fetcher", "retryable_fetch", url,
				extra,
			)

			return feed, nil
		}

		lastErr = err

		// Debug log the error
		model.DebugLogWithContext(
			fmt.Sprintf("Feed fetch attempt %d failed", attempt),
			"feed_fetcher", "retryable_fetch", url,
			map[string]any{
				keyAttempt:     attempt,
				"max_attempts": maxAttempts,
				statusError:    err.Error(),
				"retryable":    isRetryableError(err),
			},
		)

		// Don't retry on the last attempt or non-retryable errors
		if attempt >= maxAttempts || !isRetryableError(err) {
			if !isRetryableError(err) {
				model.DebugLogWithContext(
					"Error is not retryable, stopping retry attempts",
					"feed_fetcher", "retryable_fetch", url,
					map[string]any{
						keyAttempt:  attempt,
						statusError: err.Error(),
					},
				)
			}
			break
		}

		// Calculate delay and sleep before next attempt
		delay := calculateRetryDelay(attempt, config.RetryBaseDelay, config.RetryMaxDelay, config.RetryJitter)

		model.DebugLogWithContext(
			fmt.Sprintf("Retrying in %v", delay),
			"feed_fetcher", "retryable_fetch", url,
			map[string]any{
				keyAttempt:     attempt,
				"next_attempt": attempt + 1,
				"delay_ms":     delay.Milliseconds(),
			},
		)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	// Track failed feed
	if metrics != nil && metricsMutex != nil {
		metricsMutex.Lock()
		metrics.FailedFeeds++
		// Update success rate
		totalFeeds := metrics.SuccessfulFeeds + metrics.FailedFeeds
		if totalFeeds > 0 {
			metrics.RetrySuccessRate = float64(metrics.SuccessfulFeeds) / float64(totalFeeds) * 100
		}
		metricsMutex.Unlock()
	}

	// Create a comprehensive error with retry context
	return nil, model.CreateRetryError(lastErr, url, attemptCount, maxAttempts)
}

// NewStore creates a new feed store with the given configuration.
// Uses pointer to avoid copying large Config struct (192 bytes).
func NewStore(config *Config) (*Store, error) {
	if len(config.Feeds) == 0 && !config.AllowEmptyFeeds {
		return nil, model.NewFeedError(model.ErrorTypeConfiguration, "at least one feed must be specified").
			WithOperation("create_store").
			WithComponent("store_manager")
	}

	return newStoreInternal(*config)
}

// newStoreInternal contains the core store initialization logic.
//
//nolint:gocritic // takes Config by value to apply defaults to a local mutable copy
func newStoreInternal(config Config) (*Store, error) {
	applyConfigDefaults(&config)

	// Create rate-limited HTTP client with connection pooling if not provided
	if config.HTTPClient == nil {
		poolConfig := HTTPPoolConfig{
			MaxIdleConns:        config.MaxIdleConns,
			MaxConnsPerHost:     config.MaxConnsPerHost,
			MaxIdleConnsPerHost: config.MaxIdleConnsPerHost,
			IdleConnTimeout:     config.IdleConnTimeout,
		}
		config.HTTPClient = NewRateLimitedHTTPClient(config.RequestsPerSecond, config.BurstCapacity, poolConfig, config.AllowPrivateIPs, config.RateLimiterIdleTimeout)
	}

	ristrettoCache, err := ristretto.NewCache[string, *gofeed.Feed](&ristretto.Config[string, *gofeed.Feed]{
		NumCounters: 1000,
		MaxCost:     100,
		BufferItems: 64,
	})
	if err != nil {
		return nil, err
	}

	ristrettoStore := ristretto_store.NewRistretto(ristrettoCache)

	// Circuit breakers are enabled by default unless explicitly disabled.
	circuitBreakerEnabled := config.CircuitBreakerEnabled == nil || *config.CircuitBreakerEnabled
	circuitBreakers := buildCircuitBreakers(&config, circuitBreakerEnabled)

	s := &Store{
		feeds:           make(map[string]string, len(config.Feeds)),
		circuitBreakers: circuitBreakers,
		retryMetrics:    &RetryMetrics{},
		metricsMutex:    sync.RWMutex{},
	}

	// Keep a reference to the inner (non-loadable) cache so callers can peek it
	// without triggering the loader's network fetch — see cachedItemCount.
	s.feedCache = cache.New[*gofeed.Feed](ristrettoStore)
	s.feedCacheManager = cache.NewLoadable[*gofeed.Feed](
		s.makeFeedLoader(&config, circuitBreakerEnabled),
		s.feedCache,
	)

	// Build the ID-to-URL map synchronously without fetching. The cache populates
	// lazily on the first GetAllFeeds / GetFeedAndItems call via the LoadableCache
	// loader above. Pre-fetching here previously blocked NewStore for ~(n/rps)
	// seconds with a global rate limiter and caused MCP initialize timeouts on
	// large feed lists (issue #114).
	feeds := make(map[string]string, len(config.Feeds))
	for _, feedURL := range config.Feeds {
		feeds[model.GenerateFeedID(feedURL)] = feedURL
	}

	s.feeds = feeds
	return s, nil
}

// applyConfigDefaults fills in zero-valued configuration fields with their defaults.
func applyConfigDefaults(config *Config) {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.ExpireAfter == 0 {
		config.ExpireAfter = 1 * time.Hour
	}

	// Rate limiting
	if config.RequestsPerSecond <= 0 {
		config.RequestsPerSecond = 2.0 // 2 requests per second by default
	}
	if config.BurstCapacity <= 0 {
		config.BurstCapacity = 5 // Allow burst of 5 requests by default
	}
	if config.RateLimiterIdleTimeout == 0 {
		// Evict a host's limiter after an hour idle so a long-running store with
		// runtime feed churn (add_feed/remove_feed across many hosts) can't grow
		// the per-host limiter map without bound (#117). A negative value
		// disables eviction; the zero value means "unset", hence the default.
		config.RateLimiterIdleTimeout = 1 * time.Hour
	}

	applyCircuitBreakerDefaults(config)
	applyHTTPPoolDefaults(config)
	applyRetryDefaults(config)
}

// applyCircuitBreakerDefaults sets circuit breaker defaults (enabled by default).
func applyCircuitBreakerDefaults(config *Config) {
	if config.CircuitBreakerMaxRequests <= 0 {
		config.CircuitBreakerMaxRequests = 3 // Allow 3 half-open requests
	}
	if config.CircuitBreakerInterval <= 0 {
		config.CircuitBreakerInterval = 60 * time.Second // Check for recovery every 60s
	}
	if config.CircuitBreakerTimeout <= 0 {
		config.CircuitBreakerTimeout = 30 * time.Second // Open circuit for 30s before trying half-open
	}
	if config.CircuitBreakerFailureThreshold <= 0 {
		config.CircuitBreakerFailureThreshold = 3 // Open circuit after 3 consecutive failures
	}
}

// applyHTTPPoolDefaults sets HTTP connection pool defaults.
func applyHTTPPoolDefaults(config *Config) {
	if config.MaxIdleConns <= 0 {
		config.MaxIdleConns = 100 // Default to 100 idle connections total
	}
	if config.MaxConnsPerHost <= 0 {
		config.MaxConnsPerHost = 10 // Default to 10 connections per host
	}
	if config.MaxIdleConnsPerHost <= 0 {
		config.MaxIdleConnsPerHost = 5 // Default to 5 idle connections per host
	}
	if config.IdleConnTimeout <= 0 {
		config.IdleConnTimeout = 90 * time.Second // Default to 90 seconds idle timeout
	}
}

// applyRetryDefaults sets retry defaults. RetryJitter defaults to true (handled
// by the CLI flag default: "true").
func applyRetryDefaults(config *Config) {
	if config.RetryMaxAttempts <= 0 {
		config.RetryMaxAttempts = 3 // Default to 3 retry attempts
	}
	if config.RetryBaseDelay <= 0 {
		config.RetryBaseDelay = 1 * time.Second // Default to 1 second base delay
	}
	if config.RetryMaxDelay <= 0 {
		config.RetryMaxDelay = 30 * time.Second // Default to 30 seconds max delay
	}
}

// buildCircuitBreakers creates one circuit breaker per configured feed URL.
// Returns nil when circuit breaking is disabled.
func buildCircuitBreakers(config *Config, enabled bool) map[string]*gobreaker.CircuitBreaker {
	if !enabled {
		return nil
	}

	circuitBreakers := make(map[string]*gobreaker.CircuitBreaker, len(config.Feeds))
	for _, feedURL := range config.Feeds {
		settings := gobreaker.Settings{
			Name:        fmt.Sprintf("feed-%s", feedURL),
			MaxRequests: config.CircuitBreakerMaxRequests,
			Interval:    config.CircuitBreakerInterval,
			Timeout:     config.CircuitBreakerTimeout,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures >= config.CircuitBreakerFailureThreshold
			},
		}
		circuitBreakers[feedURL] = gobreaker.NewCircuitBreaker(settings)
	}
	return circuitBreakers
}

// makeFeedLoader returns the LoadableCache loader that fetches and parses a feed
// on demand, optionally guarded by a per-feed circuit breaker.
func (s *Store) makeFeedLoader(
	config *Config,
	circuitBreakerEnabled bool,
) func(ctx context.Context, key any) (*gofeed.Feed, []store.Option, error) {
	return func(ctx context.Context, key any) (*gofeed.Feed, []store.Option, error) {
		url, ok := key.(string)
		if !ok {
			return nil, nil, model.NewFeedError(model.ErrorTypeSystem, "invalid key type for cache loader").
				WithOperation("load_feed").
				WithComponent("cache_manager")
		}

		// Create parser with HTTP client
		fp := gofeed.NewParser()
		if config.HTTPClient != nil {
			fp.Client = config.HTTPClient
		}

		opts := []store.Option{store.WithExpiration(config.ExpireAfter)}

		// Use circuit breaker if enabled and configured for this URL.
		if circuitBreakerEnabled {
			if cb, exists := s.circuitBreaker(url); exists {
				feed, err := s.fetchWithCircuitBreaker(ctx, url, fp, config, cb)
				if err != nil {
					return nil, nil, err
				}
				return feed, opts, nil
			}
		}

		// Fallback to direct retryable parsing if circuit breaker not enabled or URL not found
		feed, err := retryableFeedFetch(ctx, url, fp, *config, s.retryMetrics, &s.metricsMutex)
		if err != nil {
			return nil, nil, err
		}
		return feed, opts, nil
	}
}

// fetchWithCircuitBreaker executes a retryable feed fetch through the given circuit
// breaker, translating breaker-state errors into structured FeedErrors.
func (s *Store) fetchWithCircuitBreaker(
	ctx context.Context,
	url string,
	fp *gofeed.Parser,
	config *Config,
	cb *gobreaker.CircuitBreaker,
) (*gofeed.Feed, error) {
	result, err := cb.Execute(func() (any, error) {
		return retryableFeedFetch(ctx, url, fp, *config, s.retryMetrics, &s.metricsMutex)
	})
	if err != nil {
		// Check if this is a circuit breaker error
		if errors.Is(err, gobreaker.ErrOpenState) {
			return nil, model.CreateCircuitBreakerError(url, "open")
		}
		if errors.Is(err, gobreaker.ErrTooManyRequests) {
			return nil, model.CreateCircuitBreakerError(url, "half-open")
		}
		// Return the original error (likely from retryableFeedFetch)
		return nil, err
	}
	if feed, ok := result.(*gofeed.Feed); ok {
		return feed, nil
	}
	return nil, model.NewFeedError(model.ErrorTypeSystem, "unexpected result type from circuit breaker").
		WithURL(url).
		WithOperation("load_feed").
		WithComponent("circuit_breaker")
}

// GetAllFeeds returns all configured feeds with their current status
func (s *Store) GetAllFeeds(ctx context.Context) ([]*model.FeedResult, error) {
	// Snapshot the feeds under the read lock so the fetches below don't hold it.
	entries := s.feedEntries()
	results := make([]*model.FeedResult, len(entries))
	wg := &sync.WaitGroup{}
	for idx, entry := range entries {
		wg.Add(1)
		go func(idx int, id string, url string) {
			defer wg.Done()
			feed, err := s.feedCacheManager.Get(ctx, url)

			result := &model.FeedResult{
				ID:        id,
				PublicURL: url,
			}

			// Check circuit breaker state
			if cb, exists := s.circuitBreaker(url); exists {
				result.CircuitBreakerOpen = cb.State() == gobreaker.StateOpen
			}

			if err != nil {
				result.FetchError = err.Error()
			} else {
				result.Title = feed.Title
				result.Feed = model.FromGoFeed(feed)
			}

			results[idx] = result
		}(idx, entry.id, entry.url)
	}
	wg.Wait()
	return results, nil
}

// GetFeedAndItems returns a specific feed with all its items
func (s *Store) GetFeedAndItems(ctx context.Context, id string) (*model.FeedAndItemsResult, error) {
	if url, exists := s.feedURL(id); exists {
		feed, err := s.feedCacheManager.Get(ctx, url)

		result := &model.FeedAndItemsResult{
			ID:        id,
			PublicURL: url,
		}

		// Check circuit breaker state
		if cb, exists := s.circuitBreaker(url); exists {
			result.CircuitBreakerOpen = cb.State() == gobreaker.StateOpen
		}

		if err != nil {
			result.FetchError = err.Error()
			return result, nil
		}

		result.Title = feed.Title
		result.Feed = model.FromGoFeed(feed)
		result.Items = feed.Items

		return result, nil
	}
	return nil, model.NewFeedError(model.ErrorTypeValidation, fmt.Sprintf("feed with ID %s not found", id)).
		WithOperation("get_feed_and_items").
		WithComponent("feed_store")
}

// GetRetryMetrics returns a copy of the current retry metrics
func (s *Store) GetRetryMetrics() RetryMetrics {
	s.metricsMutex.RLock()
	defer s.metricsMutex.RUnlock()
	return *s.retryMetrics
}
