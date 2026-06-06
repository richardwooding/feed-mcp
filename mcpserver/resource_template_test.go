package mcpserver

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/richardwooding/feed-mcp/model"
)

// buildTestServerSession wires a fully-registered MCP server (tools, resources,
// templates, prompts) to an in-memory transport and returns a connected client
// session. This exercises the real SDK resources/read dispatch path, unlike
// tests that call ResourceManager.ReadResource directly.
func buildTestServerSession(t *testing.T, feedID, publicURL string, items []*gofeed.Item) *mcp.ClientSession {
	t.Helper()

	feedResult := &model.FeedResult{
		ID:        feedID,
		Title:     "Template Test Feed",
		PublicURL: publicURL,
		Feed: &model.Feed{
			Title:    "Template Test Feed",
			Link:     "https://example.com",
			FeedType: "rss",
		},
	}
	allFeeds := &mockResourceAllFeedsGetter{feeds: []*model.FeedResult{feedResult}}
	feedGetter := &mockResourceFeedAndItemsGetter{feeds: map[string]*model.FeedAndItemsResult{
		feedID: {
			ID:        feedID,
			PublicURL: publicURL,
			Title:     "Template Test Feed",
			Feed:      feedResult.Feed,
			Items:     items,
		},
	}}

	srv, err := NewServer(&Config{
		Transport:          model.StdioTransport,
		AllFeedsGetter:     allFeeds,
		FeedAndItemsGetter: feedGetter,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	mcpServer := srv.buildMCPServer()

	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := mcpServer.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	return clientSession
}

func makeTestItems(n int) []*gofeed.Item {
	items := make([]*gofeed.Item, 0, n)
	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	for i := range n {
		pub := base.AddDate(0, 0, i)
		items = append(items, &gofeed.Item{
			Title:           "Item",
			Link:            "https://example.com/item",
			Published:       pub.Format(time.RFC3339),
			PublishedParsed: &pub,
		})
	}
	return items
}

// TestResourceReadWithQueryParams is a regression test for the bug where
// resources/read of a templated URI carrying a query string (e.g.
// feeds://feed/{id}/items?limit=3) returned "Resource not found" because the
// server registered only concrete URIs and no resource template. The SDK
// matches concrete URIs by exact string and then falls back to templates, so a
// query string caused the read to fail before ReadResource was reached.
func TestResourceReadWithQueryParams(t *testing.T) {
	const publicURL = "https://example.com/feed.xml"
	feedID := model.GenerateFeedID(publicURL)
	cs := buildTestServerSession(t, feedID, publicURL, makeTestItems(10))
	ctx := context.Background()

	cases := []struct {
		name string
		uri  string
	}{
		{"base feed", "feeds://feed/" + feedID},
		{"items no query", "feeds://feed/" + feedID + "/items"},
		{"meta", "feeds://feed/" + feedID + "/meta"},
		{"items with limit", "feeds://feed/" + feedID + "/items?limit=3"},
		{"items with multiple params", "feeds://feed/" + feedID + "/items?search=item&limit=2&offset=1"},
		// RFC 3339 timestamp values contain ':' — only reserved-expansion
		// template matching handles these.
		{"items with rfc3339 since", "feeds://feed/" + feedID + "/items?since=2024-01-03T00:00:00Z"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: tc.uri})
			if err != nil {
				t.Fatalf("ReadResource(%q) returned error: %v", tc.uri, err)
			}
			if len(res.Contents) == 0 || res.Contents[0].Text == "" {
				t.Fatalf("ReadResource(%q) returned empty content", tc.uri)
			}
		})
	}
}

// TestResourceQueryFilterApplied verifies that query parameters are not merely
// accepted but actually applied: a limit reduces the number of returned items.
func TestResourceQueryFilterApplied(t *testing.T) {
	const publicURL = "https://example.com/feed.xml"
	feedID := model.GenerateFeedID(publicURL)
	cs := buildTestServerSession(t, feedID, publicURL, makeTestItems(10))
	ctx := context.Background()

	countItems := func(uri string) int {
		res, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
		if err != nil {
			t.Fatalf("ReadResource(%q): %v", uri, err)
		}
		if len(res.Contents) == 0 {
			t.Fatalf("ReadResource(%q): no contents", uri)
		}
		// The items resource returns a single JSON object {items, count, ...}.
		var payload struct {
			Count int `json:"count"`
		}
		if err := json.Unmarshal([]byte(res.Contents[0].Text), &payload); err != nil {
			t.Fatalf("ReadResource(%q): unmarshal: %v", uri, err)
		}
		return payload.Count
	}

	all := countItems("feeds://feed/" + feedID + "/items")
	limited := countItems("feeds://feed/" + feedID + "/items?limit=3")

	if limited >= all {
		t.Errorf("expected limit=3 to return fewer entries than unfiltered (all=%d, limited=%d)", all, limited)
	}
	if limited == 0 {
		t.Errorf("expected limit=3 to return some entries, got 0")
	}
}
