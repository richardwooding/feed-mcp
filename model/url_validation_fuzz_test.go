package model

import (
	"testing"
)

// FuzzValidateFeedURL tests URL validation with random inputs to discover edge cases
// and potential security vulnerabilities (SSRF bypasses, parsing errors, panics)
func FuzzValidateFeedURL(f *testing.F) {
	// Seed corpus with known edge cases and attack patterns

	// Valid URLs
	f.Add("https://example.com/feed.xml", false)
	f.Add("http://feeds.example.org/rss", false)

	// Localhost patterns (should be blocked when allowPrivateIPs=false)
	f.Add("http://localhost/feed.xml", false)
	f.Add("https://127.0.0.1:8080/rss", false)
	f.Add("http://[::1]/atom", false)
	f.Add("http://127.0.0.1/feed", false)

	// Private IP ranges (should be blocked when allowPrivateIPs=false)
	f.Add("http://10.0.0.1/feed", false)
	f.Add("http://192.168.1.1/rss", false)
	f.Add("http://172.16.0.1/atom", false)
	f.Add("http://169.254.1.1/feed", false)

	// Invalid schemes (should always be blocked)
	f.Add("file:///etc/passwd", false)
	f.Add("ftp://example.com/feed", false)
	f.Add("javascript:alert('xss')", false)
	f.Add("data:text/html,<script>alert('xss')</script>", false)
	f.Add("gopher://example.com/feed", false)

	// SSRF bypass attempts
	f.Add("http://localhost@example.com/feed", false)
	f.Add("http://example.com@localhost/feed", false)
	f.Add("http://127.0.0.1.example.com/feed", false)
	f.Add("http://127.1/feed", false)
	f.Add("http://0x7f000001/feed", false) // Hex notation for 127.0.0.1
	f.Add("http://0177.0.0.1/feed", false) // Octal notation

	// URL encoding edge cases
	f.Add("http://%6C%6F%63%61%6C%68%6F%73%74/feed", false) // "localhost" URL encoded
	f.Add("http://127.0.0.1%00.example.com/feed", false)    // Null byte injection

	// Malformed URLs
	f.Add("", false)
	f.Add("not-a-url", false)
	f.Add("://example.com", false)
	f.Add("http://", false)
	f.Add("http:///feed", false)

	// Edge cases with allowPrivateIPs=true
	f.Add("http://localhost/feed", true)
	f.Add("http://192.168.1.1/feed", true)

	f.Fuzz(func(t *testing.T, url string, allowPrivateIPs bool) {
		// The function should never panic, regardless of input
		// We're testing for robustness and security, not correctness
		_ = ValidateFeedURL(url, allowPrivateIPs)
	})
}

// FuzzSanitizeFeedURLs tests batch URL validation with random inputs
func FuzzSanitizeFeedURLs(f *testing.F) {
	// Seed corpus with various URL combinations
	f.Add("https://example.com/feed.xml", false)
	f.Add("http://localhost/feed", false)
	f.Add("", false)
	f.Add("file:///etc/passwd", false)

	f.Fuzz(func(t *testing.T, url string, allowPrivateIPs bool) {
		// Test with single URL
		urls := []string{url}
		_ = SanitizeFeedURLs(urls, allowPrivateIPs)

		// Test with multiple URLs (duplicate)
		urls = []string{url, url}
		_ = SanitizeFeedURLs(urls, allowPrivateIPs)
	})
}
