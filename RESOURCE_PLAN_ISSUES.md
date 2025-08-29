# MCP Resources Implementation Issues

This document breaks down the MCP Resources implementation plan into discrete GitHub issues with clear success criteria.

## Issue 1: Core Resource Infrastructure
**Title**: Implement core MCP Resources infrastructure and URI template system

**Description**: 
Create the foundational infrastructure for MCP Resources support including ResourceManager, URI template parsing, and basic resource listing functionality.

**Acceptance Criteria**:
- [ ] Create `mcpserver/resources.go` with ResourceManager struct
- [ ] Implement URI template constants and parsing functions
- [ ] Add basic resource listing capability (`resources/list` handler)
- [ ] Add resource reading capability (`resources/read` handler)
- [ ] ResourceManager integrates with existing store interface
- [ ] Unit tests for URI template parsing and matching
- [ ] Unit tests for ResourceManager basic operations
- [ ] No breaking changes to existing MCP tools functionality

**Technical Requirements**:
- ResourceManager struct with thread-safe operations
- URI template system supporting `feeds://` scheme
- Integration with existing `store.AllFeedsGetter` interface
- Error handling using existing FeedError system

**Files to Create/Modify**:
- Create: `mcpserver/resources.go`
- Create: `mcpserver/resources_test.go`
- Modify: `mcpserver/server.go` (add resource handlers)

---

## Issue 2: Feed-to-Resource Mapping System
**Title**: Implement feed URL to resource identifier mapping

**Description**: 
Create a system to map feed URLs to resource identifiers and implement resource content serialization for individual feeds.

**Acceptance Criteria**:
- [ ] Feed URLs mapped to stable resource identifiers
- [ ] Resource URIs follow template: `feeds://feed/{feedId}`
- [ ] Feed content serialized properly for resource responses
- [ ] Feed metadata extraction for resource descriptions
- [ ] Support for all three resource types: feed, items, metadata
- [ ] Resource content includes proper MIME types and timestamps
- [ ] Unit tests for feed-to-resource mapping
- [ ] Integration tests with real feed data

**Technical Requirements**:
- Stable feed ID generation (URL hash or slug)
- JSON serialization of feed content for resources
- Metadata extraction from feed headers
- Integration with existing feed parsing logic

**Files to Create/Modify**:
- Modify: `mcpserver/resources.go`
- Modify: `model/feed.go` (add resource methods if needed)
- Create: `mcpserver/feed_resources_test.go`

---

## Issue 3: Resource Subscription Management
**Title**: Implement MCP resource subscriptions with change notifications

**Description**: 
Add support for resource subscriptions allowing clients to receive notifications when feed content changes.

**Acceptance Criteria**:
- [ ] Implement `resources/subscribe` and `resources/unsubscribe` handlers
- [ ] ResourceSession management for tracking subscriptions
- [ ] Change detection mechanism for feed updates
- [ ] Resource change notifications sent to subscribed clients
- [ ] Session cleanup when clients disconnect
- [ ] Subscription state persisted during server operation
- [ ] Unit tests for subscription lifecycle
- [ ] Integration tests for change notifications

**Technical Requirements**:
- Session management with unique session IDs
- Change detection using feed timestamps or content hashes
- Notification system using MCP protocol
- Thread-safe subscription tracking
- Memory-efficient session cleanup

**Files to Create/Modify**:
- Modify: `mcpserver/resources.go`
- Modify: `mcpserver/server.go` (add subscription handlers)
- Create: `mcpserver/subscriptions_test.go`
- Modify: `store/store.go` (add change detection if needed)

---

## Issue 4: Resource Caching Integration
**Title**: Integrate MCP Resources with existing caching system

**Description**: 
Ensure MCP Resources leverage the existing cache infrastructure and implement resource-specific caching strategies.

**Acceptance Criteria**:
- [ ] Resource content cached using existing gocache/ristretto system
- [ ] Cache keys designed for resource URIs
- [ ] Cache invalidation triggers resource change notifications
- [ ] Resource cache TTL configurable independently
- [ ] Cache hit/miss metrics available for resources
- [ ] No duplicate caching between tools and resources
- [ ] Performance tests showing cache effectiveness
- [ ] Unit tests for cache integration

**Technical Requirements**:
- Resource-specific cache key strategy
- Integration with existing cache configuration
- Cache invalidation hooks for notifications
- Performance monitoring for resource cache

**Files to Create/Modify**:
- Modify: `mcpserver/resources.go`
- Modify: `store/store.go` (add resource cache hooks)
- Create: `mcpserver/resource_cache_test.go`
- Modify: `model/config.go` (add resource cache config)

---

## Issue 5: URI Parameter Filtering
**Title**: Add URI parameter support for feed item filtering

**Description**: 
Implement URI parameter parsing to allow filtering of feed items by date, category, or other criteria.

**Acceptance Criteria**:
- [ ] URI parameter parsing for resource requests
- [ ] Support for common filters: `since`, `limit`, `category`
- [ ] Date range filtering for feed items
- [ ] Item count limiting with pagination support
- [ ] Category/tag filtering if available in feed
- [ ] Invalid parameter handling with clear error messages
- [ ] Unit tests for all filter types
- [ ] Documentation for supported parameters

**Technical Requirements**:
- URL parameter parsing and validation
- Filter logic for different feed item attributes
- Pagination support for large result sets
- Error handling for invalid parameters

**Files to Create/Modify**:
- Modify: `mcpserver/resources.go`
- Create: `mcpserver/resource_filters.go`
- Create: `mcpserver/resource_filters_test.go`

---

## Issue 6: Error Handling and Validation
**Title**: Comprehensive error handling for MCP Resources

**Description**: 
Implement robust error handling for all resource operations using the existing FeedError system.

**Acceptance Criteria**:
- [ ] All resource operations use structured FeedError types
- [ ] Resource-specific error types added to ErrorType enum
- [ ] Clear error messages for invalid resource URIs
- [ ] Proper error handling for subscription failures
- [ ] Error correlation IDs for resource operations
- [ ] Resource operation errors logged appropriately
- [ ] Unit tests for all error scenarios
- [ ] Error handling documentation

**Technical Requirements**:
- Extension of existing FeedError system
- Resource-specific error categorization
- Integration with debug logging system
- Consistent error response format

**Files to Create/Modify**:
- Modify: `model/errors.go` (add resource error types)
- Modify: `model/error_helpers.go` (add resource error helpers)
- Modify: `mcpserver/resources.go`
- Create: `mcpserver/resource_errors_test.go`

---

## Issue 7: Performance Optimization and Benchmarks
**Title**: Optimize MCP Resources performance and add benchmarks

**Description**: 
Ensure MCP Resources operations perform efficiently and establish performance benchmarks.

**Acceptance Criteria**:
- [ ] Benchmark tests for all resource operations
- [ ] Resource listing performance under 100ms for 100 feeds
- [ ] Resource reading performance under 50ms with cache hits
- [ ] Memory usage profiling for resource sessions
- [ ] Concurrent access performance testing
- [ ] Resource subscription overhead measurement
- [ ] Performance regression tests in CI
- [ ] Performance documentation and recommendations

**Technical Requirements**:
- Go benchmark tests using testing.B
- Memory profiling and optimization
- Concurrent access testing
- Performance monitoring integration

**Files to Create/Modify**:
- Create: `mcpserver/resources_bench_test.go`
- Create: `performance/resource_benchmarks.go`
- Modify: CI configuration for performance tests

---

## Issue 8: Documentation and Examples
**Title**: Create comprehensive documentation for MCP Resources

**Description**: 
Document the MCP Resources implementation with usage examples, API reference, and integration guides.

**Acceptance Criteria**:
- [ ] README section explaining MCP Resources support
- [ ] API documentation for all resource endpoints
- [ ] Usage examples for each resource type
- [ ] Client integration examples
- [ ] Performance tuning guide
- [ ] Migration guide from tools to resources
- [ ] Troubleshooting section
- [ ] Code examples tested and working

**Technical Requirements**:
- Markdown documentation
- Working code examples
- API reference generation
- Integration with existing documentation

**Files to Create/Modify**:
- Modify: `README.md`
- Create: `docs/MCP_RESOURCES.md`
- Create: `examples/resource_usage.go`
- Create: `docs/RESOURCE_API.md`

---

## Implementation Order

1. **Issue 1**: Core Resource Infrastructure (Foundation)
2. **Issue 2**: Feed-to-Resource Mapping (Core functionality)
3. **Issue 4**: Resource Caching Integration (Performance)
4. **Issue 6**: Error Handling and Validation (Robustness)
5. **Issue 3**: Resource Subscription Management (Advanced features)
6. **Issue 5**: URI Parameter Filtering (Enhancement)
7. **Issue 7**: Performance Optimization (Polish)
8. **Issue 8**: Documentation and Examples (Completion)

## Success Criteria Summary

Each issue must meet the following overall requirements:
- All tests pass (unit and integration)
- No breaking changes to existing functionality
- Code coverage maintained above 90%
- golangci-lint passes with zero violations
- Performance benchmarks within acceptable ranges
- Documentation updated appropriately
- Changes reviewed and approved via PR process