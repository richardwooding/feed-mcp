package model

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// URL validation errors
var (
	ErrInvalidURL          = errors.New("invalid URL format")
	ErrUnsupportedScheme   = errors.New("unsupported URL scheme - only HTTP and HTTPS are allowed")
	ErrPrivateIPBlocked    = errors.New("private IP addresses and localhost are blocked for security")
	ErrMissingHost         = errors.New("URL must have a valid host")
	ErrEmptyURL            = errors.New("URL cannot be empty")
)

// ValidateFeedURL validates a feed URL for security and format correctness.
// Performs comprehensive security checks including scheme validation, host verification,
// and optional private IP/localhost blocking to prevent SSRF attacks.
// Returns an error if the URL fails any security or format validation checks.
func ValidateFeedURL(rawURL string, allowPrivateIPs bool) error {
	if rawURL == "" {
		return ErrEmptyURL
	}

	// Parse the URL
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}

	// Validate scheme
	if err := validateScheme(u.Scheme); err != nil {
		return err
	}

	// Validate host
	if u.Host == "" {
		return ErrMissingHost
	}

	// Check for private IPs if not allowed
	if !allowPrivateIPs {
		if err := validateHost(u.Host); err != nil {
			return err
		}
	}

	return nil
}

// validateScheme ensures only HTTP and HTTPS schemes are allowed.
// Blocks potentially dangerous schemes like file://, ftp://, and data:// to prevent
// various attack vectors including local file inclusion and data exfiltration.
func validateScheme(scheme string) error {
	scheme = strings.ToLower(scheme)
	if scheme != "http" && scheme != "https" {
		return ErrUnsupportedScheme
	}
	return nil
}

// validateHost checks if the host resolves to private IP ranges or localhost.
// Performs DNS resolution and validates resolved IPs against private ranges (RFC 1918)
// and localhost patterns to prevent SSRF attacks against internal services.
// Allows temporarily unresolvable hosts to fail at HTTP request time.
func validateHost(host string) error {
	// Remove port if present
	hostname, _, err := net.SplitHostPort(host)
	if err != nil {
		// If SplitHostPort fails, assume no port and use the whole host
		hostname = host
	}

	// Check for localhost patterns
	if isLocalhost(hostname) {
		return ErrPrivateIPBlocked
	}

	// Try to resolve the hostname to IP addresses
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// If we can't resolve, let the HTTP client handle it later
		// This avoids blocking valid URLs that might be temporarily unresolvable
		return nil
	}

	// Check if any resolved IP is private
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return ErrPrivateIPBlocked
		}
	}

	return nil
}

// isLocalhost checks for common localhost patterns
func isLocalhost(hostname string) bool {
	hostname = strings.ToLower(hostname)
	
	localhostPatterns := []string{
		"localhost",
		"127.0.0.1", 
		"::1",
		"[::1]", // IPv6 with brackets
	}
	
	for _, pattern := range localhostPatterns {
		if hostname == pattern {
			return true
		}
	}
	
	if strings.HasPrefix(hostname, "127.") {
		return true
	}
	
	return false
}

// isPrivateIP checks if an IP address is in a private range
func isPrivateIP(ip net.IP) bool {
	// Check for IPv4 private ranges
	if ip4 := ip.To4(); ip4 != nil {
		// 10.0.0.0/8
		if ip4[0] == 10 {
			return true
		}
		// 172.16.0.0/12
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ip4[0] == 192 && ip4[1] == 168 {
			return true
		}
		// 169.254.0.0/16 (link-local)
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
		// 127.0.0.0/8 (loopback)
		if ip4[0] == 127 {
			return true
		}
	}

	// Check for IPv6 private ranges
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// Check for IPv6 unique local addresses (fc00::/7)
	if len(ip) == 16 && (ip[0]&0xfe) == 0xfc {
		return true
	}

	return false
}

// SanitizeFeedURLs validates a slice of feed URLs
func SanitizeFeedURLs(urls []string, allowPrivateIPs bool) error {
	if len(urls) == 0 {
		return errors.New("no feed URLs provided")
	}

	var invalidURLs []string
	for _, rawURL := range urls {
		if err := ValidateFeedURL(rawURL, allowPrivateIPs); err != nil {
			invalidURLs = append(invalidURLs, fmt.Sprintf("%s: %v", rawURL, err))
		}
	}

	if len(invalidURLs) > 0 {
		return fmt.Errorf("invalid feed URLs:\n%s", strings.Join(invalidURLs, "\n"))
	}

	return nil
}