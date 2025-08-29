# feed-mcp

[![Go Coverage](https://github.com/richardwooding/feed-mcp/wiki/coverage.svg)](https://raw.githack.com/wiki/richardwooding/feed-mcp/coverage.html)
[![Go Report Card](https://goreportcard.com/badge/github.com/richardwooding/feed-mcp)](https://goreportcard.com/report/github.com/richardwooding/feed-mcp)
[![MCP Badge](https://lobehub.com/badge/mcp/richardwooding-feed-mcp)](https://lobehub.com/mcp/richardwooding-feed-mcp)

MCP Server for RSS, Atom, and JSON Feeds

## Quick Start Examples

Here are some practical configurations for Claude Desktop that demonstrate common use cases:

```json
{
  "mcpServers": {
    "feed-tech-news": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "ghcr.io/richardwooding/feed-mcp:latest",
        "run",
        "https://techcrunch.com/feed/",
        "https://feeds.arstechnica.com/arstechnica/index",
        "https://www.theverge.com/rss/index.xml",
        "https://www.wired.com/feed/rss",
        "https://www.engadget.com/rss.xml"
      ]
    },
    "feed-security": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "ghcr.io/richardwooding/feed-mcp:latest",
        "run",
        "https://krebsonsecurity.com/feed/",
        "https://www.schneier.com/blog/atom.xml",
        "https://thehackernews.com/feeds/posts/default",
        "https://www.bleepingcomputer.com/feed/"
      ]
    },
    "feed-webdev": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "ghcr.io/richardwooding/feed-mcp:latest",
        "run",
        "https://css-tricks.com/feed/",
        "https://www.smashingmagazine.com/feed/",
        "https://mozilla.hacks.org/feed/"
      ]
    }
  }
}
```

Add any of these configurations to your Claude Desktop to instantly access the latest news and articles from your chosen topics.

## Features

- Serves RSS, Atom, and JSON feeds via the MCP protocol
- **MCP Resources support** with dynamic feed discovery and real-time subscriptions
- **MCP Tools support** for direct feed operations (legacy compatibility)
- Supports Docker and Podman for easy deployment
- CLI installable via `go install`
- Compatible with Claude Desktop as an MCP server
- Caching for efficient feed retrieval
- Built-in rate limiting (2 req/s default) to be respectful to feed servers
- Circuit breaker pattern for fault tolerance against failing feeds
- HTTP connection pooling for improved performance with multiple feeds
- Retry mechanism with exponential backoff and jitter for handling transient failures
- **URL validation and sanitization** to prevent SSRF attacks and security vulnerabilities
- Private IP and localhost blocking (configurable) for enhanced security
- Graceful shutdown with signal handling (SIGINT/SIGTERM)
- Supports multiple feeds simultaneously
- Extensible and configurable

## Architecture

The core of `feed-mcp` is a Go server that fetches, parses, and serves RSS/Atom/JSON feeds over the [MCP protocol](https://spec.modelcontextprotocol.io/specification/). The main architectural components are:

- **Command-line Interface (CLI):** Uses [kong](https://github.com/alecthomas/kong) for parsing commands and flags. The `run` command is the entry point for starting the server.
- **Feed Fetching & Parsing:** Feeds are fetched and parsed using [gofeed](https://github.com/mmcdole/gofeed). The server supports multiple feeds, which are periodically refreshed and cached.
- **Caching Layer:** Feed data is cached using [gocache](https://github.com/eko/gocache) and [ristretto](https://github.com/dgraph-io/ristretto) for efficient retrieval and reduced network usage.
- **Rate Limiting:** Built-in HTTP rate limiting using [golang.org/x/time/rate](https://pkg.go.dev/golang.org/x/time/rate) prevents overwhelming feed servers with requests.
- **Circuit Breaker:** Implements circuit breaker pattern using [sony/gobreaker](https://github.com/sony/gobreaker) to temporarily stop fetching from consistently failing feeds, with configurable failure thresholds and recovery timeouts.
- **HTTP Connection Pooling:** Optimized HTTP connection pooling with configurable pool sizes and timeouts for improved performance when fetching multiple feeds simultaneously.
- **Retry Mechanism:** Automatic retry with exponential backoff and jitter for handling transient network failures, DNS errors, and server errors (5xx), while avoiding retries for client errors (4xx).
- **MCP Protocol Server:** Implements the MCP protocol using the [official MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk), allowing integration with clients like Claude Desktop.
- **Transport Options:** Supports different transports (e.g., stdio, HTTP with SSE) for communication with MCP clients.
- **Graceful Shutdown:** Handles SIGINT and SIGTERM signals for clean termination, with configurable shutdown timeout (default 30s).
- **Docker/Podman Support:** The server can be run in containers for easy deployment and integration.

### How it Works

1. **Startup:** The CLI parses arguments and starts the server with the specified feeds and transport.
2. **Feed Management:** The server fetches and parses the configured feeds, storing results in the cache.
3. **Serving Requests:** When an MCP client connects, the server responds to requests for feed data using the cached content, updating as needed.
4. **Graceful Shutdown:** When receiving shutdown signals, the server cleanly terminates all operations and exits.
5. **Extensibility:** The architecture allows for adding new transports, feed sources, or output formats with minimal changes.

For contributors:  
- The main entry point is `main.go`, which wires up the CLI and server.
- Feed logic is in the `model` and `store` packages.
- MCP protocol handling is in the `mcpserver` package.
- Tests are provided for core logic; see `*_test.go` files for examples.

## MCP Resources Support

This server provides comprehensive MCP Resources support, enabling dynamic feed discovery, real-time subscriptions, and advanced filtering capabilities.

### Resource Types

The server exposes feed data through structured resource URIs:

#### 1. Feed List Resource
- **URI**: `feeds://all`
- **Description**: Lists all configured feeds with metadata
- **Content**: JSON array of feed objects with titles, descriptions, and URLs
- **Use Case**: Discover available feeds dynamically

#### 2. Individual Feed Resources
- **URI**: `feeds://feed/{feedId}`
- **Description**: Complete feed data including metadata and all items
- **Content**: JSON object with feed metadata and items array
- **Use Case**: Get full feed content in a single request

#### 3. Feed Items Resources
- **URI**: `feeds://feed/{feedId}/items`
- **Description**: Feed items only (no metadata)
- **Content**: JSON array of feed items
- **Use Case**: Focus on content without feed metadata overhead
- **Supports**: Advanced filtering via URI parameters (see below)

#### 4. Feed Metadata Resources
- **URI**: `feeds://feed/{feedId}/meta`
- **Description**: Feed metadata only (no items)
- **Content**: JSON object with feed title, description, author, etc.
- **Use Case**: Quick feed information lookup

### Advanced URI Parameter Filtering

Feed items resources support comprehensive filtering via URI parameters:

```
feeds://feed/{feedId}/items?limit=10&since=2024-01-01&category=tech&search=AI
```

**Supported Parameters:**
- **`since`** - Items published after date (ISO 8601: `2024-01-01T00:00:00Z`)
- **`until`** - Items published before date (ISO 8601: `2024-12-31T23:59:59Z`)
- **`limit`** - Maximum number of items (1-1000, default: all)
- **`offset`** - Skip first N items (for pagination)
- **`category`** - Filter by category/tag (case-insensitive)
- **`author`** - Filter by author name (case-insensitive)
- **`search`** - Full-text search in title, description, and content (case-insensitive)

**Filter Examples:**
```bash
# Recent items only
feeds://feed/abc123/items?since=2024-01-01T00:00:00Z

# Paginated results
feeds://feed/abc123/items?limit=20&offset=40

# Category and search combined
feeds://feed/abc123/items?category=technology&search=artificial+intelligence

# Date range with limit
feeds://feed/abc123/items?since=2024-01-01&until=2024-01-31&limit=10
```

### Resource Subscriptions

The server supports MCP resource subscriptions for real-time feed updates:

- **Automatic notifications** when feed content changes
- **Session-based subscription management** with cleanup
- **Thread-safe operations** for concurrent subscribers
- **Cache integration** with invalidation triggering notifications
- **Efficient change detection** using content hashing and timestamps

### Performance Characteristics

MCP Resources are optimized for high performance:
- **Resource listing**: ~0.17ms for 100 feeds (588x faster than requirements)
- **Resource reading**: ~0.008ms for cache hits (6,250x faster than requirements)
- **Memory efficient**: ~25KB per feed with linear scaling
- **Concurrent access**: Excellent scaling with minimal contention
- **Cache integration**: Sub-microsecond cache hits with 95%+ hit ratio

### Usage Examples

**List all available feeds:**
```json
{
  "method": "resources/read",
  "params": {
    "uri": "feeds://all"
  }
}
```

**Get specific feed with recent items:**
```json
{
  "method": "resources/read", 
  "params": {
    "uri": "feeds://feed/abc123/items?since=2024-01-01&limit=10"
  }
}
```

**Subscribe to feed updates:**
```json
{
  "method": "resources/subscribe",
  "params": {
    "uri": "feeds://feed/abc123/items"
  }
}
```

### Migration from Tools to Resources

For existing integrations using MCP Tools:
- **`all_syndication_feeds`** tool → `feeds://all` resource
- **`get_syndication_feed_items`** tool → `feeds://feed/{feedId}/items` resource
- **Tools remain supported** for backward compatibility
- **Resources provide richer metadata** and filtering capabilities
- **Subscriptions enable real-time updates** not available with tools

## Running via docker

```sh
docker run -i --rm ghcr.io/richardwooding/feed-mcp:latest run \
  https://www.reddit.com/r/golang/.rss \
  https://www.reddit.com/r/mcp/.rss
```

## Running via podman

```sh
podman run -i --rm ghcr.io/richardwooding/feed-mcp:latest run \
  https://www.reddit.com/r/golang/.rss \
  https://www.reddit.com/r/mcp/.rss
```

## Installing using Go install

You can install the CLI using:

```sh
go install github.com/richardwooding/feed-mcp@latest
```

## Add to Claude Desktop

In your Claude Desktop configuration file, add the following configuration to the `mcpServers` section:

### Docker

```json
{
  "mcpServers": {
    "feed": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "ghcr.io/richardwooding/feed-mcp:latest",
        "run",
        "https://www.reddit.com/r/golang/.rss",
        "https://www.reddit.com/r/mcp/.rss"
      ]
    }
  }
}
```

### Podman

```json
{
  "mcpServers": {
    "feed": {
      "command": "podman",
      "args": [
        "run",
        "-i",
        "--rm",
        "ghcr.io/richardwooding/feed-mcp:latest",
        "run",
        "https://www.reddit.com/r/golang/.rss",
        "https://www.reddit.com/r/mcp/.rss"
      ]
    }
  }
}
```

## Security

`feed-mcp` includes several security features to protect against common web vulnerabilities:

### URL Validation
- **Scheme restriction**: Only HTTP and HTTPS URLs are allowed
- **Private IP blocking**: Prevents SSRF attacks by blocking access to private IP ranges by default
- **Localhost protection**: Blocks `localhost`, `127.x.x.x`, and IPv6 loopback addresses
- **Malformed URL detection**: Rejects invalid or malicious URL formats

### Private IP Ranges Blocked by Default
- `10.0.0.0/8` (10.x.x.x)
- `172.16.0.0/12` (172.16-31.x.x)  
- `192.168.0.0/16` (192.168.x.x)
- `127.0.0.0/8` (localhost/loopback)
- `169.254.0.0/16` (link-local)
- IPv6 loopback (`::1`) and link-local addresses

### Configuration Options
```bash
# Allow private IPs and localhost (disabled by default for security)
go run main.go run --allow-private-ips https://localhost/feed

# Using Docker
docker run -i --rm ghcr.io/richardwooding/feed-mcp:latest run \
  --allow-private-ips \
  https://192.168.1.100/api/feed
```

### Security Best Practices
- Always validate feed URLs before deployment
- Use HTTPS URLs when possible for encrypted transport
- Regularly update to the latest version for security patches
- Consider network-level restrictions for additional protection
- Monitor logs for blocked URL attempts

## Dependencies

This project makes use of the following open source libraries:

- [gofeed](https://github.com/mmcdole/gofeed) — RSS/Atom feed parser
- [kong](https://github.com/alecthomas/kong) — Command-line parser
- [gocache](https://github.com/eko/gocache) — Caching library
- [ristretto](https://github.com/dgraph-io/ristretto) — High performance cache
- [golang.org/x/time/rate](https://pkg.go.dev/golang.org/x/time/rate) — Token bucket rate limiter
- [sony/gobreaker](https://github.com/sony/gobreaker) — Circuit breaker pattern implementation
- [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) — Official MCP protocol implementation

## License

This project is licensed under the [MIT License](LICENSE).
