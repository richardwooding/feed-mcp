package mcpserver

import (
	"testing"

	"github.com/mmcdole/gofeed"
)

func TestGuessMIMETypeFromURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{"JPEG extension", "https://example.com/image.jpg", "image/jpeg"},
		{"JPG extension", "https://example.com/image.JPG", "image/jpeg"},
		{"PNG extension", "https://example.com/photo.png", "image/png"},
		{"GIF extension", "https://example.com/animated.gif", "image/gif"},
		{"WebP extension", "https://example.com/modern.webp", "image/webp"},
		{"SVG extension", "https://example.com/vector.svg", "image/svg+xml"},
		{"BMP extension", "https://example.com/bitmap.bmp", "image/bmp"},
		{"ICO extension", "https://example.com/favicon.ico", "image/x-icon"},
		{"URL with query params", "https://example.com/image.jpg?size=large", "image/jpeg"},
		{"URL with fragment", "https://example.com/image.png#section", "image/png"},
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
		if links[0].MIMEType != "image/jpeg" {
			t.Errorf("Expected MIMEType %q, got %q", "image/jpeg", links[0].MIMEType)
		}
	})

	t.Run("extract from Enclosures with explicit MIME type", func(t *testing.T) {
		item := &gofeed.Item{
			Title: "Test Article",
			Enclosures: []*gofeed.Enclosure{
				{
					URL:  "https://example.com/photo1.png",
					Type: "image/png",
				},
				{
					URL:  "https://example.com/photo2.jpg",
					Type: "image/jpeg",
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
		if links[1].MIMEType != "image/jpeg" {
			t.Errorf("Expected second MIMEType %q, got %q", "image/jpeg", links[1].MIMEType)
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
		if links[0].MIMEType != "image/jpeg" {
			t.Errorf("Expected guessed MIMEType %q, got %q", "image/jpeg", links[0].MIMEType)
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
