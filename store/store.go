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

	"github.com/dgraph-io/ristretto"
	"github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/store"
	ristretto_store "github.com/eko/gocache/store/ristretto/v4"
	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/mmcdole/gofeed"
	"github.com/richardwooding/feed-mcp/model"
	"github.com/sony/gobreaker"
	"golang.org/x/time/rate"
)

// HTTPPoolConfig holds HTTP connection pool configuration
type HTTPPoolConfig struct {
	MaxIdleConns        int
	MaxConnsPerHost     int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration
}

// Config holds configuration settings for the feed store
type Config struct {
	Feeds                          []string
	Timeout                        time.Duration
	ExpireAfter                    time.Duration
	HttpClient                     *http.Client
	RequestsPerSecond              float64
	BurstCapacity                  int
	CircuitBreakerEnabled          *bool
	CircuitBreakerMaxRequests      uint32
	CircuitBreakerInterval         time.Duration
	CircuitBreakerTimeout          time.Duration
	CircuitBreakerFailureThreshold uint32
	// HTTP connection pooling settings
	MaxIdleConns        int
	MaxConnsPerHost     int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration
	// Retry mechanism settings
	RetryMaxAttempts int
	RetryBaseDelay   time.Duration
	RetryMaxDelay    time.Duration
	RetryJitter      bool
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
	circuitBreakers  map[string]*gobreaker.CircuitBreaker
	retryMetrics     *RetryMetrics
	metricsMutex     sync.RWMutex
}

// RateLimitedTransport wraps an http.RoundTripper with rate limiting
type RateLimitedTransport struct {
	transport   http.RoundTripper
	rateLimiter *rate.Limiter
}

// RoundTrip implements the http.RoundTripper interface with rate limiting
func (r *RateLimitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Wait for rate limiter permission
	err := r.rateLimiter.Wait(req.Context())
	if err != nil {
		return nil, err
	}

	// Proceed with the actual request
	return r.transport.RoundTrip(req)
}

// NewRateLimitedHTTPClient creates an HTTP client with rate limiting and connection pooling
func NewRateLimitedHTTPClient(requestsPerSecond float64, burstCapacity int, poolConfig HTTPPoolConfig) *http.Client {
	limiter := rate.NewLimiter(rate.Limit(requestsPerSecond), burstCapacity)

	// Create a custom transport with connection pooling settings
	baseTransport := &http.Transport{
		MaxIdleConns:        poolConfig.MaxIdleConns,
		MaxConnsPerHost:     poolConfig.MaxConnsPerHost,
		MaxIdleConnsPerHost: poolConfig.MaxIdleConnsPerHost,
		IdleConnTimeout:     poolConfig.IdleConnTimeout,
		// Copy other default settings from http.DefaultTransport
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	transport := &RateLimitedTransport{
		transport:   baseTransport,
		rateLimiter: limiter,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second, // Default timeout
	}
}

// isRetryableError determines if an error should trigger a retry
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Context cancellation and timeout errors are not retryable
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
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

// calculateRetryDelay calculates the delay for the next retry with exponential backoff and optional jitter
func calculateRetryDelay(attempt int, baseDelay, maxDelay time.Duration, useJitter bool) time.Duration {
	if attempt <= 0 {
		return baseDelay
	}

	// Exponential backoff: baseDelay * 2^(attempt-1)
	delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt-1)))

	// Cap at maxDelay
	if delay > maxDelay {
		delay = maxDelay
	}

	// Add jitter to avoid thundering herd
	if useJitter && delay > 0 {
		jitterRange := delay / 2
		var jitter time.Duration
		if jitterRange > 0 {
			jitter = time.Duration(rand.Int63n(int64(jitterRange)))
		} else {
			jitter = 0
		}
		delay = delay - jitterRange/2 + jitter
		// Ensure delay is never negative
		if delay < 0 {
			delay = 0
		}
	}

	return delay
}

// retryableFeedFetch performs feed fetching with retry logic and metrics tracking
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
			return feed, nil
		}

		lastErr = err

		// Don't retry on the last attempt or non-retryable errors
		if attempt >= maxAttempts || !isRetryableError(err) {
			break
		}

		// Calculate delay and sleep before next attempt
		delay := calculateRetryDelay(attempt, config.RetryBaseDelay, config.RetryMaxDelay, config.RetryJitter)

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

	return nil, lastErr
}

// NewStore creates a new feed store with the given configuration
func NewStore(config Config) (*Store, error) {

	if len(config.Feeds) == 0 {
		return nil, errors.New("at least one feedItem must be specified")
	}

	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	if config.ExpireAfter == 0 {
		config.ExpireAfter = 1 * time.Hour
	}

	// Set default rate limiting values
	if config.RequestsPerSecond <= 0 {
		config.RequestsPerSecond = 2.0 // 2 requests per second by default
	}

	if config.BurstCapacity <= 0 {
		config.BurstCapacity = 5 // Allow burst of 5 requests by default
	}

	// Set default circuit breaker values - enabled by default
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

	// Set default HTTP connection pool values
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

	// Set default retry values
	if config.RetryMaxAttempts <= 0 {
		config.RetryMaxAttempts = 3 // Default to 3 retry attempts
	}
	if config.RetryBaseDelay <= 0 {
		config.RetryBaseDelay = 1 * time.Second // Default to 1 second base delay
	}
	if config.RetryMaxDelay <= 0 {
		config.RetryMaxDelay = 30 * time.Second // Default to 30 seconds max delay
	}
	// RetryJitter defaults to true (handled by CLI flag default: "true")

	// Create rate-limited HTTP client with connection pooling if not provided
	if config.HttpClient == nil {
		poolConfig := HTTPPoolConfig{
			MaxIdleConns:        config.MaxIdleConns,
			MaxConnsPerHost:     config.MaxConnsPerHost,
			MaxIdleConnsPerHost: config.MaxIdleConnsPerHost,
			IdleConnTimeout:     config.IdleConnTimeout,
		}
		config.HttpClient = NewRateLimitedHTTPClient(config.RequestsPerSecond, config.BurstCapacity, poolConfig)
	}

	ristrettoCache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1000,
		MaxCost:     100,
		BufferItems: 64,
	})
	if err != nil {
		return nil, err
	}

	ristrettoStore := ristretto_store.NewRistretto(ristrettoCache)

	// Initialize circuit breakers map - enabled by default unless explicitly disabled
	var circuitBreakers map[string]*gobreaker.CircuitBreaker
	circuitBreakerEnabled := config.CircuitBreakerEnabled == nil || *config.CircuitBreakerEnabled

	if circuitBreakerEnabled {
		circuitBreakers = make(map[string]*gobreaker.CircuitBreaker)
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
	}

	// Create the store first
	s := &Store{
		feeds:           make(map[string]string, len(config.Feeds)),
		circuitBreakers: circuitBreakers,
		retryMetrics:    &RetryMetrics{},
		metricsMutex:    sync.RWMutex{},
	}

	loadFunction := func(ctx context.Context, key any) (*gofeed.Feed, []store.Option, error) {
		if url, ok := key.(string); ok {
			// Create parser with HTTP client
			fp := gofeed.NewParser()
			if config.HttpClient != nil {
				fp.Client = config.HttpClient
			}

			// Use circuit breaker if enabled
			if circuitBreakerEnabled {
				if cb, exists := circuitBreakers[url]; exists {
					result, err := cb.Execute(func() (interface{}, error) {
						return retryableFeedFetch(ctx, url, fp, config, s.retryMetrics, &s.metricsMutex)
					})
					if err != nil {
						return nil, nil, err
					}
					if feed, ok := result.(*gofeed.Feed); ok {
						return feed, []store.Option{store.WithExpiration(config.ExpireAfter)}, nil
					}
					return nil, nil, errors.New("unexpected result type from circuit breaker")
				}
			}

			// Fallback to direct retryable parsing if circuit breaker not enabled or URL not found
			feed, err := retryableFeedFetch(ctx, url, fp, config, s.retryMetrics, &s.metricsMutex)
			if err != nil {
				return nil, nil, err
			}
			return feed, []store.Option{store.WithExpiration(config.ExpireAfter)}, nil
		} else {
			return nil, nil, errors.New("invalid key type")
		}
	}

	cacheManager := cache.NewLoadable[*gofeed.Feed](
		loadFunction,
		cache.New[*gofeed.Feed](ristrettoStore),
	)
	s.feedCacheManager = cacheManager

	feeds := make(map[string]string, len(config.Feeds))
	var feedsMutex sync.Mutex
	wg := sync.WaitGroup{}
	for _, feedURL := range config.Feeds {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			id, _ := gonanoid.New()
			feedsMutex.Lock()
			feeds[id] = url
			feedsMutex.Unlock()
			_, _ = cacheManager.Get(context.Background(), url)

		}(feedURL)
	}
	wg.Wait()

	s.feeds = feeds
	return s, nil
}

// GetAllFeeds returns all configured feeds with their current status
func (s *Store) GetAllFeeds(ctx context.Context) ([]*model.FeedResult, error) {
	results := make([]*model.FeedResult, len(s.feeds))
	wg := &sync.WaitGroup{}
	idx := 0
	for id, url := range s.feeds {
		wg.Add(1)
		go func(idx int, id string, url string) {
			defer wg.Done()
			feed, err := s.feedCacheManager.Get(ctx, url)

			result := &model.FeedResult{
				ID:        id,
				PublicURL: url,
			}

			// Check circuit breaker state
			if s.circuitBreakers != nil {
				if cb, exists := s.circuitBreakers[url]; exists {
					result.CircuitBreakerOpen = cb.State() == gobreaker.StateOpen
				}
			}

			if err != nil {
				result.FetchError = err.Error()
			} else {
				result.Title = feed.Title
				result.Feed = model.FromGoFeed(feed)
			}

			results[idx] = result
		}(idx, id, url)
		idx++
	}
	wg.Wait()
	return results, nil
}

// GetFeedAndItems returns a specific feed with all its items
func (s *Store) GetFeedAndItems(ctx context.Context, id string) (*model.FeedAndItemsResult, error) {
	if url, exists := s.feeds[id]; exists {
		feed, err := s.feedCacheManager.Get(ctx, url)

		result := &model.FeedAndItemsResult{
			ID:        id,
			PublicURL: url,
		}

		// Check circuit breaker state
		if s.circuitBreakers != nil {
			if cb, exists := s.circuitBreakers[url]; exists {
				result.CircuitBreakerOpen = cb.State() == gobreaker.StateOpen
			}
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
	return nil, fmt.Errorf("feed with ID %s not found", id)
}

// GetRetryMetrics returns a copy of the current retry metrics
func (s *Store) GetRetryMetrics() RetryMetrics {
	s.metricsMutex.RLock()
	defer s.metricsMutex.RUnlock()
	return *s.retryMetrics
}
