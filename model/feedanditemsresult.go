package model

import (
	"github.com/mmcdole/gofeed"
)

// FeedAndItemsResult represents a feed along with its items
type FeedAndItemsResult struct {
	ID                 string         `json:"id"`
	PublicURL          string         `json:"public_url"`
	Title              string         `json:"title,omitempty"`
	FetchError         string         `json:"fetch_error,omitempty"`
	Feed               *Feed          `json:"feed_result,omitempty"`
	Items              []*gofeed.Item `json:"items,omitempty"`
	CircuitBreakerOpen bool           `json:"circuit_breaker_open,omitempty"`
}

// FeedMetadata represents feed metadata without items
type FeedMetadata struct {
	ID                 string `json:"id"`
	PublicURL          string `json:"public_url"`
	Title              string `json:"title,omitempty"`
	FetchError         string `json:"fetch_error,omitempty"`
	Feed               *Feed  `json:"feed_result,omitempty"`
	CircuitBreakerOpen bool   `json:"circuit_breaker_open,omitempty"`
}

// ToMetadata returns the feed metadata without items
func (f *FeedAndItemsResult) ToMetadata() *FeedMetadata {
	return &FeedMetadata{
		ID:                 f.ID,
		PublicURL:          f.PublicURL,
		Title:              f.Title,
		FetchError:         f.FetchError,
		Feed:               f.Feed,
		CircuitBreakerOpen: f.CircuitBreakerOpen,
	}
}
