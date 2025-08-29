// Package model provides data structures and types for the feed-mcp server.
package model

import (
	"time"

	"github.com/mmcdole/gofeed"
	ext "github.com/mmcdole/gofeed/extensions"
)

// Feed represents a syndication feed (RSS, Atom, or JSON Feed)
type Feed struct {
	PublishedParsed *time.Time               `json:"publishedParsed,omitempty"`
	Custom          map[string]string        `json:"custom,omitempty"`
	Extensions      ext.Extensions           `json:"extensions,omitempty"`
	ITunesExt       *ext.ITunesFeedExtension `json:"itunesExt,omitempty"`
	DublinCoreExt   *ext.DublinCoreExtension `json:"dcExt,omitempty"`
	Image           *gofeed.Image            `json:"image,omitempty"`
	UpdatedParsed   *time.Time               `json:"updatedParsed,omitempty"`
	Updated         string                   `json:"updated,omitempty"`
	Link            string                   `json:"link,omitempty"`
	FeedVersion     string                   `json:"feedVersion"`
	Language        string                   `json:"language,omitempty"`
	Title           string                   `json:"title,omitempty"`
	Copyright       string                   `json:"copyright,omitempty"`
	Generator       string                   `json:"generator,omitempty"`
	FeedType        string                   `json:"feedType"`
	Description     string                   `json:"description,omitempty"`
	FeedLink        string                   `json:"feedLink,omitempty"`
	Published       string                   `json:"published,omitempty"`
	Links           []string                 `json:"links,omitempty"`
	Categories      []string                 `json:"categories,omitempty"`
	Authors         []*gofeed.Person         `json:"authors,omitempty"`
}

// FromGoFeed converts a gofeed.Feed to our internal Feed representation
func FromGoFeed(inFeed *gofeed.Feed) *Feed {
	if inFeed == nil {
		return nil
	}

	return &Feed{
		Title:           inFeed.Title,
		Description:     inFeed.Description,
		Link:            inFeed.Link,
		FeedLink:        inFeed.FeedLink,
		Links:           inFeed.Links,
		Updated:         inFeed.Updated,
		UpdatedParsed:   inFeed.UpdatedParsed,
		Published:       inFeed.Published,
		PublishedParsed: inFeed.PublishedParsed,
		Authors:         inFeed.Authors,
		Language:        inFeed.Language,
		Image:           inFeed.Image,
		Copyright:       inFeed.Copyright,
		Generator:       inFeed.Generator,
		Categories:      inFeed.Categories,
		DublinCoreExt:   inFeed.DublinCoreExt,
		ITunesExt:       inFeed.ITunesExt,
		Extensions:      inFeed.Extensions,
		Custom:          inFeed.Custom,
		FeedType:        inFeed.FeedType,
		FeedVersion:     inFeed.FeedVersion,
	}
}
