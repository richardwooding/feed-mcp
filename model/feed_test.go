package model

import (
	"github.com/mmcdole/gofeed"
	"reflect"
	"testing"
)

func TestFromGoFeed(t *testing.T) {
	in := &gofeed.Feed{
		Title:       "Test Feed",
		Description: "desc",
		Link:        "http://example.com",
		FeedType:    "rss",
		FeedVersion: "2.0",
	}
	out := FromGoFeed(in)
	if out == nil {
		t.Fatal("FromGoFeed returned nil")
	}
	if out.Title != in.Title || out.Description != in.Description || out.Link != in.Link {
		t.Errorf("FromGoFeed did not copy fields correctly")
	}
	if out.FeedType != in.FeedType || out.FeedVersion != in.FeedVersion {
		t.Errorf("FromGoFeed did not copy FeedType/FeedVersion")
	}
	// Test nil input
	if FromGoFeed(nil) != nil {
		t.Errorf("FromGoFeed(nil) should return nil")
	}
	// Test all fields copied (shallow check)
	got := FromGoFeed(in)
	want := &Feed{
		Title:       "Test Feed",
		Description: "desc",
		Link:        "http://example.com",
		FeedType:    "rss",
		FeedVersion: "2.0",
	}
	if !reflect.DeepEqual(got.Title, want.Title) {
		t.Errorf("Title mismatch: got %v want %v", got.Title, want.Title)
	}
}
