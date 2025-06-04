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
	"net/http"
	"sync"
	"time"
)

type Config struct {
	Feeds       []string
	Timeout     time.Duration
	ExpireAfter time.Duration
	HttpClient  *http.Client
}

type Store struct {
	feeds            map[string]string
	feedCacheManager *cache.LoadableCache[*gofeed.Feed]
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

	ristrettoCache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1000,
		MaxCost:     100,
		BufferItems: 64,
	})
	if err != nil {
		return nil, err
	}

	ristrettoStore := ristretto_store.NewRistretto(ristrettoCache)

	loadFunction := func(ctx context.Context, key any) (*gofeed.Feed, []store.Option, error) {
		if url, ok := key.(string); ok {
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
			if err != nil {
				results[idx] = &model.FeedResult{
					ID:         id,
					PublicURL:  url,
					FetchError: err.Error(),
				}
			} else {
				results[idx] = &model.FeedResult{
					ID:        id,
					PublicURL: url,
					Title:     feed.Title,
					Feed:      model.FromGoFeed(feed),
				}
			}
		}(idx, id, url)
		idx++
	}
	wg.Wait()
	return results, nil
}

func (s *Store) GetFeedAndItems(ctx context.Context, id string) (*model.FeedAndItemsResult, error) {
	if url, exists := s.feeds[id]; exists {
		feed, err := s.feedCacheManager.Get(ctx, url)
		if err != nil {
			return &model.FeedAndItemsResult{
				ID:         id,
				PublicURL:  url,
				FetchError: err.Error(),
			}, nil
		}
		return &model.FeedAndItemsResult{
			ID:        id,
			PublicURL: url,
			Title:     feed.Title,
			Feed:      model.FromGoFeed(feed),
			Items:     feed.Items,
		}, nil
	}
	return nil, fmt.Errorf("feed with ID %s not found", id)
}
