# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

**🚀 Quick Start:** This project includes a comprehensive `Makefile` for all development tasks. Run `make help` to see all available targets.

### Common Tasks (using Makefile)
```bash
# Show all available targets with descriptions
make help

# Set up development environment
make dev-setup

# Development workflow - format, fix, and test
make dev

# Build all packages
make build

# Run comprehensive checks (format, vet, lint, test)
make check

# Run tests with various options
make test                  # All tests
make test-verbose         # Verbose output
make test-race           # With race detector
make test-coverage       # With coverage
make test-coverage-html  # Generate HTML coverage report

# Linting and formatting
make fmt        # Format code
make vet        # Run go vet
make lint       # Run comprehensive linting
make lint-fix   # Lint with auto-fix
make fix        # Format + lint-fix

# Run the application with example feeds
make run               # Tech news feeds
make run-security      # Security feeds  
make run-reddit        # Reddit feeds

# Clean up
make clean      # Remove build artifacts
make clean-all  # Clean all build artifacts and cache
```

### Direct Go Commands (alternative to Makefile)
```bash
# Build all packages
go build ./...

# Run the MCP server locally
go run main.go run <feed-urls>

# Example with multiple feeds
go run main.go run https://techcrunch.com/feed/ https://www.wired.com/feed/rss

# Example with custom retry configuration
go run main.go run --retry-max-attempts 5 --retry-base-delay 2s --retry-max-delay 60s https://unreliable-feed.example.com/rss
```

### Testing (Makefile vs Direct)
```bash
# Using Makefile (recommended)
make test                 # Run all tests
make test-verbose        # Verbose output
make test-race          # Race detector
make test-coverage      # With coverage
make test-coverage-html # HTML coverage report

# Direct Go commands
go test ./...           # Run all tests (unit tests + BDD tests)
go test -cover ./...    # Run tests with coverage
go test -v ./...        # Run tests with verbose output
go test -run TestFunctionName ./package_name  # Run a specific test
go test ./model         # Run BDD tests (Cucumber/Godog)
go test -race ./...     # Run tests with race detector
```

### Linting and Formatting (Makefile vs Direct)
```bash
# Using Makefile (recommended)
make fmt        # Format code
make vet        # Run go vet
make lint       # Run comprehensive linting  
make lint-fix   # Lint with auto-fix
make fix        # Format + lint-fix combined

# Direct Go commands
go fmt ./...    # Format code
go vet ./...    # Run go vet for static analysis

# Install and run golangci-lint v2 (comprehensive linting)
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v2.4.0
$(go env GOPATH)/bin/golangci-lint run
$(go env GOPATH)/bin/golangci-lint run --fix  # Run with auto-fix
```

### Pre-commit Hooks (Optional)
To automatically run linting before commits, you can set up pre-commit hooks:

```bash
# Using Makefile (recommended - handles installation and setup)
make pre-commit-install

# Manual setup (if you prefer direct control)
# 1. Install pre-commit (requires Python)
pip install pre-commit

# 2. Create .pre-commit-config.yaml in project root
cat > .pre-commit-config.yaml << 'EOF'
repos:
  - repo: local
    hooks:
      - id: go-fmt
        name: go fmt
        language: system
        entry: go fmt ./...
        files: \.go$
      
      - id: golangci-lint
        name: golangci-lint
        language: system
        entry: golangci-lint run --fix
        files: \.go$
        pass_filenames: false
EOF

# 3. Install the git hooks
pre-commit install
```

Now golangci-lint will run automatically on every commit.

### Docker

**Note:** Docker images are built via CI/CD using `ko` and `goreleaser`. No local Docker targets are provided in the Makefile.

```bash
# Use official images from CI/CD
docker run -i --rm ghcr.io/richardwooding/feed-mcp:latest run <feed-urls>
```

## Project Overview

This is an MCP (Model Context Protocol) server that fetches and serves RSS/Atom/JSON feeds to AI assistants like Claude. It acts as a bridge between web feeds and AI tools, enabling them to access real-time content from news sites, blogs, and other syndicated sources.

## High-Level Architecture

The architecture follows clean Go patterns with strong separation of concerns:

### Core Flow
1. **CLI Entry** (`main.go`) → Kong parses command arguments
2. **Store Initialization** → Creates feed store with caching layer
3. **Feed Fetching** → Concurrent fetching of all configured feeds
4. **MCP Server** → Exposes feeds via MCP protocol tools
5. **Transport Layer** → Handles stdio or HTTP-SSE communication

### Package Structure

**`model/` - Data Structures and Types**
- Core domain models (`Feed`, `Item`, `Author`)
- Transport enums (stdio, http-with-sse)
- Adapter functions (`FromGoFeed()` converts external → internal types)
- Global configuration and constants

**`store/` - Feed Management Layer**
- `Store` type manages feed fetching, caching, and retrieval
- Implements `AllFeedsGetter` and `FeedAndItemsGetter` interfaces
- Uses gocache + ristretto for efficient in-memory caching
- Concurrent feed fetching with goroutines and sync.WaitGroup
- Configuration includes timeout, cache expiration, rate limiting, and circuit breaker settings
- Built-in rate limiting with configurable requests per second and burst capacity
- Circuit breaker pattern for fault tolerance using sony/gobreaker library

**`mcpserver/` - MCP Protocol Implementation**
- Uses official MCP Go SDK
- Exposes three MCP tools:
  - `all_syndication_feeds` - Lists available feeds
  - `get_syndication_feed_items` - Gets specific feed with items
  - `fetch_link` - Fetches arbitrary URL content (uses Colly)
- Session management with atomic counters for unique IDs

**`cmd/` - CLI Commands**
- `RunCmd` struct implements the main `run` command
- Handles transport selection and server initialization
- Supports graceful shutdown with configurable timeout

### Key Design Patterns

**Factory Pattern**: All major components use constructors:
- `NewStore(Config)` - Creates configured store with validation
- `NewServer(Config)` - Initializes MCP server

**Interface Segregation**: Small, focused interfaces:
- `AllFeedsGetter` - For listing feeds
- `FeedAndItemsGetter` - For retrieving feed content

**Adapter Pattern**: 
- `FromGoFeed()` adapts third-party gofeed types to internal models

**Error Handling**:
- Early returns on errors (`if err != nil { return ... }`)
- Custom validation errors (e.g., `ErrInvalidTransport`)
- Error as last return parameter convention

### Testing Strategy

**Unit Tests** (`*_test.go`):
- Standard Go testing package
- Helper functions for mocking (e.g., `mockFeedServer`)
- Coverage for all core logic

**BDD Tests** (`model/features/*.feature`):
- Cucumber/Godog for behavior specifications
- Tests domain logic and conversions

**Integration Tests** (`integration_test.go`):
- End-to-end testing of feed fetching and caching

### Dependencies

Core libraries that shape the architecture:
- `github.com/mmcdole/gofeed` - Feed parsing (RSS/Atom/JSON)
- `github.com/eko/gocache` + `github.com/dgraph-io/ristretto` - Caching layer
- `github.com/modelcontextprotocol/go-sdk` - MCP protocol
- `github.com/alecthomas/kong` - CLI framework
- `github.com/gocolly/colly` - Web scraping for fetch_link
- `github.com/sony/gobreaker` - Circuit breaker pattern implementation
- `golang.org/x/time/rate` - Token bucket rate limiter

### Common Feed URLs for Testing

```bash
# Tech news feeds
go run main.go run \
  https://techcrunch.com/feed/ \
  https://www.theverge.com/rss/index.xml

# Security feeds
go run main.go run \
  https://krebsonsecurity.com/feed/ \
  https://www.schneier.com/blog/atom.xml

# Reddit feeds (JSON format)
go run main.go run \
  https://www.reddit.com/r/golang/.rss \
  https://www.reddit.com/r/mcp/.rss
```

### Concurrency Model

- Goroutines with `sync.WaitGroup` for parallel feed fetching at startup
- Atomic operations for session ID generation
- Thread-safe cache operations via gocache/ristretto
- Context-based cancellation for graceful shutdown

### Configuration Flow

1. CLI args parsed by Kong → `RunCmd` struct
2. Store config with defaults (timeout: 30s, cache expiry: 1h, rate limit: 2 req/s, circuit breaker enabled, connection pooling enabled, retry enabled with 3 attempts)
3. Server config includes store, transport, and feed URLs
4. Validation at each layer with meaningful error messages

### Rate Limiting

The feed-mcp server includes built-in rate limiting to be respectful to feed servers:

**Default Settings:**
- 2 requests per second
- Burst capacity of 5 requests
- Applied to all HTTP requests made by the feed parser

**How it Works:**
- Uses `golang.org/x/time/rate` for token bucket rate limiting
- Implements a custom `RateLimitedTransport` that wraps `http.RoundTripper`
- Automatically applied when no custom HTTP client is provided
- Rate limiting occurs at the HTTP transport layer, ensuring all feed requests are controlled

**Configuration:**
Rate limiting is configured in the `store.Config` struct:
```go
config := store.Config{
    Feeds:             []string{"https://example.com/feed.xml"},
    RequestsPerSecond: 1.0,  // 1 request per second
    BurstCapacity:     3,    // Allow burst of 3 requests
}
```

**Customization:**
- Pass a custom `HttpClient` to bypass built-in rate limiting
- Adjust `RequestsPerSecond` and `BurstCapacity` for different rate limits
- Set to 0 or negative values to use sensible defaults

### Circuit Breaker Pattern

The feed-mcp server implements a circuit breaker pattern to handle failing feeds gracefully:

**Purpose:**
- Temporarily stop fetching from consistently failing feeds
- Allow failing feeds time to recover without overwhelming them
- Provide fault tolerance and improved overall system resilience

**Default Settings:**
- Circuit breaker is **enabled** by default
- 3 consecutive failures before opening the circuit (configurable)
- 30 second timeout before attempting half-open state
- 3 maximum requests allowed in half-open state
- 60 second interval for statistical calculations

**How it Works:**
- Uses `github.com/sony/gobreaker` for circuit breaker implementation  
- Each feed gets its own circuit breaker instance
- Circuit states: Closed (normal) → Open (failing) → Half-Open (testing) → Closed
- When circuit is open, requests fail fast without attempting network calls
- Feed results include `CircuitBreakerOpen` field to indicate circuit state

**Configuration:**
Circuit breakers are configured in the `store.Config` struct:
```go
config := store.Config{
    Feeds:                          []string{"https://example.com/feed.xml"},
    // CircuitBreakerEnabled is enabled by default, set to &false to disable
    CircuitBreakerMaxRequests:      5,                    // Half-open state requests
    CircuitBreakerInterval:         2 * time.Minute,     // Statistical interval  
    CircuitBreakerTimeout:          45 * time.Second,    // Open state timeout
    CircuitBreakerFailureThreshold: 5,                   // Failures before opening circuit
}

// To explicitly disable circuit breakers:
disabled := false
config := store.Config{
    Feeds:                 []string{"https://example.com/feed.xml"},
    CircuitBreakerEnabled: &disabled,
}
```

**States and Behavior:**
- **Closed**: Feed requests work normally, failures are counted
- **Open**: All requests fail immediately, no network calls made  
- **Half-Open**: Limited requests allowed to test if feed has recovered

**Customization:**
- Circuit breakers are **enabled by default** - no configuration needed for basic functionality
- Disable with `CircuitBreakerEnabled: &false` (requires pointer to bool)
- Adjust failure threshold with `CircuitBreakerFailureThreshold` (default: 3 failures)
- Configure timeouts and intervals based on feed characteristics
- Monitor circuit breaker state via the `CircuitBreakerOpen` field in responses

### HTTP Connection Pooling

The feed-mcp server implements optimized HTTP connection pooling for improved performance when fetching multiple feeds:

**Purpose:**
- Reuse existing HTTP connections to the same hosts
- Reduce connection establishment overhead
- Improve performance for multiple feed fetches
- Prevent connection exhaustion under high load

**Default Settings:**
- 100 maximum idle connections across all hosts
- 10 maximum connections per host
- 5 maximum idle connections per host  
- 90-second idle connection timeout

**How it Works:**
- Uses Go's `http.Transport` with optimized pooling settings
- Integrates with existing rate limiting functionality
- Automatically applied when no custom HTTP client is provided
- Connections are kept alive and reused when possible
- Idle connections are cleaned up after timeout

**Configuration:**
Connection pooling is configured via CLI flags:
```bash
# Use default optimized settings
go run main.go run <feed-urls>

# Custom connection pool settings
go run main.go run \
  --max-idle-conns 200 \
  --max-conns-per-host 20 \
  --max-idle-conns-per-host 10 \
  --idle-conn-timeout 120s \
  <feed-urls>
```

**Programmatic Configuration:**
Connection pooling settings can also be configured in the `store.Config` struct:
```go
config := store.Config{
    Feeds:                []string{"https://example.com/feed.xml"},
    MaxIdleConns:         150,    // Total idle connections
    MaxConnsPerHost:      15,     // Connections per host
    MaxIdleConnsPerHost: 8,      // Idle connections per host
    IdleConnTimeout:     2 * time.Minute, // Keep-alive timeout
}
```

**Performance Benefits:**
- Reduces memory allocations by ~13% (4459 → 3871 allocs/op)
- Decreases memory usage by ~23% (518KB → 397KB per operation)
- Prevents connection exhaustion when fetching many feeds
- Improves overall feed fetching latency through connection reuse

**Monitoring:**
- Connection pool effectiveness can be observed through reduced HTTP connection establishment
- Monitor for `dial tcp` errors which indicate connection exhaustion
- Use Go's HTTP client metrics to track connection reuse rates

### Retry Mechanism with Exponential Backoff

The feed-mcp server implements automatic retry with exponential backoff and jitter to handle transient network failures:

**Purpose:**
- Handle temporary network issues, DNS failures, and server errors gracefully
- Implement exponential backoff to avoid overwhelming failing servers
- Add jitter to prevent thundering herd problems with multiple feeds
- Improve overall reliability and success rates for feed fetching

**Default Settings:**
- 3 maximum retry attempts per feed
- 1 second base delay between retries
- 30 seconds maximum delay cap
- Jitter enabled by default to add randomness

**Error Classification:**
- **Retryable Errors**: 5xx server errors, DNS failures, connection refused, timeouts, network unreachable
- **Non-Retryable Errors**: 4xx client errors (like 404 Not Found), context cancellation, invalid URLs
- Smart classification prevents wasted retry attempts on permanent failures

**How it Works:**
- Uses exponential backoff algorithm: delay = baseDelay × 2^(attempt-1)
- Adds jitter (±50% random variation) to prevent synchronized retry storms
- Integrates with existing circuit breaker and connection pooling systems
- Tracks detailed metrics for retry attempts and success rates
- Respects context cancellation for graceful shutdown

**Configuration via CLI:**
```bash
# Use default retry settings (3 attempts, 1s base, 30s max, jitter enabled)
go run main.go run <feed-urls>

# Custom retry configuration
go run main.go run \
  --retry-max-attempts 5 \
  --retry-base-delay 2s \
  --retry-max-delay 60s \
  --retry-jitter=false \
  <feed-urls>
```

**Programmatic Configuration:**
Retry settings can be configured in the `store.Config` struct:
```go
config := store.Config{
    Feeds:            []string{"https://example.com/feed.xml"},
    RetryMaxAttempts: 5,                    // Max retry attempts
    RetryBaseDelay:   2 * time.Second,      // Base delay
    RetryMaxDelay:    60 * time.Second,     // Maximum delay cap
    RetryJitter:      true,                 // Enable jitter
}
```

**Retry Metrics:**
The system tracks comprehensive retry metrics accessible via `store.GetRetryMetrics()`:
- `TotalAttempts`: Total HTTP requests made (including retries)
- `TotalRetries`: Number of retry attempts (excluding initial requests)
- `SuccessfulFeeds`: Count of feeds that succeeded (eventually)
- `FailedFeeds`: Count of feeds that failed after all retries
- `RetrySuccessRate`: Percentage of feeds that succeeded

**Best Practices:**
- Default settings work well for most feeds - no configuration needed
- Increase `RetryMaxAttempts` for unreliable feeds (up to 5-10)
- Decrease `RetryBaseDelay` for fast-recovering issues (down to 500ms)
- Increase `RetryMaxDelay` for feeds with long recovery times
- Keep jitter enabled unless you have specific requirements
- Monitor retry metrics to tune settings based on feed behavior

**Integration:**
- Works seamlessly with circuit breakers (retries happen before circuit opens)
- Respects rate limiting (each retry attempt follows rate limits)
- Uses existing HTTP connection pooling for efficiency
- Properly handles context cancellation during shutdown

### Graceful Shutdown

The feed-mcp server implements graceful shutdown to ensure clean termination and prevent resource leaks:

**Signal Handling:**
- Listens for SIGINT (Ctrl+C) and SIGTERM signals
- Automatically initiates graceful shutdown when signal is received
- Uses Go's `os/signal` package for cross-platform signal handling

**Context Propagation:**
- All server operations use Go contexts for cancellation
- Context cancellation propagates through all components:
  - MCP server operations
  - Feed fetching routines
  - HTTP transport connections

**Shutdown Timeout:**
- Configurable shutdown timeout (default: 30 seconds)
- Ensures server doesn't hang indefinitely during shutdown
- Can be configured via `--shutdown-timeout` CLI flag

**Shutdown Process:**
1. Signal received (SIGINT/SIGTERM)
2. Context cancellation propagated to all components
3. MCP server stops accepting new requests
4. Ongoing operations complete or timeout
5. Server exits cleanly

**Configuration:**
```bash
# Set custom shutdown timeout
go run main.go run --shutdown-timeout 10s <feed-urls>

# Default timeout is 30 seconds
go run main.go run <feed-urls>
```

**Testing:**
- Comprehensive tests verify graceful shutdown behavior
- Tests ensure server shuts down within expected timeouts
- Context cancellation is properly tested across all components

### URL Security and Validation

The feed-mcp server implements comprehensive URL validation and sanitization to prevent security vulnerabilities:

**Security Features:**
- **SSRF Prevention:** Blocks Server Side Request Forgery attacks by validating URLs
- **Scheme Restriction:** Only HTTP and HTTPS protocols are allowed
- **Private IP Blocking:** Prevents access to internal network resources by default
- **Input Sanitization:** Validates and sanitizes all URL inputs before processing

**URL Validation Process:**
1. **Format Validation:** Ensures URL is properly formatted and parseable
2. **Scheme Check:** Rejects non-HTTP/HTTPS schemes (file://, ftp://, javascript:, etc.)
3. **Host Validation:** Blocks private IP ranges and localhost unless explicitly allowed
4. **DNS Resolution:** Validates that hostnames don't resolve to private IPs

**Private IP Ranges Blocked:**
- `10.0.0.0/8` - Private class A networks
- `172.16.0.0/12` - Private class B networks  
- `192.168.0.0/16` - Private class C networks
- `127.0.0.0/8` - Loopback addresses (localhost)
- `169.254.0.0/16` - Link-local addresses
- IPv6 loopback (`::1`) and link-local addresses
- IPv6 unique local addresses (`fc00::/7`)

**CLI Configuration:**
```bash
# Default behavior - private IPs blocked for security
go run main.go run https://example.com/feed.xml

# Allow private IPs and localhost (use with caution)
go run main.go run --allow-private-ips https://localhost/feed.xml

# Mixed example - valid public URLs with private IP override
go run main.go run --allow-private-ips \
  https://techcrunch.com/feed/ \
  http://192.168.1.100/api/feed.json
```

**Programmatic Configuration:**
```go
// In store.Config, URLs are validated before store creation
config := store.Config{
    Feeds: []string{
        "https://example.com/feed.xml",  // Valid
        "http://192.168.1.1/feed",       // Blocked by default
        "file:///etc/passwd",            // Always blocked
    },
}

// URL validation happens in cmd.Run() before store creation
err := model.SanitizeFeedURLs(config.Feeds, allowPrivateIPs)
if err != nil {
    // Handle security validation errors
}
```

**Error Handling:**
The URL validation system provides clear, actionable error messages:
- `ErrUnsupportedScheme` - For non-HTTP/HTTPS schemes
- `ErrPrivateIPBlocked` - When private IPs are detected and blocked
- `ErrInvalidURL` - For malformed or invalid URLs
- `ErrMissingHost` - When URLs lack proper hostnames
- `ErrEmptyURL` - For empty or whitespace-only URLs

**Security Testing:**
```go
// Comprehensive test coverage includes:
func TestURLValidation(t *testing.T) {
    // Valid URL schemes
    testValidURL("https://example.com/feed")
    
    // Invalid schemes blocked
    testInvalidURL("file:///etc/passwd")
    testInvalidURL("javascript:alert('xss')")
    
    // Private IP blocking
    testBlockedURL("http://127.0.0.1/feed")
    testBlockedURL("http://192.168.1.1/api")
    
    // Bypass attempt detection
    testBypassAttempts("http://localhost@example.com/")
}
```

**Best Practices:**
- Keep `--allow-private-ips` disabled unless specifically needed for local development
- Always use HTTPS URLs when possible for encrypted transport
- Validate feed URLs in development before deploying to production
- Monitor logs for blocked URL attempts that might indicate attacks
- Consider additional network-level restrictions for defense in depth

**Integration Points:**
- URL validation occurs in `cmd.Run()` before any network operations
- Validation is fail-fast to prevent expensive operations on invalid URLs
- Errors are user-friendly and actionable for debugging
- Security is enabled by default with opt-out rather than opt-in behavior

## Important Notes

### GitHub Actions and CI/CD
- The project uses GitHub Actions for CI/CD
- Coverage reports are automatically generated and displayed as a badge
- Branch protection requires PR reviews and passing tests
- Docker images are automatically built and pushed to GitHub Container Registry

### Working with MCP Tools
The server exposes three MCP tools that Claude can use:
1. `all_syndication_feeds` - Returns a list of all configured feeds
2. `get_syndication_feed_items` - Returns items from a specific feed
3. `fetch_link` - Fetches and returns content from any URL

### MCP Resources with URI Parameter Filtering

The feed-mcp server supports MCP Resources protocol with comprehensive URI parameter filtering for precise feed content retrieval:

**Resource Types:**
- `feeds://list` - List all available feeds
- `feeds://feed/{feedId}` - Get a specific feed with metadata and items
- `feeds://feed/{feedId}/items` - Get only items from a specific feed
- `feeds://feed/{feedId}/metadata` - Get only metadata from a specific feed

**URI Parameter Filtering:**
All feed item resources support comprehensive filtering via URI parameters for precise content retrieval:

```bash
# Date range filtering (ISO 8601 format)
feeds://feed/{feedId}/items?since=2023-01-01T00:00:00Z
feeds://feed/{feedId}/items?until=2023-12-31T23:59:59Z
feeds://feed/{feedId}/items?since=2023-06-01T00:00:00Z&until=2023-06-30T23:59:59Z

# Pagination
feeds://feed/{feedId}/items?limit=10
feeds://feed/{feedId}/items?offset=20
feeds://feed/{feedId}/items?limit=5&offset=10

# Content filtering (case-insensitive)
feeds://feed/{feedId}/items?category=technology
feeds://feed/{feedId}/items?author=john%20smith
feeds://feed/{feedId}/items?search=golang

# Combined filtering
feeds://feed/{feedId}/items?since=2023-01-01T00:00:00Z&limit=10&category=tech&search=programming
```

**Supported Filter Parameters:**
- `since` - Filter items published after this date (ISO 8601 format)
- `until` - Filter items published before this date (ISO 8601 format)  
- `limit` - Maximum number of items to return (0-1000, capped at 1000)
- `offset` - Number of items to skip for pagination (0 or positive)
- `category` - Filter by category/tag (case-insensitive, checks categories and custom tags)
- `author` - Filter by author name (case-insensitive, checks main author and authors list)
- `search` - Full-text search across title, description, and content (case-insensitive)

**Filter Features:**
- **Case-insensitive matching** for category, author, and search filters
- **Date validation** with clear error messages for invalid ISO 8601 dates
- **Parameter validation** with meaningful error responses
- **Automatic limit capping** at 1000 items for performance safety
- **Date range validation** ensuring 'since' is before 'until'
- **Filter summary** included in responses showing applied filters and result counts

**Error Handling:**
Invalid parameters return structured error responses:
```json
{
  "error": "Invalid 'since' date format: parsing time \"2023-01-01\" as \"2006-01-02T15:04:05Z07:00\": cannot parse \"\" as \"T\"",
  "operation": "parse_since_parameter",
  "component": "resource_filters"
}
```

**Response Format:**
Filtered responses include summary information:
```json
{
  "items": [...],
  "filter_summary": {
    "total_items": 100,
    "filtered_items": 25,
    "applied_filters": {
      "since": "2023-01-01T00:00:00Z",
      "limit": 10,
      "category": "tech"
    }
  }
}
```

### Cache Behavior
- Default cache expiration: 10 minutes
- Cache is in-memory only (resets on restart)
- Feeds are fetched concurrently on startup
- Cache key is the feed URL

### Error Handling Philosophy
- Fail fast with clear error messages
- Validate inputs at boundaries
- Return errors, don't panic
- Log errors but continue serving other feeds if one fails
- Always work in branches and submit PRs
- Always use Context7, godoc, or github to get up to date information on libraries
- This repository has branch protection rules require pull requests. When working on any issue, create a branch, and make a pr when you are done
- Add docstring comments wherever needed
- I only want to use golangci-lint v2, it's the version installed not the config files problem
- I would prefer you use a non-cryptographic hash fucntion, hash/fnv is perfect. Please use that and note on Github Issue and tracking .md files. Carry on.