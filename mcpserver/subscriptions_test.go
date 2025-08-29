package mcpserver

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/richardwooding/feed-mcp/model"
)

// Note: Mock types are defined in server_test.go and will be reused here

func TestResourceManagerSubscriptions(t *testing.T) {
	// Create mock dependencies
	mockStore := &mockAllFeedsGetter{
		feeds: []*model.FeedResult{
			{
				ID:        "test-feed-1",
				PublicURL: "https://example.com/feed.xml",
				Title:     "Test Feed 1",
				Feed: &model.Feed{
					Title: "Test Feed 1",
				},
			},
		},
	}
	mockGetter := &mockFeedAndItemsGetter{
		feedMap: map[string]*model.FeedAndItemsResult{
			"test-feed-1": {
				ID:    "test-feed-1",
				Title: "Test Feed 1",
				Feed: &model.Feed{
					Title: "Test Feed 1",
				},
			},
		},
	}

	// Create ResourceManager
	rm := NewResourceManager(mockStore, mockGetter)

	// Test session creation
	sessionID := "test-session-1"
	session := rm.CreateSession(sessionID)

	if session.id != sessionID {
		t.Errorf("Expected session ID %s, got %s", sessionID, session.id)
	}

	// Test subscription
	testURI := "feeds://feed/test-feed-1"
	err := rm.Subscribe(sessionID, testURI)
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Verify subscription
	if !session.IsSubscribed(testURI) {
		t.Error("Session should be subscribed to the URI")
	}

	// Test subscription count
	if session.GetSubscriptionCount() != 1 {
		t.Errorf("Expected 1 subscription, got %d", session.GetSubscriptionCount())
	}

	// Test getting subscriptions
	subscriptions := session.GetSubscriptions()
	if len(subscriptions) != 1 || subscriptions[0] != testURI {
		t.Errorf("Expected [%s], got %v", testURI, subscriptions)
	}

	// Test getting subscribed sessions
	subscribedSessions := rm.GetSubscribedSessions(testURI)
	if len(subscribedSessions) != 1 || subscribedSessions[0] != sessionID {
		t.Errorf("Expected [%s], got %v", sessionID, subscribedSessions)
	}

	// Test unsubscription
	err = rm.Unsubscribe(sessionID, testURI)
	if err != nil {
		t.Fatalf("Failed to unsubscribe: %v", err)
	}

	// Verify unsubscription
	if session.IsSubscribed(testURI) {
		t.Error("Session should not be subscribed to the URI after unsubscribing")
	}

	// Test subscription count after unsubscription
	if session.GetSubscriptionCount() != 0 {
		t.Errorf("Expected 0 subscriptions, got %d", session.GetSubscriptionCount())
	}
}

func TestResourceManagerMultipleSubscriptions(t *testing.T) {
	// Create mock dependencies
	mockStore := &mockAllFeedsGetter{
		feeds: []*model.FeedResult{
			{
				ID:        "test-feed-1",
				PublicURL: "https://example.com/feed1.xml",
				Title:     "Test Feed 1",
			},
			{
				ID:        "test-feed-2",
				PublicURL: "https://example.com/feed2.xml",
				Title:     "Test Feed 2",
			},
		},
	}
	mockGetter := &mockFeedAndItemsGetter{
		feedMap: make(map[string]*model.FeedAndItemsResult),
	}

	rm := NewResourceManager(mockStore, mockGetter)

	// Create multiple sessions
	session1ID := "session-1"
	session2ID := "session-2"
	rm.CreateSession(session1ID)
	rm.CreateSession(session2ID)

	// Subscribe to different URIs
	uri1 := "feeds://feed/test-feed-1"
	uri2 := "feeds://feed/test-feed-2"
	uri3 := "feeds://all"

	// Session 1 subscribes to uri1 and uri3
	rm.Subscribe(session1ID, uri1)
	rm.Subscribe(session1ID, uri3)

	// Session 2 subscribes to uri2 and uri3
	rm.Subscribe(session2ID, uri2)
	rm.Subscribe(session2ID, uri3)

	// Test GetAllSubscribedURIs
	subscribedURIs := rm.GetAllSubscribedURIs()
	expectedURIs := map[string]bool{uri1: true, uri2: true, uri3: true}

	if len(subscribedURIs) != 3 {
		t.Errorf("Expected 3 subscribed URIs, got %d", len(subscribedURIs))
	}

	for _, uri := range subscribedURIs {
		if !expectedURIs[uri] {
			t.Errorf("Unexpected URI in subscribed list: %s", uri)
		}
	}

	// Test sessions subscribed to uri3 (both should be)
	subscribedToURI3 := rm.GetSubscribedSessions(uri3)
	if len(subscribedToURI3) != 2 {
		t.Errorf("Expected 2 sessions subscribed to %s, got %d", uri3, len(subscribedToURI3))
	}

	// Test sessions subscribed to uri1 (only session1)
	subscribedToURI1 := rm.GetSubscribedSessions(uri1)
	if len(subscribedToURI1) != 1 || subscribedToURI1[0] != session1ID {
		t.Errorf("Expected only session1 subscribed to %s, got %v", uri1, subscribedToURI1)
	}
}

func TestResourceManagerSessionCleanup(t *testing.T) {
	mockStore := &mockAllFeedsGetter{}
	mockGetter := &mockFeedAndItemsGetter{}
	rm := NewResourceManager(mockStore, mockGetter)

	// Create session and subscribe
	sessionID := "test-session"
	rm.CreateSession(sessionID)
	testURI := "feeds://feed/test"
	rm.Subscribe(sessionID, testURI)

	// Verify session exists
	_, exists := rm.GetSession(sessionID)
	if !exists {
		t.Error("Session should exist")
	}

	// Remove session
	rm.RemoveSession(sessionID)

	// Verify session is removed
	_, exists = rm.GetSession(sessionID)
	if exists {
		t.Error("Session should be removed")
	}

	// Verify no sessions are subscribed to the URI
	subscribedSessions := rm.GetSubscribedSessions(testURI)
	if len(subscribedSessions) != 0 {
		t.Errorf("Expected no subscribed sessions, got %d", len(subscribedSessions))
	}
}

func TestResourceManagerSubscriptionErrors(t *testing.T) {
	mockStore := &mockAllFeedsGetter{}
	mockGetter := &mockFeedAndItemsGetter{}
	rm := NewResourceManager(mockStore, mockGetter)

	// Test subscribing to non-existent session
	err := rm.Subscribe("non-existent-session", "some-uri")
	if err == nil {
		t.Error("Expected error when subscribing with non-existent session")
	}

	// Test unsubscribing from non-existent session
	err = rm.Unsubscribe("non-existent-session", "some-uri")
	if err == nil {
		t.Error("Expected error when unsubscribing with non-existent session")
	}
}

func TestResourceManagerChangeDetection(t *testing.T) {
	// Create mock with feeds
	mockStore := &mockAllFeedsGetter{
		feeds: []*model.FeedResult{
			{
				ID:        "feed-1",
				PublicURL: "https://example.com/feed1.xml",
				Title:     "Test Feed 1",
			},
			{
				ID:        "feed-2",
				PublicURL: "https://example.com/feed2.xml",
				Title:     "Test Feed 2",
			},
		},
	}
	mockGetter := &mockFeedAndItemsGetter{
		feedMap: make(map[string]*model.FeedAndItemsResult),
	}
	rm := NewResourceManager(mockStore, mockGetter)

	ctx := context.Background()
	changedURIs, err := rm.DetectResourceChanges(ctx)
	if err != nil {
		t.Fatalf("Failed to detect resource changes: %v", err)
	}

	// Should detect changes in feed list + all individual feed resources
	// 1 feed list + 2 feeds * 3 resources each = 7 total
	expectedMinChanges := 7
	if len(changedURIs) < expectedMinChanges {
		t.Errorf("Expected at least %d changed URIs, got %d", expectedMinChanges, len(changedURIs))
	}

	// Verify feed list URI is included
	feedListFound := false
	for _, uri := range changedURIs {
		if uri == FeedListURI {
			feedListFound = true
			break
		}
	}
	if !feedListFound {
		t.Error("Feed list URI should be in changed URIs")
	}
}

func TestResourceManagerConcurrentAccess(t *testing.T) {
	mockStore := &mockAllFeedsGetter{
		feeds: []*model.FeedResult{},
	}
	mockGetter := &mockFeedAndItemsGetter{
		feedMap: make(map[string]*model.FeedAndItemsResult),
	}
	rm := NewResourceManager(mockStore, mockGetter)

	// Test concurrent session creation and subscription
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			sessionID := fmt.Sprintf("session-%d", id)
			rm.CreateSession(sessionID)

			testURI := fmt.Sprintf("feeds://feed/test-%d", id)
			rm.Subscribe(sessionID, testURI)

			// Test subscription
			session, exists := rm.GetSession(sessionID)
			if !exists {
				t.Errorf("Session %s should exist", sessionID)
			} else if !session.IsSubscribed(testURI) {
				t.Errorf("Session %s should be subscribed to %s", sessionID, testURI)
			}

			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all sessions were created
	allURIs := rm.GetAllSubscribedURIs()
	if len(allURIs) != 10 {
		t.Errorf("Expected 10 subscribed URIs, got %d", len(allURIs))
	}
}

func TestResourceSessionMethods(t *testing.T) {
	session := &ResourceSession{
		id:            "test-session",
		subscriptions: make(map[string]bool),
		lastUpdate:    time.Now(),
	}

	// Test initial state
	if session.GetSubscriptionCount() != 0 {
		t.Error("New session should have 0 subscriptions")
	}

	// Test subscription through direct access
	testURIs := []string{
		"feeds://feed/test-1",
		"feeds://feed/test-2",
		"feeds://all",
	}

	session.mu.Lock()
	for _, uri := range testURIs {
		session.subscriptions[uri] = true
	}
	session.mu.Unlock()

	// Test subscription count
	if session.GetSubscriptionCount() != 3 {
		t.Errorf("Expected 3 subscriptions, got %d", session.GetSubscriptionCount())
	}

	// Test IsSubscribed
	for _, uri := range testURIs {
		if !session.IsSubscribed(uri) {
			t.Errorf("Session should be subscribed to %s", uri)
		}
	}

	// Test GetSubscriptions
	subscriptions := session.GetSubscriptions()
	if len(subscriptions) != 3 {
		t.Errorf("Expected 3 subscriptions, got %d", len(subscriptions))
	}

	// Verify all URIs are present
	uriMap := make(map[string]bool)
	for _, uri := range subscriptions {
		uriMap[uri] = true
	}
	for _, uri := range testURIs {
		if !uriMap[uri] {
			t.Errorf("URI %s not found in subscriptions", uri)
		}
	}
}
