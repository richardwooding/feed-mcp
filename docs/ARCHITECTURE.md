# Architecture

Technical documentation for developers and contributors.

## Table of Contents

- [Overview](#overview)
- [Project Structure](#project-structure)
- [Core Components](#core-components)
- [Data Flow](#data-flow)
- [Key Design Patterns](#key-design-patterns)
- [Development Guide](#development-guide)
- [Testing Strategy](#testing-strategy)
- [Performance](#performance)
- [Dependencies](#dependencies)

## Overview

feed-mcp is a Go application that implements the Model Context Protocol (MCP) to serve RSS/Atom/JSON feeds to AI assistants. The architecture follows clean Go patterns with strong separation of concerns.

### Tech Stack

- **Language**: Go 1.25+
- **Protocol**: MCP (Model Context Protocol)
- **CLI**: Kong for command parsing
- **Feed Parsing**: gofeed
- **Caching**: gocache + ristretto
- **Rate Limiting**: golang.org/x/time/rate
- **Circuit Breaker**: sony/gobreaker
- **Transport**: stdio, HTTP-SSE

## Project Structure

```
feed-mcp/
├── main.go                 # Application entry point
├── cmd/                    # CLI commands
│   └── run.go             # run command implementation
├── model/                  # Domain models
│   ├── feed.go            # Feed data structures
│   ├── transport.go       # Transport types
│   └── errors.go          # Error types
├── store/                  # Feed management
│   ├── store.go           # Feed fetching and caching
│   └── store_test.go      # Store tests
├── mcpserver/              # MCP protocol implementation
│   ├── server.go          # MCP server
│   ├── tools.go           # MCP tools
│   ├── resources.go       # MCP resources
│   ├── prompts.go         # MCP prompts
│   └── *_test.go          # Tests
├── docs/                   # Documentation
│   ├── ADVANCED.md        # Advanced features
│   └── ARCHITECTURE.md    # This file
├── CLAUDE.md              # Claude Code instructions
└── README.md              # User documentation
```

## Core Components

### 1. CLI Layer (`cmd/`)

**Purpose**: Parse command-line arguments and start the server.

**Key File**: `cmd/run.go`

```go
type RunCmd struct {
    Transport            string   `arg:"" optional:"" help:"Transport (stdio|http-sse)"`
    Feeds                []string `arg:"" optional:"" help:"Feed URLs"`
    OPML                 string   `name:"opml" help:"OPML file path or URL"`
    AllowRuntimeFeeds    bool     `name:"allow-runtime-feeds" help:"Enable dynamic feeds"`
    AllowPrivateIPs      bool     `name:"allow-private-ips" help:"Allow private IPs"`
    // ... more flags
}
```

**Responsibilities**:
- Parse CLI arguments using Kong
- Validate feed URLs
- Initialize store and server
- Handle graceful shutdown

### 2. Domain Models (`model/`)

**Purpose**: Define core data structures and types.

**Key Types**:

```go
// Feed represents parsed feed data
type Feed struct {
    Title       string
    Description string
    Link        string
    Items       []*Item
    // ... more fields
}

// FeedAndItemsResult combines feed metadata and items
type FeedAndItemsResult struct {
    ID                 string
    PublicURL          string
    Title              string
    Feed               *Feed
    Items              []*gofeed.Item
    CircuitBreakerOpen bool
}

// Transport types
type Transport int
const (
    UndefinedTransport Transport = iota
    StdioTransport
    HTTPWithSSETransport
)
```

**Design**: Adapts third-party `gofeed` types to internal structures.

### 3. Feed Store (`store/`)

**Purpose**: Fetch, cache, and manage feeds.

**Key Type**:

```go
type Store struct {
    feeds              []string
    cache              *cache.Cache[any]
    httpClient         *http.Client
    timeout            time.Duration
    cacheDuration      time.Duration
    circuitBreakers    map[string]*gobreaker.CircuitBreaker
    rateLimiter        *rate.Limiter
    // ... more fields
}
```

**Responsibilities**:
- Fetch feeds concurrently
- Cache feed data (in-memory, 10-minute default)
- Handle rate limiting (2 req/s default)
- Implement circuit breakers for failing feeds
- Retry failed requests with exponential backoff
- Pool HTTP connections for performance

**Key Methods**:

```go
func NewStore(config Config) (*Store, error)
func (s *Store) GetAllFeeds(ctx context.Context) ([]*model.FeedAndItemsResult, error)
func (s *Store) GetFeedAndItems(ctx context.Context, id string) (*model.FeedAndItemsResult, error)
```

### 4. MCP Server (`mcpserver/`)

**Purpose**: Implement MCP protocol to serve feeds.

**Key Type**:

```go
type Server struct {
    allFeedsGetter     AllFeedsGetter
    feedAndItemsGetter FeedAndItemsGetter
    dynamicFeedManager DynamicFeedManager
    resourceManager    *ResourceManager
    transport          model.Transport
}
```

**MCP Tools**:
- `all_syndication_feeds` - List all feeds
- `get_syndication_feed_items` - Get feed with pagination/filtering
- `fetch_link` - Fetch arbitrary URL content
- `add_feed` - Add feed at runtime (when enabled)
- `remove_feed` - Remove feed at runtime (when enabled)
- `list_managed_feeds` - List all feeds with metadata (when enabled)

**MCP Resources**:
- `feeds://all` - Feed list
- `feeds://feed/{id}` - Complete feed
- `feeds://feed/{id}/items` - Feed items with filtering
- `feeds://feed/{id}/meta` - Feed metadata only

**MCP Prompts**:
- `analyze_feed_trends` - Trend analysis
- `summarize_feeds` - Feed summaries
- `monitor_keywords` - Keyword tracking
- `compare_sources` - Source comparison
- `generate_feed_report` - Performance reports

## Data Flow

### Startup Flow

```
1. main.go
   ↓
2. Kong parses CLI args → RunCmd
   ↓
3. Validate feed URLs (SSRF protection)
   ↓
4. Create Store with config
   ↓
5. Store fetches all feeds concurrently
   ├─ Apply rate limiting
   ├─ Check circuit breakers
   ├─ Retry on failures
   └─ Cache results
   ↓
6. Create MCP Server
   ↓
7. Register tools, resources, prompts
   ↓
8. Start transport (stdio or HTTP-SSE)
   ↓
9. Listen for MCP requests
```

### Request Flow (MCP Tool)

```
1. MCP client sends tool request
   ↓
2. Server receives request
   ↓
3. Validate parameters
   ↓
4. Check cache
   ├─ HIT  → Return cached data
   └─ MISS → Fetch from source
              ↓
              Apply rate limiting
              ↓
              Check circuit breaker
              ↓
              Fetch feed (with retry)
              ↓
              Parse with gofeed
              ↓
              Cache result
   ↓
5. Apply pagination/filtering
   ↓
6. Return result to client
```

### Resource Flow (MCP Resource)

```
1. MCP client reads resource
   ↓
2. Parse URI (feeds://feed/{id}/items?filters)
   ↓
3. Extract feed ID and filter parameters
   ↓
4. Get feed from store (cached or fetch)
   ↓
5. Apply URI parameter filters
   ├─ Date range (since/until)
   ├─ Pagination (limit/offset)
   ├─ Category filter
   ├─ Author filter
   └─ Full-text search
   ↓
6. Return filtered results
```

## Key Design Patterns

### 1. Factory Pattern

All major components use constructor functions:

```go
func NewStore(config Config) (*Store, error)
func NewServer(config Config) (*Server, error)
```

### 2. Interface Segregation

Small, focused interfaces:

```go
type AllFeedsGetter interface {
    GetAllFeeds(ctx context.Context) ([]*model.FeedAndItemsResult, error)
}

type FeedAndItemsGetter interface {
    GetFeedAndItems(ctx context.Context, id string) (*model.FeedAndItemsResult, error)
}
```

### 3. Adapter Pattern

Adapts third-party types to internal models:

```go
func FromGoFeed(goFeed *gofeed.Feed) *Feed {
    // Convert gofeed.Feed → model.Feed
}
```

### 4. Circuit Breaker Pattern

Protects against cascading failures:

```go
// Each feed gets its own circuit breaker
cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
    Name:        feedURL,
    MaxRequests: 3,
    Timeout:     30 * time.Second,
    ReadyToTrip: func(counts gobreaker.Counts) bool {
        return counts.ConsecutiveFailures >= 3
    },
})
```

### 5. Error Handling

Early returns with custom error types:

```go
if err != nil {
    return nil, model.NewFeedError(model.ErrorTypeNetwork, err.Error()).
        WithOperation("fetch_feed").
        WithComponent("store")
}
```

## Development Guide

### Prerequisites

- Go 1.25+
- Docker (for containerized testing)
- golangci-lint v2

### Setup

```bash
# Clone repository
git clone https://github.com/richardwooding/feed-mcp.git
cd feed-mcp

# Install dependencies
go mod download

# Install development tools
make dev-setup
```

### Development Workflow

```bash
# Format code
make fmt

# Run linter
make lint

# Run tests
make test

# Run tests with coverage
make test-coverage

# Build binary
make build

# Run locally
go run main.go run https://techcrunch.com/feed/
```

### Code Style

- Follow [Effective Go](https://go.dev/doc/effective_go)
- Use `gofmt` for formatting
- Run `golangci-lint` before committing
- Write docstring comments for exported functions
- Keep functions small and focused
- Prefer table-driven tests

### Adding New Features

1. **Create feature branch**: `git checkout -b feature/my-feature`
2. **Update models** (if needed): Add types to `model/`
3. **Update store** (if needed): Add methods to `store/`
4. **Add MCP tools/resources/prompts**: Update `mcpserver/`
5. **Write tests**: Add `*_test.go` files
6. **Update documentation**: Update README, ADVANCED, or ARCHITECTURE
7. **Submit PR**: Create pull request for review

## Testing Strategy

### Unit Tests

```bash
# Run all tests
go test ./...

# Run specific package
go test ./store

# Run with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### BDD Tests (Cucumber/Godog)

Located in `model/features/*.feature`:

```gherkin
Feature: Feed conversion
  Scenario: Convert gofeed.Feed to model.Feed
    Given a gofeed Feed with title "Test Feed"
    When I convert it to model.Feed
    Then the model.Feed should have title "Test Feed"
```

Run with:

```bash
go test ./model
```

### Integration Tests

Test end-to-end functionality:

```go
func TestStoreIntegration(t *testing.T) {
    // Create store with real feeds
    store, err := NewStore(Config{
        Feeds: []string{"https://example.com/feed.xml"},
    })

    // Test fetching
    feeds, err := store.GetAllFeeds(context.Background())
    // Assert results
}
```

### Test Helpers

```go
// mockFeedServer creates a test HTTP server
func mockFeedServer(t *testing.T, feedXML string) *httptest.Server {
    return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/xml")
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte(feedXML))
    }))
}
```

## Performance

### Benchmarks

```bash
# Run benchmarks
go test -bench=. ./...

# With memory profiling
go test -bench=. -benchmem ./...
```

### Key Metrics

- **Feed fetching**: ~100-500ms per feed (network dependent)
- **Cache hit**: ~0.008ms
- **Resource listing**: ~0.17ms for 100 feeds
- **Memory**: ~25KB per feed
- **Concurrent fetches**: Linear scaling with goroutines

### Optimization Techniques

1. **Concurrent fetching**: Uses goroutines + sync.WaitGroup
2. **HTTP connection pooling**: Reuses connections (configurable)
3. **In-memory caching**: Ristretto for high performance
4. **Rate limiting**: Token bucket prevents overwhelming servers
5. **Circuit breakers**: Fast-fail for failing feeds
6. **Retry with backoff**: Handles transient failures efficiently

## Dependencies

### Core Libraries

```go
require (
    github.com/alecthomas/kong v1.12.1           // CLI parsing
    github.com/mmcdole/gofeed v1.3.0             // Feed parsing
    github.com/eko/gocache/lib/v4 v4.2.1         // Caching
    github.com/dgraph-io/ristretto/v2 v2.3.0     // Cache backend
    github.com/modelcontextprotocol/go-sdk v1.0.0 // MCP protocol
    github.com/sony/gobreaker v1.0.0             // Circuit breaker
    golang.org/x/time v0.13.0                    // Rate limiting
    github.com/gocolly/colly v1.2.0              // Web scraping
    github.com/google/jsonschema-go v0.3.0       // JSON schema
)
```

### Why These Dependencies?

- **Kong**: Declarative, type-safe CLI parsing
- **gofeed**: Robust RSS/Atom/JSON feed parser
- **gocache + ristretto**: High-performance caching layer
- **MCP Go SDK**: Official MCP protocol implementation
- **gobreaker**: Battle-tested circuit breaker
- **golang.org/x/time/rate**: Standard Go rate limiter
- **colly**: Lightweight web scraping for fetch_link tool

### Dependency Management

```bash
# Update dependencies
go get -u ./...

# Tidy go.mod
go mod tidy

# Verify dependencies
go mod verify

# Vendor dependencies (optional)
go mod vendor
```

## Contributing

### Pull Request Process

1. Fork the repository
2. Create feature branch
3. Make changes with tests
4. Run linter and tests locally
5. Update documentation
6. Submit PR with clear description

### Code Review Guidelines

- All PRs require review
- All tests must pass
- Linter must pass
- Code coverage should not decrease
- Documentation should be updated

### Commit Message Format

```
<type>: <subject>

<body>

<footer>
```

**Types**: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`

**Example**:
```
feat: add pagination support to get_syndication_feed_items

Adds limit and offset parameters to control response size and prevent
hitting MCP's 1MB tool result limit.

Closes #42
```

## License

MIT License - See [LICENSE](../LICENSE) for details.
