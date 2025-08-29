package mcpserver

import (
	"context"

	"github.com/richardwooding/feed-mcp/model"
)

// FeedAndItemsGetter provides a method to retrieve a specific feed with its items.
type FeedAndItemsGetter interface {
	GetFeedAndItems(ctx context.Context, id string) (*model.FeedAndItemsResult, error)
}
