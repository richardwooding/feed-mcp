# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Build and Run
```bash
# Build all packages
go build ./...

# Run the MCP server locally
go run main.go run <feed-urls>

# Example with multiple feeds
go run main.go run https://techcrunch.com/feed/ https://www.wired.com/feed/rss
```

### Testing
```bash
# Run all tests (unit tests + BDD tests)
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests with verbose output
go test -v ./...

# Run a specific test
go test -run TestFunctionName ./package_name

# Run BDD tests (Cucumber/Godog) - automatically included in go test
go test ./model

# Run tests with race detector
go test -race ./...
```

### Linting and Formatting
```bash
# Format code
go fmt ./...

# Run go vet for static analysis
go vet ./...

# Install and run golangci-lint (if available)
golangci-lint run
```

### Docker
```bash
# Build image locally (CI/CD handles official builds)
docker build -t feed-mcp:local .

# Run with Docker
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
- Configuration includes timeout, cache expiration, and rate limiting settings
- Built-in rate limiting with configurable requests per second and burst capacity

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

### Configuration Flow

1. CLI args parsed by Kong → `RunCmd` struct
2. Store config with defaults (timeout: 30s, cache expiry: 10m, rate limit: 2 req/s)
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
- This repository has branch protection rules reququire pull requests. When working on any issue, create a branch, and make a pr when you are done