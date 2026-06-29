package platform

import (
	"context"
	"errors"
	"fmt"

	"github.com/cryolitia/gitea-ai-bot/internal/config"
	"github.com/cryolitia/gitea-ai-bot/internal/core"
	"github.com/cryolitia/gitea-ai-bot/internal/review"
)

var ErrPublishUnsupported = errors.New("publish unsupported for platform")

type MultiAdapter struct {
	Gitea  Adapter
	GitHub Adapter
}

type Adapter interface {
	GetPullRequestContext(context.Context, config.EffectiveRepositoryConfig, core.ReviewRequest) (review.PullRequestContext, error)
	PublishReview(context.Context, config.EffectiveRepositoryConfig, core.ReviewRequest, core.ReviewResult) error
	PublishComment(context.Context, config.EffectiveRepositoryConfig, core.ReviewRequest, string) error
}

func (m MultiAdapter) GetPullRequestContext(ctx context.Context, effective config.EffectiveRepositoryConfig, req core.ReviewRequest) (review.PullRequestContext, error) {
	switch effective.Platform {
	case "gitea":
		return m.Gitea.GetPullRequestContext(ctx, effective, req)
	case "github":
		return m.GitHub.GetPullRequestContext(ctx, effective, req)
	default:
		return review.PullRequestContext{}, fmt.Errorf("unsupported platform %q", effective.Platform)
	}
}

func (m MultiAdapter) PublishReview(ctx context.Context, effective config.EffectiveRepositoryConfig, req core.ReviewRequest, result core.ReviewResult) error {
	switch effective.Platform {
	case "gitea":
		return m.Gitea.PublishReview(ctx, effective, req, result)
	case "github":
		return ErrPublishUnsupported
	default:
		return fmt.Errorf("unsupported platform %q", effective.Platform)
	}
}

func (m MultiAdapter) PublishComment(ctx context.Context, effective config.EffectiveRepositoryConfig, req core.ReviewRequest, body string) error {
	switch effective.Platform {
	case "gitea":
		return m.Gitea.PublishComment(ctx, effective, req, body)
	case "github":
		return ErrPublishUnsupported
	default:
		return fmt.Errorf("unsupported platform %q", effective.Platform)
	}
}
