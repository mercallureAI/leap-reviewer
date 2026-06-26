package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cryolitia/gitea-ai-bot/internal/config"
	"github.com/cryolitia/gitea-ai-bot/internal/core"
)

func TestAdapterGetPullRequestContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization header = %q, want empty", got)
		}
		switch r.URL.Path {
		case "/repos/backend-team/payment-service/pulls/42":
			_, _ = w.Write([]byte(`{
				"title": "Fix bug",
				"body": "PR body",
				"head": {
					"sha": "abc123",
					"ref": "feature-branch",
					"repo": {"clone_url": "https://github.com/backend-team/payment-service.git"}
				}
			}`))
		case "/repos/backend-team/payment-service/pulls/42/files":
			_, _ = w.Write([]byte(`[
				{"filename": "main.go", "patch": "@@ -1 +1 @@"}
			]`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	adapter := Adapter{Client: server.Client()}
	effective := config.EffectiveRepositoryConfig{
		Owner:    "backend-team",
		Repo:     "payment-service",
		BaseURL:  server.URL,
		Platform: "github",
		Auth:     config.ResolvedAuth{},
	}

	ctx, err := adapter.GetPullRequestContext(context.Background(), effective, core.ReviewRequest{PRNumber: 42})
	if err != nil {
		t.Fatalf("GetPullRequestContext() error = %v", err)
	}
	if got, want := ctx.Title, "Fix bug"; got != want {
		t.Fatalf("Title = %q, want %q", got, want)
	}
	if got, want := ctx.CloneURL, "https://github.com/backend-team/payment-service.git"; got != want {
		t.Fatalf("CloneURL = %q, want %q", got, want)
	}
	if got, want := ctx.HeadRef, "feature-branch"; got != want {
		t.Fatalf("HeadRef = %q, want %q", got, want)
	}
	if got, want := len(ctx.FilesChanged), 1; got != want {
		t.Fatalf("FilesChanged len = %d, want %d", got, want)
	}
}
