package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/eko/gocache/lib/v4/cache"
	ristretto_store "github.com/eko/gocache/store/ristretto/v4"
	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/richardwooding/feed-mcp/model"
)

const (
	// Test constants to avoid goconst violations
	errorTestSessionID = "test-session"
	errorTestFeedURI   = "feeds://feed/test"
)

// MockErrorStore implements store interfaces with controllable error conditions
type MockErrorStore struct {
	shouldError    bool
	errorType      string
	customError    error
	feeds          []*model.FeedResult
	returnNotFound bool
}

func (m *MockErrorStore) GetAllFeeds(ctx context.Context) ([]*model.FeedResult, error) {
	if m.shouldError && m.errorType == "all_feeds" {
		if m.customError != nil {
			return nil, m.customError
		}
		return nil, errors.New("mock error getting all feeds")
	}
	if m.feeds != nil {
		return m.feeds, nil
	}
	return []*model.FeedResult{}, nil
}

func (m *MockErrorStore) GetFeedAndItems(ctx context.Context, feedID string) (*model.FeedAndItemsResult, error) {
	if m.shouldError && m.errorType == "get_feed" {
		if m.customError != nil {
			return nil, m.customError
		}
		if m.returnNotFound {
			return nil, errors.New("feed not found")
		}
		return nil, errors.New("mock error getting feed")
	}

	// Return a mock feed result
	return &model.FeedAndItemsResult{
		ID:        feedID,
		Title:     "Test Feed",
		PublicURL: "http://example.com/feed",
		Feed: &model.Feed{
			Title:       "Test Feed",
			Description: "Test Description",
			Link:        "http://example.com",
			FeedLink:    "http://example.com/feed",
		},
		Items: []*gofeed.Item{
			{
				Title:           "Test Item",
				Link:            "http://example.com/item1",
				Description:     "Test item description",
				PublishedParsed: &time.Time{},
			},
		},
	}, nil
}

// createTestResourceManagerForErrors creates a ResourceManager with proper cache initialization for error testing
func createTestResourceManagerForErrors(store *MockErrorStore) *ResourceManager {
	// Create a simple in-memory cache for testing
	ristrettoCache, _ := ristretto.NewCache[string, string](&ristretto.Config[string, string]{
		NumCounters: 100,
		MaxCost:     1000,
		BufferItems: 64,
	})
	ristrettoStore := ristretto_store.NewRistretto(ristrettoCache)
	resourceCache := cache.New[string](ristrettoStore)

	return &ResourceManager{
		store:              store,
		feedAndItemsGetter: store,
		resourceCache:      resourceCache,
		sessions:           make(map[string]*ResourceSession),
		cacheConfig: &ResourceCacheConfig{
			DefaultTTL:      time.Minute * 10,
			FeedListTTL:     time.Minute * 5,
			FeedItemsTTL:    time.Minute * 10,
			FeedMetadataTTL: time.Minute * 15,
		},
		cacheMetrics: &ResourceCacheMetrics{},
	}
}

// TestResourceNotFoundErrors tests error handling for resource not found scenarios
func TestResourceNotFoundErrors(t *testing.T) {
	mockStore := &MockErrorStore{
		shouldError:    true,
		errorType:      "get_feed",
		returnNotFound: true,
	}

	rm := createTestResourceManagerForErrors(mockStore)

	ctx := context.Background()

	// Test feed resource not found
	result, err := rm.readFeed(ctx, "feeds://feed/12345")
	assert.Nil(t, result)
	require.Error(t, err)

	var feedErr *model.FeedError
	assert.True(t, errors.As(err, &feedErr))
	assert.Equal(t, model.ErrorTypeResourceNotFound, feedErr.ErrorType)
	assert.Contains(t, feedErr.Message, "Feed not found")
	assert.Equal(t, "feeds://feed/12345", feedErr.URL)
	assert.Equal(t, "read_feed", feedErr.Operation)
	assert.Equal(t, "resource_manager", feedErr.Component)
	assert.NotEmpty(t, feedErr.ID)
	assert.NotEmpty(t, feedErr.Suggestion)

	// Test feed items resource not found
	result, err = rm.readFeedItems(ctx, "feeds://feed/12345/items")
	assert.Nil(t, result)
	require.Error(t, err)

	assert.True(t, errors.As(err, &feedErr))
	assert.Equal(t, model.ErrorTypeResourceNotFound, feedErr.ErrorType)
	assert.Contains(t, feedErr.Message, "Feed not found")
	assert.Equal(t, "feeds://feed/12345/items", feedErr.URL)
	assert.Equal(t, "read_feed_items", feedErr.Operation)

	// Test feed metadata resource not found
	result, err = rm.readFeedMeta(ctx, "feeds://feed/12345/meta")
	assert.Nil(t, result)
	require.Error(t, err)

	assert.True(t, errors.As(err, &feedErr))
	assert.Equal(t, model.ErrorTypeResourceNotFound, feedErr.ErrorType)
	assert.Equal(t, "read_feed_meta", feedErr.Operation)
}

// TestResourceUnavailableErrors tests error handling for resource unavailable scenarios
func TestResourceUnavailableErrors(t *testing.T) {
	mockStore := &MockErrorStore{
		shouldError: true,
		errorType:   "get_feed",
		customError: errors.New("network timeout"),
	}

	rm := createTestResourceManagerForErrors(mockStore)

	ctx := context.Background()

	// Test feed resource unavailable
	result, err := rm.readFeed(ctx, errorTestFeedURI)
	assert.Nil(t, result)
	require.Error(t, err)

	var feedErr *model.FeedError
	assert.True(t, errors.As(err, &feedErr))
	assert.Equal(t, model.ErrorTypeResourceUnavailable, feedErr.ErrorType)
	assert.Contains(t, feedErr.Message, "network timeout")
	assert.Equal(t, errorTestFeedURI, feedErr.URL)
	assert.Equal(t, "read_feed", feedErr.Operation)
	assert.Equal(t, "resource_manager", feedErr.Component)
	assert.NotEmpty(t, feedErr.ID)
}

// TestInvalidResourceURIErrors tests error handling for invalid resource URIs
func TestInvalidResourceURIErrors(t *testing.T) {
	rm := &ResourceManager{}
	ctx := context.Background()

	testCases := []struct {
		name            string
		uri             string
		expectedMessage string
	}{
		{
			name:            "Invalid scheme",
			uri:             "http://example.com/feed",
			expectedMessage: "URI does not match any supported resource patterns",
		},
		{
			name:            "Unknown resource pattern",
			uri:             "feeds://unknown/pattern",
			expectedMessage: "URI does not match any supported resource patterns",
		},
		{
			name:            "Empty feed ID",
			uri:             "feeds://feed//items",
			expectedMessage: "URI does not match any supported resource patterns",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := rm.ReadResource(ctx, tc.uri)

			assert.Nil(t, result)
			require.Error(t, err)

			var feedErr *model.FeedError
			assert.True(t, errors.As(err, &feedErr))
			assert.Equal(t, model.ErrorTypeInvalidResourceURI, feedErr.ErrorType)
			assert.Contains(t, feedErr.Message, tc.expectedMessage)
			assert.Equal(t, tc.uri, feedErr.URL)
			assert.Equal(t, "resource_manager", feedErr.Component)
			assert.NotEmpty(t, feedErr.ID)
		})
	}
}

// TestResourceContentErrors tests error handling for resource content generation errors
func TestResourceContentErrors(t *testing.T) {
	// Create a type that will fail JSON marshaling
	type problematicType struct {
		Chan chan int `json:"chan"` // channels can't be marshaled to JSON
	}

	// Mock the marshalJSONContent function by testing the error it would generate
	uri := errorTestFeedURI
	data := map[string]interface{}{
		"problematic": problematicType{Chan: make(chan int)},
	}

	_, err := json.Marshal(data)
	require.Error(t, err)

	// Test that our error helper creates the right error type
	contentErr := model.CreateResourceContentError(err, uri, "marshal_json")

	assert.Equal(t, model.ErrorTypeResourceContent, contentErr.ErrorType)
	assert.Contains(t, contentErr.Message, "Failed to generate resource content")
	assert.Equal(t, uri, contentErr.URL)
	assert.Equal(t, "marshal_json", contentErr.Operation)
	assert.Equal(t, "resource_manager", contentErr.Component)
	assert.NotEmpty(t, contentErr.ID)
	assert.NotNil(t, contentErr.Cause)
}

// TestSessionErrors tests error handling for session management issues
func TestSessionErrors(t *testing.T) {
	sessionID := errorTestSessionID

	// Test session not found error
	sessionErr := model.CreateSessionError(
		errors.New("session not found"),
		sessionID,
		"get_session",
	)

	assert.Equal(t, model.ErrorTypeSessionNotFound, sessionErr.ErrorType)
	assert.Contains(t, sessionErr.Message, "Session not found")
	assert.Equal(t, fmt.Sprintf("session://%s", sessionID), sessionErr.URL)
	assert.Equal(t, "get_session", sessionErr.Operation)
	assert.Equal(t, "resource_manager", sessionErr.Component)
	assert.NotEmpty(t, sessionErr.ID)

	// Test general session error
	generalErr := model.CreateSessionError(
		errors.New("session timeout"),
		sessionID,
		"update_session",
	)

	assert.Equal(t, model.ErrorTypeSession, generalErr.ErrorType)
	assert.Contains(t, generalErr.Message, "Session operation failed")
	assert.Equal(t, "update_session", generalErr.Operation)
}

// TestSubscriptionErrors tests error handling for subscription issues
func TestSubscriptionErrors(t *testing.T) {
	resourceURI := errorTestFeedURI
	sessionID := errorTestSessionID

	// Test subscription already exists error
	existsErr := model.CreateSubscriptionError(
		errors.New("already subscribed"),
		resourceURI,
		sessionID,
		"subscribe",
	)

	assert.Equal(t, model.ErrorTypeSubscriptionExists, existsErr.ErrorType)
	assert.Contains(t, existsErr.Message, "Already subscribed to resource")
	assert.Equal(t, resourceURI, existsErr.URL)
	assert.Equal(t, "subscribe", existsErr.Operation)
	assert.Equal(t, "resource_manager", existsErr.Component)
	assert.NotEmpty(t, existsErr.ID)
	assert.Contains(t, existsErr.HTTPHeaders["X-Session-ID"], sessionID)

	// Test subscription not found error
	notFoundErr := model.CreateSubscriptionError(
		errors.New("no subscription"),
		resourceURI,
		sessionID,
		"unsubscribe",
	)

	assert.Equal(t, model.ErrorTypeSubscriptionNotFound, notFoundErr.ErrorType)
	assert.Contains(t, notFoundErr.Message, "Subscription not found")
}

// TestResourceCacheErrors tests error handling for cache-related issues
func TestResourceCacheErrors(t *testing.T) {
	cacheKey := "resource:feeds://feed/test"

	// Test general cache error
	cacheErr := model.CreateResourceCacheError(
		errors.New("cache connection failed"),
		cacheKey,
		"cache_get",
	)

	assert.Equal(t, model.ErrorTypeResourceCache, cacheErr.ErrorType)
	assert.Contains(t, cacheErr.Message, "Resource cache operation failed")
	assert.Equal(t, fmt.Sprintf("cache://%s", cacheKey), cacheErr.URL)
	assert.Equal(t, "cache_get", cacheErr.Operation)
	assert.Equal(t, "resource_cache", cacheErr.Component)
	assert.NotEmpty(t, cacheErr.ID)

	// Test cache invalidation error
	invalidateErr := model.CreateResourceCacheError(
		errors.New("invalidation failed"),
		cacheKey,
		"invalidate",
	)

	assert.Equal(t, model.ErrorTypeCacheInvalidation, invalidateErr.ErrorType)
	assert.Contains(t, invalidateErr.Message, "Cache invalidation failed")
}

// TestErrorWrappingAndUnwrapping tests that errors are properly wrapped and can be unwrapped
func TestErrorWrappingAndUnwrapping(t *testing.T) {
	originalErr := errors.New("original network error")

	// Test wrapping
	wrappedErr := model.CreateResourceUnavailableError(errorTestFeedURI, originalErr.Error())

	// Test that the original error message is included
	assert.Contains(t, wrappedErr.Error(), originalErr.Error())

	// Test error chain functionality
	networkErr := model.CreateNetworkError(originalErr, "http://example.com/feed")
	resourceErr := model.CreateResourceUnavailableError(errorTestFeedURI, networkErr.Error())

	// Should be able to find the network error in the chain
	var feedErr *model.FeedError
	assert.True(t, errors.As(resourceErr, &feedErr))
}

// TestErrorCorrelationIDs tests that all errors have unique correlation IDs
func TestErrorCorrelationIDs(t *testing.T) {
	// Create multiple errors and ensure they have unique IDs
	errors := []*model.FeedError{
		model.CreateResourceError(errors.New("error1"), "uri1", "op1"),
		model.CreateResourceNotFoundError("uri2", "feed2"),
		model.CreateResourceUnavailableError("uri3", "reason3"),
		model.CreateInvalidResourceURIError("uri4", "details4"),
		model.CreateResourceContentError(errors.New("error5"), "uri5", "op5"),
		model.CreateSessionError(errors.New("error6"), "session6", "op6"),
		model.CreateSubscriptionError(errors.New("error7"), "uri7", "session7", "op7"),
		model.CreateResourceCacheError(errors.New("error8"), "cache8", "op8"),
	}

	// Collect all IDs
	ids := make(map[string]bool)
	for _, err := range errors {
		assert.NotEmpty(t, err.ID, "Error should have correlation ID")
		assert.NotEmpty(t, err.Timestamp, "Error should have timestamp")

		// Check ID uniqueness
		assert.False(t, ids[err.ID], "Correlation ID should be unique: %s", err.ID)
		ids[err.ID] = true

		// Check ID format (nanoid should be alphanumeric)
		assert.Regexp(t, `^[A-Za-z0-9_-]+$`, err.ID, "Correlation ID should be valid nanoid format")
	}

	// Should have collected unique IDs for all errors
	assert.Equal(t, len(errors), len(ids), "All errors should have unique correlation IDs")
}

// TestErrorSuggestions tests that all resource error types have actionable suggestions
func TestErrorSuggestions(t *testing.T) {
	testCases := []struct {
		errorType            model.ErrorType
		shouldHaveSuggestion bool
	}{
		{model.ErrorTypeResource, true},
		{model.ErrorTypeResourceNotFound, true},
		{model.ErrorTypeResourceUnavailable, true},
		{model.ErrorTypeInvalidResourceURI, true},
		{model.ErrorTypeResourceContent, true},
		{model.ErrorTypeSession, true},
		{model.ErrorTypeSessionNotFound, true},
		{model.ErrorTypeSubscription, true},
		{model.ErrorTypeSubscriptionExists, true},
		{model.ErrorTypeSubscriptionNotFound, true},
		{model.ErrorTypeResourceCache, true},
		{model.ErrorTypeCacheInvalidation, true},
	}

	for _, tc := range testCases {
		t.Run(string(tc.errorType), func(t *testing.T) {
			err := model.NewFeedError(tc.errorType, "test message")

			if tc.shouldHaveSuggestion {
				assert.NotEmpty(t, err.Suggestion, "Error type %s should have suggestion", tc.errorType)
				assert.NotEqual(t, "Check the error details and try again", err.Suggestion,
					"Error type %s should have specific suggestion, not default", tc.errorType)
			}
		})
	}
}

// TestErrorContextEnrichment tests that errors are properly enriched with context
func TestErrorContextEnrichment(t *testing.T) {
	uri := errorTestFeedURI
	operation := "test_operation"

	err := model.CreateResourceError(errors.New("base error"), uri, operation)

	// Check context enrichment
	assert.Equal(t, uri, err.URL)
	assert.Equal(t, operation, err.Operation)
	assert.Equal(t, "resource_manager", err.Component)
	assert.NotEmpty(t, err.ID)
	assert.NotZero(t, err.Timestamp)

	// Test method chaining for additional context
	enrichedErr := err.WithHTTP(404, nil).WithNetworkError("connection refused")

	assert.Equal(t, 404, enrichedErr.HTTPStatus)
	assert.Equal(t, "connection refused", enrichedErr.NetworkError)
	assert.Equal(t, err.ID, enrichedErr.ID) // Should preserve original ID
}

// TestErrorJSONSerialization tests that errors can be properly serialized for logging/debugging
func TestErrorJSONSerialization(t *testing.T) {
	originalErr := errors.New("network connection failed")
	feedErr := model.CreateNetworkError(originalErr, "http://example.com/feed").
		WithOperation("fetch_feed").
		WithComponent("http_client").
		WithHTTP(500, nil)

	// Test JSON serialization (Cause field should be excluded)
	jsonData, err := json.Marshal(feedErr)
	require.NoError(t, err)

	var unmarshalled map[string]interface{}
	err = json.Unmarshal(jsonData, &unmarshalled)
	require.NoError(t, err)

	// Check required fields are present
	assert.NotEmpty(t, unmarshalled["id"])
	assert.NotEmpty(t, unmarshalled["timestamp"])
	// CreateNetworkError returns ErrorTypeNetwork initially, not ErrorTypeTimeout
	assert.Equal(t, string(model.ErrorTypeNetwork), unmarshalled["error_type"])
	assert.NotEmpty(t, unmarshalled["message"])
	assert.NotEmpty(t, unmarshalled["suggestion"])
	assert.Equal(t, "http://example.com/feed", unmarshalled["url"])
	assert.Equal(t, "fetch_feed", unmarshalled["operation"])
	assert.Equal(t, "http_client", unmarshalled["component"])
	assert.Equal(t, float64(500), unmarshalled["http_status"])

	// Cause field should not be in JSON
	_, hasNilCause := unmarshalled["cause"]
	assert.False(t, hasNilCause, "Cause field should not be serialized to JSON")
}
