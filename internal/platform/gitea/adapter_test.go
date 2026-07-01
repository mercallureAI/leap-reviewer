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
	"github.com/cryolitia/gitea-ai-bot/internal/review"
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

func TestUpdatePullRequestBodyPatchesPullRequest(t *testing.T) {
	var method string
	var path string
	var payload struct {
		Body string `json:"body"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
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
	effective := config.EffectiveRepositoryConfig{Owner: "team", Repo: "repo", BaseURL: server.URL, Auth: config.ResolvedAuth{Token: "token"}}

	if err := adapter.UpdatePullRequestBody(context.Background(), effective, core.ReviewRequest{PRNumber: 1}, "new body"); err != nil {
		t.Fatalf("UpdatePullRequestBody() error = %v", err)
	}
	if got, want := method, http.MethodPatch; got != want {
		t.Fatalf("method = %q, want %q", got, want)
	}
	if got, want := path, "/api/v1/repos/team/repo/pulls/1"; got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	if got, want := payload.Body, "new body"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestGetAskContextForIssueUsesDefaultBranchHead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/team/repo/issues/77":
			_, _ = w.Write([]byte(`{"title":"Issue title","body":"Issue body"}`))
		case "/api/v1/repos/team/repo":
			_, _ = w.Write([]byte(`{"default_branch":"main","clone_url":"https://gitea.example.com/team/repo.git"}`))
		case "/api/v1/repos/team/repo/branches/main":
			_, _ = w.Write([]byte(`{"name":"main","commit":{"id":"abc123"}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	adapter := Adapter{Client: server.Client()}
	effective := config.EffectiveRepositoryConfig{Owner: "team", Repo: "repo", BaseURL: server.URL, Auth: config.ResolvedAuth{Token: "token"}}

	ctx, err := adapter.GetAskContext(context.Background(), effective, core.ReviewRequest{IssueNumber: 77})
	if err != nil {
		t.Fatalf("GetAskContext() error = %v", err)
	}
	if got, want := ctx.Title, "Issue title"; got != want {
		t.Fatalf("Title = %q, want %q", got, want)
	}
	if got, want := ctx.Body, "Issue body"; got != want {
		t.Fatalf("Body = %q, want %q", got, want)
	}
	if got, want := ctx.CloneURL, "https://gitea.example.com/team/repo.git"; got != want {
		t.Fatalf("CloneURL = %q, want %q", got, want)
	}
	if got, want := ctx.HeadRef, "main"; got != want {
		t.Fatalf("HeadRef = %q, want %q", got, want)
	}
	if got, want := ctx.HeadSHA, "abc123"; got != want {
		t.Fatalf("HeadSHA = %q, want %q", got, want)
	}
	if got, want := len(ctx.FilesChanged), 0; got != want {
		t.Fatalf("FilesChanged len = %d, want %d", got, want)
	}
	_ = review.PullRequestContext{}
}

func TestGetRepositoryPermissionParsesWriteAccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/api/v1/repos/team/repo/collaborators/alice/permission"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		_, _ = w.Write([]byte(`{"permission":"write","role_name":"contributor","user":{"login":"alice"}}`))
	}))
	defer server.Close()

	adapter := Adapter{Client: server.Client()}
	effective := config.EffectiveRepositoryConfig{Owner: "team", Repo: "repo", BaseURL: server.URL, Auth: config.ResolvedAuth{Token: "token"}}

	permission, err := adapter.GetRepositoryPermission(context.Background(), effective, "alice")
	if err != nil {
		t.Fatalf("GetRepositoryPermission() error = %v", err)
	}
	if !permission.Push {
		t.Fatal("Push = false, want true")
	}
	if permission.Admin {
		t.Fatal("Admin = true, want false")
	}
	if got, want := permission.Role, "contributor"; got != want {
		t.Fatalf("Role = %q, want %q", got, want)
	}
}

func TestGetRepositoryPermissionParsesObjectAccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"permission":{"admin":false,"pull":true,"push":true},"role_name":"write","user":{"login":"alice"}}`))
	}))
	defer server.Close()

	adapter := Adapter{Client: server.Client()}
	effective := config.EffectiveRepositoryConfig{Owner: "team", Repo: "repo", BaseURL: server.URL, Auth: config.ResolvedAuth{Token: "token"}}

	permission, err := adapter.GetRepositoryPermission(context.Background(), effective, "alice")
	if err != nil {
		t.Fatalf("GetRepositoryPermission() error = %v", err)
	}
	if !permission.Push || !permission.Pull {
		t.Fatalf("permission = %#v, want push and pull true", permission)
	}
}
