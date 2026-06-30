package summarize

import (
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cryolitia/gitea-ai-bot/internal/config"
	"github.com/cryolitia/gitea-ai-bot/internal/core"
	"github.com/cryolitia/gitea-ai-bot/internal/profiles"
	"github.com/cryolitia/gitea-ai-bot/internal/review"
)

type ConfigProfileLoader interface {
	Load(instanceKey, owner, repo, profile string) (config.EffectiveRepositoryConfig, profiles.Definition, error)
}

type Platform interface {
	GetPullRequestContext(context.Context, config.EffectiveRepositoryConfig, core.ReviewRequest) (review.PullRequestContext, error)
}

type WorkspacePreparer interface {
	Prepare(context.Context, review.PrepareRequest) (review.PreparedWorkspace, error)
}

type Runner interface {
	RunText(context.Context, review.ExecuteRequest) (string, error)
}

type Service struct {
	Loader    ConfigProfileLoader
	Platform  Platform
	Workspace WorkspacePreparer
	Executor  Runner
	Progress  func(string)
}

func (s Service) Execute(ctx context.Context, req core.ReviewRequest) (core.SummarizeResult, error) {
	s.progress("loading config and profile")
	effective, profile, err := s.Loader.Load(req.InstanceKey, req.Owner, req.Repo, req.ProfileName)
	if err != nil {
		return core.SummarizeResult{}, err
	}

	s.progress("fetching pull request context")
	prContext, err := s.Platform.GetPullRequestContext(ctx, effective, req)
	if err != nil {
		return core.SummarizeResult{}, err
	}
	if source, ok := detectSummarySource(prContext.Body); ok {
		s.progress("summarize completed")
		return core.SummarizeResult{AlreadySummarized: true, Source: source, OriginalBody: prContext.Body}, nil
	}

	headSHA := req.HeadSHA
	if headSHA == "" {
		headSHA = prContext.HeadSHA
	}

	s.progress("preparing workspace")
	workspace, err := s.Workspace.Prepare(ctx, review.PrepareRequest{
		InstanceKey: req.InstanceKey,
		Owner:       req.Owner,
		Repo:        req.Repo,
		HeadSHA:     headSHA,
		HeadRef:     prContext.HeadRef,
		RepoURL:     prContext.CloneURL,
	})
	if err != nil {
		return core.SummarizeResult{}, err
	}

	prompt := buildPrompt(profile, prContext)
	s.progress("running opencode summarize")
	body, err := s.Executor.RunText(ctx, review.ExecuteRequest{
		Provider:       effective.AskModel.Provider,
		Model:          effective.AskModel.Model,
		Workspace:      workspace.Path,
		Prompt:         prompt,
		TimeoutSeconds: effective.Config.OpencodeTimeoutSeconds,
	})
	if err != nil {
		return core.SummarizeResult{}, err
	}

	s.progress("summarize completed")
	return core.SummarizeResult{Body: normalizeBody(body), OriginalBody: prContext.Body}, nil
}

func (s Service) progress(message string) {
	if s.Progress != nil {
		s.Progress(message)
	}
}

func buildPrompt(profile profiles.Definition, prContext review.PullRequestContext) string {
	var b strings.Builder
	b.WriteString(embeddedSystemPrompt())
	b.WriteString("\n\n")
	b.WriteString(profile.SummarizePrompt)
	b.WriteString("\n\n")
	b.WriteString(embeddedInstructionPrompt())
	b.WriteString("\n")
	b.WriteString("PR Title: ")
	b.WriteString(prContext.Title)
	b.WriteString("\n")
	if prContext.Body != "" {
		b.WriteString("PR Body: ")
		b.WriteString(prContext.Body)
		b.WriteString("\n")
	}
	for _, file := range prContext.FilesChanged {
		b.WriteString(fmt.Sprintf("File: %s\n%s\n", file.Path, file.Patch))
	}
	b.WriteString("Question: rewrite the pull request body only.\n")
	return b.String()
}

func normalizeBody(body string) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return ""
	}
	lines := strings.Split(trimmed, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			return strings.Join(lines[i:], "\n")
		}
	}
	return trimmed
}

var summaryMarkerPattern = regexp.MustCompile(`<!--\s*review-bot:summarized source=([^\s>]+)\s*-->`)

func detectSummarySource(body string) (string, bool) {
	matches := summaryMarkerPattern.FindStringSubmatch(body)
	if len(matches) != 2 {
		return "", false
	}
	return matches[1], true
}

func BuildPublishedBody(originalBody, source, summary string) string {
	trimmedOriginal := strings.TrimSpace(originalBody)
	trimmedSummary := strings.TrimSpace(summary)
	marker := fmt.Sprintf("<!-- review-bot:summarized source=%s -->", source)
	if trimmedOriginal == "" {
		return strings.TrimSpace("---\n\n" + marker + "\n\n" + trimmedSummary)
	}
	return trimmedOriginal + "\n\n---\n\n" + marker + "\n\n" + trimmedSummary
}

//go:embed prompts/system.md
var systemPrompt string

//go:embed prompts/instructions.md
var instructionPrompt string

func embeddedSystemPrompt() string { return strings.TrimSpace(systemPrompt) }
func embeddedInstructionPrompt() string { return strings.TrimSpace(instructionPrompt) }
func systemPromptSourcePath() string { return filepath.Join("internal", "summarize", "prompts", "system.md") }
func instructionPromptSourcePath() string {
	return filepath.Join("internal", "summarize", "prompts", "instructions.md")
}
