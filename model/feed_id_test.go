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
