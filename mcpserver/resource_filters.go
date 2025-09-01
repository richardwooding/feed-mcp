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
	// Existing filters
	Since    *time.Time // Filter items since this date
	Until    *time.Time // Filter items until this date
	Limit    *int       // Maximum number of items to return
	Offset   *int       // Number of items to skip (for pagination)
	Category string     // Filter by category/tag
	Author   string     // Filter by author
	Search   string     // Search in title/description

	// Enhanced filters (Phase 2)
	Language   string // Filter by language (en, es, fr, etc.)
	MinLength  *int   // Minimum content length
	MaxLength  *int   // Maximum content length
	HasMedia   *bool  // Only items with images/video
	Sentiment  string // positive, negative, neutral
	Duplicates *bool  // Include/exclude duplicate content
	SortBy     string // date, relevance, popularity
	Format     string // json, xml, html, markdown
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

	// Parse boolean parameters
	if err := parseBooleanParameters(query, params, resourceURI); err != nil {
		return nil, err
	}

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

	// Parse 'min_length' parameter
	if minLengthStr := query.Get("min_length"); minLengthStr != "" {
		minLength, err := strconv.Atoi(minLengthStr)
		if err != nil || minLength < 0 {
			return model.NewFeedError(model.ErrorTypeValidation, "Invalid 'min_length' value: must be non-negative integer").
				WithURL(resourceURI).
				WithOperation("parse_min_length_parameter").
				WithComponent("resource_filters")
		}
		params.MinLength = &minLength
	}

	// Parse 'max_length' parameter
	if maxLengthStr := query.Get("max_length"); maxLengthStr != "" {
		maxLength, err := strconv.Atoi(maxLengthStr)
		if err != nil || maxLength < 0 {
			return model.NewFeedError(model.ErrorTypeValidation, "Invalid 'max_length' value: must be non-negative integer").
				WithURL(resourceURI).
				WithOperation("parse_max_length_parameter").
				WithComponent("resource_filters")
		}
		params.MaxLength = &maxLength
	}

	return nil
}

// parseStringParameters handles category, author, search, and enhanced parameter parsing
func parseStringParameters(query url.Values, params *FilterParams) {
	parseBasicStringParams(query, params)
	parseEnhancedStringParams(query, params)
}

// parseBasicStringParams parses basic string parameters
func parseBasicStringParams(query url.Values, params *FilterParams) {
	if category := query.Get("category"); category != "" {
		params.Category = category
	}
	if author := query.Get("author"); author != "" {
		params.Author = author
	}
	if search := query.Get("search"); search != "" {
		params.Search = search
	}
}

// parseEnhancedStringParams parses Phase 2 enhanced string parameters
func parseEnhancedStringParams(query url.Values, params *FilterParams) {
	if language := query.Get("language"); language != "" {
		params.Language = language
	}

	if sentiment := query.Get("sentiment"); isValidSentiment(sentiment) {
		params.Sentiment = sentiment
	}

	if sortBy := query.Get("sort_by"); isValidSortBy(sortBy) {
		params.SortBy = sortBy
	}

	if format := query.Get("format"); isValidFormat(format) {
		params.Format = format
	}
}

// isValidSentiment checks if sentiment value is valid
func isValidSentiment(sentiment string) bool {
	return sentiment == "positive" || sentiment == "negative" || sentiment == "neutral"
}

// isValidSortBy checks if sort_by value is valid
func isValidSortBy(sortBy string) bool {
	return sortBy == "date" || sortBy == "relevance" || sortBy == "popularity"
}

// isValidFormat checks if format value is valid
func isValidFormat(format string) bool {
	return format == "json" || format == "xml" || format == "html" || format == "markdown"
}

// parseBooleanParameters handles has_media and duplicates parameter parsing
func parseBooleanParameters(query url.Values, params *FilterParams, resourceURI string) error {
	// Parse 'has_media' parameter
	if hasMediaStr := query.Get("has_media"); hasMediaStr != "" {
		hasMedia, err := strconv.ParseBool(hasMediaStr)
		if err != nil {
			return model.NewFeedError(model.ErrorTypeValidation, "Invalid 'has_media' value: must be true or false").
				WithURL(resourceURI).
				WithOperation("parse_has_media_parameter").
				WithComponent("resource_filters")
		}
		params.HasMedia = &hasMedia
	}

	// Parse 'duplicates' parameter
	if duplicatesStr := query.Get("duplicates"); duplicatesStr != "" {
		duplicates, err := strconv.ParseBool(duplicatesStr)
		if err != nil {
			return model.NewFeedError(model.ErrorTypeValidation, "Invalid 'duplicates' value: must be true or false").
				WithURL(resourceURI).
				WithOperation("parse_duplicates_parameter").
				WithComponent("resource_filters")
		}
		params.Duplicates = &duplicates
	}

	return nil
}

// validateParameterCombinations validates that parameter combinations are valid
func validateParameterCombinations(params *FilterParams, resourceURI string) error {
	// Validate date range
	if params.Since != nil && params.Until != nil && params.Since.After(*params.Until) {
		return model.NewFeedError(model.ErrorTypeValidation, "'since' date must be before 'until' date").
			WithURL(resourceURI).
			WithOperation("validate_date_range").
			WithComponent("resource_filters")
	}

	// Validate length parameters
	if params.MinLength != nil && params.MaxLength != nil && *params.MinLength > *params.MaxLength {
		return model.NewFeedError(model.ErrorTypeValidation, "'min_length' must be less than or equal to 'max_length'").
			WithURL(resourceURI).
			WithOperation("validate_length_range").
			WithComponent("resource_filters")
	}

	// Validate language parameter format (basic validation)
	if params.Language != "" && len(params.Language) > 10 {
		return model.NewFeedError(model.ErrorTypeValidation, "'language' parameter must be a valid language code (max 10 characters)").
			WithURL(resourceURI).
			WithOperation("validate_language_parameter").
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
	return passesDateFilters(item, filters) &&
		passesBasicFilters(item, filters) &&
		passesEnhancedFilters(item, filters)
}

// passesDateFilters checks if item passes date-based filtering
func passesDateFilters(item *gofeed.Item, filters *FilterParams) bool {
	if item.PublishedParsed == nil {
		return true
	}

	if filters.Since != nil && item.PublishedParsed.Before(*filters.Since) {
		return false
	}

	if filters.Until != nil && item.PublishedParsed.After(*filters.Until) {
		return false
	}

	return true
}

// passesBasicFilters checks category, author, and search filters
func passesBasicFilters(item *gofeed.Item, filters *FilterParams) bool {
	if filters.Category != "" && !hasCategory(item, filters.Category) {
		return false
	}

	if filters.Author != "" && !hasAuthor(item, filters.Author) {
		return false
	}

	if filters.Search != "" && !matchesSearch(item, filters.Search) {
		return false
	}

	return true
}

// passesEnhancedFilters checks Phase 2 enhanced filters
func passesEnhancedFilters(item *gofeed.Item, filters *FilterParams) bool {
	if filters.Language != "" && !hasLanguage(item, filters.Language) {
		return false
	}

	if !passesContentLengthFilter(item, filters) {
		return false
	}

	if !passesMediaFilter(item, filters) {
		return false
	}

	if filters.Sentiment != "" && !matchesSentiment(item, filters.Sentiment) {
		return false
	}

	return true
}

// passesContentLengthFilter checks min/max content length filters
func passesContentLengthFilter(item *gofeed.Item, filters *FilterParams) bool {
	contentLength := getContentLength(item)

	if filters.MinLength != nil && contentLength < *filters.MinLength {
		return false
	}

	if filters.MaxLength != nil && contentLength > *filters.MaxLength {
		return false
	}

	return true
}

// passesMediaFilter checks has_media filter
func passesMediaFilter(item *gofeed.Item, filters *FilterParams) bool {
	if filters.HasMedia == nil {
		return true
	}

	itemHasMedia := hasMedia(item)
	return *filters.HasMedia == itemHasMedia
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
		appliedFilters := buildAppliedFiltersMap(filters)
		if len(appliedFilters) > 0 {
			summary.AppliedFilters = appliedFilters
		}
	}

	return summary
}

// buildAppliedFiltersMap builds the map of applied filters
func buildAppliedFiltersMap(filters *FilterParams) map[string]any {
	appliedFilters := make(map[string]any)

	// Add basic filters
	addBasicFiltersToMap(appliedFilters, filters)

	// Add enhanced filters
	addEnhancedFiltersToMap(appliedFilters, filters)

	return appliedFilters
}

// addBasicFiltersToMap adds basic filter parameters to the map
func addBasicFiltersToMap(appliedFilters map[string]any, filters *FilterParams) {
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
}

// addEnhancedFiltersToMap adds Phase 2 enhanced filter parameters to the map
func addEnhancedFiltersToMap(appliedFilters map[string]any, filters *FilterParams) {
	if filters.Language != "" {
		appliedFilters["language"] = filters.Language
	}
	if filters.MinLength != nil {
		appliedFilters["min_length"] = *filters.MinLength
	}
	if filters.MaxLength != nil {
		appliedFilters["max_length"] = *filters.MaxLength
	}
	if filters.HasMedia != nil {
		appliedFilters["has_media"] = *filters.HasMedia
	}
	if filters.Sentiment != "" {
		appliedFilters["sentiment"] = filters.Sentiment
	}
	if filters.Duplicates != nil {
		appliedFilters["duplicates"] = *filters.Duplicates
	}
	if filters.SortBy != "" {
		appliedFilters["sort_by"] = filters.SortBy
	}
	if filters.Format != "" {
		appliedFilters["format"] = filters.Format
	}
}

// Enhanced filter helper functions (Phase 2)

// hasLanguage performs basic language detection/filtering
// This is a simple implementation - for production, consider using proper language detection libraries
func hasLanguage(item *gofeed.Item, language string) bool {
	language = strings.ToLower(language)

	// Check explicit metadata sources first
	if hasLanguageInMetadata(item, language) {
		return true
	}

	// Use content-based heuristics as fallback
	return hasLanguageInContent(item, language)
}

// hasLanguageInMetadata checks for language in item metadata
func hasLanguageInMetadata(item *gofeed.Item, language string) bool {
	// Check Dublin Core extension
	if item.DublinCoreExt != nil {
		for _, itemLang := range item.DublinCoreExt.Language {
			if strings.EqualFold(itemLang, language) {
				return true
			}
		}
	}

	// Check custom fields
	if item.Custom != nil {
		if lang, exists := item.Custom["language"]; exists && strings.EqualFold(lang, language) {
			return true
		}
		if lang, exists := item.Custom["lang"]; exists && strings.EqualFold(lang, language) {
			return true
		}
	}

	return false
}

// hasLanguageInContent uses simple heuristics to detect language in content
func hasLanguageInContent(item *gofeed.Item, language string) bool {
	content := strings.ToLower(item.Title + " " + item.Description + " " + item.Content)

	switch language {
	case "en", "english":
		return detectEnglishWords(content)
	case "es", "spanish":
		return detectSpanishWords(content)
	default:
		return strings.Contains(content, language)
	}
}

// detectEnglishWords checks for common English words
func detectEnglishWords(content string) bool {
	englishWords := []string{"the", "and", "that", "have", "for", "not", "with", "you", "this", "but"}
	return countWordMatches(content, englishWords) >= 3
}

// detectSpanishWords checks for common Spanish words
func detectSpanishWords(content string) bool {
	spanishWords := []string{"que", "con", "para", "una", "por", "como", "del", "los", "las", "más"}
	return countWordMatches(content, spanishWords) >= 3
}

// countWordMatches counts how many words from the list appear in content
func countWordMatches(content string, words []string) int {
	matches := 0
	for _, word := range words {
		if strings.Contains(content, " "+word+" ") {
			matches++
		}
	}
	return matches
}

// getContentLength calculates the approximate content length of a feed item
func getContentLength(item *gofeed.Item) int {
	totalLength := 0

	if item.Title != "" {
		totalLength += len(item.Title)
	}
	if item.Description != "" {
		totalLength += len(item.Description)
	}
	if item.Content != "" {
		totalLength += len(item.Content)
	}

	return totalLength
}

// hasMedia checks if a feed item contains media elements (images, videos, etc.)
func hasMedia(item *gofeed.Item) bool {
	// Check enclosures for media
	for _, enclosure := range item.Enclosures {
		if enclosure.Type != "" {
			mediaTypes := []string{"image/", "video/", "audio/"}
			for _, mediaType := range mediaTypes {
				if strings.HasPrefix(enclosure.Type, mediaType) {
					return true
				}
			}
		}
	}

	// Check content for media tags (basic HTML parsing)
	content := item.Content
	if content == "" {
		content = item.Description
	}

	mediaTags := []string{"<img", "<video", "<audio", "<picture"}
	contentLower := strings.ToLower(content)
	for _, tag := range mediaTags {
		if strings.Contains(contentLower, tag) {
			return true
		}
	}

	return false
}

// matchesSentiment performs basic sentiment analysis
// This is a simplified implementation - for production, use proper sentiment analysis libraries
func matchesSentiment(item *gofeed.Item, sentiment string) bool {
	sentiment = strings.ToLower(sentiment)
	content := strings.ToLower(item.Title + " " + item.Description + " " + item.Content)

	switch sentiment {
	case "positive":
		positiveWords := []string{
			"good", "great", "excellent", "amazing", "wonderful", "fantastic",
			"success", "win", "love", "best", "awesome", "brilliant",
			"perfect", "outstanding", "remarkable", "superb", "magnificent",
		}
		positiveCount := 0
		for _, word := range positiveWords {
			if strings.Contains(content, word) {
				positiveCount++
			}
		}
		return positiveCount >= 2

	case "negative":
		negativeWords := []string{
			"bad", "terrible", "awful", "horrible", "worst", "hate",
			"fail", "failure", "problem", "issue", "crisis", "disaster",
			"disappointing", "unfortunate", "tragic", "sad", "angry",
		}
		negativeCount := 0
		for _, word := range negativeWords {
			if strings.Contains(content, word) {
				negativeCount++
			}
		}
		return negativeCount >= 2

	case "neutral":
		// Neutral is default if not clearly positive or negative
		// Check for absence of strong sentiment indicators
		return !matchesSentiment(item, "positive") && !matchesSentiment(item, "negative")

	default:
		return false
	}
}
