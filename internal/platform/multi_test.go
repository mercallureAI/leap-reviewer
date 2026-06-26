package platform

import (
	"context"
	"errors"
	"testing"

	"github.com/cryolitia/gitea-ai-bot/internal/config"
	"github.com/cryolitia/gitea-ai-bot/internal/core"
	"github.com/cryolitia/gitea-ai-bot/internal/review"
)

func TestMultiAdapterRoutesByPlatform(t *testing.T) {
	github := fakeAdapter{ctx: review.PullRequestContext{Title: "gh"}}
	gitea := fakeAdapter{ctx: review.PullRequestContext{Title: "gt"}}
	m := MultiAdapter{Gitea: gitea, GitHub: github}

	ctx, err := m.GetPullRequestContext(context.Background(), config.EffectiveRepositoryConfig{Platform: "github"}, core.ReviewRequest{})
	if err != nil {
		t.Fatalf("GetPullRequestContext() error = %v", err)
	}
	if got, want := ctx.Title, "gh"; got != want {
		t.Fatalf("Title = %q, want %q", got, want)
	}
}

func TestMultiAdapterRejectsGitHubPublish(t *testing.T) {
	m := MultiAdapter{GitHub: fakeAdapter{}}
	err := m.PublishReview(context.Background(), config.EffectiveRepositoryConfig{Platform: "github"}, core.ReviewRequest{}, core.ReviewResult{})
	if err == nil {
		t.Fatal("PublishReview() error = nil, want unsupported error")
	}
	if !errors.Is(err, ErrPublishUnsupported) {
		t.Fatalf("PublishReview() error = %v, want ErrPublishUnsupported", err)
	}
}

type fakeAdapter struct {
	ctx review.PullRequestContext
	err error
}

func (f fakeAdapter) GetPullRequestContext(context.Context, config.EffectiveRepositoryConfig, core.ReviewRequest) (review.PullRequestContext, error) {
	return f.ctx, f.err
}

func (f fakeAdapter) PublishReview(context.Context, config.EffectiveRepositoryConfig, core.ReviewRequest, core.ReviewResult) error {
	return f.err
}
