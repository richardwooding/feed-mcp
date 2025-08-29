// Package model provides helper functions for creating structured errors.
package model

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"
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
			message = "Connection failed"
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

// CreateParsingError creates a FeedError for feed parsing issues
func CreateParsingError(err error, feedURL, content string) *FeedError {
	errorType := ErrorTypeParsing
	message := "Failed to parse feed"

	// Categorize parsing errors based on content
	if err != nil {
		errStr := strings.ToLower(err.Error())

		if strings.Contains(errStr, "xml") {
			errorType = ErrorTypeMalformedXML
			message = "Feed contains malformed XML"
		} else if strings.Contains(errStr, "json") {
			errorType = ErrorTypeMalformedJSON
			message = "Feed contains malformed JSON"
		} else if strings.Contains(errStr, "empty") || strings.Contains(errStr, "no content") {
			errorType = ErrorTypeEmptyFeed
			message = "Feed is empty or contains no content"
		}
	}

	fe := NewFeedErrorWithCause(errorType, message, err).
		WithURL(feedURL).
		WithOperation("parse_feed").
		WithComponent("feed_parser")

	// Try to extract parsing context from error
	if parseCtx := extractParseContext(err, content); parseCtx != nil {
		fe.WithParseContext(parseCtx)
	}

	return fe
}

// CreateValidationError creates a FeedError for URL validation issues
func CreateValidationError(err error, feedURL string) *FeedError {
	errorType := ErrorTypeValidation
	message := "URL validation failed"

	// Map existing validation errors to our error types
	if err != nil {
		switch err {
		case ErrInvalidURL:
			errorType = ErrorTypeInvalidURL
			message = "Invalid URL format"
		case ErrUnsupportedScheme:
			errorType = ErrorTypeUnsupportedScheme
			message = "Unsupported URL scheme"
		case ErrPrivateIPBlocked:
			errorType = ErrorTypePrivateIP
			message = "Private IP address blocked"
		case ErrMissingHost:
			errorType = ErrorTypeInvalidURL
			message = "URL missing host"
		case ErrEmptyURL:
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
func CreateCircuitBreakerError(feedURL string, state string) *FeedError {
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
	if feedErr, ok := lastErr.(*FeedError); ok {
		errorType = feedErr.ErrorType
	}

	return NewFeedErrorWithCause(errorType, message, lastErr).
		WithURL(feedURL).
		WithOperation("retry_fetch").
		WithComponent("retry_manager").
		WithRetryContext(attempt, maxAttempts, 0)
}

// CreateRateLimitError creates a FeedError for rate limiting
func CreateRateLimitError(feedURL string, retryAfter time.Duration) *FeedError {
	message := "Request rate limit exceeded"

	return NewFeedError(ErrorTypeRateLimit, message).
		WithURL(feedURL).
		WithOperation("fetch_feed").
		WithComponent("rate_limiter").
		WithRetryContext(0, 0, retryAfter)
}

// Helper functions to categorize network errors

// isTimeoutError checks if the error is related to timeouts
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	// Check for context timeout
	if err == context.DeadlineExceeded {
		return true
	}

	// Check for net.Error timeout
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
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
	if dnsErr, ok := err.(*net.DNSError); ok {
		return dnsErr != nil
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

	// Check for specific syscall errors
	if opErr, ok := err.(*net.OpError); ok {
		if syscallErr, ok := opErr.Err.(*syscall.Errno); ok {
			// Common connection errors
			switch *syscallErr {
			case syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.ECONNABORTED:
				return true
			case syscall.EHOSTUNREACH, syscall.ENETUNREACH:
				return true
			}
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

// extractParseContext attempts to extract parsing context from error messages
func extractParseContext(err error, content string) *ParseContext {
	if err == nil {
		return nil
	}

	errStr := err.Error()
	ctx := &ParseContext{}

	// Try to extract line number from common XML parsing errors
	// Format: "XML syntax error on line X"
	if strings.Contains(errStr, "line") {
		parts := strings.Split(errStr, " ")
		for i, part := range parts {
			if part == "line" && i+1 < len(parts) {
				if lineNum, parseErr := strconv.Atoi(parts[i+1]); parseErr == nil {
					ctx.LineNumber = lineNum
					break
				}
			}
		}
	}

	// Determine feed format from content
	contentLower := strings.TrimSpace(strings.ToLower(content))
	if strings.HasPrefix(contentLower, "{") {
		ctx.FeedFormat = "JSON"
	} else if strings.HasPrefix(contentLower, "<") {
		if strings.Contains(contentLower, "<rss") {
			ctx.FeedFormat = "RSS"
		} else if strings.Contains(contentLower, "<feed") {
			ctx.FeedFormat = "Atom"
		} else {
			ctx.FeedFormat = "XML"
		}
	}

	// Extract content snippet around error location
	if ctx.LineNumber > 0 && content != "" {
		lines := strings.Split(content, "\n")
		if ctx.LineNumber <= len(lines) {
			// Get a few lines around the error for context
			start := max(0, ctx.LineNumber-3)
			end := min(len(lines), ctx.LineNumber+2)

			contextLines := lines[start:end]
			ctx.ContentSnippet = strings.Join(contextLines, "\n")
		}
	}

	// Only return context if we found useful information
	if ctx.LineNumber > 0 || ctx.FeedFormat != "" || ctx.ContentSnippet != "" {
		return ctx
	}

	return nil
}

// Helper functions for min/max (Go 1.21+ has built-in versions)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
