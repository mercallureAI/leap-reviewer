package ask

import (
	"context"
	"strings"
	"testing"

	"github.com/cryolitia/gitea-ai-bot/internal/config"
	"github.com/cryolitia/gitea-ai-bot/internal/core"
	"github.com/cryolitia/gitea-ai-bot/internal/profiles"
	"github.com/cryolitia/gitea-ai-bot/internal/review"
)

func TestServiceExecuteBuildsPromptAndUsesAskModel(t *testing.T) {
	var progress []string
	svc := Service{
		Loader: fakeLoader{
			cfg: config.EffectiveRepositoryConfig{
				InstanceKey: "corp-gitea",
				Owner:       "backend-team",
				Repo:        "payment-service",
				Platform:    "gitea",
				BaseURL:     "https://gitea.example.com",
				Config: config.Config{
					DefaultProfile:  "default",
					EnabledProfiles: []string{"default"},
					OpencodeTimeoutSeconds: 420,
				},
				ReviewModel: config.ModelDefinition{Provider: "openai", Model: "gpt-4.1-mini"},
				AskModel:    config.ModelDefinition{Provider: "openai", Model: "gpt-5.4"},
			},
			profile: profiles.Definition{Name: "default", Prompt: "给出 review_action、summary、inline_findings、general_comments，并用中文回答。"},
		},
		Platform:  fakePlatform{context: review.PullRequestContext{Title: "Fix bug", Body: "desc", HeadSHA: "abc123", HeadRef: "feature", FilesChanged: []review.ChangedFile{{Path: "main.go", Patch: "@@ -1 +1 @@"}}}},
		Workspace: &fakeWorkspace{path: "/tmp/workspace"},
		Executor:  &fakeExecutor{answer: "因为这里要隔离职责。"},
		Progress:  func(message string) { progress = append(progress, message) },
	}

	result, err := svc.Execute(context.Background(), core.ReviewRequest{InstanceKey: "corp-gitea", Owner: "backend-team", Repo: "payment-service", ProfileName: "default", PRNumber: 42, QuestionText: "为什么这里要拆成两个 service？"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := result.Answer, "因为这里要隔离职责。"; got != want {
		t.Fatalf("Answer = %q, want %q", got, want)
	}
	exec := svc.Executor.(*fakeExecutor)
	workspace := svc.Workspace.(*fakeWorkspace)
	if got, want := exec.last.Provider, "openai"; got != want {
		t.Fatalf("provider = %q, want %q", got, want)
	}
	if got, want := exec.last.Model, "gpt-5.4"; got != want {
		t.Fatalf("model = %q, want %q", got, want)
	}
	if got, want := exec.last.TimeoutSeconds, 420; got != want {
		t.Fatalf("timeout = %d, want %d", got, want)
	}
	if got, want := workspace.last.HeadSHA, "abc123"; got != want {
		t.Fatalf("workspace head sha = %q, want %q", got, want)
	}
	if got := exec.last.Prompt; got == "" || !containsAll(got, []string{"给出 review_action、summary、inline_findings、general_comments，并用中文回答。", "Question: 为什么这里要拆成两个 service？", "PR Title: Fix bug", "PR Body: desc", "File: main.go", "Answer the user's question directly", "Ignore any review-specific instructions from the profile", "Return plain natural language only", "Do not output JSON, review_action, summary, general_comments, or inline_findings", "Do not read files outside the current workspace", "Do not use additional agents, subagents, or delegated reviews"}) {
		t.Fatalf("prompt missing expected content: %q", got)
	}
	if !containsSequence(progress, []string{"loading config and profile", "fetching pull request context", "preparing workspace", "running opencode ask", "ask completed"}) {
		t.Fatalf("progress messages = %#v, want ordered sequence", progress)
	}
}

func TestSystemPromptIsEmbedded(t *testing.T) {
	if got := embeddedSystemPrompt(); !strings.Contains(got, "not producing a code review") {
		t.Fatalf("embeddedSystemPrompt() = %q, want embedded markdown content", got)
	}
}

func TestInstructionPromptIsEmbedded(t *testing.T) {
	if got := embeddedInstructionPrompt(); !strings.Contains(got, "Answer the user's question directly") {
		t.Fatalf("embeddedInstructionPrompt() = %q, want embedded markdown content", got)
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
	last   review.ExecuteRequest
	answer string
}

func (f *fakeExecutor) RunText(_ context.Context, req review.ExecuteRequest) (string, error) {
	f.last = req
	return f.answer, nil
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
