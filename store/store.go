package store

import (
	"context"
	"errors"
	"fmt"
	"github.com/dgraph-io/ristretto"
	"github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/store"
	ristretto_store "github.com/eko/gocache/store/ristretto/v4"
	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/mmcdole/gofeed"
	"github.com/richardwooding/feed-mcp/model"
	"github.com/sony/gobreaker"
	"golang.org/x/time/rate"
	"net/http"
	"sync"
	"time"
)

type Config struct {
	Feeds                        []string
	Timeout                      time.Duration
	ExpireAfter                  time.Duration
	HttpClient                   *http.Client
	RequestsPerSecond            float64
	BurstCapacity                int
	CircuitBreakerEnabled        *bool
	CircuitBreakerMaxRequests    uint32
	CircuitBreakerInterval       time.Duration
	CircuitBreakerTimeout        time.Duration
	CircuitBreakerFailureThreshold uint32
}

type Store struct {
	feeds            map[string]string
	feedCacheManager *cache.LoadableCache[*gofeed.Feed]
	circuitBreakers  map[string]*gobreaker.CircuitBreaker
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

// NewRateLimitedHTTPClient creates an HTTP client with rate limiting
func NewRateLimitedHTTPClient(requestsPerSecond float64, burstCapacity int) *http.Client {
	limiter := rate.NewLimiter(rate.Limit(requestsPerSecond), burstCapacity)

	transport := &RateLimitedTransport{
		transport:   http.DefaultTransport,
		rateLimiter: limiter,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second, // Default timeout
	}
}

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

	// Create rate-limited HTTP client if not provided
	if config.HttpClient == nil {
		config.HttpClient = NewRateLimitedHTTPClient(config.RequestsPerSecond, config.BurstCapacity)
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

	loadFunction := func(ctx context.Context, key any) (*gofeed.Feed, []store.Option, error) {
		if url, ok := key.(string); ok {
			// Use circuit breaker if enabled
			if circuitBreakerEnabled {
				if cb, exists := circuitBreakers[url]; exists {
					result, err := cb.Execute(func() (interface{}, error) {
						fp := gofeed.NewParser()
						if config.HttpClient != nil {
							fp.Client = config.HttpClient
						}
						expireContext, cancel := context.WithTimeout(ctx, config.Timeout)
						defer cancel()
						return fp.ParseURLWithContext(url, expireContext)
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

			// Fallback to direct parsing if circuit breaker not enabled or URL not found
			fp := gofeed.NewParser()
			if config.HttpClient != nil {
				fp.Client = config.HttpClient
			}
			expireContext, cancel := context.WithTimeout(ctx, config.Timeout)
			defer cancel()
			feed, err := fp.ParseURLWithContext(url, expireContext)
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

	feeds := make(map[string]string, len(config.Feeds))
	wg := sync.WaitGroup{}
	for _, feedURL := range config.Feeds {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			id, _ := gonanoid.New()
			feeds[id] = url
			_, _ = cacheManager.Get(context.Background(), url)

		}(feedURL)
	}
	wg.Wait()

	return &Store{
		feeds:            feeds,
		feedCacheManager: cacheManager,
		circuitBreakers:  circuitBreakers,
	}, nil
}

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
