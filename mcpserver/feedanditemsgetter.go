package mcpserver

import (
	"context"
	"github.com/richardwooding/feed-mcp/model"
)

type FeedAndItemsGetter interface {
	GetFeedAndItems(ctx context.Context, id string) (*model.FeedAndItemsResult, error)
}
