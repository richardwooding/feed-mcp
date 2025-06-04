package model

import (
	"github.com/mmcdole/gofeed"
)

type FeedAndItemsResult struct {
	ID         string         `json:"id"`
	PublicURL  string         `json:"public_url"`
	Title      string         `json:"title,omitempty"`
	FetchError string         `json:"fetch_error,omitempty"`
	Feed       *Feed          `json:"feed_result,omitempty"`
	Items      []*gofeed.Item `json:"items,omitempty"`
}
