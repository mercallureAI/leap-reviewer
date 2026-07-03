package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

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
	waitForCondition(t, func() bool {
		return askReviewer.Last().QuestionText == "why split this" && strings.Contains(commentPublisher.Body(), "回答：\n因为需要隔离职责。")
	}, "ask comment publish")
	if !strings.Contains(commentPublisher.Body(), "问题：\nwhy split this") {
		t.Fatalf("comment body = %q, want question block", commentPublisher.Body())
	}
	if !strings.Contains(commentPublisher.Body(), "回答：\n因为需要隔离职责。") {
		t.Fatalf("comment body = %q, want answer block", commentPublisher.Body())
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
	waitForCondition(t, func() bool {
		return askReviewer.Last().IssueNumber == 77 && strings.Contains(commentPublisher.Body(), "回答：\n这是一个关于打包问题的 issue。")
	}, "issue ask comment publish")
	if got, want := askReviewer.Last().IssueNumber, 77; got != want {
		t.Fatalf("IssueNumber = %d, want %d", got, want)
	}
	if got := askReviewer.Last().PRNumber; got != 0 {
		t.Fatalf("PRNumber = %d, want 0", got)
	}
	if !strings.Contains(commentPublisher.Body(), "回答：\n这是一个关于打包问题的 issue。") {
		t.Fatalf("comment body = %q, want answer block", commentPublisher.Body())
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
	waitForCondition(t, func() bool { return bodyUpdater.Body() == want }, "summarize body update")
	if got := bodyUpdater.Body(); got != want {
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
	waitForCondition(t, func() bool { return bodyUpdater.Body() != "" }, "summarize body update by permission")
	if got := bodyUpdater.Body(); got == "" {
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
	waitForCondition(t, func() bool { return strings.Contains(commentPublisher.Body(), "@bob 你当前没有修改这个 PR 描述的权限") }, "summarize permission fallback")
	if got := bodyUpdater.Body(); got != "" {
		t.Fatalf("body updater should not run, got %q", got)
	}
	if !strings.Contains(commentPublisher.Body(), "@bob 你当前没有修改这个 PR 描述的权限") {
		t.Fatalf("comment body = %q, want permission explanation", commentPublisher.Body())
	}
	if !strings.Contains(commentPublisher.Body(), "new body") {
		t.Fatalf("comment body = %q, want summary body", commentPublisher.Body())
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
	waitForCondition(t, func() bool { return strings.Contains(commentPublisher.Body(), "已经由 alice 总结过") }, "summarize already summarized comment")
	if got := bodyUpdater.Body(); got != "" {
		t.Fatalf("body updater should not run, got %q", got)
	}
	if !strings.Contains(commentPublisher.Body(), "已经由 alice 总结过") {
		t.Fatalf("comment body = %q, want already summarized notice", commentPublisher.Body())
	}
}

func TestHandlerReturnsAcceptedBeforeAskCompletes(t *testing.T) {
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
		Config: config.Config{DefaultProfile: "default", EnabledProfiles: []string{"default"}, CommandReviewEnabled: true, AllowedCommands: []string{"ask"}},
	}, profile: profiles.Definition{Name: "default"}}
	askStarted := make(chan struct{})
	releaseAsk := make(chan struct{})
	published := make(chan struct{}, 1)
	askReviewer := &fakeAskReviewer{
		result: core.AskResult{Answer: "因为需要隔离职责。"},
		execute: func(_ context.Context, req core.ReviewRequest) (core.AskResult, error) {
			close(askStarted)
			<-releaseAsk
			return core.AskResult{Answer: "因为需要隔离职责。"}, nil
		},
	}
	commentPublisher := &fakeCommentPublisher{publishHook: func(string) { published <- struct{}{} }}
	handler := Handler{InstanceKey: "corp-gitea", Loader: loader, AskReviewer: askReviewer, CommentPublisher: commentPublisher}

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("X-Gitea-Event", "issue_comment")
	req.Header.Set("X-Gitea-Signature", signature("secret", body))
	rr := httptest.NewRecorder()
	done := make(chan struct{})

	go func() {
		handler.ServeHTTP(rr, req)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("ServeHTTP did not return before ask completed")
	}
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusAccepted)
	}
	select {
	case <-askStarted:
	case <-time.After(time.Second):
		t.Fatal("ask reviewer did not start")
	}
	close(releaseAsk)
	select {
	case <-published:
	case <-time.After(time.Second):
		t.Fatal("comment was not published after ask completed")
	}
}

func TestHandlerDetachesAskExecutionFromRequestContext(t *testing.T) {
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
		Config: config.Config{DefaultProfile: "default", EnabledProfiles: []string{"default"}, CommandReviewEnabled: true, AllowedCommands: []string{"ask"}},
	}, profile: profiles.Definition{Name: "default"}}
	releaseAsk := make(chan struct{})
	published := make(chan struct{}, 1)
	askReviewer := &fakeAskReviewer{
		execute: func(ctx context.Context, req core.ReviewRequest) (core.AskResult, error) {
			<-releaseAsk
			select {
			case <-ctx.Done():
				return core.AskResult{}, ctx.Err()
			default:
				return core.AskResult{Answer: "因为需要隔离职责。"}, nil
			}
		},
	}
	commentPublisher := &fakeCommentPublisher{publishHook: func(string) { published <- struct{}{} }}
	handler := Handler{InstanceKey: "corp-gitea", Loader: loader, AskReviewer: askReviewer, CommentPublisher: commentPublisher}

	requestCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body))).WithContext(requestCtx)
	req.Header.Set("X-Gitea-Event", "issue_comment")
	req.Header.Set("X-Gitea-Signature", signature("secret", body))
	rr := httptest.NewRecorder()
	done := make(chan struct{})

	go func() {
		handler.ServeHTTP(rr, req)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("ServeHTTP did not return before background execution")
	}
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusAccepted)
	}
	cancel()
	close(releaseAsk)
	select {
	case <-published:
	case <-time.After(time.Second):
		t.Fatal("comment was not published after request context cancellation")
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
	mu      sync.Mutex
	last    core.ReviewRequest
	result  core.AskResult
	execute func(context.Context, core.ReviewRequest) (core.AskResult, error)
}

func (f *fakeAskReviewer) Execute(ctx context.Context, req core.ReviewRequest) (core.AskResult, error) {
	f.mu.Lock()
	f.last = req
	execute := f.execute
	result := f.result
	f.mu.Unlock()
	if execute != nil {
		return execute(ctx, req)
	}
	return result, nil
}

func (f *fakeAskReviewer) Last() core.ReviewRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.last
}

type fakeCommentPublisher struct {
	mu          sync.Mutex
	body        string
	publishHook func(string)
}

func (f *fakeCommentPublisher) PublishComment(_ context.Context, _ config.EffectiveRepositoryConfig, _ core.ReviewRequest, body string) error {
	f.mu.Lock()
	f.body = body
	hook := f.publishHook
	f.mu.Unlock()
	if hook != nil {
		hook(body)
	}
	return nil
}

func (f *fakeCommentPublisher) Body() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.body
}

type fakeSummarizeReviewer struct {
	result core.SummarizeResult
}

func (f *fakeSummarizeReviewer) Execute(_ context.Context, _ core.ReviewRequest) (core.SummarizeResult, error) {
	return f.result, nil
}

type fakeBodyUpdater struct {
	mu   sync.Mutex
	body string
}

func (f *fakeBodyUpdater) UpdatePullRequestBody(_ context.Context, _ config.EffectiveRepositoryConfig, _ core.ReviewRequest, body string) error {
	f.mu.Lock()
	f.body = body
	f.mu.Unlock()
	return nil
}

func (f *fakeBodyUpdater) Body() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.body
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

func waitForCondition(t *testing.T, check func() bool, description string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}
