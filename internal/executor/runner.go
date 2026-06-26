package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cryolitia/gitea-ai-bot/internal/core"
	inner "github.com/cryolitia/gitea-ai-bot/internal/executor/opencode"
	"github.com/cryolitia/gitea-ai-bot/internal/resultparser"
	"github.com/cryolitia/gitea-ai-bot/internal/review"
)

type Runner struct {
	Executor interface {
		Execute(context.Context, inner.Request) (inner.Result, error)
	}
	TempDir  string
}

func (r Runner) Run(ctx context.Context, req review.ExecuteRequest) (core.ReviewResult, error) {
	resultDir := req.Workspace
	if resultDir == "" {
		resultDir = r.TempDir
	}
	if resultDir == "" {
		resultDir = os.TempDir()
	}
	resultFile := filepath.Join(resultDir, ".review-result.json")
	_, _ = os.Stat(resultFile)
	if err := os.RemoveAll(resultFile); err != nil && !os.IsNotExist(err) {
		return core.ReviewResult{}, err
	}
	_, err := r.Executor.Execute(ctx, inner.Request{
		Provider:   req.Provider,
		Model:      req.Model,
		Workspace:  req.Workspace,
		Prompt:     req.Prompt,
		ResultPath: resultFile,
	})
	if err != nil {
		return core.ReviewResult{}, err
	}
	parsed, err := resultparser.ParseFile(resultFile)
	if err != nil {
		return core.ReviewResult{}, fmt.Errorf("parse opencode result: %w", err)
	}
	return parsed, nil
}
