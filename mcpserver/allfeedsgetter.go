package mcpserver

import (
	"context"

	"github.com/richardwooding/feed-mcp/model"
)

// AllFeedsGetter provides a method to retrieve all available feeds.
type AllFeedsGetter interface {
	GetAllFeeds(ctx context.Context) ([]*model.FeedResult, error)
}
