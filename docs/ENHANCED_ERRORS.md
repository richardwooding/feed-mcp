# Enhanced Error Context and Debugging

This document describes the enhanced error handling and debugging features added to feed-mcp.

## Enhanced Error Types

feed-mcp now provides structured error information with rich context to help with debugging and troubleshooting.

### Error Structure

All errors now include:

- **Unique correlation ID** for tracking related failures
- **Timestamps** showing when errors occurred  
- **Error categorization** (network, HTTP, parsing, validation, etc.)
- **Contextual information** (URLs, operations, components)
- **Actionable suggestions** for resolving issues
- **HTTP details** (status codes, relevant headers)
- **Retry context** (attempt numbers, max attempts)
- **Parsing context** (line numbers, content snippets for malformed feeds)

### Error Categories

| Error Type | Description | Example Scenarios |
|------------|-------------|-------------------|
| `network` | Network connectivity issues | General network problems |
| `timeout` | Request timeouts | Slow server responses |
| `connection_failed` | Connection failures | Server unreachable, connection refused |
| `dns_resolution` | DNS lookup failures | Invalid domain names |
| `http_client_error` | HTTP 4xx errors | 404 Not Found, 403 Forbidden |
| `http_server_error` | HTTP 5xx errors | 500 Internal Server Error |
| `parsing` | Feed parsing failures | Malformed XML/JSON |
| `validation` | URL validation failures | Invalid URLs, private IPs |
| `circuit_breaker` | Circuit breaker triggered | Service temporarily unavailable |
| `rate_limit` | Rate limiting active | Too many requests |

### Example Error Output

```
Request timed out | URL: https://example.com/feed.xml | Operation: fetch_feed | HTTP Status: 408 | Type: timeout | ID: abc123def456
```

## Debug Logging

Enable enhanced debug logging to get detailed information about feed fetching, caching, retries, and errors.

### Environment Variables

Configure debug logging using these environment variables:

```bash
# Enable debug mode
export FEED_MCP_DEBUG=true

# Set log level (ERROR, WARN, INFO, DEBUG)
export FEED_MCP_LOG_LEVEL=DEBUG

# Enable JSON formatted logs (useful for log analysis tools)
export FEED_MCP_JSON_LOGS=true
```

### Debug Output Examples

**Text Format:**
```
2025-08-29T10:15:30.123Z [DEBUG] Successfully fetched feed component=feed_fetcher operation=retryable_fetch url=https://example.com/feed.xml items_count=25

2025-08-29T10:15:35.456Z [DEBUG] Feed fetch attempt 1 failed component=feed_fetcher operation=retryable_fetch url=https://example.com/feed.xml attempt=1 max_attempts=3 error="connection refused" retryable=true

2025-08-29T10:15:35.500Z [DEBUG] Retrying in 2s component=feed_fetcher operation=retryable_fetch url=https://example.com/feed.xml attempt=1 next_attempt=2 delay_ms=2000
```

**JSON Format:**
```json
{
  "timestamp": "2025-08-29T10:15:30.123Z",
  "level": "DEBUG",
  "message": "Successfully fetched feed",
  "component": "feed_fetcher",
  "operation": "retryable_fetch", 
  "url": "https://example.com/feed.xml",
  "extra": {
    "items_count": 25
  }
}
```

## Error Context Examples

### Network Errors

Network errors include specific categorization:

```go
// Timeout error
{
  "id": "abc123",
  "timestamp": "2025-08-29T10:15:30Z",
  "error_type": "timeout", 
  "message": "Request timed out",
  "suggestion": "Check network connectivity or increase timeout duration",
  "url": "https://example.com/feed.xml",
  "operation": "fetch_feed",
  "component": "http_client"
}

// DNS resolution error  
{
  "error_type": "dns_resolution",
  "message": "DNS resolution failed", 
  "suggestion": "Check DNS settings and verify the domain name is correct"
}

// Connection refused
{
  "error_type": "connection_failed",
  "message": "Connection failed",
  "suggestion": "Verify the URL is accessible and the server is running"
}
```

### HTTP Errors

HTTP errors include status codes and relevant response headers:

```go
{
  "error_type": "http_server_error",
  "message": "Server error: 503 Service Unavailable", 
  "http_status": 503,
  "http_headers": {
    "Server": "nginx/1.14.0",
    "Retry-After": "300",
    "Content-Type": "text/html"
  },
  "suggestion": "The server is experiencing issues, try again later"
}
```

### Parsing Errors

Parsing errors include context about where parsing failed:

```go
{
  "error_type": "malformed_xml",
  "message": "Feed contains malformed XML",
  "parse_context": {
    "line_number": 42,
    "column_number": 15, 
    "content_snippet": "<item>\n  <title>Article Title\n  <description>Missing closing title tag",
    "feed_format": "RSS"
  },
  "suggestion": "The feed contains invalid XML, contact the feed provider"
}
```

### Retry Context

Errors from retry operations include attempt information:

```go
{
  "error_type": "network",
  "message": "All retry attempts exhausted (3/3)",
  "attempt": 3,
  "max_attempts": 3,
  "suggestion": "Check network connectivity or increase timeout duration"
}
```

## Circuit Breaker Context

Circuit breaker errors include state information:

```go
{
  "error_type": "circuit_breaker", 
  "message": "Circuit breaker is open",
  "suggestion": "Service is temporarily unavailable due to repeated failures"
}
```

## Usage Tips

### 1. Enable Debug Logging During Development

```bash
export FEED_MCP_DEBUG=true
export FEED_MCP_LOG_LEVEL=DEBUG
go run main.go run https://example.com/feed.xml
```

### 2. Use JSON Logs for Production Analysis

```bash
export FEED_MCP_DEBUG=true  
export FEED_MCP_JSON_LOGS=true
go run main.go run https://feeds.example.com/rss | jq '.'
```

### 3. Filter Logs by Component

```bash
# Show only feed fetcher logs
go run main.go run https://example.com/feed.xml 2>&1 | grep "feed_fetcher"

# Show only circuit breaker events
go run main.go run https://example.com/feed.xml 2>&1 | grep "circuit_breaker"
```

### 4. Monitor Retry Patterns

Debug logs help identify problematic feeds that require frequent retries:

```bash
# Look for retry patterns
go run main.go run https://unreliable.example.com/feed.xml 2>&1 | grep -i retry
```

### 5. Analyze Error Correlation IDs

Use correlation IDs to trace related errors across different components:

```bash
# Find all log entries for a specific error ID
go run main.go run https://example.com/feed.xml 2>&1 | grep "abc123def456"
```

## Benefits

1. **Faster Troubleshooting**: Detailed error context helps identify root causes quickly
2. **Better Monitoring**: Structured errors enable better alerting and monitoring
3. **Improved User Experience**: Actionable suggestions help users resolve issues
4. **Enhanced Debugging**: Debug logs provide visibility into internal operations
5. **Correlation Tracking**: Unique error IDs help trace issues across components
6. **Production Ready**: JSON logs integrate well with log analysis tools