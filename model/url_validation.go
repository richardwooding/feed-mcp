package model

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/richardwooding/ssrfguard"
)

// resolver resolves named feed hosts during URL validation. It is a package
// variable so tests can substitute a hermetic resolver that fails fast instead
// of performing real DNS lookups; production leaves it as net.DefaultResolver.
var resolver = net.DefaultResolver

// validateResolveTimeout bounds DNS resolution during URL validation. ssrfguard
// resolves named hosts to check them against blocked ranges; without a deadline
// a slow or unreachable resolver would stall validation — notably at startup,
// when every configured feed URL is sanitized.
const validateResolveTimeout = 5 * time.Second

// URL validation errors. These remain the package's public sentinels (matched
// with errors.Is by enhanced error reporting) and are mapped from the
// equivalent github.com/richardwooding/ssrfguard errors.
var (
	ErrInvalidURL        = errors.New("invalid URL format")
	ErrUnsupportedScheme = errors.New("unsupported URL scheme - only HTTP and HTTPS are allowed")
	ErrPrivateIPBlocked  = errors.New("private IP addresses and localhost are blocked for security")
	ErrMissingHost       = errors.New("URL must have a valid host")
	ErrEmptyURL          = errors.New("URL cannot be empty")
)

// ValidateFeedURL validates a feed URL for security and format correctness.
// It performs SSRF-focused checks — scheme validation, host verification, and
// (unless allowPrivateIPs is set) blocking of private, loopback, link-local, and
// metadata addresses — via github.com/richardwooding/ssrfguard, returning this
// package's sentinel errors so callers can match them with errors.Is.
//
// DNS resolution of named hosts is bounded by validateResolveTimeout so a slow
// or unreachable resolver cannot stall validation.
func ValidateFeedURL(rawURL string, allowPrivateIPs bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), validateResolveTimeout)
	defer cancel()
	guard := ssrfguard.New(
		ssrfguard.WithAllowPrivate(allowPrivateIPs),
		ssrfguard.WithResolver(resolver),
	)
	return mapSSRFError(guard.ValidateURLContext(ctx, rawURL))
}

// mapSSRFError translates ssrfguard sentinel errors into this package's
// equivalents, preserving the existing public error API.
func mapSSRFError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ssrfguard.ErrEmptyURL):
		return ErrEmptyURL
	case errors.Is(err, ssrfguard.ErrUnsupportedScheme):
		return ErrUnsupportedScheme
	case errors.Is(err, ssrfguard.ErrMissingHost):
		return ErrMissingHost
	case errors.Is(err, ssrfguard.ErrBlockedAddress):
		return ErrPrivateIPBlocked
	case errors.Is(err, ssrfguard.ErrInvalidURL):
		return ErrInvalidURL
	default:
		return err
	}
}

// SanitizeFeedURLs validates a slice of feed URLs.
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
