package mcpserver

import (
	"context"
	"testing"

	"github.com/mmcdole/gofeed"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/richardwooding/feed-mcp/model"
)

const (
	testMIMETypeJPEG = "image/jpeg"
	testMIMETypePNG  = "image/png"
	testMIMETypeGIF  = "image/gif"
)

func TestGuessMIMETypeFromURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{"JPEG extension", "https://example.com/image.jpg", testMIMETypeJPEG},
		{"JPG extension", "https://example.com/image.JPG", testMIMETypeJPEG},
		{"PNG extension", "https://example.com/photo.png", testMIMETypePNG},
		{"GIF extension", "https://example.com/animated.gif", testMIMETypeGIF},
		{"WebP extension", "https://example.com/modern.webp", "image/webp"},
		{"SVG extension", "https://example.com/vector.svg", "image/svg+xml"},
		{"BMP extension", "https://example.com/bitmap.bmp", "image/bmp"},
		{"ICO extension", "https://example.com/favicon.ico", "image/x-icon"},
		{"URL with query params", "https://example.com/image.jpg?size=large", testMIMETypeJPEG},
		{"URL with fragment", "https://example.com/image.png#section", testMIMETypePNG},
		{"No extension", "https://example.com/image", ""},         // Empty string for unknown
		{"Unknown extension", "https://example.com/file.xyz", ""}, // Empty string for unknown
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := guessMIMETypeFromURL(tt.url)
			if result != tt.expected {
				t.Errorf("guessMIMETypeFromURL(%q) = %q, want %q", tt.url, result, tt.expected)
			}
		})
	}
}

//nolint:gocognit // Test function complexity is acceptable for comprehensive test coverage
func TestExtractImageLinks(t *testing.T) {
	t.Run("extract from Item.Image only", func(t *testing.T) {
		item := &gofeed.Item{
			Title: "Test Article",
			Image: &gofeed.Image{
				URL:   "https://example.com/featured.jpg",
				Title: "Featured Image",
			},
		}

		links := extractImageLinks(item)

		if len(links) != 1 {
			t.Fatalf("Expected 1 image link, got %d", len(links))
		}

		if links[0].URI != "https://example.com/featured.jpg" {
			t.Errorf("Expected URI %q, got %q", "https://example.com/featured.jpg", links[0].URI)
		}
		if links[0].Title != "Featured Image" {
			t.Errorf("Expected Title %q, got %q", "Featured Image", links[0].Title)
		}
		if links[0].MIMEType != testMIMETypeJPEG {
			t.Errorf("Expected MIMEType %q, got %q", testMIMETypeJPEG, links[0].MIMEType)
		}
	})

	t.Run("extract from Enclosures with explicit MIME type", func(t *testing.T) {
		item := &gofeed.Item{
			Title: "Test Article",
			Enclosures: []*gofeed.Enclosure{
				{
					URL:  "https://example.com/photo1.png",
					Type: testMIMETypePNG,
				},
				{
					URL:  "https://example.com/photo2.jpg",
					Type: testMIMETypeJPEG,
				},
			},
		}

		links := extractImageLinks(item)

		if len(links) != 2 {
			t.Fatalf("Expected 2 image links, got %d", len(links))
		}

		if links[0].URI != "https://example.com/photo1.png" {
			t.Errorf("Expected first URI %q, got %q", "https://example.com/photo1.png", links[0].URI)
		}
		if links[0].MIMEType != "image/png" {
			t.Errorf("Expected first MIMEType %q, got %q", "image/png", links[0].MIMEType)
		}

		if links[1].URI != "https://example.com/photo2.jpg" {
			t.Errorf("Expected second URI %q, got %q", "https://example.com/photo2.jpg", links[1].URI)
		}
		if links[1].MIMEType != testMIMETypeJPEG {
			t.Errorf("Expected second MIMEType %q, got %q", testMIMETypeJPEG, links[1].MIMEType)
		}
	})

	t.Run("filter non-image enclosures", func(t *testing.T) {
		item := &gofeed.Item{
			Title: "Test Article",
			Enclosures: []*gofeed.Enclosure{
				{
					URL:  "https://example.com/photo.png",
					Type: "image/png",
				},
				{
					URL:  "https://example.com/video.mp4",
					Type: "video/mp4", // Should be filtered out
				},
				{
					URL:  "https://example.com/audio.mp3",
					Type: "audio/mp3", // Should be filtered out
				},
			},
		}

		links := extractImageLinks(item)

		if len(links) != 1 {
			t.Fatalf("Expected 1 image link (non-images filtered), got %d", len(links))
		}

		if links[0].URI != "https://example.com/photo.png" {
			t.Errorf("Expected URI %q, got %q", "https://example.com/photo.png", links[0].URI)
		}
	})

	t.Run("guess MIME type for enclosures without Type", func(t *testing.T) {
		item := &gofeed.Item{
			Title: "Test Article",
			Enclosures: []*gofeed.Enclosure{
				{
					URL:  "https://example.com/photo.jpg",
					Type: "", // No type provided
				},
				{
					URL:  "https://example.com/document.pdf",
					Type: "", // Non-image, should be filtered
				},
			},
		}

		links := extractImageLinks(item)

		if len(links) != 1 {
			t.Fatalf("Expected 1 image link (guessed from URL), got %d", len(links))
		}

		if links[0].URI != "https://example.com/photo.jpg" {
			t.Errorf("Expected URI %q, got %q", "https://example.com/photo.jpg", links[0].URI)
		}
		if links[0].MIMEType != testMIMETypeJPEG {
			t.Errorf("Expected guessed MIMEType %q, got %q", testMIMETypeJPEG, links[0].MIMEType)
		}
	})

	t.Run("combine Item.Image and Enclosures", func(t *testing.T) {
		item := &gofeed.Item{
			Title: "Test Article",
			Image: &gofeed.Image{
				URL:   "https://example.com/featured.jpg",
				Title: "Featured",
			},
			Enclosures: []*gofeed.Enclosure{
				{
					URL:  "https://example.com/gallery1.png",
					Type: "image/png",
				},
				{
					URL:  "https://example.com/gallery2.gif",
					Type: "image/gif",
				},
			},
		}

		links := extractImageLinks(item)

		if len(links) != 3 {
			t.Fatalf("Expected 3 image links (1 featured + 2 gallery), got %d", len(links))
		}

		// First should be featured image
		if links[0].URI != "https://example.com/featured.jpg" {
			t.Errorf("Expected first URI %q, got %q", "https://example.com/featured.jpg", links[0].URI)
		}

		// Then enclosures
		if links[1].URI != "https://example.com/gallery1.png" {
			t.Errorf("Expected second URI %q, got %q", "https://example.com/gallery1.png", links[1].URI)
		}
		if links[2].URI != "https://example.com/gallery2.gif" {
			t.Errorf("Expected third URI %q, got %q", "https://example.com/gallery2.gif", links[2].URI)
		}
	})

	t.Run("handle empty/nil item", func(t *testing.T) {
		item := &gofeed.Item{
			Title: "Test Article",
		}

		links := extractImageLinks(item)

		if len(links) != 0 {
			t.Errorf("Expected 0 image links for item with no images, got %d", len(links))
		}
	})

	t.Run("skip enclosures with empty URLs", func(t *testing.T) {
		item := &gofeed.Item{
			Title: "Test Article",
			Enclosures: []*gofeed.Enclosure{
				{
					URL:  "",
					Type: "image/png",
				},
				{
					URL:  "https://example.com/valid.jpg",
					Type: "image/jpeg",
				},
			},
		}

		links := extractImageLinks(item)

		if len(links) != 1 {
			t.Fatalf("Expected 1 image link (empty URL skipped), got %d", len(links))
		}

		if links[0].URI != "https://example.com/valid.jpg" {
			t.Errorf("Expected URI %q, got %q", "https://example.com/valid.jpg", links[0].URI)
		}
	})

	t.Run("skip Item.Image with empty URL", func(t *testing.T) {
		item := &gofeed.Item{
			Title: "Test Article",
			Image: &gofeed.Image{
				URL:   "",
				Title: "Empty URL",
			},
		}

		links := extractImageLinks(item)

		if len(links) != 0 {
			t.Errorf("Expected 0 image links for Item.Image with empty URL, got %d", len(links))
		}
	})
}

//nolint:gocognit // Test function complexity is acceptable for comprehensive test coverage
func TestBuildFeedContentWithImages(t *testing.T) {
	t.Run("images have itemIndex in Meta", func(t *testing.T) {
		items := []*gofeed.Item{
			{
				Title: "Item 0",
				Link:  "https://example.com/item0",
				Image: &gofeed.Image{
					URL:   "https://example.com/image0.jpg",
					Title: "Image 0",
				},
			},
			{
				Title: "Item 1",
				Link:  "https://example.com/item1",
				Enclosures: []*gofeed.Enclosure{
					{
						URL:  "https://example.com/image1a.png",
						Type: "image/png",
					},
					{
						URL:  "https://example.com/image1b.gif",
						Type: "image/gif",
					},
				},
			},
			{
				Title: "Item 2",
				Link:  "https://example.com/item2",
				// No images
			},
		}

		feed := &model.FeedAndItemsResult{
			ID:        "test-feed",
			PublicURL: "https://example.com/feed",
			Title:     "Test Feed",
			Feed: &model.Feed{
				Title:       "Test Feed",
				Description: "Test Description",
			},
			Items: items,
		}

		// Create a minimal Server instance to call buildFeedContent
		server := &Server{}
		paginationInfo := PaginationInfo{
			Limit:      10,
			Offset:     0,
			TotalItems: len(items),
			HasMore:    false,
		}

		// Call buildFeedContent with includeImages=true, embedImages=false
		ctx := context.Background()
		content := server.buildFeedContent(ctx, feed, items, paginationInfo, false, 0, true, false)

		// Verify structure:
		// [0] TextContent (feed metadata)
		// [1] TextContent (item 0)
		// [2] ResourceLink (item 0 image) - itemIndex: 0
		// [3] TextContent (item 1)
		// [4] ResourceLink (item 1 image a) - itemIndex: 1
		// [5] ResourceLink (item 1 image b) - itemIndex: 1
		// [6] TextContent (item 2)

		expectedContentCount := 7
		if len(content) != expectedContentCount {
			t.Fatalf("Expected %d content items, got %d", expectedContentCount, len(content))
		}

		// Check item 0's image has itemIndex: 0
		resourceLink0, ok := content[2].(*mcp.ResourceLink)
		if !ok {
			t.Fatalf("Expected content[2] to be ResourceLink, got %T", content[2])
		}
		if resourceLink0.Meta == nil {
			t.Fatal("Expected ResourceLink Meta to be set")
		}
		itemIndex0, ok := resourceLink0.Meta["itemIndex"].(int)
		if !ok {
			t.Fatalf("Expected itemIndex to be int, got %T", resourceLink0.Meta["itemIndex"])
		}
		if itemIndex0 != 0 {
			t.Errorf("Expected item 0 image to have itemIndex=0, got %d", itemIndex0)
		}

		// Check item 1's first image has itemIndex: 1
		resourceLink1a, ok := content[4].(*mcp.ResourceLink)
		if !ok {
			t.Fatalf("Expected content[4] to be ResourceLink, got %T", content[4])
		}
		if resourceLink1a.Meta == nil {
			t.Fatal("Expected ResourceLink Meta to be set")
		}
		itemIndex1a, ok := resourceLink1a.Meta["itemIndex"].(int)
		if !ok {
			t.Fatalf("Expected itemIndex to be int, got %T", resourceLink1a.Meta["itemIndex"])
		}
		if itemIndex1a != 1 {
			t.Errorf("Expected item 1 first image to have itemIndex=1, got %d", itemIndex1a)
		}

		// Check item 1's second image has itemIndex: 1
		resourceLink1b, ok := content[5].(*mcp.ResourceLink)
		if !ok {
			t.Fatalf("Expected content[5] to be ResourceLink, got %T", content[5])
		}
		if resourceLink1b.Meta == nil {
			t.Fatal("Expected ResourceLink Meta to be set")
		}
		itemIndex1b, ok := resourceLink1b.Meta["itemIndex"].(int)
		if !ok {
			t.Fatalf("Expected itemIndex to be int, got %T", resourceLink1b.Meta["itemIndex"])
		}
		if itemIndex1b != 1 {
			t.Errorf("Expected item 1 second image to have itemIndex=1, got %d", itemIndex1b)
		}
	})

	t.Run("no images when includeImages=false", func(t *testing.T) {
		items := []*gofeed.Item{
			{
				Title: "Item with image",
				Link:  "https://example.com/item",
				Image: &gofeed.Image{
					URL:   "https://example.com/image.jpg",
					Title: "Image",
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

		// Create a minimal Server instance to call buildFeedContent
		server := &Server{}
		paginationInfo := PaginationInfo{
			Limit:      10,
			Offset:     0,
			TotalItems: len(items),
			HasMore:    false,
		}

		// Call buildFeedContent with includeImages=false, embedImages=false
		ctx := context.Background()
		content := server.buildFeedContent(ctx, feed, items, paginationInfo, false, 0, false, false)

		// Should only have feed metadata + item content (no images)
		expectedContentCount := 2
		if len(content) != expectedContentCount {
			t.Fatalf("Expected %d content items, got %d", expectedContentCount, len(content))
		}

		// Verify no ResourceLinks
		for i, c := range content {
			if _, isResourceLink := c.(*mcp.ResourceLink); isResourceLink {
				t.Errorf("Expected no ResourceLinks when includeImages=false, found one at index %d", i)
			}
		}
	})
}
