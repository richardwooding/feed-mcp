// Package examples demonstrates MCP Resources usage patterns for feed-mcp.
// This file provides comprehensive examples of how to integrate with and use
// the MCP Resources API for various feed management scenarios.
package examples

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/richardwooding/feed-mcp/model"
)

// ResourceExample demonstrates common MCP Resources usage patterns
type ResourceExample struct {
	client MCPClient // Interface for MCP client implementation
}

// MCPClient represents an MCP protocol client (interface for example purposes)
type MCPClient interface {
	ListResources(ctx context.Context) (*ListResourcesResponse, error)
	ReadResource(ctx context.Context, uri string) (*ReadResourceResponse, error)
	SubscribeResource(ctx context.Context, uri string) error
	UnsubscribeResource(ctx context.Context, uri string) error
}

// ListResourcesResponse represents the response from MCP resources/list method
type ListResourcesResponse struct {
	Resources []Resource `json:"resources"`
}

// ReadResourceResponse represents the response from MCP resources/read method
type ReadResourceResponse struct {
	Contents []ResourceContent `json:"contents"`
}

// Resource represents a single MCP resource with metadata
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType"`
}

// ResourceContent represents the content of a read resource
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

// DiscoverFeeds demonstrates how to discover available feeds using MCP Resources
func (r *ResourceExample) DiscoverFeeds(ctx context.Context) error {
	fmt.Println("=== Discovering Available Feeds ===")

	// List all available resources
	resp, err := r.client.ListResources(ctx)
	if err != nil {
		return fmt.Errorf("failed to list resources: %w", err)
	}

	fmt.Printf("Found %d resources:\n", len(resp.Resources))
	for _, resource := range resp.Resources {
		fmt.Printf("- %s: %s\n", resource.URI, resource.Name)
		fmt.Printf("  Description: %s\n", resource.Description)
		fmt.Printf("  MIME Type: %s\n\n", resource.MimeType)
	}

	return nil
}

// GetAllFeeds demonstrates how to retrieve the complete list of available feeds
func (r *ResourceExample) GetAllFeeds(ctx context.Context) ([]*model.FeedResult, error) {
	fmt.Println("=== Getting All Feeds ===")

	resp, err := r.client.ReadResource(ctx, "feeds://all")
	if err != nil {
		return nil, fmt.Errorf("failed to read feeds list: %w", err)
	}

	if len(resp.Contents) == 0 {
		return nil, fmt.Errorf("no content in response")
	}

	var feeds []*model.FeedResult
	if err := json.Unmarshal([]byte(resp.Contents[0].Text), &feeds); err != nil {
		return nil, fmt.Errorf("failed to parse feeds: %w", err)
	}

	fmt.Printf("Retrieved %d feeds:\n", len(feeds))
	for _, feed := range feeds {
		fmt.Printf("- %s (%s)\n", feed.Title, feed.ID)
		fmt.Printf("  URL: %s\n", feed.PublicURL)
	}

	return feeds, nil
}

// GetRecentItems demonstrates how to retrieve recent items from a specific feed with date filtering
func (r *ResourceExample) GetRecentItems(ctx context.Context, feedID string, limit int) error {
	fmt.Printf("=== Getting Recent Items from Feed %s ===\n", feedID)

	// Get items from last 7 days with limit
	since := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	uri := fmt.Sprintf("feeds://feed/%s/items?since=%s&limit=%d", feedID, since, limit)

	resp, err := r.client.ReadResource(ctx, uri)
	if err != nil {
		return fmt.Errorf("failed to read feed items: %w", err)
	}

	if len(resp.Contents) == 0 {
		return fmt.Errorf("no content in response")
	}

	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(resp.Contents[0].Text), &items); err != nil {
		return fmt.Errorf("failed to parse items: %w", err)
	}

	fmt.Printf("Retrieved %d recent items:\n", len(items))
	for i, item := range items {
		title, _ := item["title"].(string)
		published, _ := item["published"].(string)
		link, _ := item["link"].(string)

		fmt.Printf("%d. %s\n", i+1, title)
		fmt.Printf("   Published: %s\n", published)
		fmt.Printf("   Link: %s\n\n", link)
	}

	return nil
}

// SearchFeedContent demonstrates how to search for specific content within a feed using full-text search
func (r *ResourceExample) SearchFeedContent(ctx context.Context, feedID, searchTerm string) error {
	fmt.Printf("=== Searching for '%s' in Feed %s ===\n", searchTerm, feedID)

	// Search with additional filters
	uri := fmt.Sprintf("feeds://feed/%s/items?search=%s&limit=5", feedID, searchTerm)

	resp, err := r.client.ReadResource(ctx, uri)
	if err != nil {
		return fmt.Errorf("failed to search feed: %w", err)
	}

	if len(resp.Contents) == 0 {
		fmt.Println("No matching items found")
		return nil
	}

	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(resp.Contents[0].Text), &items); err != nil {
		return fmt.Errorf("failed to parse search results: %w", err)
	}

	fmt.Printf("Found %d matching items:\n", len(items))
	for i, item := range items {
		title, _ := item["title"].(string)
		description, _ := item["description"].(string)

		fmt.Printf("%d. %s\n", i+1, title)
		if len(description) > 150 {
			description = description[:150] + "..."
		}
		fmt.Printf("   %s\n\n", description)
	}

	return nil
}

// GetFeedMetadata demonstrates how to retrieve feed metadata without items for efficient metadata access
func (r *ResourceExample) GetFeedMetadata(ctx context.Context, feedID string) error {
	fmt.Printf("=== Getting Metadata for Feed %s ===\n", feedID)

	uri := fmt.Sprintf("feeds://feed/%s/meta", feedID)

	resp, err := r.client.ReadResource(ctx, uri)
	if err != nil {
		return fmt.Errorf("failed to read feed metadata: %w", err)
	}

	if len(resp.Contents) == 0 {
		return fmt.Errorf("no metadata available")
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(resp.Contents[0].Text), &metadata); err != nil {
		return fmt.Errorf("failed to parse metadata: %w", err)
	}

	fmt.Printf("Feed Metadata:\n")
	if feed, ok := metadata["feed"].(map[string]interface{}); ok {
		if title, ok := feed["title"].(string); ok {
			fmt.Printf("  Title: %s\n", title)
		}
		if desc, ok := feed["description"].(string); ok {
			fmt.Printf("  Description: %s\n", desc)
		}
		if lang, ok := feed["language"].(string); ok {
			fmt.Printf("  Language: %s\n", lang)
		}
		if updated, ok := feed["updated"].(string); ok {
			fmt.Printf("  Last Updated: %s\n", updated)
		}
	}

	return nil
}

// GetTechNewsItems demonstrates how to filter feed items by category for technology-related content
func (r *ResourceExample) GetTechNewsItems(ctx context.Context, feedID string) error {
	fmt.Printf("=== Getting Technology News from Feed %s ===\n", feedID)

	// Filter by technology-related categories
	categories := []string{"technology", "tech", "ai", "software", "programming"}

	for _, category := range categories {
		uri := fmt.Sprintf("feeds://feed/%s/items?category=%s&limit=3", feedID, category)

		resp, err := r.client.ReadResource(ctx, uri)
		if err != nil {
			log.Printf("Failed to get items for category %s: %v", category, err)
			continue
		}

		if len(resp.Contents) == 0 {
			continue
		}

		var items []map[string]interface{}
		if err := json.Unmarshal([]byte(resp.Contents[0].Text), &items); err != nil {
			log.Printf("Failed to parse items for category %s: %v", category, err)
			continue
		}

		if len(items) > 0 {
			fmt.Printf("Category '%s' - %d items:\n", category, len(items))
			for _, item := range items {
				title, _ := item["title"].(string)
				fmt.Printf("  - %s\n", title)
			}
			fmt.Println()
		}
	}

	return nil
}

// ReadFeedWithPagination demonstrates how to implement pagination for large feeds using limit and offset
func (r *ResourceExample) ReadFeedWithPagination(ctx context.Context, feedID string, pageSize int) error {
	fmt.Printf("=== Reading Feed %s with Pagination (page size: %d) ===\n", feedID, pageSize)

	offset := 0
	page := 1

	for {
		uri := fmt.Sprintf("feeds://feed/%s/items?limit=%d&offset=%d", feedID, pageSize, offset)

		resp, err := r.client.ReadResource(ctx, uri)
		if err != nil {
			return fmt.Errorf("failed to read page %d: %w", page, err)
		}

		if len(resp.Contents) == 0 {
			break
		}

		var items []map[string]interface{}
		if err := json.Unmarshal([]byte(resp.Contents[0].Text), &items); err != nil {
			return fmt.Errorf("failed to parse page %d: %w", page, err)
		}

		if len(items) == 0 {
			break
		}

		fmt.Printf("Page %d - %d items:\n", page, len(items))
		for i, item := range items {
			title, _ := item["title"].(string)
			fmt.Printf("  %d. %s\n", offset+i+1, title)
		}

		if len(items) < pageSize {
			break // Last page
		}

		offset += pageSize
		page++
		fmt.Println()
	}

	return nil
}

// MonitorFeedUpdates demonstrates how to set up real-time monitoring using MCP resource subscriptions
func (r *ResourceExample) MonitorFeedUpdates(ctx context.Context, feedIDs []string) error {
	fmt.Println("=== Setting up Real-time Feed Monitoring ===")

	// Subscribe to multiple feeds
	for _, feedID := range feedIDs {
		uri := fmt.Sprintf("feeds://feed/%s/items", feedID)
		if err := r.client.SubscribeResource(ctx, uri); err != nil {
			log.Printf("Failed to subscribe to feed %s: %v", feedID, err)
			continue
		}
		fmt.Printf("Subscribed to feed: %s\n", feedID)
	}

	fmt.Println("\nMonitoring for feed updates... (Press Ctrl+C to stop)")
	fmt.Println("Note: This example shows subscription setup. In a real implementation,")
	fmt.Println("you would handle incoming resource update notifications here.")

	// In a real implementation, you would:
	// 1. Listen for resource update notifications
	// 2. Handle the notifications (e.g., refresh UI, send alerts)
	// 3. Implement proper cleanup on shutdown

	return nil
}

// AnalyzeFeedActivity demonstrates how to analyze feed posting patterns using date range filtering
func (r *ResourceExample) AnalyzeFeedActivity(ctx context.Context, feedID string, days int) error {
	fmt.Printf("=== Analyzing Feed Activity for Last %d Days ===\n", days)

	// Get items for each day to analyze posting patterns
	for i := 0; i < days; i++ {
		date := time.Now().AddDate(0, 0, -i)
		since := date.Format("2006-01-02")
		until := date.AddDate(0, 0, 1).Format("2006-01-02")

		uri := fmt.Sprintf("feeds://feed/%s/items?since=%s&until=%s", feedID, since, until)

		resp, err := r.client.ReadResource(ctx, uri)
		if err != nil {
			log.Printf("Failed to get items for %s: %v", since, err)
			continue
		}

		if len(resp.Contents) == 0 {
			continue
		}

		var items []map[string]interface{}
		if err := json.Unmarshal([]byte(resp.Contents[0].Text), &items); err != nil {
			log.Printf("Failed to parse items for %s: %v", since, err)
			continue
		}

		fmt.Printf("%s: %d items\n", since, len(items))
	}

	return nil
}

// AggregateMultipleFeedContent demonstrates how to aggregate content across multiple feeds with search filtering
func (r *ResourceExample) AggregateMultipleFeedContent(ctx context.Context, feedIDs []string, searchTerm string) error {
	fmt.Printf("=== Aggregating Content for '%s' across %d feeds ===\n", searchTerm, len(feedIDs))

	allItems := make([]map[string]interface{}, 0)

	for _, feedID := range feedIDs {
		uri := fmt.Sprintf("feeds://feed/%s/items?search=%s&limit=10", feedID, searchTerm)

		resp, err := r.client.ReadResource(ctx, uri)
		if err != nil {
			log.Printf("Failed to search feed %s: %v", feedID, err)
			continue
		}

		if len(resp.Contents) == 0 {
			continue
		}

		var items []map[string]interface{}
		if err := json.Unmarshal([]byte(resp.Contents[0].Text), &items); err != nil {
			log.Printf("Failed to parse items from feed %s: %v", feedID, err)
			continue
		}

		// Add feed ID to each item for tracking
		for i := range items {
			items[i]["feedId"] = feedID
		}

		allItems = append(allItems, items...)
	}

	fmt.Printf("Found %d total matching items across all feeds:\n", len(allItems))
	for i, item := range allItems {
		title, _ := item["title"].(string)
		feedID, _ := item["feedId"].(string)
		published, _ := item["published"].(string)

		fmt.Printf("%d. %s [Feed: %s]\n", i+1, title, feedID)
		fmt.Printf("   Published: %s\n\n", published)
	}

	return nil
}

// RunAllExamples demonstrates all usage patterns
func RunAllExamples(ctx context.Context, client MCPClient) error {
	example := &ResourceExample{client: client}

	fmt.Println("Running MCP Resources Usage Examples")
	fmt.Println("=====================================")

	// Example 1: Discovery
	if err := example.DiscoverFeeds(ctx); err != nil {
		return fmt.Errorf("discovery example failed: %w", err)
	}

	// Get feed list for subsequent examples
	feeds, err := example.GetAllFeeds(ctx)
	if err != nil {
		return fmt.Errorf("get feeds example failed: %w", err)
	}

	if len(feeds) == 0 {
		fmt.Println("No feeds available for remaining examples")
		return nil
	}

	// Use first feed for single-feed examples
	firstFeedID := feeds[0].ID

	// Example 3: Recent items
	if err := example.GetRecentItems(ctx, firstFeedID, 5); err != nil {
		log.Printf("Recent items example failed: %v", err)
	}

	// Example 4: Search
	if err := example.SearchFeedContent(ctx, firstFeedID, "technology"); err != nil {
		log.Printf("Search example failed: %v", err)
	}

	// Example 5: Metadata
	if err := example.GetFeedMetadata(ctx, firstFeedID); err != nil {
		log.Printf("Metadata example failed: %v", err)
	}

	// Example 6: Category filtering
	if err := example.GetTechNewsItems(ctx, firstFeedID); err != nil {
		log.Printf("Category filtering example failed: %v", err)
	}

	// Example 7: Pagination
	if err := example.ReadFeedWithPagination(ctx, firstFeedID, 3); err != nil {
		log.Printf("Pagination example failed: %v", err)
	}

	// Example 8: Subscriptions
	feedIDs := []string{firstFeedID}
	if len(feeds) > 1 {
		feedIDs = append(feedIDs, feeds[1].ID)
	}
	if err := example.MonitorFeedUpdates(ctx, feedIDs); err != nil {
		log.Printf("Monitoring example failed: %v", err)
	}

	// Example 9: Date analysis
	if err := example.AnalyzeFeedActivity(ctx, firstFeedID, 7); err != nil {
		log.Printf("Activity analysis example failed: %v", err)
	}

	// Example 10: Multi-feed aggregation
	if len(feeds) > 1 {
		feedIDs := make([]string, 0, len(feeds))
		for _, feed := range feeds {
			feedIDs = append(feedIDs, feed.ID)
		}
		if err := example.AggregateMultipleFeedContent(ctx, feedIDs, "news"); err != nil {
			log.Printf("Multi-feed aggregation example failed: %v", err)
		}
	}

	fmt.Println("\n=== All Examples Completed ===")
	return nil
}
