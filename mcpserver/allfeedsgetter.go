package mcpserver

import (
	"context"
	"github.com/richardwooding/feed-mcp/model"
)

type AllFeedsGetter interface {
	GetAllFeeds(ctx context.Context) ([]*model.FeedResult, error)
}
