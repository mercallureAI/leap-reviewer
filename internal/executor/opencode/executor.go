package opencode

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Request struct {
	Provider   string
	Model      string
	Workspace  string
	Prompt     string
	ResultPath string
	ExtraEnv   []string
}

type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

type Executor struct {
	Environment []string
}

func (e Executor) Execute(ctx context.Context, req Request) (Result, error) {
	workspacePath, err := filepath.Abs(req.Workspace)
	if err != nil {
		return Result{}, err
	}
	resultPath, err := filepath.Abs(req.ResultPath)
	if err != nil {
		return Result{}, err
	}
	modelRef := req.Model
	if req.Provider != "" {
		modelRef = req.Provider + "/" + req.Model
	}
	message := req.Prompt + "\n\nWrite the final structured JSON review result to this exact file path: " + resultPath + "\nDo not omit writing the file.\nDo not read files outside the current workspace.\nDo not use additional agents, subagents, or delegated reviews. Complete the review directly in the current workspace.\nIf any tool call is denied or any permission request is rejected, continue the review with the information already available and still write the final structured JSON review result."
	args := []string{
		"opencode",
		"run",
		"--pure",
		"--model", modelRef,
		"--dir", workspacePath,
		message,
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/env", args...)
	cmd.Dir = workspacePath
	cmd.Env = append(os.Environ(), e.Environment...)
	cmd.Env = append(cmd.Env, req.ExtraEnv...)
	cmd.Env = append(cmd.Env, "OPENCODE_RESULT_PATH="+resultPath)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	result := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if err == nil {
		if _, statErr := os.Stat(resultPath); statErr != nil {
			return result, fmt.Errorf("opencode did not write result file %s: %w; stdout=%q stderr=%q", resultPath, statErr, strings.TrimSpace(result.Stdout), strings.TrimSpace(result.Stderr))
		}
		return result, nil
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
		return result, fmt.Errorf("opencode exited with code %d: %w: %s", result.ExitCode, err, strings.TrimSpace(result.Stderr))
	}
	return result, err
}
