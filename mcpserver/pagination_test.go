package mcpserver

import (
	"fmt"
	"testing"

	"github.com/mmcdole/gofeed"
)

// Helper to create test items
func createTestItems(count int) []*gofeed.Item {
	items := make([]*gofeed.Item, count)
	for i := 0; i < count; i++ {
		items[i] = &gofeed.Item{
			Title:       fmt.Sprintf("Item %d", i),
			Description: fmt.Sprintf("Description for item %d", i),
			Content:     fmt.Sprintf("Full content for item %d", i),
			Link:        fmt.Sprintf("https://example.com/item/%d", i),
		}
	}
	return items
}

func TestPaginationLogic(t *testing.T) {
	tests := []struct {
		name             string
		totalItems       int
		limit            *int
		offset           *int
		expectedReturned int
		expectedHasMore  bool
	}{
		{
			name:             "default pagination (50 items from 150)",
			totalItems:       150,
			limit:            nil,
			offset:           nil,
			expectedReturned: 50,
			expectedHasMore:  true,
		},
		{
			name:             "limit 10 items",
			totalItems:       150,
			limit:            ptrInt(10),
			offset:           nil,
			expectedReturned: 10,
			expectedHasMore:  true,
		},
		{
			name:             "offset 50, limit 50",
			totalItems:       150,
			limit:            ptrInt(50),
			offset:           ptrInt(50),
			expectedReturned: 50,
			expectedHasMore:  true,
		},
		{
			name:             "offset 140, limit 50 (partial page)",
			totalItems:       150,
			limit:            ptrInt(50),
			offset:           ptrInt(140),
			expectedReturned: 10,
			expectedHasMore:  false,
		},
		{
			name:             "limit exceeds max (should cap at 100)",
			totalItems:       150,
			limit:            ptrInt(200),
			offset:           nil,
			expectedReturned: 100,
			expectedHasMore:  true,
		},
		{
			name:             "offset beyond total items",
			totalItems:       150,
			limit:            nil,
			offset:           ptrInt(200),
			expectedReturned: 0,
			expectedHasMore:  false,
		},
		{
			name:             "small feed (20 items, default limit)",
			totalItems:       20,
			limit:            nil,
			offset:           nil,
			expectedReturned: 20,
			expectedHasMore:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Apply pagination logic (same as in server.go)
			limit := 50
			if tt.limit != nil {
				limit = *tt.limit
				if limit > 100 {
					limit = 100
				}
				if limit < 0 {
					limit = 0
				}
			}

			offset := 0
			if tt.offset != nil {
				offset = *tt.offset
				if offset < 0 {
					offset = 0
				}
			}

			startIdx := offset
			if startIdx > tt.totalItems {
				startIdx = tt.totalItems
			}

			endIdx := startIdx + limit
			if endIdx > tt.totalItems {
				endIdx = tt.totalItems
			}

			returnedItems := endIdx - startIdx
			hasMore := endIdx < tt.totalItems

			if returnedItems != tt.expectedReturned {
				t.Errorf("Expected %d returned items, got %d", tt.expectedReturned, returnedItems)
			}

			if hasMore != tt.expectedHasMore {
				t.Errorf("Expected has_more=%v, got %v", tt.expectedHasMore, hasMore)
			}
		})
	}
}

func TestProcessItemForOutput(t *testing.T) {
	testItem := &gofeed.Item{
		Title:       "Test Item",
		Description: "This is a long description that should be truncated if max length is set",
		Content:     "This is even longer content that should also be truncated when max length is applied to the item",
		Link:        "https://example.com/item",
	}

	tests := []struct {
		name             string
		includeContent   bool
		maxContentLength int
		expectContent    bool
		expectTruncation bool
	}{
		{
			name:           "include full content",
			includeContent: true,
			expectContent:  true,
		},
		{
			name:           "exclude content",
			includeContent: false,
			expectContent:  false,
		},
		{
			name:             "truncate content at 20 chars",
			includeContent:   true,
			maxContentLength: 20,
			expectContent:    true,
			expectTruncation: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processItemForOutput(testItem, tt.includeContent, tt.maxContentLength)

			if tt.expectContent {
				if result.Content == "" && result.Description == "" {
					t.Error("Expected content fields to be populated")
				}
				if tt.expectTruncation {
					if len(result.Content) > tt.maxContentLength+20 { // +20 for truncation marker
						t.Errorf("Content not truncated: length=%d, max=%d", len(result.Content), tt.maxContentLength)
					}
				}
			} else {
				if result.Content != "" || result.Description != "" {
					t.Error("Expected content fields to be empty")
				}
			}

			// Verify title and link are never stripped
			if result.Title != testItem.Title {
				t.Error("Title should never be modified")
			}
			if result.Link != testItem.Link {
				t.Error("Link should never be modified")
			}
		})
	}
}

// Helper function
func ptrInt(i int) *int {
	return &i
}
