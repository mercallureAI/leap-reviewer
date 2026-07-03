package opencode

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type Request struct {
	Provider   string
	Model      string
	Workspace  string
	Prompt     string
	ResultPath string
	TimeoutSeconds int
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
	if req.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutSeconds)*time.Second)
		defer cancel()
	}
	workspacePath, err := filepath.Abs(req.Workspace)
	if err != nil {
		return Result{}, err
	}
	resultPath := ""
	if req.ResultPath != "" {
		resultPath, err = filepath.Abs(req.ResultPath)
		if err != nil {
			return Result{}, err
		}
	}
	modelRef := req.Model
	if req.Provider != "" {
		modelRef = req.Provider + "/" + req.Model
	}
	message := req.Prompt
	if resultPath != "" {
		message += "\n\nWrite the final structured JSON review result to this exact file path: " + resultPath + "\nDo not omit writing the file.\nEven if some checks cannot be completed, still write the final structured JSON review result."
	}
	message += "\nDo not read files outside the current workspace.\nDo not use additional agents, subagents, or delegated reviews. Complete the review directly in the current workspace.\nIf any tool call is denied or any permission request is rejected, continue the work with the information already available."
	args := []string{
		"opencode",
		"run",
		"--pure",
		"--model", modelRef,
		"--dir", workspacePath,
		message,
	}
	cmd := exec.Command("/usr/bin/env", args...)
	cmd.Dir = workspacePath
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = append(os.Environ(), e.Environment...)
	cmd.Env = append(cmd.Env, req.ExtraEnv...)
	if resultPath != "" {
		cmd.Env = append(cmd.Env, "OPENCODE_RESULT_PATH="+resultPath)
	}

	stdoutFile, err := os.CreateTemp("", "opencode-stdout-*")
	if err != nil {
		return Result{}, err
	}
	defer func() {
		_ = stdoutFile.Close()
		_ = os.Remove(stdoutFile.Name())
	}()
	stderrFile, err := os.CreateTemp("", "opencode-stderr-*")
	if err != nil {
		return Result{}, err
	}
	defer func() {
		_ = stderrFile.Close()
		_ = os.Remove(stderrFile.Name())
	}()
	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile

	slog.InfoContext(ctx, "starting opencode",
		slog.String("workspace", workspacePath),
		slog.String("model", modelRef),
		slog.Int("timeout_seconds", req.TimeoutSeconds),
	)
	if err := cmd.Start(); err != nil {
		slog.ErrorContext(ctx, "failed to start opencode", slog.String("error", err.Error()))
		return Result{}, err
	}
	slog.InfoContext(ctx, "opencode started", slog.Int("pid", cmd.Process.Pid))
	if ctx.Done() != nil {
		go func() {
			<-ctx.Done()
			if cmd.Process != nil {
				err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				slog.WarnContext(ctx, "opencode timeout reached",
					slog.Int("pid", cmd.Process.Pid),
					slog.String("context_error", ctx.Err().Error()),
					slog.String("kill_error", errorString(err)),
				)
			}
		}()
	}
	slog.InfoContext(ctx, "waiting for opencode", slog.Int("pid", cmd.Process.Pid))
	err = cmd.Wait()
	slog.InfoContext(ctx, "opencode wait returned",
		slog.Int("pid", cmd.Process.Pid),
		slog.String("wait_error", errorString(err)),
		slog.String("context_error", errorString(ctx.Err())),
	)
	result := Result{}
	if stdoutBytes, readErr := os.ReadFile(stdoutFile.Name()); readErr == nil {
		result.Stdout = string(stdoutBytes)
	}
	if stderrBytes, readErr := os.ReadFile(stderrFile.Name()); readErr == nil {
		result.Stderr = string(stderrBytes)
	}
	if ctx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("opencode timed out after %d seconds: %w", req.TimeoutSeconds, ctx.Err())
	}
	if err == nil {
		if resultPath == "" {
			return result, nil
		}
		if _, statErr := os.Stat(resultPath); statErr != nil {
			return result, fmt.Errorf("opencode did not write result file %s: %w; stdout=%q stderr=%q", resultPath, statErr, strings.TrimSpace(result.Stdout), strings.TrimSpace(result.Stderr))
		}
		return result, nil
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
		slog.ErrorContext(ctx, "opencode exited with error",
			slog.Int("pid", cmd.Process.Pid),
			slog.Int("exit_code", result.ExitCode),
			slog.String("stdout", summarizeLogOutput(result.Stdout)),
			slog.String("stderr", summarizeLogOutput(result.Stderr)),
		)
		return result, fmt.Errorf("opencode exited with code %d: %w: %s", result.ExitCode, err, strings.TrimSpace(result.Stderr))
	}
	return result, err
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func summarizeLogOutput(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= 500 {
		return trimmed
	}
	return trimmed[:500]
}
