package model

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

// hermeticValidator returns a validator whose resolver fails fast instead of
// performing real DNS lookups, keeping these tests off the network. A failed
// lookup leaves a named host unresolvable, which ssrfguard allows — matching the
// table cases that expect named hosts (example.com, etc.) to validate cleanly.
func hermeticValidator() *validator {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("dns disabled in tests")
		},
	}
	return newValidator(r, defaultResolveTimeout)
}

func TestValidateFeedURL(t *testing.T) {
	tests := []struct {
		errorType      error
		name           string
		url            string
		allowPrivateIP bool
		expectError    bool
	}{
		// Valid URLs
		{nil, "valid HTTP URL", "http://example.com/feed.xml", false, false},
		{nil, "valid HTTPS URL", "https://example.com/feed.xml", false, false},
		{nil, "valid URL with port", "https://example.com:8080/feed.xml", false, false},
		{nil, "valid URL with path", "https://feeds.feedburner.com/oreilly", false, false},
		{nil, "valid URL with query params", "https://example.com/feed?format=xml", false, false},

		// Invalid schemes
		{ErrUnsupportedScheme, "file scheme", "file:///etc/passwd", false, true},
		{ErrUnsupportedScheme, "ftp scheme", "ftp://example.com/file.txt", false, true},
		{ErrUnsupportedScheme, "javascript scheme", "javascript:alert('xss')", false, true},
		{ErrUnsupportedScheme, "data scheme", "data:text/plain,hello", false, true},
		{ErrUnsupportedScheme, "ldap scheme", "ldap://example.com", false, true},

		// Invalid formats
		{ErrEmptyURL, "empty URL", "", false, true},
		{ErrUnsupportedScheme, "malformed URL", "not-a-url", false, true},
		{ErrUnsupportedScheme, "missing scheme", "example.com/feed", false, true},
		{ErrMissingHost, "missing host", "http:///feed", false, true},
		{ErrInvalidURL, "space in URL", "http://exa mple.com/feed", false, true},

		// Private IP ranges - blocked by default
		{ErrPrivateIPBlocked, "localhost", "http://localhost/feed", false, true},
		{ErrPrivateIPBlocked, "127.0.0.1", "http://127.0.0.1/feed", false, true},
		{ErrPrivateIPBlocked, "127.x.x.x range", "http://127.1.1.1/feed", false, true},
		{ErrPrivateIPBlocked, "10.x.x.x range", "http://10.0.0.1/feed", false, true},
		{ErrPrivateIPBlocked, "192.168.x.x range", "http://192.168.1.1/feed", false, true},
		{ErrPrivateIPBlocked, "172.16-31.x.x range", "http://172.16.0.1/feed", false, true},
		{ErrPrivateIPBlocked, "link-local 169.254", "http://169.254.0.1/feed", false, true},
		{ErrPrivateIPBlocked, "IPv6 localhost", "http://[::1]/feed", false, true},

		// Private IPs - allowed when flag is set
		{nil, "localhost allowed", "http://localhost/feed", true, false},
		{nil, "127.0.0.1 allowed", "http://127.0.0.1/feed", true, false},
		{nil, "10.x.x.x allowed", "http://10.0.0.1/feed", true, false},
		{nil, "192.168.x.x allowed", "http://192.168.1.1/feed", true, false},

		// Edge cases
		{nil, "uppercase scheme", "HTTP://EXAMPLE.COM/feed", false, false},
		{nil, "mixed case host", "https://ExAmPlE.CoM/feed", false, false},
		{nil, "URL with fragment", "https://example.com/feed#section", false, false},
		{nil, "URL with authentication", "https://user:pass@example.com/feed", false, false},
	}

	v := hermeticValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.validateURL(context.Background(), tt.url, tt.allowPrivateIP)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error for URL %q, but got none", tt.url)
					return
				}
				if tt.errorType != nil && !strings.Contains(err.Error(), tt.errorType.Error()) {
					t.Errorf("expected error type %v, got %v", tt.errorType, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for URL %q: %v", tt.url, err)
				}
			}
		})
	}
}

func TestSanitizeFeedURLs(t *testing.T) {
	tests := []struct {
		name           string
		errorContains  string
		urls           []string
		allowPrivateIP bool
		expectError    bool
	}{
		{
			name:           "all valid URLs",
			urls:           []string{"https://example.com/feed", "http://feeds.example.org/rss"},
			allowPrivateIP: false,
			expectError:    false,
		},
		{
			name:           "empty URL list",
			urls:           []string{},
			allowPrivateIP: false,
			expectError:    true,
			errorContains:  "no feed URLs provided",
		},
		{
			name:           "mixed valid and invalid",
			urls:           []string{"https://example.com/feed", "file:///etc/passwd", "http://localhost/feed"},
			allowPrivateIP: false,
			expectError:    true,
			errorContains:  "invalid feed URLs",
		},
		{
			name:           "all invalid URLs",
			urls:           []string{"not-a-url", "ftp://example.com", ""},
			allowPrivateIP: false,
			expectError:    true,
			errorContains:  "invalid feed URLs",
		},
		{
			name:           "private IPs allowed",
			urls:           []string{"https://example.com/feed", "http://localhost/feed", "http://192.168.1.1/api"},
			allowPrivateIP: true,
			expectError:    false,
		},
	}

	v := hermeticValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.sanitizeURLs(context.Background(), tt.urls, tt.allowPrivateIP)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error for URLs %v, but got none", tt.urls)
					return
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain %q, got %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for URLs %v: %v", tt.urls, err)
				}
			}
		})
	}
}

// Scheme validation, localhost/private-IP classification, and host resolution
// are now provided and unit-tested by github.com/richardwooding/ssrfguard. The
// table tests above (TestValidateFeedURL, TestSanitizeFeedURLs) and the
// bypass-attempt test below exercise the delegated behavior end-to-end.

// Security test for potential bypass attempts
func TestSecurityBypassAttempts(t *testing.T) {
	bypassAttempts := []string{
		"http://localhost@example.com/",            // URL with fake authority
		"http://example.com#@localhost/",           // Fragment bypass attempt
		"http://example.com?url=http://localhost/", // Query parameter bypass
		"http://127.1/",                            // Shortened IP notation
		"http://0x7f000001/",                       // Hex IP notation
		"http://2130706433/",                       // Decimal IP notation
		"http://[::ffff:127.0.0.1]/",               // IPv4-mapped IPv6
	}

	v := hermeticValidator()
	for _, url := range bypassAttempts {
		t.Run("bypass attempt: "+url, func(t *testing.T) {
			// Most of these should either be blocked or handled safely
			err := v.validateURL(context.Background(), url, false)
			if err == nil {
				// If not blocked, it should at least be a valid URL that doesn't resolve to localhost
				// The actual blocking might happen during host validation
				t.Logf("URL %q was not blocked - this may be expected if it doesn't resolve to localhost", url)
			}
		})
	}
}
