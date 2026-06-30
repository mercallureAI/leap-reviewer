package ask

import (
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
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
	GetAskContext(context.Context, config.EffectiveRepositoryConfig, core.ReviewRequest) (review.PullRequestContext, error)
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

func (s Service) Execute(ctx context.Context, req core.ReviewRequest) (core.AskResult, error) {
	s.progress("loading config and profile")
	effective, profile, err := s.Loader.Load(req.InstanceKey, req.Owner, req.Repo, req.ProfileName)
	if err != nil {
		return core.AskResult{}, err
	}

	s.progress("fetching ask context")
	prContext, err := s.Platform.GetAskContext(ctx, effective, req)
	if err != nil {
		return core.AskResult{}, err
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
		return core.AskResult{}, err
	}

	prompt := buildPrompt(profile, prContext, req.QuestionText)
	s.progress("running opencode ask")
	answer, err := s.Executor.RunText(ctx, review.ExecuteRequest{
		Provider:  effective.AskModel.Provider,
		Model:     effective.AskModel.Model,
		Workspace: workspace.Path,
		Prompt:    prompt,
		TimeoutSeconds: effective.Config.OpencodeTimeoutSeconds,
	})
	if err != nil {
		return core.AskResult{}, err
	}

	s.progress("ask completed")
	return core.AskResult{Answer: answer}, nil
}

func (s Service) progress(message string) {
	if s.Progress != nil {
		s.Progress(message)
	}
}

func buildPrompt(profile profiles.Definition, prContext review.PullRequestContext, question string) string {
	var b strings.Builder
	b.WriteString(embeddedSystemPrompt())
	b.WriteString("\n\n")
	b.WriteString(profile.AskPrompt)
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
	b.WriteString("Question: ")
	b.WriteString(question)
	b.WriteString("\n")
	return b.String()
}

//go:embed prompts/system.md
var systemPrompt string

//go:embed prompts/instructions.md
var instructionPrompt string

func embeddedSystemPrompt() string {
	return strings.TrimSpace(systemPrompt)
}

func embeddedInstructionPrompt() string {
	return strings.TrimSpace(instructionPrompt)
}

func systemPromptSourcePath() string {
	return filepath.Join("internal", "ask", "prompts", "system.md")
}

func instructionPromptSourcePath() string {
	return filepath.Join("internal", "ask", "prompts", "instructions.md")
}
