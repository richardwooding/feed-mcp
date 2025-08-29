// Package model defines core data structures and error types for the feed MCP server.
package model

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// ErrorType represents different categories of errors that can occur
type ErrorType string

const (
	// ErrorTypeNetwork represents general network-related errors
	ErrorTypeNetwork ErrorType = "network"
	// ErrorTypeTimeout represents request timeout errors
	ErrorTypeTimeout ErrorType = "timeout"
	// ErrorTypeConnectionFailed represents connection establishment failures
	ErrorTypeConnectionFailed ErrorType = "connection_failed"
	// ErrorTypeDNSResolution represents DNS resolution failures
	ErrorTypeDNSResolution ErrorType = "dns_resolution"

	// ErrorTypeHTTP represents general HTTP errors
	ErrorTypeHTTP ErrorType = "http"
	// ErrorTypeHTTPClientError represents HTTP 4xx client errors
	ErrorTypeHTTPClientError ErrorType = "http_client_error" // 4xx
	// ErrorTypeHTTPServerError represents HTTP 5xx server errors
	ErrorTypeHTTPServerError ErrorType = "http_server_error" // 5xx
	// ErrorTypeHTTPRedirect represents HTTP 3xx redirect issues
	ErrorTypeHTTPRedirect ErrorType = "http_redirect" // 3xx with issues

	// ErrorTypeParsing represents feed parsing errors
	ErrorTypeParsing ErrorType = "parsing"
	// ErrorTypeInvalidFormat represents invalid feed format errors
	ErrorTypeInvalidFormat ErrorType = "invalid_format"
	// ErrorTypeEmptyFeed represents empty or no-content feed errors
	ErrorTypeEmptyFeed ErrorType = "empty_feed"
	// ErrorTypeMalformedXML represents malformed XML feed errors
	ErrorTypeMalformedXML ErrorType = "malformed_xml"
	// ErrorTypeMalformedJSON represents malformed JSON feed errors
	ErrorTypeMalformedJSON ErrorType = "malformed_json"

	// ErrorTypeValidation represents URL validation errors
	ErrorTypeValidation ErrorType = "validation"
	// ErrorTypeInvalidURL represents invalid URL format errors
	ErrorTypeInvalidURL ErrorType = "invalid_url"
	// ErrorTypeUnsupportedScheme represents unsupported URL scheme errors
	ErrorTypeUnsupportedScheme ErrorType = "unsupported_scheme"
	// ErrorTypePrivateIP represents private IP address blocked errors
	ErrorTypePrivateIP ErrorType = "private_ip_blocked"

	// ErrorTypeConfiguration represents configuration-related errors
	ErrorTypeConfiguration ErrorType = "configuration"
	// ErrorTypeTransport represents transport configuration errors
	ErrorTypeTransport ErrorType = "transport"

	// ErrorTypeSystem represents system-level errors
	ErrorTypeSystem ErrorType = "system"
	// ErrorTypeCircuitBreaker represents circuit breaker state errors
	ErrorTypeCircuitBreaker ErrorType = "circuit_breaker"
	// ErrorTypeRateLimit represents rate limiting errors
	ErrorTypeRateLimit ErrorType = "rate_limit"
	// ErrorTypeCache represents caching-related errors
	ErrorTypeCache ErrorType = "cache"

	// ErrorTypeInternal represents internal server errors
	ErrorTypeInternal ErrorType = "internal"
	// ErrorTypeUnknown represents unknown or unclassified errors
	ErrorTypeUnknown ErrorType = "unknown"
)

// FeedError represents a structured error with additional context for debugging
type FeedError struct {
	// Core error information
	ID         string    `json:"id"`         // Unique correlation ID for tracking
	Timestamp  time.Time `json:"timestamp"`  // When the error occurred
	ErrorType  ErrorType `json:"error_type"` // Category of error
	Message    string    `json:"message"`    // Human-readable error message
	Suggestion string    `json:"suggestion"` // Actionable suggestion for resolution

	// Context information
	URL       string `json:"url,omitempty"`       // Feed URL that caused the error
	Operation string `json:"operation,omitempty"` // What operation was being performed
	Component string `json:"component,omitempty"` // Which component generated the error

	// HTTP-specific context
	HTTPStatus  int               `json:"http_status,omitempty"`  // HTTP status code
	HTTPHeaders map[string]string `json:"http_headers,omitempty"` // Relevant HTTP headers

	// Network-specific context
	NetworkError string `json:"network_error,omitempty"` // Specific network error details

	// Parsing-specific context
	ParseContext *ParseContext `json:"parse_context,omitempty"` // Context for parsing errors

	// Retry context
	Attempt     int           `json:"attempt,omitempty"`      // Which retry attempt this was
	MaxAttempts int           `json:"max_attempts,omitempty"` // Maximum retry attempts configured
	RetryAfter  time.Duration `json:"retry_after,omitempty"`  // How long before next retry

	// Original error for wrapping
	Cause error `json:"-"` // Original error (not serialized to JSON)
}

// ParseContext provides additional context for parsing errors
type ParseContext struct {
	LineNumber     int    `json:"line_number,omitempty"`     // Line where parsing failed
	ColumnNumber   int    `json:"column_number,omitempty"`   // Column where parsing failed
	ContentSnippet string `json:"content_snippet,omitempty"` // Relevant content around the error
	FeedFormat     string `json:"feed_format,omitempty"`     // Expected format (RSS, Atom, JSON)
}

// Error implements the error interface
func (fe *FeedError) Error() string {
	var parts []string

	// Start with the basic message
	if fe.Message != "" {
		parts = append(parts, fe.Message)
	}

	// Add URL context if available
	if fe.URL != "" {
		parts = append(parts, fmt.Sprintf("URL: %s", fe.URL))
	}

	// Add operation context
	if fe.Operation != "" {
		parts = append(parts, fmt.Sprintf("Operation: %s", fe.Operation))
	}

	// Add HTTP status if relevant
	if fe.HTTPStatus != 0 {
		parts = append(parts, fmt.Sprintf("HTTP Status: %d", fe.HTTPStatus))
	}

	// Add error type and ID for debugging
	parts = append(parts, fmt.Sprintf("Type: %s", fe.ErrorType), fmt.Sprintf("ID: %s", fe.ID))

	return strings.Join(parts, " | ")
}

// Unwrap returns the underlying cause for error wrapping support
func (fe *FeedError) Unwrap() error {
	return fe.Cause
}

// NewFeedError creates a new FeedError with basic information
func NewFeedError(errorType ErrorType, message string) *FeedError {
	id, _ := gonanoid.New() // Generate unique correlation ID

	return &FeedError{
		ID:         id,
		Timestamp:  time.Now().UTC(),
		ErrorType:  errorType,
		Message:    message,
		Suggestion: getSuggestionForErrorType(errorType),
	}
}

// NewFeedErrorWithCause creates a new FeedError wrapping an existing error
func NewFeedErrorWithCause(errorType ErrorType, message string, cause error) *FeedError {
	fe := NewFeedError(errorType, message)
	fe.Cause = cause
	return fe
}

// WithURL adds URL context to the error
func (fe *FeedError) WithURL(url string) *FeedError {
	fe.URL = url
	return fe
}

// WithOperation adds operation context to the error
func (fe *FeedError) WithOperation(operation string) *FeedError {
	fe.Operation = operation
	return fe
}

// WithComponent adds component context to the error
func (fe *FeedError) WithComponent(component string) *FeedError {
	fe.Component = component
	return fe
}

// WithHTTP adds HTTP-specific context to the error
func (fe *FeedError) WithHTTP(status int, headers http.Header) *FeedError {
	fe.HTTPStatus = status

	// Convert selected headers to map for context
	if headers != nil {
		fe.HTTPHeaders = make(map[string]string)

		// Include relevant headers for debugging
		relevantHeaders := []string{
			"Content-Type", "Content-Length", "Server", "Cache-Control",
			"Etag", "Last-Modified", "Retry-After", "X-RateLimit-Remaining",
		}

		for _, header := range relevantHeaders {
			if value := headers.Get(header); value != "" {
				fe.HTTPHeaders[header] = value
			}
		}
	}

	return fe
}

// WithNetworkError adds network-specific context
func (fe *FeedError) WithNetworkError(networkErr string) *FeedError {
	fe.NetworkError = networkErr
	return fe
}

// WithParseContext adds parsing-specific context
func (fe *FeedError) WithParseContext(ctx *ParseContext) *FeedError {
	fe.ParseContext = ctx
	return fe
}

// WithRetryContext adds retry attempt information
func (fe *FeedError) WithRetryContext(attempt, maxAttempts int, retryAfter time.Duration) *FeedError {
	fe.Attempt = attempt
	fe.MaxAttempts = maxAttempts
	fe.RetryAfter = retryAfter
	return fe
}

// getSuggestionForErrorType returns actionable suggestions based on error type
func getSuggestionForErrorType(errorType ErrorType) string {
	suggestions := map[ErrorType]string{
		ErrorTypeTimeout:           "Check network connectivity or increase timeout duration",
		ErrorTypeConnectionFailed:  "Verify the URL is accessible and the server is running",
		ErrorTypeDNSResolution:     "Check DNS settings and verify the domain name is correct",
		ErrorTypeHTTPClientError:   "Verify the URL is correct and accessible",
		ErrorTypeHTTPServerError:   "The server is experiencing issues, try again later",
		ErrorTypeInvalidFormat:     "Ensure the feed URL returns valid RSS, Atom, or JSON feed content",
		ErrorTypeEmptyFeed:         "The feed appears to be empty, check if it contains any items",
		ErrorTypeMalformedXML:      "The feed contains invalid XML, contact the feed provider",
		ErrorTypeMalformedJSON:     "The feed contains invalid JSON, contact the feed provider",
		ErrorTypeInvalidURL:        "Check the URL format and ensure it's a valid HTTP/HTTPS URL",
		ErrorTypeUnsupportedScheme: "Only HTTP and HTTPS URLs are supported",
		ErrorTypePrivateIP:         "Private IP addresses are blocked for security, use --allow-private-ips if needed",
		ErrorTypeCircuitBreaker:    "Service is temporarily unavailable due to repeated failures",
		ErrorTypeRateLimit:         "Request rate limit exceeded, reduce the number of concurrent requests",
		ErrorTypeTransport:         "Check transport configuration (stdio, http-with-sse)",
		ErrorTypeConfiguration:     "Review configuration parameters for correctness",
		ErrorTypeSystem:            "Check system resources and permissions",
		ErrorTypeInternal:          "Internal server error occurred, check logs for details",
	}

	if suggestion, exists := suggestions[errorType]; exists {
		return suggestion
	}

	return "Check the error details and try again"
}
