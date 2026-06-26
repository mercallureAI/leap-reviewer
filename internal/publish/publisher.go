package publish

import (
	"context"

	"github.com/cryolitia/gitea-ai-bot/internal/config"
	"github.com/cryolitia/gitea-ai-bot/internal/core"
)

type PlatformPublisher interface {
	PublishReview(context.Context, config.EffectiveRepositoryConfig, core.ReviewRequest, core.ReviewResult) error
}

type Publisher struct {
	Platform PlatformPublisher
}

func (p Publisher) Publish(ctx context.Context, effective config.EffectiveRepositoryConfig, req core.ReviewRequest, result core.ReviewResult) error {
	if req.DryRun || !req.Publish {
		return nil
	}
	return p.Platform.PublishReview(ctx, effective, req, result)
}
