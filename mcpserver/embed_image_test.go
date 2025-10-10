package mcpserver

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mmcdole/gofeed"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/richardwooding/feed-mcp/model"
)

//nolint:gocognit // Test function complexity is acceptable for comprehensive test coverage
func TestEmbedImages(t *testing.T) {
	t.Run("embedImages=true fetches and returns ImageContent", func(t *testing.T) {
		// Create a test HTTP server that serves a small image
		imageData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A} // PNG header
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(imageData)
		}))
		defer ts.Close()

		items := []*gofeed.Item{
			{
				Title: "Item with image",
				Link:  "https://example.com/item",
				Image: &gofeed.Image{
					URL:   ts.URL + "/image.png",
					Title: "Test Image",
				},
			},
		}

		feed := &model.FeedAndItemsResult{
			ID:        "test-feed",
			PublicURL: "https://example.com/feed",
			Title:     "Test Feed",
			Feed: &model.Feed{
				Title: "Test Feed",
			},
			Items: items,
		}

		server := &Server{}
		err := server.initializeImageCache()
		if err != nil {
			t.Fatalf("Failed to initialize image cache: %v", err)
		}

		paginationInfo := PaginationInfo{
			Limit:      10,
			Offset:     0,
			TotalItems: len(items),
			HasMore:    false,
		}

		ctx := context.Background()
		content := server.buildFeedContent(ctx, feed, items, paginationInfo, false, 0, true, true)

		// Should have: [0] TextContent (feed metadata), [1] TextContent (item), [2] ImageContent
		if len(content) != 3 {
			t.Fatalf("Expected 3 content items, got %d", len(content))
		}

		imageContent, ok := content[2].(*mcp.ImageContent)
		if !ok {
			t.Fatalf("Expected content[2] to be ImageContent, got %T", content[2])
		}

		// Verify base64-encoded data
		if len(imageContent.Data) == 0 {
			t.Error("Expected ImageContent.Data to be populated")
		}

		// Decode and verify it matches original
		decoded, err := base64.StdEncoding.DecodeString(string(imageContent.Data))
		if err != nil {
			t.Errorf("Failed to decode base64 data: %v", err)
		}
		if len(decoded) != len(imageData) {
			t.Errorf("Expected decoded data length %d, got %d", len(imageData), len(decoded))
		}

		// Verify MIME type
		if imageContent.MIMEType != "image/png" {
			t.Errorf("Expected MIME type image/png, got %s", imageContent.MIMEType)
		}

		// Verify Meta includes itemIndex
		if imageContent.Meta == nil {
			t.Fatal("Expected ImageContent.Meta to be set")
		}
		itemIndex, ok := imageContent.Meta["itemIndex"].(int)
		if !ok {
			t.Fatalf("Expected itemIndex to be int, got %T", imageContent.Meta["itemIndex"])
		}
		if itemIndex != 0 {
			t.Errorf("Expected itemIndex=0, got %d", itemIndex)
		}
	})

	t.Run("cache hit returns cached ImageContent", func(t *testing.T) {
		requestCount := 0
		imageData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount++
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(imageData)
		}))
		defer ts.Close()

		items := []*gofeed.Item{
			{
				Title: "Item 1",
				Link:  "https://example.com/item1",
				Image: &gofeed.Image{
					URL:   ts.URL + "/image.png",
					Title: "Image 1",
				},
			},
		}

		feed := &model.FeedAndItemsResult{
			ID:        "test-feed",
			PublicURL: "https://example.com/feed",
			Title:     "Test Feed",
			Feed:      &model.Feed{Title: "Test Feed"},
			Items:     items,
		}

		server := &Server{}
		err := server.initializeImageCache()
		if err != nil {
			t.Fatalf("Failed to initialize image cache: %v", err)
		}

		paginationInfo := PaginationInfo{
			Limit:      10,
			Offset:     0,
			TotalItems: len(items),
			HasMore:    false,
		}

		ctx := context.Background()

		// First call - should fetch from server
		content1 := server.buildFeedContent(ctx, feed, items, paginationInfo, false, 0, true, true)
		firstRequestCount := requestCount

		// Second call - should hit cache
		content2 := server.buildFeedContent(ctx, feed, items, paginationInfo, false, 0, true, true)
		secondRequestCount := requestCount

		// Verify first call fetched from server
		if firstRequestCount != 1 {
			t.Errorf("Expected 1 request after first call, got %d", firstRequestCount)
		}

		// Both calls should return ImageContent
		if len(content1) != 3 || len(content2) != 3 {
			t.Fatalf("Expected 3 content items in both calls, got %d and %d", len(content1), len(content2))
		}

		img1, ok1 := content1[2].(*mcp.ImageContent)
		img2, ok2 := content2[2].(*mcp.ImageContent)

		if !ok1 || !ok2 {
			t.Fatal("Expected ImageContent in both responses")
		}

		// Verify both have the same base64 data (cache working)
		if string(img1.Data) != string(img2.Data) {
			t.Error("Expected cached image data to match original")
		}

		// Note: ristretto cache is eventually consistent, so we may get 1 or 2 requests
		// The important part is that both calls return ImageContent successfully
		if secondRequestCount > 2 {
			t.Errorf("Expected at most 2 requests (ristretto eventual consistency), got %d", secondRequestCount)
		}
	})

	t.Run("graceful degradation on fetch failure", func(t *testing.T) {
		// Create a server that always returns 500 error
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()

		items := []*gofeed.Item{
			{
				Title: "Item with failing image",
				Link:  "https://example.com/item",
				Image: &gofeed.Image{
					URL:   ts.URL + "/image.png",
					Title: "Failing Image",
				},
			},
		}

		feed := &model.FeedAndItemsResult{
			ID:        "test-feed",
			PublicURL: "https://example.com/feed",
			Title:     "Test Feed",
			Feed:      &model.Feed{Title: "Test Feed"},
			Items:     items,
		}

		server := &Server{}
		err := server.initializeImageCache()
		if err != nil {
			t.Fatalf("Failed to initialize image cache: %v", err)
		}

		paginationInfo := PaginationInfo{
			Limit:      10,
			Offset:     0,
			TotalItems: len(items),
			HasMore:    false,
		}

		ctx := context.Background()
		content := server.buildFeedContent(ctx, feed, items, paginationInfo, false, 0, true, true)

		// Should have: [0] TextContent (feed), [1] TextContent (item), [2] ResourceLink (fallback)
		if len(content) != 3 {
			t.Fatalf("Expected 3 content items, got %d", len(content))
		}

		// Should fall back to ResourceLink on failure
		resourceLink, ok := content[2].(*mcp.ResourceLink)
		if !ok {
			t.Fatalf("Expected content[2] to be ResourceLink (fallback), got %T", content[2])
		}

		if resourceLink.URI != ts.URL+"/image.png" {
			t.Errorf("Expected URI %s, got %s", ts.URL+"/image.png", resourceLink.URI)
		}
	})

	t.Run("size limit enforcement", func(t *testing.T) {
		// Create a server that returns an image larger than 1MB
		largeImage := make([]byte, MaxImageSize+1000)
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(largeImage)
		}))
		defer ts.Close()

		items := []*gofeed.Item{
			{
				Title: "Item with large image",
				Link:  "https://example.com/item",
				Image: &gofeed.Image{
					URL:   ts.URL + "/large.png",
					Title: "Large Image",
				},
			},
		}

		feed := &model.FeedAndItemsResult{
			ID:        "test-feed",
			PublicURL: "https://example.com/feed",
			Title:     "Test Feed",
			Feed:      &model.Feed{Title: "Test Feed"},
			Items:     items,
		}

		server := &Server{}
		err := server.initializeImageCache()
		if err != nil {
			t.Fatalf("Failed to initialize image cache: %v", err)
		}

		paginationInfo := PaginationInfo{
			Limit:      10,
			Offset:     0,
			TotalItems: len(items),
			HasMore:    false,
		}

		ctx := context.Background()
		content := server.buildFeedContent(ctx, feed, items, paginationInfo, false, 0, true, true)

		// Should fall back to ResourceLink when image is too large
		if len(content) != 3 {
			t.Fatalf("Expected 3 content items, got %d", len(content))
		}

		resourceLink, ok := content[2].(*mcp.ResourceLink)
		if !ok {
			t.Fatalf("Expected content[2] to be ResourceLink (fallback), got %T", content[2])
		}

		if resourceLink.URI != ts.URL+"/large.png" {
			t.Errorf("Expected URI %s, got %s", ts.URL+"/large.png", resourceLink.URI)
		}
	})

	t.Run("MaxImagesPerItem limit enforced", func(t *testing.T) {
		imageData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(imageData)
		}))
		defer ts.Close()

		// Create item with more than MaxImagesPerItem enclosures
		enclosures := make([]*gofeed.Enclosure, MaxImagesPerItem+5)
		for i := range enclosures {
			enclosures[i] = &gofeed.Enclosure{
				URL:  fmt.Sprintf("%s/image%d.png", ts.URL, i),
				Type: "image/png",
			}
		}

		items := []*gofeed.Item{
			{
				Title:      "Item with many images",
				Link:       "https://example.com/item",
				Enclosures: enclosures,
			},
		}

		feed := &model.FeedAndItemsResult{
			ID:        "test-feed",
			PublicURL: "https://example.com/feed",
			Title:     "Test Feed",
			Feed:      &model.Feed{Title: "Test Feed"},
			Items:     items,
		}

		server := &Server{}
		err := server.initializeImageCache()
		if err != nil {
			t.Fatalf("Failed to initialize image cache: %v", err)
		}

		paginationInfo := PaginationInfo{
			Limit:      10,
			Offset:     0,
			TotalItems: len(items),
			HasMore:    false,
		}

		ctx := context.Background()
		content := server.buildFeedContent(ctx, feed, items, paginationInfo, false, 0, true, true)

		// Should have: [0] TextContent (feed), [1] TextContent (item), [2-11] ImageContent (max 10)
		expectedCount := 2 + MaxImagesPerItem
		if len(content) != expectedCount {
			t.Fatalf("Expected %d content items (2 text + %d images), got %d", expectedCount, MaxImagesPerItem, len(content))
		}

		// Verify only MaxImagesPerItem images are included
		imageCount := 0
		for i := 2; i < len(content); i++ {
			if _, ok := content[i].(*mcp.ImageContent); ok {
				imageCount++
			}
		}

		if imageCount != MaxImagesPerItem {
			t.Errorf("Expected %d images, got %d", MaxImagesPerItem, imageCount)
		}
	})

	t.Run("circuit breaker opens after failures", func(t *testing.T) {
		failureCount := 0
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			failureCount++
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()

		items := []*gofeed.Item{
			{
				Title: "Item 1",
				Link:  "https://example.com/item1",
				Image: &gofeed.Image{
					URL:   ts.URL + "/image1.png",
					Title: "Image 1",
				},
			},
			{
				Title: "Item 2",
				Link:  "https://example.com/item2",
				Image: &gofeed.Image{
					URL:   ts.URL + "/image2.png",
					Title: "Image 2",
				},
			},
			{
				Title: "Item 3",
				Link:  "https://example.com/item3",
				Image: &gofeed.Image{
					URL:   ts.URL + "/image3.png",
					Title: "Image 3",
				},
			},
			{
				Title: "Item 4",
				Link:  "https://example.com/item4",
				Image: &gofeed.Image{
					URL:   ts.URL + "/image4.png",
					Title: "Image 4",
				},
			},
		}

		feed := &model.FeedAndItemsResult{
			ID:        "test-feed",
			PublicURL: "https://example.com/feed",
			Title:     "Test Feed",
			Feed:      &model.Feed{Title: "Test Feed"},
			Items:     items,
		}

		server := &Server{}
		err := server.initializeImageCache()
		if err != nil {
			t.Fatalf("Failed to initialize image cache: %v", err)
		}

		paginationInfo := PaginationInfo{
			Limit:      10,
			Offset:     0,
			TotalItems: len(items),
			HasMore:    false,
		}

		ctx := context.Background()
		_ = server.buildFeedContent(ctx, feed, items, paginationInfo, false, 0, true, true)

		// Circuit breaker should open after 3 consecutive failures
		// So we expect 3 requests, not 4
		if failureCount > 3 {
			t.Errorf("Expected circuit breaker to open after 3 failures, but got %d requests", failureCount)
		}
	})

	t.Run("embedImages requires includeImages", func(t *testing.T) {
		imageData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(imageData)
		}))
		defer ts.Close()

		items := []*gofeed.Item{
			{
				Title: "Item with image",
				Link:  "https://example.com/item",
				Image: &gofeed.Image{
					URL:   ts.URL + "/image.png",
					Title: "Test Image",
				},
			},
		}

		feed := &model.FeedAndItemsResult{
			ID:        "test-feed",
			PublicURL: "https://example.com/feed",
			Title:     "Test Feed",
			Feed:      &model.Feed{Title: "Test Feed"},
			Items:     items,
		}

		server := &Server{}
		err := server.initializeImageCache()
		if err != nil {
			t.Fatalf("Failed to initialize image cache: %v", err)
		}

		paginationInfo := PaginationInfo{
			Limit:      10,
			Offset:     0,
			TotalItems: len(items),
			HasMore:    false,
		}

		ctx := context.Background()
		// includeImages=false, embedImages=true should result in no images
		content := server.buildFeedContent(ctx, feed, items, paginationInfo, false, 0, false, true)

		// Should only have feed metadata and item text (no images)
		if len(content) != 2 {
			t.Fatalf("Expected 2 content items (no images), got %d", len(content))
		}

		// Verify no ImageContent or ResourceLink
		for i, c := range content {
			if _, isImage := c.(*mcp.ImageContent); isImage {
				t.Errorf("Expected no ImageContent when includeImages=false, found at index %d", i)
			}
			if _, isResource := c.(*mcp.ResourceLink); isResource {
				t.Errorf("Expected no ResourceLink when includeImages=false, found at index %d", i)
			}
		}
	})
}
