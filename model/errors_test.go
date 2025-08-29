package model

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNewFeedError(t *testing.T) {
	err := NewFeedError(ErrorTypeTimeout, "request timed out")

	if err.ErrorType != ErrorTypeTimeout {
		t.Errorf("expected ErrorType %v, got %v", ErrorTypeTimeout, err.ErrorType)
	}

	if err.Message != "request timed out" {
		t.Errorf("expected message 'request timed out', got %q", err.Message)
	}

	if err.ID == "" {
		t.Error("expected non-empty correlation ID")
	}

	if err.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}

	if err.Suggestion == "" {
		t.Error("expected non-empty suggestion")
	}
}

func TestFeedError_Error(t *testing.T) {
	err := NewFeedError(ErrorTypeTimeout, "request timed out").
		WithURL("https://example.com/feed.xml").
		WithOperation("fetch_feed").
		WithHTTP(408, nil)

	errStr := err.Error()

	// Check that all important information is present
	expectedParts := []string{
		"request timed out",
		"URL: https://example.com/feed.xml",
		"Operation: fetch_feed",
		"HTTP Status: 408",
		"Type: timeout",
		"ID:",
	}

	for _, part := range expectedParts {
		if !strings.Contains(errStr, part) {
			t.Errorf("expected error string to contain %q, got %q", part, errStr)
		}
	}
}

func TestFeedError_Unwrap(t *testing.T) {
	originalErr := errors.New("original error")
	feedErr := NewFeedErrorWithCause(ErrorTypeNetwork, "network error", originalErr)

	unwrapped := feedErr.Unwrap()
	if unwrapped != originalErr {
		t.Errorf("expected unwrapped error %v, got %v", originalErr, unwrapped)
	}
}

func TestFeedError_WithHTTP(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Content-Type", "application/xml")
	headers.Set("Server", "nginx/1.14.0")
	headers.Set("Cache-Control", "max-age=3600")

	err := NewFeedError(ErrorTypeHTTPServerError, "server error").
		WithHTTP(503, headers)

	if err.HTTPStatus != 503 {
		t.Errorf("expected HTTP status 503, got %d", err.HTTPStatus)
	}

	if err.HTTPHeaders["Content-Type"] != "application/xml" {
		t.Errorf("expected Content-Type header, got %v", err.HTTPHeaders)
	}

	if err.HTTPHeaders["Server"] != "nginx/1.14.0" {
		t.Errorf("expected Server header, got %v", err.HTTPHeaders)
	}
}

func TestFeedError_WithParseContext(t *testing.T) {
	parseCtx := &ParseContext{
		LineNumber:     42,
		ColumnNumber:   15,
		ContentSnippet: "<item>malformed xml",
		FeedFormat:     "RSS",
	}

	err := NewFeedError(ErrorTypeMalformedXML, "XML parsing error").
		WithParseContext(parseCtx)

	if err.ParseContext.LineNumber != 42 {
		t.Errorf("expected line number 42, got %d", err.ParseContext.LineNumber)
	}

	if err.ParseContext.FeedFormat != "RSS" {
		t.Errorf("expected feed format RSS, got %s", err.ParseContext.FeedFormat)
	}
}

func TestFeedError_WithRetryContext(t *testing.T) {
	err := NewFeedError(ErrorTypeNetwork, "network error").
		WithRetryContext(3, 5, 30*time.Second)

	if err.Attempt != 3 {
		t.Errorf("expected attempt 3, got %d", err.Attempt)
	}

	if err.MaxAttempts != 5 {
		t.Errorf("expected max attempts 5, got %d", err.MaxAttempts)
	}

	if err.RetryAfter != 30*time.Second {
		t.Errorf("expected retry after 30s, got %v", err.RetryAfter)
	}
}

func TestGetSuggestionForErrorType(t *testing.T) {
	testCases := []struct {
		errorType        ErrorType
		expectedKeywords []string
	}{
		{
			errorType:        ErrorTypeTimeout,
			expectedKeywords: []string{"network", "timeout"},
		},
		{
			errorType:        ErrorTypeHTTPClientError,
			expectedKeywords: []string{"URL", "accessible"},
		},
		{
			errorType:        ErrorTypePrivateIP,
			expectedKeywords: []string{"private", "allow-private-ips"},
		},
		{
			errorType:        ErrorTypeMalformedXML,
			expectedKeywords: []string{"XML", "provider"},
		},
	}

	for _, tc := range testCases {
		t.Run(string(tc.errorType), func(t *testing.T) {
			suggestion := getSuggestionForErrorType(tc.errorType)

			if suggestion == "" {
				t.Errorf("expected non-empty suggestion for error type %v", tc.errorType)
			}

			suggestionLower := strings.ToLower(suggestion)
			for _, keyword := range tc.expectedKeywords {
				if !strings.Contains(suggestionLower, strings.ToLower(keyword)) {
					t.Errorf("expected suggestion for %v to contain %q, got %q",
						tc.errorType, keyword, suggestion)
				}
			}
		})
	}
}

func TestCreateNetworkError(t *testing.T) {
	testCases := []struct {
		name         string
		inputError   error
		expectedType ErrorType
		expectedMsg  string
	}{
		{
			name:         "timeout error",
			inputError:   fmt.Errorf("context deadline exceeded"),
			expectedType: ErrorTypeTimeout,
			expectedMsg:  "Request timed out",
		},
		{
			name:         "DNS error",
			inputError:   fmt.Errorf("no such host"),
			expectedType: ErrorTypeDNSResolution,
			expectedMsg:  "DNS resolution failed",
		},
		{
			name:         "connection error",
			inputError:   fmt.Errorf("connection refused"),
			expectedType: ErrorTypeConnectionFailed,
			expectedMsg:  "Connection failed",
		},
		{
			name:         "generic network error",
			inputError:   fmt.Errorf("network unreachable"),
			expectedType: ErrorTypeConnectionFailed,
			expectedMsg:  "Connection failed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := CreateNetworkError(tc.inputError, "https://example.com/feed.xml")

			if err.ErrorType != tc.expectedType {
				t.Errorf("expected error type %v, got %v", tc.expectedType, err.ErrorType)
			}

			if err.Message != tc.expectedMsg {
				t.Errorf("expected message %q, got %q", tc.expectedMsg, err.Message)
			}

			if err.URL != "https://example.com/feed.xml" {
				t.Errorf("expected URL to be set, got %q", err.URL)
			}

			if err.Operation != "fetch_feed" {
				t.Errorf("expected operation to be 'fetch_feed', got %q", err.Operation)
			}
		})
	}
}

func TestCreateHTTPError(t *testing.T) {
	testCases := []struct {
		name         string
		statusCode   int
		expectedType ErrorType
	}{
		{
			name:         "client error 404",
			statusCode:   404,
			expectedType: ErrorTypeHTTPClientError,
		},
		{
			name:         "server error 500",
			statusCode:   500,
			expectedType: ErrorTypeHTTPServerError,
		},
		{
			name:         "redirect error 301",
			statusCode:   301,
			expectedType: ErrorTypeHTTPRedirect,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{
				StatusCode: tc.statusCode,
				Status:     fmt.Sprintf("%d Status", tc.statusCode),
				Header:     make(http.Header),
			}
			resp.Header.Set("Content-Type", "text/html")

			err := CreateHTTPError(resp, "https://example.com/feed.xml")

			if err.ErrorType != tc.expectedType {
				t.Errorf("expected error type %v, got %v", tc.expectedType, err.ErrorType)
			}

			if err.HTTPStatus != tc.statusCode {
				t.Errorf("expected HTTP status %d, got %d", tc.statusCode, err.HTTPStatus)
			}

			if err.HTTPHeaders["Content-Type"] != "text/html" {
				t.Errorf("expected Content-Type header to be preserved")
			}
		})
	}
}

func TestCreateValidationError(t *testing.T) {
	testCases := []struct {
		name         string
		inputError   error
		expectedType ErrorType
	}{
		{
			name:         "invalid URL",
			inputError:   ErrInvalidURL,
			expectedType: ErrorTypeInvalidURL,
		},
		{
			name:         "unsupported scheme",
			inputError:   ErrUnsupportedScheme,
			expectedType: ErrorTypeUnsupportedScheme,
		},
		{
			name:         "private IP blocked",
			inputError:   ErrPrivateIPBlocked,
			expectedType: ErrorTypePrivateIP,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := CreateValidationError(tc.inputError, "invalid-url")

			if err.ErrorType != tc.expectedType {
				t.Errorf("expected error type %v, got %v", tc.expectedType, err.ErrorType)
			}

			if err.Cause != tc.inputError {
				t.Errorf("expected cause to be preserved, got %v", err.Cause)
			}
		})
	}
}
