package executor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cryolitia/gitea-ai-bot/internal/core"
	inner "github.com/cryolitia/gitea-ai-bot/internal/executor/opencode"
	"github.com/cryolitia/gitea-ai-bot/internal/review"
)

func TestRunnerWritesResultFileInsideWorkspace(t *testing.T) {
	workspaceDir := t.TempDir()
	stub := &stubExecutor{result: core.ReviewResult{ReviewAction: "comment", Summary: "ok"}}
	runner := Runner{Executor: stub}

	_, err := runner.Run(context.Background(), review.ExecuteRequest{
		Provider:  "openai",
		Model:     "gpt-4.1-mini",
		Workspace: workspaceDir,
		Prompt:    "review this pr",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	want := filepath.Join(workspaceDir, ".review-result.json")
	if got := stub.request.ResultPath; got != want {
		t.Fatalf("ResultPath = %q, want %q", got, want)
	}
}

func TestRunnerReturnsStdoutForTextRequests(t *testing.T) {
	workspaceDir := t.TempDir()
	stub := &stubExecutor{stdout: "direct answer\n"}
	runner := Runner{Executor: stub}

	answer, err := runner.RunText(context.Background(), review.ExecuteRequest{
		Provider:  "openai",
		Model:     "gpt-5.4",
		Workspace: workspaceDir,
		Prompt:    "answer the question",
	})
	if err != nil {
		t.Fatalf("RunText() error = %v", err)
	}
	if got, want := answer, "direct answer"; got != want {
		t.Fatalf("answer = %q, want %q", got, want)
	}
	if got := stub.request.ResultPath; got != "" {
		t.Fatalf("ResultPath = %q, want empty", got)
	}
}

func TestRunnerFallsBackToStderrForTextRequests(t *testing.T) {
	workspaceDir := t.TempDir()
	stub := &stubExecutor{stderr: "fallback answer\n"}
	runner := Runner{Executor: stub}

	answer, err := runner.RunText(context.Background(), review.ExecuteRequest{
		Provider:  "openai",
		Model:     "gpt-5.4",
		Workspace: workspaceDir,
		Prompt:    "answer the question",
	})
	if err != nil {
		t.Fatalf("RunText() error = %v", err)
	}
	if got, want := answer, "fallback answer"; got != want {
		t.Fatalf("answer = %q, want %q", got, want)
	}
}

type stubExecutor struct {
	request inner.Request
	result  core.ReviewResult
	stdout  string
	stderr  string
}

func (s *stubExecutor) Execute(_ context.Context, req inner.Request) (inner.Result, error) {
	s.request = req
	if req.ResultPath == "" {
		return inner.Result{Stdout: s.stdout, Stderr: s.stderr}, nil
	}
	return inner.Result{}, writeResult(req.ResultPath, s.result)
}

func writeResult(path string, result core.ReviewResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
