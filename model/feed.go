package model

import (
	"github.com/mmcdole/gofeed"
	ext "github.com/mmcdole/gofeed/extensions"
	"time"
)

// Feed represents a syndication feed (RSS, Atom, or JSON Feed)
type Feed struct {
	Title           string                   `json:"title,omitempty"`
	Description     string                   `json:"description,omitempty"`
	Link            string                   `json:"link,omitempty"`
	FeedLink        string                   `json:"feedLink,omitempty"`
	Links           []string                 `json:"links,omitempty"`
	Updated         string                   `json:"updated,omitempty"`
	UpdatedParsed   *time.Time               `json:"updatedParsed,omitempty"`
	Published       string                   `json:"published,omitempty"`
	PublishedParsed *time.Time               `json:"publishedParsed,omitempty"`
	Authors         []*gofeed.Person         `json:"authors,omitempty"`
	Language        string                   `json:"language,omitempty"`
	Image           *gofeed.Image            `json:"image,omitempty"`
	Copyright       string                   `json:"copyright,omitempty"`
	Generator       string                   `json:"generator,omitempty"`
	Categories      []string                 `json:"categories,omitempty"`
	DublinCoreExt   *ext.DublinCoreExtension `json:"dcExt,omitempty"`
	ITunesExt       *ext.ITunesFeedExtension `json:"itunesExt,omitempty"`
	Extensions      ext.Extensions           `json:"extensions,omitempty"`
	Custom          map[string]string        `json:"custom,omitempty"`
	FeedType        string                   `json:"feedType"`
	FeedVersion     string                   `json:"feedVersion"`
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
