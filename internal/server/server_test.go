package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cryolitia/gitea-ai-bot/internal/config"
	"github.com/cryolitia/gitea-ai-bot/internal/core"
	"github.com/cryolitia/gitea-ai-bot/internal/profiles"
)

func TestParseIssueCommentTriggerParsesAskQuestion(t *testing.T) {
	payload := []byte(`{
		"action": "created",
		"is_pull": true,
		"comment": {"body": "/ask security why this change", "id": 12},
		"repository": {"owner": {"username": "team"}, "name": "repo"},
		"issue": {"number": 42},
		"sender": {"username": "alice"}
	}`)

	req, handled, err := ParseIssueCommentTrigger(payload, "default", []string{"default", "security"})
	if err != nil {
		t.Fatalf("ParseIssueCommentTrigger() error = %v", err)
	}
	if !handled {
		t.Fatal("ParseIssueCommentTrigger() handled = false, want true")
	}
	if got, want := req.CommandText, "/ask security why this change"; got != want {
		t.Fatalf("CommandText = %q, want %q", got, want)
	}
	if got, want := req.ProfileName, "security"; got != want {
		t.Fatalf("ProfileName = %q, want %q", got, want)
	}
	if got, want := req.QuestionText, "why this change"; got != want {
		t.Fatalf("QuestionText = %q, want %q", got, want)
	}
	if got, want := req.TriggerType, "command"; got != want {
		t.Fatalf("TriggerType = %q, want %q", got, want)
	}
	if got, want := req.TriggerUser, "alice"; got != want {
		t.Fatalf("TriggerUser = %q, want %q", got, want)
	}
}

func TestHandlerPublishesAskCommentWhenAllowed(t *testing.T) {
	body := []byte(`{
		"action": "created",
		"is_pull": true,
		"comment": {"body": "/ask why split this", "id": 12},
		"repository": {"owner": {"username": "team"}, "name": "repo"},
		"issue": {"number": 42},
		"sender": {"username": "alice"}
	}`)
	loader := fakeLoader{cfg: config.EffectiveRepositoryConfig{
		Owner:    "team",
		Repo:     "repo",
		Platform: "gitea",
		Auth:     config.ResolvedAuth{WebhookSecret: "secret"},
		Config: config.Config{
			DefaultProfile:       "default",
			EnabledProfiles:      []string{"default", "security"},
			CommandReviewEnabled: true,
			AllowedCommands:      []string{"review", "ask"},
		},
	}, profile: profiles.Definition{Name: "default"}}
	askReviewer := &fakeAskReviewer{result: core.AskResult{Answer: "因为需要隔离职责。"}}
	commentPublisher := &fakeCommentPublisher{}
	handler := Handler{InstanceKey: "corp-gitea", Loader: loader, AskReviewer: askReviewer, CommentPublisher: commentPublisher}

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("X-Gitea-Event", "issue_comment")
	req.Header.Set("X-Gitea-Signature", signature("secret", body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusAccepted)
	}
	if askReviewer.last.QuestionText != "why split this" {
		t.Fatalf("QuestionText = %q, want %q", askReviewer.last.QuestionText, "why split this")
	}
	if !strings.Contains(commentPublisher.body, "问题：\nwhy split this") {
		t.Fatalf("comment body = %q, want question block", commentPublisher.body)
	}
	if !strings.Contains(commentPublisher.body, "回答：\n因为需要隔离职责。") {
		t.Fatalf("comment body = %q, want answer block", commentPublisher.body)
	}
}

func TestHandlerPublishesAskCommentForIssueWhenAllowed(t *testing.T) {
	body := []byte(`{
		"action": "created",
		"is_pull": false,
		"comment": {"body": "/ask what does this issue mean", "id": 12},
		"repository": {"owner": {"username": "team"}, "name": "repo"},
		"issue": {"number": 77},
		"sender": {"username": "alice"}
	}`)
	loader := fakeLoader{cfg: config.EffectiveRepositoryConfig{
		Owner:    "team",
		Repo:     "repo",
		Platform: "gitea",
		Auth:     config.ResolvedAuth{WebhookSecret: "secret"},
		Config: config.Config{
			DefaultProfile:       "default",
			EnabledProfiles:      []string{"default"},
			CommandReviewEnabled: true,
			AllowedCommands:      []string{"ask"},
		},
	}, profile: profiles.Definition{Name: "default"}}
	askReviewer := &fakeAskReviewer{result: core.AskResult{Answer: "这是一个关于打包问题的 issue。"}}
	commentPublisher := &fakeCommentPublisher{}
	handler := Handler{InstanceKey: "corp-gitea", Loader: loader, AskReviewer: askReviewer, CommentPublisher: commentPublisher}

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("X-Gitea-Event", "issue_comment")
	req.Header.Set("X-Gitea-Signature", signature("secret", body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusAccepted)
	}
	if got, want := askReviewer.last.IssueNumber, 77; got != want {
		t.Fatalf("IssueNumber = %d, want %d", got, want)
	}
	if got := askReviewer.last.PRNumber; got != 0 {
		t.Fatalf("PRNumber = %d, want 0", got)
	}
	if !strings.Contains(commentPublisher.body, "回答：\n这是一个关于打包问题的 issue。") {
		t.Fatalf("comment body = %q, want answer block", commentPublisher.body)
	}
}

func TestHandlerIgnoresAskWhenCommandNotAllowed(t *testing.T) {
	body := []byte(`{
		"action": "created",
		"is_pull": true,
		"comment": {"body": "/ask why split this", "id": 12},
		"repository": {"owner": {"username": "team"}, "name": "repo"},
		"issue": {"number": 42},
		"sender": {"username": "alice"}
	}`)
	handler := Handler{InstanceKey: "corp-gitea", Loader: fakeLoader{cfg: config.EffectiveRepositoryConfig{
		Owner:    "team",
		Repo:     "repo",
		Platform: "gitea",
		Auth:     config.ResolvedAuth{WebhookSecret: "secret"},
		Config: config.Config{
			DefaultProfile:       "default",
			EnabledProfiles:      []string{"default"},
			CommandReviewEnabled: true,
			AllowedCommands:      []string{"review"},
		},
	}, profile: profiles.Definition{Name: "default"}}, AskReviewer: &fakeAskReviewer{}, CommentPublisher: &fakeCommentPublisher{}}

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("X-Gitea-Event", "issue_comment")
	req.Header.Set("X-Gitea-Signature", signature("secret", body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusAccepted)
	}
}

func TestHandlerUpdatesPullRequestBodyForSummarizeWhenAllowed(t *testing.T) {
	body := []byte(`{
		"action": "created",
		"is_pull": true,
		"comment": {"body": "/summarize", "id": 12},
		"repository": {"owner": {"username": "team"}, "name": "repo"},
		"issue": {"number": 42},
		"sender": {"username": "alice"}
	}`)
	loader := fakeLoader{cfg: config.EffectiveRepositoryConfig{
		Owner: "team", Repo: "repo", Platform: "gitea", Auth: config.ResolvedAuth{WebhookSecret: "secret"},
		Config: config.Config{DefaultProfile: "default", EnabledProfiles: []string{"default"}, CommandReviewEnabled: true, AllowedCommands: []string{"review", "ask", "summarize"}},
	}, profile: profiles.Definition{Name: "default"}}
	summarizeReviewer := &fakeSummarizeReviewer{result: core.SummarizeResult{Body: "new body", OriginalBody: "original body", Source: "alice", OriginalAuthor: "alice"}}
	bodyUpdater := &fakeBodyUpdater{}
	handler := Handler{InstanceKey: "corp-gitea", Loader: loader, SummarizeReviewer: summarizeReviewer, BodyUpdater: bodyUpdater, CommentPublisher: &fakeCommentPublisher{}, PermissionChecker: fakePermissionChecker{permission: core.RepositoryPermission{Push: false}}}

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("X-Gitea-Event", "issue_comment")
	req.Header.Set("X-Gitea-Signature", signature("secret", body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusAccepted)
	}
	want := "original body\n\n---\n\n<!-- review-bot:summarized source=alice -->\n\nnew body"
	if got := bodyUpdater.body; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestHandlerUpdatesPullRequestBodyForSummarizeWhenUserHasWritePermission(t *testing.T) {
	body := []byte(`{
		"action": "created",
		"is_pull": true,
		"comment": {"body": "/summarize", "id": 12},
		"repository": {"owner": {"username": "team"}, "name": "repo"},
		"issue": {"number": 42},
		"sender": {"username": "bob"}
	}`)
	loader := fakeLoader{cfg: config.EffectiveRepositoryConfig{Owner: "team", Repo: "repo", Platform: "gitea", Auth: config.ResolvedAuth{WebhookSecret: "secret"}, Config: config.Config{DefaultProfile: "default", EnabledProfiles: []string{"default"}, CommandReviewEnabled: true, AllowedCommands: []string{"summarize"}}}, profile: profiles.Definition{Name: "default"}}
	summarizeReviewer := &fakeSummarizeReviewer{result: core.SummarizeResult{Body: "new body", OriginalBody: "original body", OriginalAuthor: "alice"}}
	bodyUpdater := &fakeBodyUpdater{}
	handler := Handler{InstanceKey: "corp-gitea", Loader: loader, SummarizeReviewer: summarizeReviewer, BodyUpdater: bodyUpdater, CommentPublisher: &fakeCommentPublisher{}, PermissionChecker: fakePermissionChecker{permission: core.RepositoryPermission{Push: true}}}

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("X-Gitea-Event", "issue_comment")
	req.Header.Set("X-Gitea-Signature", signature("secret", body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusAccepted)
	}
	if got := bodyUpdater.body; got == "" {
		t.Fatal("body updater did not run")
	}
}

func TestHandlerFallsBackToCommentForSummarizeWithoutPermission(t *testing.T) {
	body := []byte(`{
		"action": "created",
		"is_pull": true,
		"comment": {"body": "/summarize", "id": 12},
		"repository": {"owner": {"username": "team"}, "name": "repo"},
		"issue": {"number": 42},
		"sender": {"username": "bob"}
	}`)
	loader := fakeLoader{cfg: config.EffectiveRepositoryConfig{Owner: "team", Repo: "repo", Platform: "gitea", Auth: config.ResolvedAuth{WebhookSecret: "secret"}, Config: config.Config{DefaultProfile: "default", EnabledProfiles: []string{"default"}, CommandReviewEnabled: true, AllowedCommands: []string{"summarize"}}}, profile: profiles.Definition{Name: "default"}}
	summarizeReviewer := &fakeSummarizeReviewer{result: core.SummarizeResult{Body: "new body", OriginalBody: "original body", OriginalAuthor: "alice"}}
	commentPublisher := &fakeCommentPublisher{}
	bodyUpdater := &fakeBodyUpdater{}
	handler := Handler{InstanceKey: "corp-gitea", Loader: loader, SummarizeReviewer: summarizeReviewer, BodyUpdater: bodyUpdater, CommentPublisher: commentPublisher, PermissionChecker: fakePermissionChecker{permission: core.RepositoryPermission{Push: false}}}

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("X-Gitea-Event", "issue_comment")
	req.Header.Set("X-Gitea-Signature", signature("secret", body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusAccepted)
	}
	if got := bodyUpdater.body; got != "" {
		t.Fatalf("body updater should not run, got %q", got)
	}
	if !strings.Contains(commentPublisher.body, "@bob 你当前没有修改这个 PR 描述的权限") {
		t.Fatalf("comment body = %q, want permission explanation", commentPublisher.body)
	}
	if !strings.Contains(commentPublisher.body, "new body") {
		t.Fatalf("comment body = %q, want summary body", commentPublisher.body)
	}
}

func TestHandlerSkipsSummarizeWhenAlreadySummarized(t *testing.T) {
	body := []byte(`{
		"action": "created",
		"is_pull": true,
		"comment": {"body": "/summarize", "id": 12},
		"repository": {"owner": {"username": "team"}, "name": "repo"},
		"issue": {"number": 42},
		"sender": {"username": "alice"}
	}`)
	loader := fakeLoader{cfg: config.EffectiveRepositoryConfig{
		Owner: "team", Repo: "repo", Platform: "gitea", Auth: config.ResolvedAuth{WebhookSecret: "secret"},
		Config: config.Config{DefaultProfile: "default", EnabledProfiles: []string{"default"}, CommandReviewEnabled: true, AllowedCommands: []string{"summarize"}},
	}, profile: profiles.Definition{Name: "default"}}
	summarizeReviewer := &fakeSummarizeReviewer{result: core.SummarizeResult{AlreadySummarized: true, Source: "alice", OriginalBody: "original body"}}
	bodyUpdater := &fakeBodyUpdater{}
	commentPublisher := &fakeCommentPublisher{}
	handler := Handler{InstanceKey: "corp-gitea", Loader: loader, SummarizeReviewer: summarizeReviewer, BodyUpdater: bodyUpdater, CommentPublisher: commentPublisher}

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("X-Gitea-Event", "issue_comment")
	req.Header.Set("X-Gitea-Signature", signature("secret", body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusAccepted)
	}
	if got := bodyUpdater.body; got != "" {
		t.Fatalf("body updater should not run, got %q", got)
	}
	if !strings.Contains(commentPublisher.body, "已经由 alice 总结过") {
		t.Fatalf("comment body = %q, want already summarized notice", commentPublisher.body)
	}
}

type fakeLoader struct {
	cfg     config.EffectiveRepositoryConfig
	profile profiles.Definition
}

func (f fakeLoader) Load(instanceKey, owner, repo, profile string) (config.EffectiveRepositoryConfig, profiles.Definition, error) {
	return f.cfg, f.profile, nil
}

type fakeAskReviewer struct {
	last   core.ReviewRequest
	result core.AskResult
}

func (f *fakeAskReviewer) Execute(_ context.Context, req core.ReviewRequest) (core.AskResult, error) {
	f.last = req
	return f.result, nil
}

type fakeCommentPublisher struct {
	body string
}

func (f *fakeCommentPublisher) PublishComment(_ context.Context, _ config.EffectiveRepositoryConfig, _ core.ReviewRequest, body string) error {
	f.body = body
	return nil
}

type fakeSummarizeReviewer struct {
	result core.SummarizeResult
}

func (f *fakeSummarizeReviewer) Execute(_ context.Context, _ core.ReviewRequest) (core.SummarizeResult, error) {
	return f.result, nil
}

type fakeBodyUpdater struct{ body string }

func (f *fakeBodyUpdater) UpdatePullRequestBody(_ context.Context, _ config.EffectiveRepositoryConfig, _ core.ReviewRequest, body string) error {
	f.body = body
	return nil
}

type fakePermissionChecker struct{ permission core.RepositoryPermission }

func (f fakePermissionChecker) GetRepositoryPermission(context.Context, config.EffectiveRepositoryConfig, string) (core.RepositoryPermission, error) {
	return f.permission, nil
}

func signature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
