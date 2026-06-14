// Package model provides helper functions for creating structured errors.
package model

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"syscall"
)

// Repeated error/format message strings.
const (
	msgConnectionFailed = "Connection failed"
)

// CreateNetworkError creates a FeedError for network-related issues
func CreateNetworkError(err error, feedURL string) *FeedError {
	errorType := ErrorTypeNetwork
	message := "Network error occurred"

	// Categorize the specific network error
	if err != nil {
		// Check for timeout errors
		if isTimeoutError(err) {
			errorType = ErrorTypeTimeout
			message = "Request timed out"
		} else if isDNSError(err) {
			errorType = ErrorTypeDNSResolution
			message = "DNS resolution failed"
		} else if isConnectionError(err) {
			errorType = ErrorTypeConnectionFailed
			message = msgConnectionFailed
		}
	}

	return NewFeedErrorWithCause(errorType, message, err).
		WithURL(feedURL).
		WithOperation("fetch_feed").
		WithComponent("http_client")
}

// CreateHTTPError creates a FeedError for HTTP response errors
func CreateHTTPError(resp *http.Response, feedURL string) *FeedError {
	var errorType ErrorType
	var message string

	status := resp.StatusCode

	switch {
	case status >= 400 && status < 500:
		errorType = ErrorTypeHTTPClientError
		message = fmt.Sprintf("Client error: %s", resp.Status)
	case status >= 500:
		errorType = ErrorTypeHTTPServerError
		message = fmt.Sprintf("Server error: %s", resp.Status)
	case status >= 300 && status < 400:
		errorType = ErrorTypeHTTPRedirect
		message = fmt.Sprintf("Redirect error: %s", resp.Status)
	default:
		errorType = ErrorTypeHTTP
		message = fmt.Sprintf("HTTP error: %s", resp.Status)
	}

	return NewFeedError(errorType, message).
		WithURL(feedURL).
		WithOperation("fetch_feed").
		WithComponent("http_client").
		WithHTTP(status, resp.Header)
}

// CreateValidationError creates a FeedError for URL validation issues
func CreateValidationError(err error, feedURL string) *FeedError {
	errorType := ErrorTypeValidation
	message := "URL validation failed"

	// Map existing validation errors to our error types
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidURL):
			errorType = ErrorTypeInvalidURL
			message = "Invalid URL format"
		case errors.Is(err, ErrUnsupportedScheme):
			errorType = ErrorTypeUnsupportedScheme
			message = "Unsupported URL scheme"
		case errors.Is(err, ErrPrivateIPBlocked):
			errorType = ErrorTypePrivateIP
			message = "Private IP address blocked"
		case errors.Is(err, ErrMissingHost):
			errorType = ErrorTypeInvalidURL
			message = "URL missing host"
		case errors.Is(err, ErrEmptyURL):
			errorType = ErrorTypeInvalidURL
			message = "URL cannot be empty"
		}
	}

	return NewFeedErrorWithCause(errorType, message, err).
		WithURL(feedURL).
		WithOperation("validate_url").
		WithComponent("url_validator")
}

// CreateCircuitBreakerError creates a FeedError for circuit breaker events
func CreateCircuitBreakerError(feedURL, state string) *FeedError {
	message := fmt.Sprintf("Circuit breaker is %s", state)

	return NewFeedError(ErrorTypeCircuitBreaker, message).
		WithURL(feedURL).
		WithOperation("fetch_feed").
		WithComponent("circuit_breaker")
}

// CreateRetryError creates a FeedError when all retry attempts are exhausted
func CreateRetryError(lastErr error, feedURL string, attempt, maxAttempts int) *FeedError {
	message := fmt.Sprintf("All retry attempts exhausted (%d/%d)", attempt, maxAttempts)

	// Preserve the error type from the last error if it's a FeedError
	errorType := ErrorTypeNetwork
	feedErr := &FeedError{}
	if errors.As(lastErr, &feedErr) {
		errorType = feedErr.ErrorType
	}

	return NewFeedErrorWithCause(errorType, message, lastErr).
		WithURL(feedURL).
		WithOperation("retry_fetch").
		WithComponent("retry_manager").
		WithRetryContext(attempt, maxAttempts, 0)
}

// Helper functions to categorize network errors

// isTimeoutError checks if the error is related to timeouts
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	// Check for context timeout
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Check for net.Error timeout
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// Check error message for timeout indicators
	errStr := strings.ToLower(err.Error())
	timeoutKeywords := []string{"timeout", "deadline exceeded", "timed out"}
	for _, keyword := range timeoutKeywords {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}

	return false
}

// isDNSError checks if the error is related to DNS resolution
func isDNSError(err error) bool {
	if err == nil {
		return false
	}

	// Check for DNS error types
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}

	// Check error message for DNS indicators
	errStr := strings.ToLower(err.Error())
	dnsKeywords := []string{
		"no such host", "dns", "name resolution", "hostname",
		"name or service not known", "nodename nor servname provided",
	}
	for _, keyword := range dnsKeywords {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}

	return false
}

// isConnectionError checks if the error is related to connection issues
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}

	// Check for specific syscall errors using errors.Is for better cross-platform compatibility
	opErr := &net.OpError{}
	if errors.As(err, &opErr) {
		// Common connection errors
		if errors.Is(opErr.Err, syscall.ECONNREFUSED) ||
			errors.Is(opErr.Err, syscall.ECONNRESET) ||
			errors.Is(opErr.Err, syscall.ECONNABORTED) ||
			errors.Is(opErr.Err, syscall.EHOSTUNREACH) ||
			errors.Is(opErr.Err, syscall.ENETUNREACH) {
			return true
		}
	}

	// Check error message for connection indicators
	errStr := strings.ToLower(err.Error())
	connKeywords := []string{
		"connection refused", "connection reset", "connection aborted",
		"host unreachable", "network unreachable", "no route to host",
	}
	for _, keyword := range connKeywords {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}

	return false
}

// Resource-specific error helpers for MCP Resources

// CreateResourceError creates a FeedError for general resource issues
func CreateResourceError(err error, resourceURI, operation string) *FeedError {
	errorType := ErrorTypeResource
	message := "Resource operation failed"

	// Categorize based on the operation type
	if operation != "" {
		switch operation {
		case "read_resource":
			message = "Failed to read resource"
		case "list_resources":
			message = "Failed to list resources"
		case "subscribe":
			errorType = ErrorTypeSubscription
			message = "Failed to subscribe to resource"
		case "unsubscribe":
			errorType = ErrorTypeSubscription
			message = "Failed to unsubscribe from resource"
		}
	}

	return NewFeedErrorWithCause(errorType, message, err).
		WithURL(resourceURI).
		WithOperation(operation).
		WithComponent("resource_manager")
}

// CreateResourceNotFoundError creates a FeedError for resource not found
func CreateResourceNotFoundError(resourceURI, feedID string) *FeedError {
	message := "Resource not found"
	if feedID != "" {
		message = fmt.Sprintf("Feed not found: %s", feedID)
	}

	return NewFeedError(ErrorTypeResourceNotFound, message).
		WithURL(resourceURI).
		WithOperation("read_resource").
		WithComponent("resource_manager")
}

// CreateResourceUnavailableError creates a FeedError for temporarily unavailable resources
func CreateResourceUnavailableError(resourceURI, reason string) *FeedError {
	message := "Resource temporarily unavailable"
	if reason != "" {
		message = fmt.Sprintf("Resource unavailable: %s", reason)
	}

	return NewFeedError(ErrorTypeResourceUnavailable, message).
		WithURL(resourceURI).
		WithOperation("read_resource").
		WithComponent("resource_manager")
}

// CreateInvalidResourceURIError creates a FeedError for invalid resource URIs
func CreateInvalidResourceURIError(resourceURI, details string) *FeedError {
	message := "Invalid resource URI"
	if details != "" {
		message = fmt.Sprintf("Invalid resource URI: %s", details)
	}

	return NewFeedError(ErrorTypeInvalidResourceURI, message).
		WithURL(resourceURI).
		WithOperation("parse_resource_uri").
		WithComponent("resource_manager")
}

// CreateResourceContentError creates a FeedError for resource content generation issues
func CreateResourceContentError(err error, resourceURI, operation string) *FeedError {
	message := "Failed to generate resource content"

	return NewFeedErrorWithCause(ErrorTypeResourceContent, message, err).
		WithURL(resourceURI).
		WithOperation(operation).
		WithComponent("resource_manager")
}

// CreateSessionError creates a FeedError for session management issues
func CreateSessionError(err error, sessionID, operation string) *FeedError {
	errorType := ErrorTypeSession
	message := "Session operation failed"

	// Categorize session errors
	if err != nil {
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "not found") || strings.Contains(errStr, "does not exist") {
			errorType = ErrorTypeSessionNotFound
			message = "Session not found"
		}
	}

	fe := NewFeedErrorWithCause(errorType, message, err).
		WithOperation(operation).
		WithComponent("resource_manager")

	// Add session ID as URL context for tracking
	if sessionID != "" {
		fe = fe.WithURL(fmt.Sprintf("session://%s", sessionID))
	}

	return fe
}

// CreateSubscriptionError creates a FeedError for subscription issues
func CreateSubscriptionError(err error, resourceURI, sessionID, operation string) *FeedError {
	errorType := ErrorTypeSubscription
	message := "Subscription operation failed"

	// Categorize subscription errors
	if err != nil {
		errStr := strings.ToLower(err.Error())
		switch {
		case strings.Contains(errStr, "already subscribed") || strings.Contains(errStr, "exists"):
			errorType = ErrorTypeSubscriptionExists
			message = "Already subscribed to resource"
		case strings.Contains(errStr, "not found") || strings.Contains(errStr, "no subscription"):
			errorType = ErrorTypeSubscriptionNotFound
			message = "Subscription not found"
		}
	}

	fe := NewFeedErrorWithCause(errorType, message, err).
		WithURL(resourceURI).
		WithOperation(operation).
		WithComponent("resource_manager")

	// Add session context
	if sessionID != "" {
		fe.HTTPHeaders = map[string]string{
			"X-Session-ID": sessionID,
		}
	}

	return fe
}

// CreateResourceCacheError creates a FeedError for resource cache issues
func CreateResourceCacheError(err error, cacheKey, operation string) *FeedError {
	errorType := ErrorTypeResourceCache
	message := "Resource cache operation failed"

	// Categorize cache errors
	if operation == "invalidate" || operation == "cache_invalidation" {
		errorType = ErrorTypeCacheInvalidation
		message = "Cache invalidation failed"
	}

	fe := NewFeedErrorWithCause(errorType, message, err).
		WithOperation(operation).
		WithComponent("resource_cache")

	// Use cache key as URL context
	if cacheKey != "" {
		fe = fe.WithURL(fmt.Sprintf("cache://%s", cacheKey))
	}

	return fe
}
