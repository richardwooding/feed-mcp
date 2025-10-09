# Advanced Features

This document covers advanced features and configurations for power users of feed-mcp.

## Table of Contents

- [Dynamic Feed Management](#dynamic-feed-management)
- [MCP Resources](#mcp-resources)
- [Intelligent Prompts](#intelligent-prompts)
- [OPML Support](#opml-support)
- [Performance Tuning](#performance-tuning)
- [Security Configuration](#security-configuration)

## Dynamic Feed Management

Add, remove, and manage feeds at runtime without restarting the server.

### Enabling

```bash
# Start with dynamic feed management
feed-mcp run --allow-runtime-feeds

# Or with Docker
docker run -i --rm ghcr.io/richardwooding/feed-mcp:latest run --allow-runtime-feeds
```

### Available Tools

#### `add_feed` - Add Feeds at Runtime

```json
{
  "tool": "add_feed",
  "arguments": {
    "url": "https://techcrunch.com/feed/",
    "title": "TechCrunch",
    "category": "Technology",
    "description": "Latest technology news"
  }
}
```

**Parameters:**
- `url` (required) - RSS/Atom/JSON feed URL
- `title` (optional) - Human-readable feed title
- `category` (optional) - Category for organization
- `description` (optional) - Feed description

#### `remove_feed` - Remove Feeds

```json
{
  "tool": "remove_feed",
  "arguments": {
    "feedId": "abc123"
  }
}
```

Or by URL:

```json
{
  "tool": "remove_feed",
  "arguments": {
    "url": "https://techcrunch.com/feed/"
  }
}
```

#### `list_managed_feeds` - View All Feeds

```json
{
  "tool": "list_managed_feeds",
  "arguments": {}
}
```

**Response includes:**
- Feed ID, URL, title, category, description
- Status (active, error, paused)
- Source (startup, opml, runtime)
- Last fetched timestamp and error details
- Current item count

### Feed Sources

- **`startup`** - Feeds from command line arguments
- **`opml`** - Feeds loaded from OPML files
- **`runtime`** - Feeds added dynamically via `add_feed`

### Limitations

- Feeds stored in memory only (lost on restart)
- Cannot modify startup or OPML feeds at runtime
- No persistent configuration storage
- Runtime-added feeds only can be removed

## MCP Resources

Advanced feed access with filtering, subscriptions, and real-time updates.

### Resource URIs

#### List All Feeds
```
feeds://all
```

Returns JSON array of all feeds with metadata.

#### Get Complete Feed
```
feeds://feed/{feedId}
```

Returns feed metadata and all items.

#### Get Feed Items Only
```
feeds://feed/{feedId}/items
```

Returns only items (no metadata). Supports filtering (see below).

#### Get Feed Metadata Only
```
feeds://feed/{feedId}/meta
```

Returns only metadata (no items).

### Advanced Filtering

Feed items resources support comprehensive URI parameter filtering:

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
- **`search`** - Full-text search in title, description, content (case-insensitive)

**Examples:**

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

Subscribe to feeds for real-time updates:

```json
{
  "method": "resources/subscribe",
  "params": {
    "uri": "feeds://feed/abc123/items"
  }
}
```

**Features:**
- Automatic notifications when content changes
- Session-based subscription management
- Thread-safe concurrent subscribers
- Cache integration with invalidation triggering

### Performance

- **Resource listing**: ~0.17ms for 100 feeds
- **Resource reading**: ~0.008ms for cache hits
- **Memory**: ~25KB per feed
- **Cache hit ratio**: 95%+

## Intelligent Prompts

AI-powered feed analysis and insights.

### `analyze_feed_trends`

Analyze patterns and trends across feeds over time.

**Parameters:**
- `timeframe` (optional) - Time period (e.g., '24h', '7d', '30d') - default: '24h'
- `categories` (optional) - Comma-separated categories to filter

**Example:**
```
Analyze feed trends for the past week focusing on technology categories
```

**Insights:**
- Publication frequency patterns
- Topic distribution and themes
- Source activity levels and error rates
- Content pattern analysis

### `summarize_feeds`

Generate comprehensive feed summaries.

**Parameters:**
- `feed_ids` (optional) - Comma-separated feed IDs - default: all
- `summary_type` (optional) - 'brief', 'detailed', 'executive' - default: 'brief'

**Example:**
```
Generate a detailed summary of all technology feeds
```

**Summary Types:**
- **Brief** - Quick overview with key metrics
- **Detailed** - Complete breakdown per feed
- **Executive** - Strategic overview with recommendations

### `monitor_keywords`

Track keywords/topics across all feeds with alerts.

**Parameters:**
- `keywords` (required) - Comma-separated keywords or phrases
- `timeframe` (optional) - Time period - default: '24h'
- `alert_threshold` (optional) - Minimum mentions for alert - default: 1

**Example:**
```
Monitor keywords "artificial intelligence, machine learning, AI" for alerts
```

**Features:**
- Cross-feed keyword tracking
- Smart alert system
- Trend analysis
- Contextual recommendations

### `compare_sources`

Compare coverage across different feed sources.

**Parameters:**
- `topic` (required) - Topic or keyword to compare
- `feed_ids` (optional) - Specific feeds to compare - default: all

**Example:**
```
Compare how different sources cover "climate change"
```

**Analysis:**
- Coverage depth comparison
- Unique perspectives
- Content gap analysis
- Source reliability metrics

### `generate_feed_report`

Generate comprehensive performance reports.

**Parameters:**
- `report_type` (optional) - 'performance', 'content', 'engagement', 'comprehensive' - default: 'comprehensive'
- `timeframe` (optional) - Report period (e.g., '7d', '30d', '90d') - default: '7d'

**Example:**
```
Generate a comprehensive feed performance report for the past month
```

**Report Types:**
- **Performance** - System health, uptime, error rates
- **Content** - Volume metrics, quality analysis
- **Engagement** - Usage patterns, popular content
- **Comprehensive** - Complete overview with recommendations

## OPML Support

Import feed subscriptions from RSS readers.

### Usage

```bash
# Local OPML file
feed-mcp run --opml feeds.opml

# Remote OPML URL
feed-mcp run --opml https://example.com/my-feeds.opml
```

### Docker with OPML

```json
{
  "mcpServers": {
    "feed-from-opml": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "-v", "/path/to/your/feeds.opml:/feeds.opml:ro",
        "ghcr.io/richardwooding/feed-mcp:latest",
        "run", "--opml", "/feeds.opml"
      ]
    }
  }
}
```

### OPML Format

Standard OPML 2.0 with nested categories:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
    <head>
        <title>My Feed Subscriptions</title>
    </head>
    <body>
        <outline text="Technology">
            <outline text="TechCrunch" xmlUrl="https://techcrunch.com/feed/" />
            <outline text="The Verge" xmlUrl="https://www.theverge.com/rss/index.xml" />
        </outline>
        <outline text="Security">
            <outline text="Krebs on Security" xmlUrl="https://krebsonsecurity.com/feed/" />
        </outline>
    </body>
</opml>
```

### Exporting from Readers

- **Feedly**: Settings → OPML → Export
- **Inoreader**: Preferences → Folders and Tags → Export OPML
- **NewsBlur**: Account → Import/Export → Export Stories
- **The Old Reader**: Settings → Import/Export → Export

## Performance Tuning

### Rate Limiting

Control request rate to feed servers:

```bash
feed-mcp run \
  --rate-limit 1.0 \
  --rate-burst 3 \
  https://example.com/feed.xml
```

**Parameters:**
- `--rate-limit` - Requests per second (default: 2.0)
- `--rate-burst` - Burst capacity (default: 5)

### Circuit Breakers

Automatically handle failing feeds:

```bash
feed-mcp run \
  --circuit-breaker-threshold 5 \
  --circuit-breaker-timeout 45s \
  https://example.com/feed.xml
```

**Parameters:**
- `--circuit-breaker-threshold` - Failures before opening circuit (default: 3)
- `--circuit-breaker-timeout` - Open state timeout (default: 30s)
- `--circuit-breaker-max-requests` - Half-open state requests (default: 3)

**States:**
- **Closed** - Normal operation
- **Open** - Failing, requests fail fast
- **Half-Open** - Testing recovery

### Connection Pooling

Optimize HTTP connections:

```bash
feed-mcp run \
  --max-idle-conns 200 \
  --max-conns-per-host 20 \
  --max-idle-conns-per-host 10 \
  --idle-conn-timeout 120s \
  https://example.com/feed.xml
```

**Parameters:**
- `--max-idle-conns` - Total idle connections (default: 100)
- `--max-conns-per-host` - Connections per host (default: 10)
- `--max-idle-conns-per-host` - Idle connections per host (default: 5)
- `--idle-conn-timeout` - Keep-alive timeout (default: 90s)

### Retry Configuration

Handle transient failures:

```bash
feed-mcp run \
  --retry-max-attempts 5 \
  --retry-base-delay 2s \
  --retry-max-delay 60s \
  https://example.com/feed.xml
```

**Parameters:**
- `--retry-max-attempts` - Maximum retry attempts (default: 3)
- `--retry-base-delay` - Base delay between retries (default: 1s)
- `--retry-max-delay` - Maximum delay cap (default: 30s)
- `--retry-jitter` - Enable jitter (default: true)

**Retryable Errors:**
- 5xx server errors
- DNS failures
- Connection refused
- Network unreachable
- Timeouts

**Non-Retryable Errors:**
- 4xx client errors (404, etc.)
- Context cancellation
- Invalid URLs

### Cache Configuration

The cache is in-memory with 10-minute default expiration. To adjust:

```bash
# Modify store/store.go constants
const (
    DefaultCacheDuration = 10 * time.Minute
)
```

## Security Configuration

### URL Validation

By default, private IPs and localhost are blocked:

**Blocked by default:**
- `10.0.0.0/8` (10.x.x.x)
- `172.16.0.0/12` (172.16-31.x.x)
- `192.168.0.0/16` (192.168.x.x)
- `127.0.0.0/8` (localhost)
- `169.254.0.0/16` (link-local)
- IPv6 loopback and link-local

**Allow private IPs:**

```bash
feed-mcp run --allow-private-ips http://192.168.1.100/feed
```

### Best Practices

- Keep `--allow-private-ips` disabled in production
- Always use HTTPS when possible
- Validate feed URLs before deployment
- Monitor logs for blocked URL attempts
- Regularly update to latest version

## Migration from Tools to Resources

For existing MCP Tools usage:

| Tool | Resource Equivalent |
|------|-------------------|
| `all_syndication_feeds` | `feeds://all` |
| `get_syndication_feed_items` | `feeds://feed/{id}/items` |

**Benefits of Resources:**
- Advanced filtering (date ranges, categories, search)
- Real-time subscriptions
- Richer metadata
- Better performance
- Cache integration

**Tools remain supported** for backward compatibility.
