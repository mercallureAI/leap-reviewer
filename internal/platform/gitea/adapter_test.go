package gitea

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cryolitia/gitea-ai-bot/internal/config"
	"github.com/cryolitia/gitea-ai-bot/internal/core"
)

func TestPublishReviewDegradesMultilineFindingToStartLine(t *testing.T) {
	var payload createReviewRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := Adapter{Client: server.Client()}
	effective := config.EffectiveRepositoryConfig{
		Owner:   "team",
		Repo:    "repo",
		BaseURL: server.URL,
		Auth:    config.ResolvedAuth{Token: "token"},
	}
	result := core.ReviewResult{
		ReviewAction: "comment",
		Summary:      "Summary",
		InlineFindings: []core.InlineFinding{{
			Position: core.InlinePosition{Path: "main.go", StartLine: 10, EndLine: 12, StartSide: "RIGHT", EndSide: "RIGHT"},
			Title:    "Range issue",
			Body:     "Body",
		}},
	}

	if err := adapter.PublishReview(context.Background(), effective, core.ReviewRequest{PRNumber: 1}, result); err != nil {
		t.Fatalf("PublishReview() error = %v", err)
	}
	if got, want := len(payload.Comments), 1; got != want {
		t.Fatalf("comments len = %d, want %d", got, want)
	}
	if got, want := payload.Comments[0].NewLineNum, 10; got != want {
		t.Fatalf("NewLineNum = %d, want %d", got, want)
	}
	if !strings.Contains(payload.Comments[0].Body, "Lines 10-12") {
		t.Fatalf("comment body = %q, want line range marker", payload.Comments[0].Body)
	}
}

func TestPublishCommentPostsIssueComment(t *testing.T) {
	var path string
	var payload struct {
		Body string `json:"body"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		defer r.Body.Close()
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := Adapter{Client: server.Client()}
	effective := config.EffectiveRepositoryConfig{
		Owner:   "team",
		Repo:    "repo",
		BaseURL: server.URL,
		Auth:    config.ResolvedAuth{Token: "token"},
	}

	if err := adapter.PublishComment(context.Background(), effective, core.ReviewRequest{PRNumber: 1}, "plain answer"); err != nil {
		t.Fatalf("PublishComment() error = %v", err)
	}
	if got, want := path, "/api/v1/repos/team/repo/issues/1/comments"; got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	if got, want := payload.Body, "plain answer"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}
