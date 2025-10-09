package mcpserver

import (
	"testing"

	"github.com/mmcdole/gofeed"
)

// applyPaginationParams applies the same pagination logic as server.go
func applyPaginationParams(totalItems int, limit *int, offset *int) (returnedItems int, hasMore bool) {
	// Apply limit
	effectiveLimit := DefaultItemLimit
	if limit != nil {
		effectiveLimit = *limit
		if effectiveLimit > MaxItemLimit {
			effectiveLimit = MaxItemLimit
		}
		if effectiveLimit < 0 {
			effectiveLimit = 0
		}
	}

	// Apply offset
	effectiveOffset := 0
	if offset != nil {
		effectiveOffset = *offset
		if effectiveOffset < 0 {
			effectiveOffset = 0
		}
	}

	// Calculate pagination
	startIdx := effectiveOffset
	if startIdx > totalItems {
		startIdx = totalItems
	}

	endIdx := startIdx + effectiveLimit
	if endIdx > totalItems {
		endIdx = totalItems
	}

	returnedItems = endIdx - startIdx
	hasMore = endIdx < totalItems
	return returnedItems, hasMore
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
		{"default pagination (10 items from 150)", 150, nil, nil, 10, true},
		{"limit 5 items", 150, ptrInt(5), nil, 5, true},
		{"offset 10, limit 10", 150, ptrInt(10), ptrInt(10), 10, true},
		{"offset 15, limit 10 (partial page)", 25, ptrInt(10), ptrInt(15), 10, false},
		{"limit exceeds max (should cap at 20)", 150, ptrInt(200), nil, 20, true},
		{"offset beyond total items", 150, nil, ptrInt(200), 0, false},
		{"small feed (5 items, default limit)", 5, nil, nil, 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			returnedItems, hasMore := applyPaginationParams(tt.totalItems, tt.limit, tt.offset)

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

	t.Run("include full content (unlimited)", func(t *testing.T) {
		result := processItemForOutput(testItem, true, 0)
		verifyContentIncluded(t, result, testItem)
		verifyMetadataPreserved(t, result, testItem)
	})

	t.Run("exclude content (default behavior)", func(t *testing.T) {
		result := processItemForOutput(testItem, false, 0)
		verifyContentExcluded(t, result)
		verifyMetadataPreserved(t, result, testItem)
	})

	t.Run("truncate content at 500 chars (default when included)", func(t *testing.T) {
		result := processItemForOutput(testItem, true, DefaultContentLength)
		verifyContentTruncated(t, result, DefaultContentLength)
		verifyMetadataPreserved(t, result, testItem)
	})

	t.Run("truncate content at 20 chars (custom)", func(t *testing.T) {
		result := processItemForOutput(testItem, true, 20)
		verifyContentTruncated(t, result, 20)
		verifyMetadataPreserved(t, result, testItem)
	})
}

func verifyContentIncluded(t *testing.T, result, original *gofeed.Item) {
	t.Helper()
	if result.Content == "" && result.Description == "" {
		t.Error("Expected content fields to be populated")
	}
}

func verifyContentExcluded(t *testing.T, result *gofeed.Item) {
	t.Helper()
	if result.Content != "" || result.Description != "" {
		t.Error("Expected content fields to be empty")
	}
}

func verifyContentTruncated(t *testing.T, result *gofeed.Item, maxLen int) {
	t.Helper()
	maxExpectedLen := maxLen + len(TruncationMarker)
	if len(result.Content) > maxExpectedLen {
		t.Errorf("Content not truncated: length=%d, max=%d", len(result.Content), maxExpectedLen)
	}
}

func verifyMetadataPreserved(t *testing.T, result, original *gofeed.Item) {
	t.Helper()
	if result.Title != original.Title {
		t.Error("Title should never be modified")
	}
	if result.Link != original.Link {
		t.Error("Link should never be modified")
	}
}

// Helper function
func ptrInt(i int) *int {
	return &i
}
