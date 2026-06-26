package review

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cryolitia/gitea-ai-bot/internal/config"
	"github.com/cryolitia/gitea-ai-bot/internal/core"
	"github.com/cryolitia/gitea-ai-bot/internal/profiles"
)

func TestServiceExecuteBuildsPromptAndParsesExecutorResult(t *testing.T) {
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
					DefaultProfile:        "default",
					EnabledProfiles:       []string{"default"},
					InlineCommentLimit:    5,
					PublishInlineComments: true,
				},
				Model: config.ModelDefinition{Provider: "openai", Model: "gpt-4.1-mini"},
			},
			profile: profiles.Definition{Name: "default", Prompt: "check for bugs", InlineEnabled: true, InlineLimit: 5},
		},
		Platform:  fakePlatform{context: PullRequestContext{Title: "Fix bug", Body: "desc", HeadSHA: "abc123", HeadRef: "feature", FilesChanged: []ChangedFile{{Path: "main.go", Patch: "@@ -1 +1 @@"}}}},
		Workspace: &fakeWorkspace{path: "/tmp/workspace"},
		Executor:  &fakeExecutor{result: core.ReviewResult{ReviewAction: "comment", Summary: "Looks okay"}},
		Progress: func(message string) { progress = append(progress, message) },
	}

	result, err := svc.Execute(context.Background(), core.ReviewRequest{InstanceKey: "corp-gitea", Owner: "backend-team", Repo: "payment-service", ProfileName: "default", PRNumber: 42})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := result.ReviewAction, "comment"; got != want {
		t.Fatalf("ReviewAction = %q, want %q", got, want)
	}
	exec := svc.Executor.(*fakeExecutor)
	workspace := svc.Workspace.(*fakeWorkspace)
	if got, want := exec.last.Provider, "openai"; got != want {
		t.Fatalf("provider = %q, want %q", got, want)
	}
	if got, want := workspace.last.HeadSHA, "abc123"; got != want {
		t.Fatalf("workspace head sha = %q, want %q", got, want)
	}
	if got, want := workspace.last.HeadRef, "feature"; got != want {
		t.Fatalf("workspace head ref = %q, want %q", got, want)
	}
	if got := exec.last.Prompt; got == "" || !containsAll(got, []string{"You are the system-level review orchestrator", "check for bugs", "Fix bug", "main.go", "Do not read files outside the current workspace", "Do not use additional agents, subagents, or delegated reviews", "If any tool call is denied", "still write the final structured JSON review result"}) {
		t.Fatalf("prompt missing expected content: %q", got)
	}
	if got := exec.last.Prompt; !strings.Contains(got, "You are the system-level review orchestrator") || strings.Index(got, "You are the system-level review orchestrator") > strings.Index(got, "check for bugs") {
		t.Fatalf("system prompt should appear before profile prompt: %q", got)
	}
	if !containsSequence(progress, []string{"loading config and profile", "fetching pull request context", "preparing workspace", "running opencode review", "review completed"}) {
		t.Fatalf("progress messages = %#v, want ordered sequence", progress)
	}
}

func TestSystemPromptIsEmbedded(t *testing.T) {
	if got := embeddedSystemPrompt(); !strings.Contains(got, "You are the system-level review orchestrator") {
		t.Fatalf("embeddedSystemPrompt() = %q, want embedded markdown content", got)
	}
}

func TestSystemPromptSourcePathIsExplicit(t *testing.T) {
	if got, want := systemPromptSourcePath(), filepath.Join("internal", "review", "prompts", "system.md"); got != want {
		t.Fatalf("systemPromptSourcePath() = %q, want %q", got, want)
	}
}

type fakeLoader struct {
	cfg     config.EffectiveRepositoryConfig
	profile profiles.Definition
}

func (f fakeLoader) Load(instanceKey, owner, repo, profile string) (config.EffectiveRepositoryConfig, profiles.Definition, error) {
	return f.cfg, f.profile, nil
}

type fakePlatform struct{ context PullRequestContext }

func (f fakePlatform) GetPullRequestContext(context.Context, config.EffectiveRepositoryConfig, core.ReviewRequest) (PullRequestContext, error) {
	return f.context, nil
}

type fakeWorkspace struct {
	path string
	last PrepareRequest
}

func (f *fakeWorkspace) Prepare(_ context.Context, req PrepareRequest) (PreparedWorkspace, error) {
	f.last = req
	return PreparedWorkspace{Path: f.path}, nil
}

type fakeExecutor struct {
	last   ExecuteRequest
	result core.ReviewResult
}

func (f *fakeExecutor) Run(_ context.Context, req ExecuteRequest) (core.ReviewResult, error) {
	f.last = req
	return f.result, nil
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
