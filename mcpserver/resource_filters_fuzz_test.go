package mcpserver

import (
	"testing"
)

// FuzzParseURIParameters tests URI parameter parsing with random inputs to discover
// date parsing vulnerabilities, integer overflow, injection attacks, and panics
func FuzzParseURIParameters(f *testing.F) {
	// Seed corpus with valid URI patterns

	// Valid URIs with various filter combinations
	f.Add("feeds://feed/test-feed/items")
	f.Add("feeds://feed/test-feed/items?limit=10")
	f.Add("feeds://feed/test-feed/items?limit=10&offset=5")
	f.Add("feeds://feed/test-feed/items?since=2023-01-01T00:00:00Z")
	f.Add("feeds://feed/test-feed/items?until=2023-12-31T23:59:59Z")
	f.Add("feeds://feed/test-feed/items?since=2023-01-01T00:00:00Z&until=2023-12-31T23:59:59Z")
	f.Add("feeds://feed/test-feed/items?category=technology")
	f.Add("feeds://feed/test-feed/items?author=john")
	f.Add("feeds://feed/test-feed/items?search=golang")

	// Complex filter combinations
	f.Add("feeds://feed/test-feed/items?limit=10&offset=5&category=tech&author=jane&search=programming")
	f.Add("feeds://feed/test-feed/items?since=2023-01-01T00:00:00Z&limit=20&category=news")

	// Edge cases for numeric parameters
	f.Add("feeds://feed/test-feed/items?limit=0")
	f.Add("feeds://feed/test-feed/items?limit=1000")
	f.Add("feeds://feed/test-feed/items?limit=9999999") // Above max
	f.Add("feeds://feed/test-feed/items?limit=-1")      // Negative (invalid)
	f.Add("feeds://feed/test-feed/items?offset=0")
	f.Add("feeds://feed/test-feed/items?offset=999999")
	f.Add("feeds://feed/test-feed/items?offset=-5")      // Negative (invalid)

	// Invalid numeric formats
	f.Add("feeds://feed/test-feed/items?limit=abc")
	f.Add("feeds://feed/test-feed/items?limit=10.5")
	f.Add("feeds://feed/test-feed/items?limit=")
	f.Add("feeds://feed/test-feed/items?offset=xyz")

	// Date format edge cases
	f.Add("feeds://feed/test-feed/items?since=2023-01-01")           // Missing time
	f.Add("feeds://feed/test-feed/items?since=invalid-date")
	f.Add("feeds://feed/test-feed/items?since=")
	f.Add("feeds://feed/test-feed/items?until=2023-13-45T99:99:99Z") // Invalid date
	f.Add("feeds://feed/test-feed/items?since=0000-00-00T00:00:00Z")
	f.Add("feeds://feed/test-feed/items?since=9999-12-31T23:59:59Z")

	// Boolean parameter edge cases
	f.Add("feeds://feed/test-feed/items?has_media=true")
	f.Add("feeds://feed/test-feed/items?has_media=false")
	f.Add("feeds://feed/test-feed/items?has_media=1")
	f.Add("feeds://feed/test-feed/items?has_media=0")
	f.Add("feeds://feed/test-feed/items?has_media=yes")
	f.Add("feeds://feed/test-feed/items?has_media=no")
	f.Add("feeds://feed/test-feed/items?has_media=invalid")
	f.Add("feeds://feed/test-feed/items?has_media=")

	// String parameter edge cases
	f.Add("feeds://feed/test-feed/items?category=")
	f.Add("feeds://feed/test-feed/items?author=")
	f.Add("feeds://feed/test-feed/items?search=")
	f.Add("feeds://feed/test-feed/items?category=" + string(make([]byte, 10000))) // Very long
	f.Add("feeds://feed/test-feed/items?search=<script>alert('xss')</script>")    // XSS attempt

	// URL encoding edge cases
	f.Add("feeds://feed/test-feed/items?search=hello%20world")
	f.Add("feeds://feed/test-feed/items?category=tech%26security")
	f.Add("feeds://feed/test-feed/items?author=john%3Ddoe")

	// Special characters in parameters
	f.Add("feeds://feed/test-feed/items?search=a&b=c")
	f.Add("feeds://feed/test-feed/items?category=tech&&&limit=10")
	f.Add("feeds://feed/test-feed/items?search=?query=test")

	// Malformed URIs
	f.Add("")
	f.Add("not-a-uri")
	f.Add("://no-scheme")
	f.Add("feeds://")
	f.Add("feeds://feed/test?")
	f.Add("feeds://feed/test?&&&")

	// Multiple values for same parameter
	f.Add("feeds://feed/test-feed/items?limit=10&limit=20")
	f.Add("feeds://feed/test-feed/items?since=2023-01-01T00:00:00Z&since=2024-01-01T00:00:00Z")

	// Enhanced filter parameters (Phase 2)
	f.Add("feeds://feed/test-feed/items?language=en")
	f.Add("feeds://feed/test-feed/items?min_length=100")
	f.Add("feeds://feed/test-feed/items?max_length=500")
	f.Add("feeds://feed/test-feed/items?sentiment=positive")
	f.Add("feeds://feed/test-feed/items?sort_by=date")
	f.Add("feeds://feed/test-feed/items?format=json")

	f.Fuzz(func(t *testing.T, resourceURI string) {
		// The function should never panic, regardless of input
		// We're testing for robustness in parameter parsing
		_, _ = ParseURIParameters(resourceURI)
	})
}
