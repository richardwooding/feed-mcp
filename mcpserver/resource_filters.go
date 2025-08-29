// Package mcpserver implements URI parameter filtering for MCP Resources
package mcpserver

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/richardwooding/feed-mcp/model"
)

// FilterParams represents parsed URI parameters for filtering
type FilterParams struct {
	Since    *time.Time // Filter items since this date
	Until    *time.Time // Filter items until this date
	Limit    *int       // Maximum number of items to return
	Offset   *int       // Number of items to skip (for pagination)
	Category string     // Filter by category/tag
	Author   string     // Filter by author
	Search   string     // Search in title/description
}

// ParseURIParameters extracts and validates filter parameters from a resource URI
func ParseURIParameters(resourceURI string) (*FilterParams, error) {
	parsedURL, err := url.Parse(resourceURI)
	if err != nil {
		return nil, model.NewFeedError(model.ErrorTypeValidation, "Invalid URI format").
			WithURL(resourceURI).
			WithOperation("parse_uri_parameters").
			WithComponent("resource_filters")
	}

	params := &FilterParams{}
	query := parsedURL.Query()

	// Parse time parameters
	if err := parseTimeParameters(query, params, resourceURI); err != nil {
		return nil, err
	}

	// Parse numeric parameters
	if err := parseNumericParameters(query, params, resourceURI); err != nil {
		return nil, err
	}

	// Parse string parameters
	parseStringParameters(query, params)

	// Validate parameter combinations
	if err := validateParameterCombinations(params, resourceURI); err != nil {
		return nil, err
	}

	return params, nil
}

// parseTimeParameters handles since and until date parameter parsing
func parseTimeParameters(query url.Values, params *FilterParams, resourceURI string) error {
	// Parse 'since' parameter (ISO 8601 date)
	if sinceStr := query.Get("since"); sinceStr != "" {
		since, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			return model.NewFeedError(model.ErrorTypeValidation, fmt.Sprintf("Invalid 'since' date format: %s", err.Error())).
				WithURL(resourceURI).
				WithOperation("parse_since_parameter").
				WithComponent("resource_filters")
		}
		params.Since = &since
	}

	// Parse 'until' parameter (ISO 8601 date)
	if untilStr := query.Get("until"); untilStr != "" {
		until, err := time.Parse(time.RFC3339, untilStr)
		if err != nil {
			return model.NewFeedError(model.ErrorTypeValidation, fmt.Sprintf("Invalid 'until' date format: %s", err.Error())).
				WithURL(resourceURI).
				WithOperation("parse_until_parameter").
				WithComponent("resource_filters")
		}
		params.Until = &until
	}

	return nil
}

// parseNumericParameters handles limit and offset parameter parsing
func parseNumericParameters(query url.Values, params *FilterParams, resourceURI string) error {
	// Parse 'limit' parameter
	if limitStr := query.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit < 0 {
			return model.NewFeedError(model.ErrorTypeValidation, "Invalid 'limit' value: must be non-negative integer").
				WithURL(resourceURI).
				WithOperation("parse_limit_parameter").
				WithComponent("resource_filters")
		}
		if limit > 1000 { // Reasonable upper limit
			limit = 1000
		}
		params.Limit = &limit
	}

	// Parse 'offset' parameter (for pagination)
	if offsetStr := query.Get("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil || offset < 0 {
			return model.NewFeedError(model.ErrorTypeValidation, "Invalid 'offset' value: must be non-negative integer").
				WithURL(resourceURI).
				WithOperation("parse_offset_parameter").
				WithComponent("resource_filters")
		}
		params.Offset = &offset
	}

	return nil
}

// parseStringParameters handles category, author, and search parameter parsing
func parseStringParameters(query url.Values, params *FilterParams) {
	// Parse 'category' parameter
	if category := query.Get("category"); category != "" {
		params.Category = category
	}

	// Parse 'author' parameter
	if author := query.Get("author"); author != "" {
		params.Author = author
	}

	// Parse 'search' parameter (for title/description search)
	if search := query.Get("search"); search != "" {
		params.Search = search
	}
}

// validateParameterCombinations validates that parameter combinations are valid
func validateParameterCombinations(params *FilterParams, resourceURI string) error {
	// Validate parameter combinations
	if params.Since != nil && params.Until != nil && params.Since.After(*params.Until) {
		return model.NewFeedError(model.ErrorTypeValidation, "'since' date must be before 'until' date").
			WithURL(resourceURI).
			WithOperation("validate_date_range").
			WithComponent("resource_filters")
	}

	return nil
}

// ApplyFilters applies the filter parameters to a slice of feed items
func ApplyFilters(items []*gofeed.Item, filters *FilterParams) []*gofeed.Item {
	if filters == nil {
		return items
	}

	var filteredItems []*gofeed.Item

	for _, item := range items {
		if shouldIncludeItem(item, filters) {
			filteredItems = append(filteredItems, item)
		}
	}

	// Apply pagination (offset and limit)
	if filters.Offset != nil {
		offset := *filters.Offset
		if offset >= len(filteredItems) {
			return []*gofeed.Item{} // Return empty slice if offset is too large
		}
		filteredItems = filteredItems[offset:]
	}

	if filters.Limit != nil {
		limit := *filters.Limit
		if limit < len(filteredItems) {
			filteredItems = filteredItems[:limit]
		}
	}

	return filteredItems
}

// shouldIncludeItem determines if an item should be included based on filter criteria
func shouldIncludeItem(item *gofeed.Item, filters *FilterParams) bool {
	// Date filtering (since)
	if filters.Since != nil && item.PublishedParsed != nil {
		if item.PublishedParsed.Before(*filters.Since) {
			return false
		}
	}

	// Date filtering (until)
	if filters.Until != nil && item.PublishedParsed != nil {
		if item.PublishedParsed.After(*filters.Until) {
			return false
		}
	}

	// Category filtering
	if filters.Category != "" {
		if !hasCategory(item, filters.Category) {
			return false
		}
	}

	// Author filtering
	if filters.Author != "" {
		if !hasAuthor(item, filters.Author) {
			return false
		}
	}

	// Search filtering (in title and description)
	if filters.Search != "" {
		if !matchesSearch(item, filters.Search) {
			return false
		}
	}

	return true
}

// hasCategory checks if an item has the specified category/tag
func hasCategory(item *gofeed.Item, category string) bool {
	// Check categories
	for _, cat := range item.Categories {
		if strings.EqualFold(cat, category) {
			return true
		}
	}

	// Check custom fields that might contain categories/tags
	if item.Custom != nil {
		// Check common tag fields
		if tags, ok := item.Custom["tags"]; ok && tags != "" {
			tagList := strings.Split(tags, ",")
			for _, tag := range tagList {
				if strings.EqualFold(strings.TrimSpace(tag), category) {
					return true
				}
			}
		}
	}

	return false
}

// hasAuthor checks if an item has the specified author
func hasAuthor(item *gofeed.Item, author string) bool {
	// Check main author
	if item.Author != nil && strings.EqualFold(item.Author.Name, author) {
		return true
	}

	// Check authors list
	for _, a := range item.Authors {
		if strings.EqualFold(a.Name, author) {
			return true
		}
	}

	return false
}

// matchesSearch checks if an item matches the search term in title or description
func matchesSearch(item *gofeed.Item, search string) bool {
	searchLower := strings.ToLower(search)

	// Check title
	if strings.Contains(strings.ToLower(item.Title), searchLower) {
		return true
	}

	// Check description
	if strings.Contains(strings.ToLower(item.Description), searchLower) {
		return true
	}

	// Check content
	if strings.Contains(strings.ToLower(item.Content), searchLower) {
		return true
	}

	return false
}

// FilterSummary provides information about applied filters and results
type FilterSummary struct {
	TotalItems     int            `json:"total_items"`
	FilteredItems  int            `json:"filtered_items"`
	AppliedFilters map[string]any `json:"applied_filters,omitempty"`
}

// CreateFilterSummary creates a summary of the filtering operation
func CreateFilterSummary(originalCount, filteredCount int, filters *FilterParams) *FilterSummary {
	summary := &FilterSummary{
		TotalItems:    originalCount,
		FilteredItems: filteredCount,
	}

	if filters != nil {
		appliedFilters := make(map[string]any)

		if filters.Since != nil {
			appliedFilters["since"] = filters.Since.Format(time.RFC3339)
		}
		if filters.Until != nil {
			appliedFilters["until"] = filters.Until.Format(time.RFC3339)
		}
		if filters.Limit != nil {
			appliedFilters["limit"] = *filters.Limit
		}
		if filters.Offset != nil {
			appliedFilters["offset"] = *filters.Offset
		}
		if filters.Category != "" {
			appliedFilters["category"] = filters.Category
		}
		if filters.Author != "" {
			appliedFilters["author"] = filters.Author
		}
		if filters.Search != "" {
			appliedFilters["search"] = filters.Search
		}

		if len(appliedFilters) > 0 {
			summary.AppliedFilters = appliedFilters
		}
	}

	return summary
}
