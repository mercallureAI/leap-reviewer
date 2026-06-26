package opencode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteInvokesOpencodeAndWritesResultFile(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	workDir := filepath.Join(root, "work")
	resultPath := filepath.Join(root, "result.json")
	argsPath := filepath.Join(root, "args.txt")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}

	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > \"$OPENCODE_ARGS_PATH\"\n" +
		"printf '{\"review_action\":\"comment\",\"summary\":\"ok\",\"general_comments\":[],\"inline_findings\":[],\"warnings\":[]}' > \"$OPENCODE_RESULT_PATH\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "opencode"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake opencode: %v", err)
	}

	exec := Executor{Environment: []string{"OPENCODE_ARGS_PATH=" + argsPath}}
	result, err := exec.Execute(context.Background(), Request{
		Provider:   "openai",
		Model:      "gpt-4.1-mini",
		Workspace:  workDir,
		Prompt:     "review this pr",
		ResultPath: resultPath,
		ExtraEnv:   []string{"PATH=" + binDir + ":" + os.Getenv("PATH")},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", result.ExitCode)
	}

	argsBytes, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	args := string(argsBytes)
	for _, want := range []string{"run", "--pure", "--model", "openai/gpt-4.1-mini", "--dir", workDir} {
		if !strings.Contains(args, want) {
			t.Fatalf("args %q do not contain %q", args, want)
		}
	}
	for _, want := range []string{"Do not read files outside the current workspace", "Do not use additional agents, subagents, or delegated reviews", "If any tool call is denied or any permission request is rejected", "still write the final structured JSON review result"} {
		if !strings.Contains(args, want) {
			t.Fatalf("args %q do not contain %q", args, want)
		}
	}
	if !strings.Contains(args, resultPath) {
		t.Fatalf("args %q do not reference result path %q", args, resultPath)
	}
	if _, err := os.Stat(resultPath); err != nil {
		t.Fatalf("result file not created: %v", err)
	}
}

func TestExecuteReportsMissingResultFileWithCommandOutput(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	workDir := filepath.Join(root, "work")
	resultPath := filepath.Join(workDir, ".review-result.json")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}

	script := "#!/bin/sh\n" +
		"printf 'stdout marker\\n'\n" +
		"printf 'stderr marker\\n' >&2\n"
	if err := os.WriteFile(filepath.Join(binDir, "opencode"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake opencode: %v", err)
	}

	exec := Executor{}
	_, err := exec.Execute(context.Background(), Request{
		Provider:   "openai",
		Model:      "gpt-4.1-mini",
		Workspace:  workDir,
		Prompt:     "review this pr",
		ResultPath: resultPath,
		ExtraEnv:   []string{"PATH=" + binDir + ":" + os.Getenv("PATH")},
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want missing result file error")
	}
	message := err.Error()
	for _, want := range []string{"did not write result file", "stdout marker", "stderr marker"} {
		if !strings.Contains(message, want) {
			t.Fatalf("error %q does not contain %q", message, want)
		}
	}
}
