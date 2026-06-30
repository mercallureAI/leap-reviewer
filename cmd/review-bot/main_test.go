package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cryolitia/gitea-ai-bot/internal/config"
	"github.com/cryolitia/gitea-ai-bot/internal/core"
)

func TestParseCLIRecognizesDaemonCommand(t *testing.T) {
	cli, command, err := parseCLI([]string{"daemon", "--instance", "corp-gitea"})
	if err != nil {
		t.Fatalf("parseCLI() error = %v", err)
	}
	if got, want := command, "daemon"; got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}
	if got, want := cli.Daemon.Instance, "corp-gitea"; got != want {
		t.Fatalf("instance = %q, want %q", got, want)
	}
}

func TestParseCLIRecognizesAskCommand(t *testing.T) {
	cli, command, err := parseCLI([]string{"ask", "--platform", "gitea", "--owner", "team", "--repo", "repo", "--pr", "42", "--provider", "openai", "--model", "gpt-5.4", "--question", "why split this", "--timeout-seconds", "90"})
	if err != nil {
		t.Fatalf("parseCLI() error = %v", err)
	}
	if got, want := command, "ask"; got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}
	if got, want := cli.Ask.Question, "why split this"; got != want {
		t.Fatalf("question = %q, want %q", got, want)
	}
	if got, want := cli.Ask.TimeoutSeconds, 90; got != want {
		t.Fatalf("timeout = %d, want %d", got, want)
	}
}

func TestParseCLIRecognizesAskIssueCommand(t *testing.T) {
	cli, command, err := parseCLI([]string{"ask", "--platform", "gitea", "--owner", "team", "--repo", "repo", "--issue", "7", "--provider", "openai", "--model", "gpt-5.4", "--question", "what is this issue about"})
	if err != nil {
		t.Fatalf("parseCLI() error = %v", err)
	}
	if got, want := command, "ask"; got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}
	if got, want := cli.Ask.Issue, 7; got != want {
		t.Fatalf("issue = %d, want %d", got, want)
	}
}

func TestParseCLIRecognizesSummarizeCommand(t *testing.T) {
	cli, command, err := parseCLI([]string{"summarize", "--platform", "gitea", "--owner", "team", "--repo", "repo", "--pr", "42", "--provider", "openai", "--model", "gpt-5.4", "--timeout-seconds", "90"})
	if err != nil {
		t.Fatalf("parseCLI() error = %v", err)
	}
	if got, want := command, "summarize"; got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}
	if got, want := cli.Summarize.TimeoutSeconds, 90; got != want {
		t.Fatalf("timeout = %d, want %d", got, want)
	}
}

func TestParseCLIMissingAskQuestionFails(t *testing.T) {
	_, _, err := parseCLI([]string{"ask", "--platform", "gitea", "--owner", "team", "--repo", "repo", "--pr", "42", "--provider", "openai", "--model", "gpt-5.4"})
	if err == nil {
		t.Fatal("parseCLI() error = nil, want missing question error")
	}
}

func TestBuildAskOneShotLoaderUsesIssueNumber(t *testing.T) {
	root := t.TempDir()
	profilesDir := filepath.Join(root, "profiles", "default")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatalf("mkdir profiles: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "profile.yaml"), []byte("target: pull_request\nlanguage: zh-CN\ninline_enabled: true\ninline_limit: 10\n"), 0o644); err != nil {
		t.Fatalf("write profile.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "prompt.md"), []byte("shared prompt"), 0o644); err != nil {
		t.Fatalf("write prompt.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "ask.md"), []byte("ask prompt"), 0o644); err != nil {
		t.Fatalf("write ask.md: %v", err)
	}

	_, req, err := buildAskOneShotLoader(askCommand{
		Config:         root,
		ProfilesDir:    filepath.Join(root, "profiles"),
		Platform:       "gitea",
		Owner:          "team",
		Repo:           "repo",
		Issue:          7,
		Provider:       "openai",
		Model:          "gpt-5.4",
		Question:       "what is this issue about",
		TimeoutSeconds: 90,
	})
	if err != nil {
		t.Fatalf("buildAskOneShotLoader() error = %v", err)
	}
	if got, want := req.IssueNumber, 7; got != want {
		t.Fatalf("IssueNumber = %d, want %d", got, want)
	}
	if got := req.PRNumber; got != 0 {
		t.Fatalf("PRNumber = %d, want 0", got)
	}
	_ = config.EffectiveRepositoryConfig{}
}

func TestParseRepoSpec(t *testing.T) {
	tests := []struct {
		name     string
		spec     string
		platform string
		owner    string
		repo     string
		ok       bool
	}{
		{name: "github repo", spec: "github:nixos/nixpkgs", platform: "github", owner: "nixos", repo: "nixpkgs", ok: true},
		{name: "gitea repo", spec: "gitea:team/service", platform: "gitea", owner: "team", repo: "service", ok: true},
		{name: "missing platform", spec: "nixos/nixpkgs", ok: false},
		{name: "missing repo", spec: "github:nixos", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			platform, owner, repo, ok := parseRepoSpec(tt.spec)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if !ok {
				return
			}
			if platform != tt.platform || owner != tt.owner || repo != tt.repo {
				t.Fatalf("got %q %q %q, want %q %q %q", platform, owner, repo, tt.platform, tt.owner, tt.repo)
			}
		})
	}
}

func TestLoadDotEnvLoadsMissingVariables(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".env")
	if err := os.WriteFile(path, []byte("GITHUB_TOKEN=from-dotenv\nGITEA_TOKEN_DEFAULT=abc\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	_ = os.Unsetenv("GITHUB_TOKEN")
	_ = os.Unsetenv("GITEA_TOKEN_DEFAULT")
	t.Cleanup(func() {
		_ = os.Unsetenv("GITHUB_TOKEN")
		_ = os.Unsetenv("GITEA_TOKEN_DEFAULT")
	})

	if err := loadDotEnv(path); err != nil {
		t.Fatalf("loadDotEnv() error = %v", err)
	}
	if got, want := os.Getenv("GITHUB_TOKEN"), "from-dotenv"; got != want {
		t.Fatalf("GITHUB_TOKEN = %q, want %q", got, want)
	}
	if got, want := os.Getenv("GITEA_TOKEN_DEFAULT"), "abc"; got != want {
		t.Fatalf("GITEA_TOKEN_DEFAULT = %q, want %q", got, want)
	}
}

func TestLoadDotEnvDoesNotOverrideExistingVariables(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".env")
	if err := os.WriteFile(path, []byte("GITHUB_TOKEN=from-dotenv\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	t.Setenv("GITHUB_TOKEN", "from-env")

	if err := loadDotEnv(path); err != nil {
		t.Fatalf("loadDotEnv() error = %v", err)
	}
	if got, want := os.Getenv("GITHUB_TOKEN"), "from-env"; got != want {
		t.Fatalf("GITHUB_TOKEN = %q, want %q", got, want)
	}
}

func TestPrintResultIncludesInlineFindingDetails(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	printResult(logger, core.ReviewResult{
		ReviewAction: "comment",
		Summary:      "summary",
		InlineFindings: []core.InlineFinding{{
			Position: core.InlinePosition{Path: "pkgs/test.nix", StartLine: 53, EndLine: 68},
			Title:    "Missing dep",
			Body:     "Need cairo",
		}},
	})

	output := buf.String()
	for _, want := range []string{"review finished", "inline finding", "pkgs/test.nix", "53", "68", "Missing dep", "Need cairo"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output %q does not contain %q", output, want)
		}
	}
	_ = context.Background()
}
