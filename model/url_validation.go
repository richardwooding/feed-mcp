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
// when every configured feed URL is sanitized. It is a var (not a const) so
// tests can shorten it.
var validateResolveTimeout = 5 * time.Second

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
// It is a convenience wrapper around ValidateFeedURLContext using a background
// context; callers with a context should prefer ValidateFeedURLContext so that
// cancellation and deadlines propagate.
func ValidateFeedURL(rawURL string, allowPrivateIPs bool) error {
	return ValidateFeedURLContext(context.Background(), rawURL, allowPrivateIPs)
}

// ValidateFeedURLContext is ValidateFeedURL with a caller-supplied context.
//
// DNS resolution of named hosts is bounded by validateResolveTimeout (derived
// from ctx) so a slow or unreachable resolver cannot stall validation. The two
// timeout sources are treated differently: if only that internal budget elapses
// while ctx itself is still live, the URL is allowed — the host was merely slow
// to resolve, and the dial-time guard re-checks the destination at connect time.
// Genuine cancellation or a deadline on ctx propagates to the caller.
func ValidateFeedURLContext(ctx context.Context, rawURL string, allowPrivateIPs bool) error {
	cctx, cancel := context.WithTimeout(ctx, validateResolveTimeout)
	defer cancel()
	guard := ssrfguard.New(
		ssrfguard.WithAllowPrivate(allowPrivateIPs),
		ssrfguard.WithResolver(resolver),
	)
	err := mapSSRFError(guard.ValidateURLContext(cctx, rawURL))
	if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
		return nil
	}
	return err
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

// SanitizeFeedURLs validates a slice of feed URLs. It is a convenience wrapper
// around SanitizeFeedURLsContext using a background context.
func SanitizeFeedURLs(urls []string, allowPrivateIPs bool) error {
	return SanitizeFeedURLsContext(context.Background(), urls, allowPrivateIPs)
}

// SanitizeFeedURLsContext is SanitizeFeedURLs with a caller-supplied context.
// If ctx is canceled (or its deadline elapses) mid-batch, it returns ctx.Err()
// promptly rather than continuing to validate the remaining URLs.
func SanitizeFeedURLsContext(ctx context.Context, urls []string, allowPrivateIPs bool) error {
	if len(urls) == 0 {
		return errors.New("no feed URLs provided")
	}

	var invalidURLs []string
	for _, rawURL := range urls {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := ValidateFeedURLContext(ctx, rawURL, allowPrivateIPs); err != nil {
			invalidURLs = append(invalidURLs, fmt.Sprintf("%s: %v", rawURL, err))
		}
	}

	if len(invalidURLs) > 0 {
		return fmt.Errorf("invalid feed URLs:\n%s", strings.Join(invalidURLs, "\n"))
	}

	return nil
}
