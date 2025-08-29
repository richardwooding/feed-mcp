# MCP Resources API Reference

This document provides comprehensive API reference for the MCP Resources implementation in feed-mcp.

## Resource URIs

All resource URIs follow the `feeds://` scheme with specific patterns for different resource types.

### URI Patterns

| Resource Type | URI Pattern | Description |
|---------------|-------------|-------------|
| Feed List | `feeds://all` | Lists all configured feeds |
| Feed Complete | `feeds://feed/{feedId}` | Complete feed with metadata and items |
| Feed Items | `feeds://feed/{feedId}/items` | Feed items only (supports filtering) |
| Feed Metadata | `feeds://feed/{feedId}/meta` | Feed metadata only |

### Feed ID Generation

Feed IDs are generated using FNV (Fowler-Noll-Vo) hash of the feed URL:
- **Algorithm**: FNV-1a 32-bit hash
- **Format**: Lowercase hexadecimal string
- **Example**: `https://example.com/feed.xml` → `a1b2c3d4`

## MCP Protocol Methods

### resources/list

Lists all available resources.

**Request:**
```json
{
  "method": "resources/list",
  "params": {}
}
```

**Response:**
```json
{
  "resources": [
    {
      "uri": "feeds://all",
      "name": "All Feeds",
      "description": "List of all configured syndication feeds",
      "mimeType": "application/json"
    },
    {
      "uri": "feeds://feed/a1b2c3d4",
      "name": "Example Feed",
      "description": "Complete feed data with metadata and items",
      "mimeType": "application/json"
    },
    {
      "uri": "feeds://feed/a1b2c3d4/items",
      "name": "Example Feed Items",
      "description": "Items from Example Feed (supports filtering)",
      "mimeType": "application/json"
    },
    {
      "uri": "feeds://feed/a1b2c3d4/meta",
      "name": "Example Feed Metadata",
      "description": "Metadata for Example Feed",
      "mimeType": "application/json"
    }
  ]
}
```

### resources/read

Reads content from a specific resource.

**Request:**
```json
{
  "method": "resources/read",
  "params": {
    "uri": "feeds://feed/a1b2c3d4/items?limit=10&since=2024-01-01"
  }
}
```

**Response:**
```json
{
  "contents": [
    {
      "uri": "feeds://feed/a1b2c3d4/items?limit=10&since=2024-01-01",
      "mimeType": "application/json",
      "text": "[{\"title\":\"Item 1\",\"description\":\"...\"}]"
    }
  ]
}
```

### resources/subscribe

Subscribe to resource change notifications.

**Request:**
```json
{
  "method": "resources/subscribe",
  "params": {
    "uri": "feeds://feed/a1b2c3d4/items"
  }
}
```

**Response:**
```json
{
  "result": {}
}
```

### resources/unsubscribe

Unsubscribe from resource change notifications.

**Request:**
```json
{
  "method": "resources/unsubscribe", 
  "params": {
    "uri": "feeds://feed/a1b2c3d4/items"
  }
}
```

**Response:**
```json
{
  "result": {}
}
```

## Resource Content Formats

### Feed List Resource (`feeds://all`)

Returns an array of feed objects:

```json
[
  {
    "id": "a1b2c3d4",
    "title": "Example Tech Blog",
    "publicUrl": "https://example.com/feed.xml",
    "description": "Latest technology news and updates",
    "language": "en",
    "lastUpdated": "2024-01-15T10:30:00Z",
    "itemCount": 25
  }
]
```

### Feed Complete Resource (`feeds://feed/{feedId}`)

Returns complete feed data with metadata and items:

```json
{
  "id": "a1b2c3d4",
  "title": "Example Tech Blog",
  "publicUrl": "https://example.com/feed.xml",
  "feed": {
    "title": "Example Tech Blog",
    "description": "Latest technology news and updates",
    "link": "https://example.com",
    "language": "en",
    "copyright": "© 2024 Example Corp",
    "generator": "WordPress",
    "authors": [{"name": "Editor", "email": "editor@example.com"}],
    "categories": ["Technology", "News"],
    "updated": "2024-01-15T10:30:00Z"
  },
  "items": [
    {
      "title": "Breaking: New AI Development",
      "description": "Detailed article about the latest AI breakthrough...",
      "link": "https://example.com/article/ai-breakthrough",
      "published": "2024-01-15T09:00:00Z",
      "authors": [{"name": "Jane Smith", "email": "jane@example.com"}],
      "categories": ["AI", "Technology"],
      "guid": "https://example.com/article/ai-breakthrough"
    }
  ]
}
```

### Feed Items Resource (`feeds://feed/{feedId}/items`)

Returns array of feed items only:

```json
[
  {
    "title": "Breaking: New AI Development",
    "description": "Detailed article about the latest AI breakthrough...",
    "link": "https://example.com/article/ai-breakthrough",
    "published": "2024-01-15T09:00:00Z",
    "authors": [{"name": "Jane Smith", "email": "jane@example.com"}],
    "categories": ["AI", "Technology"],
    "guid": "https://example.com/article/ai-breakthrough"
  }
]
```

### Feed Metadata Resource (`feeds://feed/{feedId}/meta`)

Returns feed metadata only:

```json
{
  "id": "a1b2c3d4",
  "title": "Example Tech Blog",
  "publicUrl": "https://example.com/feed.xml",
  "feed": {
    "title": "Example Tech Blog",
    "description": "Latest technology news and updates",
    "link": "https://example.com",
    "language": "en",
    "copyright": "© 2024 Example Corp",
    "generator": "WordPress",
    "authors": [{"name": "Editor", "email": "editor@example.com"}],
    "categories": ["Technology", "News"],
    "updated": "2024-01-15T10:30:00Z"
  }
}
```

## URI Parameter Filtering

Feed items resources support advanced filtering via URI parameters.

### Supported Parameters

| Parameter | Type | Description | Example |
|-----------|------|-------------|---------|
| `since` | ISO 8601 Date | Items published after date | `since=2024-01-01T00:00:00Z` |
| `until` | ISO 8601 Date | Items published before date | `until=2024-01-31T23:59:59Z` |
| `limit` | Integer | Maximum items (1-1000) | `limit=10` |
| `offset` | Integer | Skip first N items | `offset=20` |
| `category` | String | Filter by category (case-insensitive) | `category=technology` |
| `author` | String | Filter by author (case-insensitive) | `author=jane+smith` |
| `search` | String | Full-text search (case-insensitive) | `search=artificial+intelligence` |

### Parameter Validation

- **Date formats**: ISO 8601 with timezone (`2024-01-01T00:00:00Z`) or date only (`2024-01-01`)
- **Limit bounds**: 1 ≤ limit ≤ 1000 (default: unlimited)
- **Offset**: Must be ≥ 0 (default: 0)
- **String parameters**: URL-encoded, case-insensitive matching
- **Search scope**: Searches across item title, description, and content

### Filtering Examples

**Date range filtering:**
```
feeds://feed/a1b2c3d4/items?since=2024-01-01&until=2024-01-31
```

**Pagination:**
```
feeds://feed/a1b2c3d4/items?limit=20&offset=40
```

**Category filtering:**
```
feeds://feed/a1b2c3d4/items?category=AI&limit=10
```

**Full-text search:**
```
feeds://feed/a1b2c3d4/items?search=machine+learning&limit=5
```

**Combined filtering:**
```
feeds://feed/a1b2c3d4/items?since=2024-01-01&category=tech&search=AI&limit=10
```

## Error Responses

All errors follow the MCP protocol error format:

### Invalid Resource URI

```json
{
  "error": {
    "code": -32602,
    "message": "Invalid resource URI format",
    "data": {
      "uri": "feeds://invalid/uri",
      "details": "URI must follow pattern: feeds://all or feeds://feed/{feedId}[/items|/meta]"
    }
  }
}
```

### Resource Not Found

```json
{
  "error": {
    "code": -32602,
    "message": "Resource not found",
    "data": {
      "uri": "feeds://feed/nonexistent",
      "details": "Feed with ID 'nonexistent' not found"
    }
  }
}
```

### Invalid Parameter

```json
{
  "error": {
    "code": -32602,
    "message": "Invalid parameter value",
    "data": {
      "parameter": "since",
      "value": "invalid-date",
      "details": "Date must be in ISO 8601 format (YYYY-MM-DDTHH:MM:SSZ or YYYY-MM-DD)"
    }
  }
}
```

### Resource Unavailable

```json
{
  "error": {
    "code": -32603,
    "message": "Resource temporarily unavailable", 
    "data": {
      "uri": "feeds://feed/a1b2c3d4",
      "details": "Feed fetch failed: connection timeout"
    }
  }
}
```

## Resource Notifications

When subscribed to resources, clients receive notifications when content changes:

### Resource Updated Notification

```json
{
  "method": "notifications/resources/updated",
  "params": {
    "uri": "feeds://feed/a1b2c3d4/items"
  }
}
```

### Notification Triggers

Resource change notifications are triggered by:
- **Feed content updates** (new items, modified items)
- **Feed metadata changes** (title, description updates)
- **Cache invalidation** (manual or automatic)
- **Feed availability changes** (feed becomes available/unavailable)

## Performance Characteristics

### Response Times (Typical)

| Operation | Cache Hit | Cache Miss | Notes |
|-----------|-----------|------------|-------|
| List Resources | ~0.25ms | ~175ms | Lists all available resources |
| Read Feed List | ~0.25ms | ~175ms | All feeds with metadata |
| Read Feed Items | ~7.6ms | ~15ms | Includes filtering processing |
| Read Feed Metadata | ~11.7ms | ~20ms | Feed metadata only |
| Subscribe/Unsubscribe | ~0.27ms | ~0.27ms | Session management |

### Memory Usage

- **Base overhead**: ~2MB for 100 feeds
- **Per feed**: ~25KB including metadata and items
- **Cache scaling**: Linear with configured cache size
- **Session overhead**: ~1KB per active subscription

### Concurrency

- **Thread-safe operations**: All resource operations are thread-safe
- **Concurrent performance**: ~59ms for mixed operations under load
- **Lock contention**: Minimal with RWMutex for read-heavy workloads
- **Subscription scalability**: Zero-allocation subscription operations

## Client Integration

### Session Management

Resources use session-based subscription management:

1. **Session Creation**: Automatic on first subscription
2. **Session Tracking**: Per-client subscription state
3. **Session Cleanup**: Automatic on client disconnect
4. **Session Isolation**: Subscriptions are isolated per session

### Recommended Usage Patterns

**Discovery Pattern:**
1. Use `feeds://all` to discover available feeds
2. Subscribe to feeds of interest for real-time updates
3. Use filtered item requests for specific content needs

**Polling Alternative:**
- Replace periodic polling with resource subscriptions
- Receive notifications only when content actually changes
- Reduce bandwidth and processing overhead

**Batch Operations:**
- Use feed complete resources for initial data loading
- Switch to items-only resources for updates
- Leverage caching for repeated access patterns

### Error Handling

Implement proper error handling for:
- **Transient failures**: Retry with exponential backoff
- **Invalid URIs**: Validate URI format before requests
- **Parameter validation**: Check parameter bounds and formats
- **Resource unavailability**: Implement fallback strategies

## Migration Guide

### From MCP Tools to Resources

| MCP Tool | MCP Resource | Benefits |
|----------|--------------|----------|
| `all_syndication_feeds` | `feeds://all` | Structured metadata, subscriptions |
| `get_syndication_feed_items` | `feeds://feed/{id}/items` | Filtering, pagination, subscriptions |
| `fetch_link` | Not applicable | Use for non-feed URL fetching |

### Backward Compatibility

- **Tools remain available** for existing integrations
- **No breaking changes** to existing tool interfaces
- **Resource adoption** can be gradual and selective
- **Performance benefits** available immediately with resources