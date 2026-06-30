package review

import (
	"context"
	"fmt"
	"strings"

	"github.com/cryolitia/gitea-ai-bot/internal/config"
	"github.com/cryolitia/gitea-ai-bot/internal/core"
	"github.com/cryolitia/gitea-ai-bot/internal/profiles"
)

type ChangedFile struct {
	Path  string
	Patch string
}

type PullRequestContext struct {
	Title        string
	Body         string
	CloneURL     string
	HeadSHA      string
	HeadRef      string
	FilesChanged []ChangedFile
}

type PrepareRequest struct {
	InstanceKey string
	Owner       string
	Repo        string
	HeadSHA     string
	HeadRef     string
	RepoURL     string
}

type PreparedWorkspace struct {
	Path string
}

type ExecuteRequest struct {
	Provider  string
	Model     string
	Workspace string
	Prompt    string
	TimeoutSeconds int
}

type ConfigProfileLoader interface {
	Load(instanceKey, owner, repo, profile string) (config.EffectiveRepositoryConfig, profiles.Definition, error)
}

type Platform interface {
	GetPullRequestContext(context.Context, config.EffectiveRepositoryConfig, core.ReviewRequest) (PullRequestContext, error)
}

type WorkspacePreparer interface {
	Prepare(context.Context, PrepareRequest) (PreparedWorkspace, error)
}

type Runner interface {
	Run(context.Context, ExecuteRequest) (core.ReviewResult, error)
}

type Service struct {
	Loader    ConfigProfileLoader
	Platform  Platform
	Workspace WorkspacePreparer
	Executor  Runner
	Progress  func(string)
}

func (s Service) Execute(ctx context.Context, req core.ReviewRequest) (core.ReviewResult, error) {
	s.progress("loading config and profile")
	effective, profile, err := s.Loader.Load(req.InstanceKey, req.Owner, req.Repo, req.ProfileName)
	if err != nil {
		return core.ReviewResult{}, err
	}

	s.progress("fetching pull request context")
	prContext, err := s.Platform.GetPullRequestContext(ctx, effective, req)
	if err != nil {
		return core.ReviewResult{}, err
	}

	headSHA := req.HeadSHA
	if headSHA == "" {
		headSHA = prContext.HeadSHA
	}

	s.progress("preparing workspace")
	workspace, err := s.Workspace.Prepare(ctx, PrepareRequest{
		InstanceKey: req.InstanceKey,
		Owner:       req.Owner,
		Repo:        req.Repo,
		HeadSHA:     headSHA,
		HeadRef:     prContext.HeadRef,
		RepoURL:     prContext.CloneURL,
	})
	if err != nil {
		return core.ReviewResult{}, err
	}

	prompt := buildPrompt(profile, prContext)
	s.progress("running opencode review")
	result, err := s.Executor.Run(ctx, ExecuteRequest{
		Provider:  effective.ReviewModel.Provider,
		Model:     effective.ReviewModel.Model,
		Workspace: workspace.Path,
		Prompt:    prompt,
		TimeoutSeconds: effective.Config.OpencodeTimeoutSeconds,
	})
	if err != nil {
		return core.ReviewResult{}, err
	}

	s.progress("review completed")
	return result, nil
}

func (s Service) progress(message string) {
	if s.Progress != nil {
		s.Progress(message)
	}
}

func buildPrompt(profile profiles.Definition, prContext PullRequestContext) string {
	var b strings.Builder
	b.WriteString(embeddedSystemPrompt())
	b.WriteString("\n\n")
	b.WriteString(profile.ReviewPrompt)
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
	return b.String()
}
