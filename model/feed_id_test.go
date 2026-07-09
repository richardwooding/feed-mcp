package model

import (
	"testing"
)

func TestGenerateFeedID(t *testing.T) {
	testCases := []struct {
		name     string
		feedURL  string
		expected string
	}{
		{
			name:     "BBC News Africa RSS",
			feedURL:  "https://feeds.bbci.co.uk/news/world/africa/rss.xml",
			expected: "feeds.bbci.co.uk-news-world-afri-377c6c95", // Truncated at 40 chars with hash
		},
		{
			name:     "Simple domain",
			feedURL:  "https://example.com/feed.xml",
			expected: "example.com-feed-xml",
		},
		{
			name:     "Domain only",
			feedURL:  "https://example.com",
			expected: "example.com",
		},
		{
			name:     "Long URL should be truncated with hash",
			feedURL:  "https://very-long-domain-name-example.com/very/long/path/with/many/segments/that/exceeds/forty/characters/feed.xml",
			expected: "very-long-domain-name-example.co-babf4c88", // Should be truncated to 32 chars + 8 char hash
		},
		{
			name:     "URL with special characters",
			feedURL:  "https://example.com/news & events/feed.xml",
			expected: "example.com-news-events-feed-xml", // Spaces and & replaced with single dash
		},
		{
			// Regression test for #113: FNV-32a hash with a leading zero nibble
			// (Sum32 < 0x10000000) would render as < 8 hex chars under %x and
			// panic when sliced to [:8]. Zero-padding via %08x prevents this.
			name:     "Long URL with leading-zero hash",
			feedURL:  "https://very-long-domain-name-example.com/p22/feed.xml",
			expected: "very-long-domain-name-example.co-001353e5",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GenerateFeedID(tc.feedURL)
			if result != tc.expected {
				t.Errorf("GenerateFeedID(%q) = %q, want %q", tc.feedURL, result, tc.expected)
			}
		})
	}
}

func TestGenerateFeedID_QueryStringUniqueness(t *testing.T) {
	// Feeds that share a host and path but differ only in their query string
	// must not collide. YouTube channel feeds are the canonical example: every
	// channel is served from /feeds/videos.xml and identified by channel_id.
	// A collision here makes store.LoadFeeds silently drop every channel but
	// the last one, because feeds are keyed by GenerateFeedID.
	urls := []string{
		"https://www.youtube.com/feeds/videos.xml?channel_id=UCEXbEm8RUvRkKUEo2RjKd9w",
		"https://www.youtube.com/feeds/videos.xml?channel_id=UCOiNhz5lCJiFIAZPsGZ5_9Q",
		"https://example.com/feed.xml?format=rss&limit=10",
		"https://example.com/feed.xml?format=rss&limit=20",
	}

	seen := make(map[string]string, len(urls))
	for _, u := range urls {
		id := GenerateFeedID(u)
		if prev, dup := seen[id]; dup {
			t.Errorf("GenerateFeedID collision: %q and %q both produce %q", prev, u, id)
		}
		seen[id] = u
	}
}

func TestGenerateFeedID_QueryStringDoesNotAffectQuerylessURLs(t *testing.T) {
	// Adding query-string disambiguation must not change the ID of any URL
	// that has no query string, otherwise existing feed IDs would churn.
	if got, want := GenerateFeedID("https://example.com/feed.xml"), "example.com-feed-xml"; got != want {
		t.Errorf("GenerateFeedID(%q) = %q, want %q", "https://example.com/feed.xml", got, want)
	}
}

func TestGenerateFeedID_Consistency(t *testing.T) {
	// Test that the same URL always generates the same ID
	url := "https://feeds.bbci.co.uk/news/world/africa/rss.xml"

	id1 := GenerateFeedID(url)
	id2 := GenerateFeedID(url)

	if id1 != id2 {
		t.Errorf("GenerateFeedID should be deterministic, got %q and %q", id1, id2)
	}
}

func TestGenerateFeedID_InvalidURL(t *testing.T) {
	// Test with invalid URL that should fallback to hash
	invalidURL := "not-a-url"
	result := GenerateFeedID(invalidURL)

	// Should start with "feed-" followed by hash
	if len(result) == 0 {
		t.Error("GenerateFeedID should not return empty string")
	}

	// Should be deterministic even for invalid URLs
	result2 := GenerateFeedID(invalidURL)
	if result != result2 {
		t.Errorf("GenerateFeedID should be deterministic for invalid URLs, got %q and %q", result, result2)
	}
}
