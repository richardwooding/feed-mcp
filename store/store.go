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
	Feeds      []string
	HttpClient *http.Client
}

type feedItem struct {
	title string
	url   string
}

type Store struct {
	feeds            map[string]feedItem
	feedCacheManager *cache.LoadableCache[*gofeed.Feed]
}

func NewStore(config Config) (*Store, error) {

	if len(config.Feeds) == 0 {
		return nil, errors.New("at least one feedItem must be specified")
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
			feed, err := fp.ParseURLWithContext(url, ctx)
			if err != nil {
				return nil, nil, err
			}
			return feed, []store.Option{store.WithExpiration(5 * time.Minute)}, nil
		} else {
			return nil, nil, errors.New("invalid key type")
		}
	}

	cacheManager := cache.NewLoadable[*gofeed.Feed](
		loadFunction,
		cache.New[*gofeed.Feed](ristrettoStore),
	)

	wg := &sync.WaitGroup{}
	feeds := make(map[string]feedItem, len(config.Feeds))
	for _, feedURL := range config.Feeds {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			id, err := gonanoid.New()
			if err != nil {
				fmt.Printf("error generating id: %s\n", err)
				return
			}
			feed, err := cacheManager.Get(context.Background(), url)
			if err != nil {
				fmt.Printf("Failed to load feedItem %s: %v\n", url, err)
				return
			}
			feeds[id] = feedItem{feed.Title, url}
		}(feedURL)
	}

	return &Store{
		feeds:            feeds,
		feedCacheManager: cacheManager,
	}, nil
}

func (s *Store) GetAllFeeds(ctx context.Context) ([]*model.FeedResult, error) {
	results := make([]*model.FeedResult, len(s.feeds))
	idx := 0
	for id, item := range s.feeds {
		feed, err := s.feedCacheManager.Get(ctx, item.url)
		if err != nil {
			results[idx] = &model.FeedResult{
				ID:         id,
				PublicURL:  item.url,
				FetchError: err.Error(),
			}
		} else {
			results[idx] = &model.FeedResult{
				ID:        id,
				PublicURL: item.url,
				Title:     feed.Title,
				Feed:      model.FromGoFeed(feed),
			}
		}
		idx++
	}
	return results, nil
}

func (s *Store) GetFeedAndItems(ctx context.Context, id string) (*model.FeedAndItemsResult, error) {
	if item, exists := s.feeds[id]; exists {
		feed, err := s.feedCacheManager.Get(ctx, item.url)
		if err != nil {
			return &model.FeedAndItemsResult{
				ID:         id,
				PublicURL:  item.url,
				FetchError: err.Error(),
			}, nil
		}
		return &model.FeedAndItemsResult{
			ID:        id,
			PublicURL: item.url,
			Title:     feed.Title,
			Feed:      model.FromGoFeed(feed),
			Items:     feed.Items,
		}, nil
	}
	return nil, fmt.Errorf("feed with ID %s not found", id)
}
