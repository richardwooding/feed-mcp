package mcpserver

import (
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
)

func TestParseURIParameters(t *testing.T) {
	tests := []struct {
		name        string
		uri         string
		expectError bool
		expected    *FilterParams
	}{
		{
			name:        "No parameters",
			uri:         "feeds://feed/test-feed/items",
			expectError: false,
			expected:    &FilterParams{},
		},
		{
			name:        "Valid since parameter",
			uri:         "feeds://feed/test-feed/items?since=2023-01-01T00:00:00Z",
			expectError: false,
			expected: &FilterParams{
				Since: parseTimePtr("2023-01-01T00:00:00Z"),
			},
		},
		{
			name:        "Valid until parameter",
			uri:         "feeds://feed/test-feed/items?until=2023-12-31T23:59:59Z",
			expectError: false,
			expected: &FilterParams{
				Until: parseTimePtr("2023-12-31T23:59:59Z"),
			},
		},
		{
			name:        "Valid limit parameter",
			uri:         "feeds://feed/test-feed/items?limit=10",
			expectError: false,
			expected: &FilterParams{
				Limit: intPtr(10),
			},
		},
		{
			name:        "Valid offset parameter",
			uri:         "feeds://feed/test-feed/items?offset=5",
			expectError: false,
			expected: &FilterParams{
				Offset: intPtr(5),
			},
		},
		{
			name:        "Valid category parameter",
			uri:         "feeds://feed/test-feed/items?category=tech",
			expectError: false,
			expected: &FilterParams{
				Category: "tech",
			},
		},
		{
			name:        "Valid author parameter",
			uri:         "feeds://feed/test-feed/items?author=john",
			expectError: false,
			expected: &FilterParams{
				Author: "john",
			},
		},
		{
			name:        "Valid search parameter",
			uri:         "feeds://feed/test-feed/items?search=golang",
			expectError: false,
			expected: &FilterParams{
				Search: "golang",
			},
		},
		{
			name:        "Multiple parameters",
			uri:         "feeds://feed/test-feed/items?since=2023-01-01T00:00:00Z&limit=5&category=tech",
			expectError: false,
			expected: &FilterParams{
				Since:    parseTimePtr("2023-01-01T00:00:00Z"),
				Limit:    intPtr(5),
				Category: "tech",
			},
		},
		{
			name:        "Invalid since format",
			uri:         "feeds://feed/test-feed/items?since=2023-01-01",
			expectError: true,
			expected:    nil,
		},
		{
			name:        "Invalid limit value",
			uri:         "feeds://feed/test-feed/items?limit=-5",
			expectError: true,
			expected:    nil,
		},
		{
			name:        "Limit too high (should cap at 1000)",
			uri:         "feeds://feed/test-feed/items?limit=2000",
			expectError: false,
			expected: &FilterParams{
				Limit: intPtr(1000),
			},
		},
		{
			name:        "Invalid offset value",
			uri:         "feeds://feed/test-feed/items?offset=-1",
			expectError: true,
			expected:    nil,
		},
		{
			name:        "Since after until (invalid)",
			uri:         "feeds://feed/test-feed/items?since=2023-12-01T00:00:00Z&until=2023-01-01T00:00:00Z",
			expectError: true,
			expected:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseURIParameters(tt.uri)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
				return
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if tt.expectError {
				return // Skip further checks if we expected an error
			}

			// Compare results
			if !compareFilterParams(result, tt.expected) {
				t.Errorf("Expected %+v, got %+v", tt.expected, result)
			}
		})
	}
}

func TestApplyFilters(t *testing.T) {
	// Create test items
	baseTime := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)

	items := []*gofeed.Item{
		{
			Title:           "Go Programming Basics",
			Description:     "Learn the fundamentals of Go programming",
			PublishedParsed: &baseTime,
			Categories:      []string{"programming", "go"},
			Author:          &gofeed.Person{Name: "John Doe"},
		},
		{
			Title:           "Advanced JavaScript Techniques",
			Description:     "Master advanced JavaScript concepts",
			PublishedParsed: timePtr(baseTime.Add(24 * time.Hour)),
			Categories:      []string{"programming", "javascript"},
			Author:          &gofeed.Person{Name: "Jane Smith"},
		},
		{
			Title:           "Web Design Trends 2023",
			Description:     "Latest trends in web design",
			PublishedParsed: timePtr(baseTime.Add(48 * time.Hour)),
			Categories:      []string{"design", "web"},
			Author:          &gofeed.Person{Name: "Bob Wilson"},
		},
		{
			Title:           "Database Optimization",
			Description:     "How to optimize your database queries",
			PublishedParsed: timePtr(baseTime.Add(72 * time.Hour)),
			Categories:      []string{"database", "performance"},
			Author:          &gofeed.Person{Name: "Alice Johnson"},
		},
	}

	tests := []struct {
		name           string
		filters        *FilterParams
		expectedCount  int
		expectedTitles []string
	}{
		{
			name:          "No filters",
			filters:       nil,
			expectedCount: 4,
			expectedTitles: []string{
				"Go Programming Basics",
				"Advanced JavaScript Techniques",
				"Web Design Trends 2023",
				"Database Optimization",
			},
		},
		{
			name: "Limit filter",
			filters: &FilterParams{
				Limit: intPtr(2),
			},
			expectedCount: 2,
			expectedTitles: []string{
				"Go Programming Basics",
				"Advanced JavaScript Techniques",
			},
		},
		{
			name: "Offset filter",
			filters: &FilterParams{
				Offset: intPtr(2),
			},
			expectedCount: 2,
			expectedTitles: []string{
				"Web Design Trends 2023",
				"Database Optimization",
			},
		},
		{
			name: "Limit and offset",
			filters: &FilterParams{
				Limit:  intPtr(1),
				Offset: intPtr(1),
			},
			expectedCount: 1,
			expectedTitles: []string{
				"Advanced JavaScript Techniques",
			},
		},
		{
			name: "Since filter",
			filters: &FilterParams{
				Since: timePtr(baseTime.Add(25 * time.Hour)),
			},
			expectedCount: 2,
			expectedTitles: []string{
				"Web Design Trends 2023",
				"Database Optimization",
			},
		},
		{
			name: "Until filter",
			filters: &FilterParams{
				Until: timePtr(baseTime.Add(25 * time.Hour)),
			},
			expectedCount: 2,
			expectedTitles: []string{
				"Go Programming Basics",
				"Advanced JavaScript Techniques",
			},
		},
		{
			name: "Category filter",
			filters: &FilterParams{
				Category: "programming",
			},
			expectedCount: 2,
			expectedTitles: []string{
				"Go Programming Basics",
				"Advanced JavaScript Techniques",
			},
		},
		{
			name: "Author filter",
			filters: &FilterParams{
				Author: "Jane Smith",
			},
			expectedCount: 1,
			expectedTitles: []string{
				"Advanced JavaScript Techniques",
			},
		},
		{
			name: "Search filter (title)",
			filters: &FilterParams{
				Search: "Go",
			},
			expectedCount: 1,
			expectedTitles: []string{
				"Go Programming Basics",
			},
		},
		{
			name: "Search filter (description)",
			filters: &FilterParams{
				Search: "database",
			},
			expectedCount: 1,
			expectedTitles: []string{
				"Database Optimization",
			},
		},
		{
			name: "Combined filters",
			filters: &FilterParams{
				Category: "programming",
				Limit:    intPtr(1),
			},
			expectedCount: 1,
			expectedTitles: []string{
				"Go Programming Basics",
			},
		},
		{
			name: "No matches",
			filters: &FilterParams{
				Category: "nonexistent",
			},
			expectedCount:  0,
			expectedTitles: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ApplyFilters(items, tt.filters)

			if len(result) != tt.expectedCount {
				t.Errorf("Expected %d items, got %d", tt.expectedCount, len(result))
			}

			for i, expectedTitle := range tt.expectedTitles {
				if i >= len(result) {
					t.Errorf("Expected item %d with title '%s', but result has only %d items",
						i, expectedTitle, len(result))
					continue
				}
				if result[i].Title != expectedTitle {
					t.Errorf("Expected item %d to have title '%s', got '%s'",
						i, expectedTitle, result[i].Title)
				}
			}
		})
	}
}

func TestHasCategory(t *testing.T) {
	item := &gofeed.Item{
		Categories: []string{"tech", "programming", "golang"},
	}

	if !hasCategory(item, "tech") {
		t.Error("Should find category 'tech'")
	}
	if !hasCategory(item, "TECH") { // Case insensitive
		t.Error("Should find category 'TECH' (case insensitive)")
	}
	if hasCategory(item, "design") {
		t.Error("Should not find category 'design'")
	}
}

func TestHasAuthor(t *testing.T) {
	item := &gofeed.Item{
		Author: &gofeed.Person{Name: "John Doe"},
		Authors: []*gofeed.Person{
			{Name: "Jane Smith"},
			{Name: "Bob Wilson"},
		},
	}

	if !hasAuthor(item, "John Doe") {
		t.Error("Should find main author 'John Doe'")
	}
	if !hasAuthor(item, "jane smith") { // Case insensitive
		t.Error("Should find author 'jane smith' (case insensitive)")
	}
	if hasAuthor(item, "Alice Johnson") {
		t.Error("Should not find author 'Alice Johnson'")
	}
}

func TestMatchesSearch(t *testing.T) {
	item := &gofeed.Item{
		Title:       "Go Programming Tutorial",
		Description: "Learn advanced Go programming techniques",
		Content:     "This tutorial covers goroutines and channels",
	}

	if !matchesSearch(item, "Go") {
		t.Error("Should match 'Go' in title")
	}
	if !matchesSearch(item, "advanced") {
		t.Error("Should match 'advanced' in description")
	}
	if !matchesSearch(item, "goroutines") {
		t.Error("Should match 'goroutines' in content")
	}
	if !matchesSearch(item, "PROGRAMMING") { // Case insensitive
		t.Error("Should match 'PROGRAMMING' (case insensitive)")
	}
	if matchesSearch(item, "nonexistent") {
		t.Error("Should not match 'nonexistent'")
	}
}

func TestCreateFilterSummary(t *testing.T) {
	filters := &FilterParams{
		Since:    parseTimePtr("2023-01-01T00:00:00Z"),
		Limit:    intPtr(10),
		Category: "tech",
	}

	summary := CreateFilterSummary(100, 25, filters)

	if summary.TotalItems != 100 {
		t.Errorf("Expected TotalItems to be 100, got %d", summary.TotalItems)
	}
	if summary.FilteredItems != 25 {
		t.Errorf("Expected FilteredItems to be 25, got %d", summary.FilteredItems)
	}

	if summary.AppliedFilters == nil {
		t.Error("Expected AppliedFilters to be non-nil")
		return
	}

	if summary.AppliedFilters["since"] != "2023-01-01T00:00:00Z" {
		t.Errorf("Expected since filter to be '2023-01-01T00:00:00Z', got %v",
			summary.AppliedFilters["since"])
	}
	if summary.AppliedFilters["limit"] != 10 {
		t.Errorf("Expected limit filter to be 10, got %v", summary.AppliedFilters["limit"])
	}
	if summary.AppliedFilters["category"] != "tech" {
		t.Errorf("Expected category filter to be 'tech', got %v", summary.AppliedFilters["category"])
	}
}

func TestFilterParamsValidation(t *testing.T) {
	t.Run("Valid date range", func(t *testing.T) {
		_, err := ParseURIParameters("feeds://feed/test?since=2023-01-01T00:00:00Z&until=2023-12-31T23:59:59Z")
		if err != nil {
			t.Errorf("Expected no error for valid date range, got: %v", err)
		}
	})

	t.Run("Invalid date range (since after until)", func(t *testing.T) {
		_, err := ParseURIParameters("feeds://feed/test?since=2023-12-31T23:59:59Z&until=2023-01-01T00:00:00Z")
		if err == nil {
			t.Error("Expected error for invalid date range")
		}
	})

	t.Run("Boundary values", func(t *testing.T) {
		// Test limit = 0 (should be valid)
		params, err := ParseURIParameters("feeds://feed/test?limit=0")
		if err != nil {
			t.Errorf("Expected no error for limit=0, got: %v", err)
		}
		if params.Limit == nil || *params.Limit != 0 {
			t.Errorf("Expected limit to be 0, got %v", params.Limit)
		}

		// Test offset = 0 (should be valid)
		params, err = ParseURIParameters("feeds://feed/test?offset=0")
		if err != nil {
			t.Errorf("Expected no error for offset=0, got: %v", err)
		}
		if params.Offset == nil || *params.Offset != 0 {
			t.Errorf("Expected offset to be 0, got %v", params.Offset)
		}
	})
}

// Helper functions for tests
func parseTimePtr(timeStr string) *time.Time {
	t, _ := time.Parse(time.RFC3339, timeStr)
	return &t
}

func intPtr(i int) *int {
	return &i
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func compareFilterParams(a, b *FilterParams) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Compare Since
	if (a.Since == nil) != (b.Since == nil) {
		return false
	}
	if a.Since != nil && !a.Since.Equal(*b.Since) {
		return false
	}

	// Compare Until
	if (a.Until == nil) != (b.Until == nil) {
		return false
	}
	if a.Until != nil && !a.Until.Equal(*b.Until) {
		return false
	}

	// Compare Limit
	if (a.Limit == nil) != (b.Limit == nil) {
		return false
	}
	if a.Limit != nil && *a.Limit != *b.Limit {
		return false
	}

	// Compare Offset
	if (a.Offset == nil) != (b.Offset == nil) {
		return false
	}
	if a.Offset != nil && *a.Offset != *b.Offset {
		return false
	}

	// Compare strings
	return a.Category == b.Category && a.Author == b.Author && a.Search == b.Search
}
