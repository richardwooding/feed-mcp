// Package model provides shared utilities for feed ID generation
package model

import (
	"fmt"
	"hash/fnv"
	"net/url"
	"regexp"
	"strings"
)

// GenerateFeedID creates a stable, deterministic feed ID from a URL.
// This generates human-readable IDs like "feeds.bbci.co.uk-news-world-africa"
// with hash suffixes for uniqueness when needed.
func GenerateFeedID(feedURL string) string {
	// Parse URL to extract host and path for a more readable ID
	if parsedURL, err := url.Parse(feedURL); err == nil {
		// Create a slug-like ID from the host and path
		slug := strings.ToLower(parsedURL.Host)
		if parsedURL.Path != "" && parsedURL.Path != "/" {
			// Clean the path and append to host
			path := strings.Trim(parsedURL.Path, "/")
			path = regexp.MustCompile(`[^a-z0-9-_]`).ReplaceAllString(path, "-")
			path = regexp.MustCompile(`-+`).ReplaceAllString(path, "-")
			slug = slug + "-" + path
		}
		// Truncate if too long and add hash suffix for uniqueness
		if len(slug) > 40 {
			h := fnv.New32a()
			_, _ = h.Write([]byte(feedURL)) // FNV hash Write never returns an error
			hashStr := fmt.Sprintf("%x", h.Sum32())[:8]
			slug = slug[:32] + "-" + hashStr
		}
		return slug
	}

	// Fallback to hash if URL parsing fails
	h := fnv.New32a()
	_, _ = h.Write([]byte(feedURL)) // FNV hash Write never returns an error
	return fmt.Sprintf("feed-%x", h.Sum32())
}