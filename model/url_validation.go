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

// defaultResolveTimeout bounds DNS resolution during URL validation. ssrfguard
// resolves named hosts to check them against blocked ranges; without a deadline
// a slow or unreachable resolver would stall validation — notably at startup,
// when every configured feed URL is sanitized.
const defaultResolveTimeout = 5 * time.Second

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

// validator performs SSRF-focused feed URL validation. Holding the resolver and
// resolve timeout as fields (rather than package globals) keeps validation
// configurable and lets tests substitute a hermetic resolver without mutating
// shared state, so the package's tests stay parallel-safe.
type validator struct {
	resolver       *net.Resolver
	resolveTimeout time.Duration
}

// newValidator builds a validator, applying defaults for a nil resolver or a
// non-positive timeout.
func newValidator(resolver *net.Resolver, resolveTimeout time.Duration) *validator {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if resolveTimeout <= 0 {
		resolveTimeout = defaultResolveTimeout
	}
	return &validator{resolver: resolver, resolveTimeout: resolveTimeout}
}

// defaultValidator backs the package-level functions. It is constructed once and
// never mutated, so it is safe for concurrent use.
var defaultValidator = newValidator(net.DefaultResolver, defaultResolveTimeout)

// validateURL validates a single URL, failing closed: a canceled context or an
// elapsed resolve deadline returns the context error rather than allowing the
// URL. Callers that want to tolerate a resolve timeout (for example, so slow DNS
// doesn't block startup) must decide that explicitly — see SanitizeURLs and the
// startup path in cmd. The dial-time guard remains a second layer of defense.
func (v *validator) validateURL(ctx context.Context, rawURL string, allowPrivateIPs bool) error {
	cctx, cancel := context.WithTimeout(ctx, v.resolveTimeout)
	defer cancel()
	guard := ssrfguard.New(
		ssrfguard.WithAllowPrivate(allowPrivateIPs),
		ssrfguard.WithResolver(v.resolver),
	)
	return mapSSRFError(guard.ValidateURLContext(cctx, rawURL))
}

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

// ValidateFeedURLContext is ValidateFeedURL with a caller-supplied context that
// governs DNS resolution. It fails closed: a canceled context or an elapsed
// resolve deadline returns the context error.
func ValidateFeedURLContext(ctx context.Context, rawURL string, allowPrivateIPs bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return defaultValidator.validateURL(ctx, rawURL, allowPrivateIPs)
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

// sanitizeURLs validates a slice of URLs, failing closed. A context error
// (cancellation, or an elapsed resolve deadline on any URL) is returned directly
// so callers can match it with errors.Is rather than having it folded into the
// formatted "invalid feed URLs" message.
func (v *validator) sanitizeURLs(ctx context.Context, urls []string, allowPrivateIPs bool) error {
	if len(urls) == 0 {
		return errors.New("no feed URLs provided")
	}

	var invalidURLs []string
	for _, rawURL := range urls {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := v.validateURL(ctx, rawURL, allowPrivateIPs); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			invalidURLs = append(invalidURLs, fmt.Sprintf("%s: %v", rawURL, err))
		}
	}

	if len(invalidURLs) > 0 {
		return fmt.Errorf("invalid feed URLs:\n%s", strings.Join(invalidURLs, "\n"))
	}

	return nil
}

// SanitizeFeedURLs validates a slice of feed URLs. It is a convenience wrapper
// around SanitizeFeedURLsContext using a background context.
func SanitizeFeedURLs(urls []string, allowPrivateIPs bool) error {
	return SanitizeFeedURLsContext(context.Background(), urls, allowPrivateIPs)
}

// SanitizeFeedURLsContext is SanitizeFeedURLs with a caller-supplied context. It
// fails closed: a context error (cancellation or an elapsed resolve deadline on
// any URL) is returned directly. Callers that want to tolerate a resolve timeout
// (e.g. so slow DNS doesn't block startup) should check for context.DeadlineExceeded
// themselves — see the startup path in cmd.
func SanitizeFeedURLsContext(ctx context.Context, urls []string, allowPrivateIPs bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return defaultValidator.sanitizeURLs(ctx, urls, allowPrivateIPs)
}
