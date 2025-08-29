# MCP Resources Implementation Plan

## Overview

This document outlines the comprehensive plan to add MCP Resources support to the feed-mcp project. MCP Resources will enable dynamic feed discovery, real-time updates via subscriptions, and parameterized feed access using URI templates.

## Architecture Overview

The feed-mcp server currently exposes feeds via MCP Tools. Adding MCP Resources support will enable:
- **Dynamic feed discovery** as resources instead of static tools
- **Real-time feed updates** via resource subscriptions
- **Parameterized feed access** using URI templates
- **Rich metadata** for each feed resource

## Core Components Design

### A. Resource Manager (`mcpserver/resources.go`)
```go
type ResourceManager struct {
    store      store.AllFeedsGetter
    sessions   map[string]*ResourceSession
    mu         sync.RWMutex
}

type ResourceSession struct {
    id           string
    subscriptions map[string]bool // resource URI -> subscribed
    lastUpdate   time.Time
}
```

### B. Resource Types
1. **Feed List Resource**: `feeds://all` - Lists all available feeds
2. **Individual Feed Resources**: `feeds://feed/{feedId}` - Specific feed content
3. **Feed Items Resources**: `feeds://feed/{feedId}/items` - Feed items only
4. **Feed Metadata Resources**: `feeds://feed/{feedId}/meta` - Feed metadata

### C. URI Template System
```go
const (
    FeedListURI     = "feeds://all"
    FeedURI         = "feeds://feed/{feedId}"
    FeedItemsURI    = "feeds://feed/{feedId}/items"
    FeedMetaURI     = "feeds://feed/{feedId}/meta"
)
```

## Integration Points

### A. Modify MCP Server (`mcpserver/server.go`)
- Add resource handlers alongside existing tool handlers
- Implement `resources/list`, `resources/read`, `resources/subscribe`
- Add resource change notifications

### B. Extend Store Interface (`store/store.go`)
- Add resource subscription capabilities
- Implement change detection for notifications
- Add feed metadata extraction

### C. Update Configuration (`model/config.go`)
- Add resource-specific configuration options
- Enable/disable resource features

## Implementation Phases

### Phase 1: Core Resource Infrastructure
- [ ] Create `ResourceManager` with basic resource listing
- [ ] Implement URI template parsing and matching
- [ ] Add resource read functionality for static resources
- [ ] Update MCP server to handle resource requests

### Phase 2: Dynamic Feed Resources  
- [ ] Map feed URLs to resource identifiers
- [ ] Implement feed content serialization for resources
- [ ] Add feed metadata resource support
- [ ] Create resource content caching layer

### Phase 3: Resource Subscriptions
- [ ] Implement resource subscription management
- [ ] Add change detection for feed updates
- [ ] Create notification system for resource changes
- [ ] Add subscription cleanup and session management

### Phase 4: Advanced Features
- [ ] Add feed item filtering via URI parameters
- [ ] Implement resource pagination for large feeds
- [ ] Add resource access control and rate limiting
- [ ] Create resource analytics and monitoring

## Technical Specifications

### Resource Content Format
```json
{
  "uri": "feeds://feed/techcrunch",
  "name": "TechCrunch Feed",
  "description": "Latest technology news from TechCrunch",
  "mimeType": "application/json",
  "lastModified": "2024-08-29T10:30:00Z"
}
```

### Feed Resource Content
```json
{
  "feed": {
    "title": "TechCrunch",
    "description": "The latest technology news...",
    "link": "https://techcrunch.com",
    "updated": "2024-08-29T10:30:00Z"
  },
  "items": [
    {
      "title": "Breaking: New AI Breakthrough",
      "link": "https://techcrunch.com/article/...",
      "published": "2024-08-29T09:15:00Z"
    }
  ]
}
```

## Implementation Roadmap

### Immediate Next Steps (Week 1-2)
1. **Create resource infrastructure** in `mcpserver/resources.go`
2. **Add MCP SDK resource handlers** to existing server
3. **Implement basic feed-to-resource mapping**
4. **Add unit tests for resource functionality**

### Short Term (Week 3-4)  
1. **Implement resource subscriptions** with change detection
2. **Add resource caching layer** integrated with existing cache
3. **Create resource content serialization**
4. **Add integration tests**

### Medium Term (Month 2)
1. **Add URI parameter filtering** for feed items
2. **Implement resource pagination** for large feeds  
3. **Add comprehensive error handling** using existing FeedError system
4. **Create resource documentation**

### Long Term (Month 3+)
1. **Add resource access control** and rate limiting
2. **Implement advanced filtering** and search capabilities
3. **Add resource analytics** and monitoring
4. **Performance optimization** and scaling considerations

## Benefits of MCP Resources Integration

1. **Dynamic Discovery**: Clients can discover available feeds at runtime
2. **Real-time Updates**: Automatic notifications when feeds change
3. **Flexible Access**: URI templates allow parameterized access patterns
4. **Better UX**: Resources provide richer metadata than tools
5. **Standards Compliance**: Full MCP specification compliance
6. **Future Extensibility**: Foundation for advanced feed management features

## Success Metrics

- [ ] All MCP Resources methods implemented and tested
- [ ] Resource subscriptions working with real-time notifications
- [ ] URI template system supporting parameterized access
- [ ] Integration with existing caching and error handling systems
- [ ] Comprehensive test coverage (>90%)
- [ ] Documentation and examples for resource usage
- [ ] Performance benchmarks showing minimal overhead
- [ ] Backwards compatibility maintained with existing tools