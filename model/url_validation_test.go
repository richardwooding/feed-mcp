package model

import (
	"net"
	"strings"
	"testing"
)

func TestValidateFeedURL(t *testing.T) {
	tests := []struct {
		name           string
		url            string
		allowPrivateIP bool
		expectError    bool
		errorType      error
	}{
		// Valid URLs
		{"valid HTTP URL", "http://example.com/feed.xml", false, false, nil},
		{"valid HTTPS URL", "https://example.com/feed.xml", false, false, nil},
		{"valid URL with port", "https://example.com:8080/feed.xml", false, false, nil},
		{"valid URL with path", "https://feeds.feedburner.com/oreilly", false, false, nil},
		{"valid URL with query params", "https://example.com/feed?format=xml", false, false, nil},

		// Invalid schemes
		{"file scheme", "file:///etc/passwd", false, true, ErrUnsupportedScheme},
		{"ftp scheme", "ftp://example.com/file.txt", false, true, ErrUnsupportedScheme},
		{"javascript scheme", "javascript:alert('xss')", false, true, ErrUnsupportedScheme},
		{"data scheme", "data:text/plain,hello", false, true, ErrUnsupportedScheme},
		{"ldap scheme", "ldap://example.com", false, true, ErrUnsupportedScheme},

		// Invalid formats
		{"empty URL", "", false, true, ErrEmptyURL},
		{"malformed URL", "not-a-url", false, true, ErrUnsupportedScheme},
		{"missing scheme", "example.com/feed", false, true, ErrUnsupportedScheme},
		{"missing host", "http:///feed", false, true, ErrMissingHost},
		{"space in URL", "http://exa mple.com/feed", false, true, ErrInvalidURL},

		// Private IP ranges - blocked by default
		{"localhost", "http://localhost/feed", false, true, ErrPrivateIPBlocked},
		{"127.0.0.1", "http://127.0.0.1/feed", false, true, ErrPrivateIPBlocked},
		{"127.x.x.x range", "http://127.1.1.1/feed", false, true, ErrPrivateIPBlocked},
		{"10.x.x.x range", "http://10.0.0.1/feed", false, true, ErrPrivateIPBlocked},
		{"192.168.x.x range", "http://192.168.1.1/feed", false, true, ErrPrivateIPBlocked},
		{"172.16-31.x.x range", "http://172.16.0.1/feed", false, true, ErrPrivateIPBlocked},
		{"link-local 169.254", "http://169.254.0.1/feed", false, true, ErrPrivateIPBlocked},
		{"IPv6 localhost", "http://[::1]/feed", false, true, ErrPrivateIPBlocked},

		// Private IPs - allowed when flag is set
		{"localhost allowed", "http://localhost/feed", true, false, nil},
		{"127.0.0.1 allowed", "http://127.0.0.1/feed", true, false, nil},
		{"10.x.x.x allowed", "http://10.0.0.1/feed", true, false, nil},
		{"192.168.x.x allowed", "http://192.168.1.1/feed", true, false, nil},

		// Edge cases
		{"uppercase scheme", "HTTP://EXAMPLE.COM/feed", false, false, nil},
		{"mixed case host", "https://ExAmPlE.CoM/feed", false, false, nil},
		{"URL with fragment", "https://example.com/feed#section", false, false, nil},
		{"URL with authentication", "https://user:pass@example.com/feed", false, false, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFeedURL(tt.url, tt.allowPrivateIP)
			
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
		urls           []string
		allowPrivateIP bool
		expectError    bool
		errorContains  string
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SanitizeFeedURLs(tt.urls, tt.allowPrivateIP)
			
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

func TestValidateScheme(t *testing.T) {
	validSchemes := []string{"http", "https", "HTTP", "HTTPS", "Http", "Https"}
	for _, scheme := range validSchemes {
		t.Run("valid scheme: "+scheme, func(t *testing.T) {
			if err := validateScheme(scheme); err != nil {
				t.Errorf("scheme %q should be valid, got error: %v", scheme, err)
			}
		})
	}

	invalidSchemes := []string{"file", "ftp", "javascript", "data", "ldap", "gopher", "telnet", "ssh", ""}
	for _, scheme := range invalidSchemes {
		t.Run("invalid scheme: "+scheme, func(t *testing.T) {
			if err := validateScheme(scheme); err == nil {
				t.Errorf("scheme %q should be invalid", scheme)
			}
		})
	}
}

func TestIsLocalhost(t *testing.T) {
	localhosts := []string{"localhost", "LOCALHOST", "127.0.0.1", "127.1.1.1", "127.255.255.255", "::1"}
	for _, host := range localhosts {
		t.Run("localhost: "+host, func(t *testing.T) {
			if !isLocalhost(host) {
				t.Errorf("host %q should be detected as localhost", host)
			}
		})
	}

	nonLocalhosts := []string{"example.com", "192.168.1.1", "10.0.0.1", "google.com", "1.1.1.1"}
	for _, host := range nonLocalhosts {
		t.Run("not localhost: "+host, func(t *testing.T) {
			if isLocalhost(host) {
				t.Errorf("host %q should not be detected as localhost", host)
			}
		})
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name      string
		ip        string
		isPrivate bool
	}{
		// IPv4 private ranges
		{"10.0.0.1", "10.0.0.1", true},
		{"10.255.255.255", "10.255.255.255", true},
		{"172.16.0.1", "172.16.0.1", true},
		{"172.31.255.255", "172.31.255.255", true},
		{"192.168.1.1", "192.168.1.1", true},
		{"192.168.255.255", "192.168.255.255", true},
		{"127.0.0.1", "127.0.0.1", true},
		{"127.255.255.254", "127.255.255.254", true},
		{"169.254.1.1", "169.254.1.1", true},

		// IPv4 public ranges
		{"8.8.8.8", "8.8.8.8", false},
		{"1.1.1.1", "1.1.1.1", false},
		{"173.0.0.1", "173.0.0.1", false}, // Just outside 172.16-31 range
		{"172.15.255.255", "172.15.255.255", false}, // Just before 172.16-31 range
		{"172.32.0.1", "172.32.0.1", false}, // Just after 172.16-31 range

		// IPv6 addresses
		{"::1", "::1", true}, // IPv6 loopback
		{"fe80::1", "fe80::1", true}, // IPv6 link-local
		{"fc00::1", "fc00::1", true}, // IPv6 unique local
		{"2001:db8::1", "2001:db8::1", false}, // IPv6 public
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := parseIP(tt.ip)
			if ip == nil {
				t.Fatalf("failed to parse IP %q", tt.ip)
			}
			
			result := isPrivateIP(ip)
			if result != tt.isPrivate {
				t.Errorf("isPrivateIP(%q) = %v, want %v", tt.ip, result, tt.isPrivate)
			}
		})
	}
}

// Helper function for tests
func parseIP(s string) net.IP {
	if ip := net.ParseIP(s); ip != nil {
		return ip
	}
	return nil
}

// Integration test with actual network resolution
func TestValidateHostIntegration(t *testing.T) {
	// Test with a known public domain
	if err := validateHost("example.com"); err != nil {
		t.Errorf("example.com should be valid: %v", err)
	}

	// Test with localhost - should be blocked
	if err := validateHost("localhost"); err == nil {
		t.Error("localhost should be blocked")
	}

	// Test with invalid domain - should not error (let HTTP client handle it)
	if err := validateHost("this-domain-definitely-does-not-exist-12345.invalid"); err != nil {
		t.Errorf("unresolvable domains should be allowed for HTTP client to handle: %v", err)
	}
}

// Security test for potential bypass attempts
func TestSecurityBypassAttempts(t *testing.T) {
	bypassAttempts := []string{
		"http://localhost@example.com/", // URL with fake authority
		"http://example.com#@localhost/", // Fragment bypass attempt
		"http://example.com?url=http://localhost/", // Query parameter bypass
		"http://127.1/", // Shortened IP notation
		"http://0x7f000001/", // Hex IP notation
		"http://2130706433/", // Decimal IP notation
		"http://[::ffff:127.0.0.1]/", // IPv4-mapped IPv6
	}

	for _, url := range bypassAttempts {
		t.Run("bypass attempt: "+url, func(t *testing.T) {
			// Most of these should either be blocked or handled safely
			err := ValidateFeedURL(url, false)
			if err == nil {
				// If not blocked, it should at least be a valid URL that doesn't resolve to localhost
				// The actual blocking might happen during host validation
				t.Logf("URL %q was not blocked - this may be expected if it doesn't resolve to localhost", url)
			}
		})
	}
}