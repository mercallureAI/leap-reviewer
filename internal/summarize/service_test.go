package summarize

import (
	"context"
	"strings"
	"testing"

	"github.com/cryolitia/gitea-ai-bot/internal/config"
	"github.com/cryolitia/gitea-ai-bot/internal/core"
	"github.com/cryolitia/gitea-ai-bot/internal/profiles"
	"github.com/cryolitia/gitea-ai-bot/internal/review"
)

func TestServiceExecuteBuildsPromptAndUsesSummarizePrompt(t *testing.T) {
	var progress []string
	svc := Service{
		Loader: fakeLoader{
			cfg: config.EffectiveRepositoryConfig{
				InstanceKey: "corp-gitea",
				Owner:       "backend-team",
				Repo:        "payment-service",
				Platform:    "gitea",
				BaseURL:     "https://gitea.example.com",
				Config: config.Config{DefaultProfile: "default", EnabledProfiles: []string{"default"}, OpencodeTimeoutSeconds: 420},
				AskModel:    config.ModelDefinition{Provider: "openai", Model: "gpt-5.4"},
			},
			profile: profiles.Definition{Name: "default", SummarizePrompt: "rewrite the PR body in Chinese"},
		},
		Platform:  fakePlatform{context: review.PullRequestContext{Title: "Fix bug", Body: "desc", HeadSHA: "abc123", HeadRef: "feature", FilesChanged: []review.ChangedFile{{Path: "main.go", Patch: "@@ -1 +1 @@"}}}},
		Workspace: &fakeWorkspace{path: "/tmp/workspace"},
		Executor:  &fakeExecutor{body: "新的 PR 描述"},
		Progress:  func(message string) { progress = append(progress, message) },
	}

	result, err := svc.Execute(context.Background(), core.ReviewRequest{InstanceKey: "corp-gitea", Owner: "backend-team", Repo: "payment-service", ProfileName: "default", PRNumber: 42})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := result.Body, "新的 PR 描述"; got != want {
		t.Fatalf("Body = %q, want %q", got, want)
	}
	exec := svc.Executor.(*fakeExecutor)
	if got, want := exec.last.Model, "gpt-5.4"; got != want {
		t.Fatalf("model = %q, want %q", got, want)
	}
	if got, want := exec.last.TimeoutSeconds, 420; got != want {
		t.Fatalf("timeout = %d, want %d", got, want)
	}
	if got := exec.last.Prompt; got == "" || !containsAll(got, []string{"rewrite the PR body in Chinese", "PR Title: Fix bug", "PR Body: desc", "File: main.go", "Question:", "Do not reveal your step-by-step reasoning", "Do not describe your process", "Start directly with the final PR body", "The very first line of your response must be the beginning of the PR body", "Do not write any text before the first heading or paragraph of the final body"}) {
		t.Fatalf("prompt missing expected content: %q", got)
	}
	if !containsSequence(progress, []string{"loading config and profile", "fetching pull request context", "preparing workspace", "running opencode summarize", "summarize completed"}) {
		t.Fatalf("progress messages = %#v, want ordered sequence", progress)
	}
}

func TestNormalizeBodyStripsReasoningPreamble(t *testing.T) {
	input := "先查看当前 PR 涉及的变更，确认更新内容和影响范围。\n继续确认当前分支相对基线的提交内容。\n## 概要\n\n这里是正文。"
	if got, want := normalizeBody(input), "## 概要\n\n这里是正文。"; got != want {
		t.Fatalf("normalizeBody() = %q, want %q", got, want)
	}
}

func TestExecuteSkipsWhenBodyAlreadySummarized(t *testing.T) {
	svc := Service{
		Loader: fakeLoader{
			cfg: config.EffectiveRepositoryConfig{
				InstanceKey: "corp-gitea",
				Owner:       "backend-team",
				Repo:        "payment-service",
				Platform:    "gitea",
				BaseURL:     "https://gitea.example.com",
				Config:      config.Config{DefaultProfile: "default", EnabledProfiles: []string{"default"}, OpencodeTimeoutSeconds: 420},
				AskModel:    config.ModelDefinition{Provider: "openai", Model: "gpt-5.4"},
			},
			profile: profiles.Definition{Name: "default", SummarizePrompt: "rewrite the PR body in Chinese"},
		},
		Platform:  fakePlatform{context: review.PullRequestContext{Title: "Fix bug", Body: "existing body\n\n---\n\n<!-- review-bot:summarized source=alice -->\n\nold summary", HeadSHA: "abc123", HeadRef: "feature"}},
		Workspace: &fakeWorkspace{path: "/tmp/workspace"},
		Executor:  &fakeExecutor{body: "should not be used"},
	}

	result, err := svc.Execute(context.Background(), core.ReviewRequest{InstanceKey: "corp-gitea", Owner: "backend-team", Repo: "payment-service", ProfileName: "default", PRNumber: 42})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !result.AlreadySummarized {
		t.Fatal("AlreadySummarized = false, want true")
	}
	if got, want := result.Source, "alice"; got != want {
		t.Fatalf("Source = %q, want %q", got, want)
	}
	if got := svc.Executor.(*fakeExecutor).last.Prompt; got != "" {
		t.Fatalf("executor prompt = %q, want empty because summarize should skip", got)
	}
}

func TestBuildPublishedBodyAppendsMarkerAndSummary(t *testing.T) {
	got := BuildPublishedBody("original body", "cli", "## 概要\n\nsummary")
	want := "original body\n\n---\n\n<!-- review-bot:summarized source=cli -->\n\n## 概要\n\nsummary"
	if got != want {
		t.Fatalf("buildPublishedBody() = %q, want %q", got, want)
	}
}

func TestDetectSummarySource(t *testing.T) {
	body := "original\n\n---\n\n<!-- review-bot:summarized source=alice -->\n\nsummary"
	source, ok := detectSummarySource(body)
	if !ok {
		t.Fatal("detectSummarySource() ok = false, want true")
	}
	if source != "alice" {
		t.Fatalf("source = %q, want %q", source, "alice")
	}
}

type fakeLoader struct {
	cfg     config.EffectiveRepositoryConfig
	profile profiles.Definition
}

func (f fakeLoader) Load(instanceKey, owner, repo, profile string) (config.EffectiveRepositoryConfig, profiles.Definition, error) {
	return f.cfg, f.profile, nil
}

type fakePlatform struct{ context review.PullRequestContext }

func (f fakePlatform) GetPullRequestContext(context.Context, config.EffectiveRepositoryConfig, core.ReviewRequest) (review.PullRequestContext, error) {
	return f.context, nil
}

type fakeWorkspace struct {
	path string
	last review.PrepareRequest
}

func (f *fakeWorkspace) Prepare(_ context.Context, req review.PrepareRequest) (review.PreparedWorkspace, error) {
	f.last = req
	return review.PreparedWorkspace{Path: f.path}, nil
}

type fakeExecutor struct {
	last review.ExecuteRequest
	body string
}

func (f *fakeExecutor) RunText(_ context.Context, req review.ExecuteRequest) (string, error) {
	f.last = req
	return f.body, nil
}

func containsAll(haystack string, needles []string) bool {
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			return false
		}
	}
	return true
}

func containsSequence(values, want []string) bool {
	if len(want) == 0 {
		return true
	}
	index := 0
	for _, value := range values {
		if value == want[index] {
			index++
			if index == len(want) {
				return true
			}
		}
	}
	return false
}
