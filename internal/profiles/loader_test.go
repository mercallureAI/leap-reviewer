package profiles

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAllUsesFolderNameAsProfileName(t *testing.T) {
	root := t.TempDir()
	profilesDir := filepath.Join(root, "profiles", "security")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatalf("mkdir profiles: %v", err)
	}

	if err := os.WriteFile(filepath.Join(profilesDir, "profile.yaml"), []byte("target: pull_request\nlanguage: zh-CN\ninline_enabled: true\ninline_limit: 10\n"), 0o644); err != nil {
		t.Fatalf("write profile.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "prompt.md"), []byte("review security issues"), 0o644); err != nil {
		t.Fatalf("write prompt.md: %v", err)
	}

	loaded, err := LoadAll(filepath.Join(root, "profiles"))
	if err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}

	profile, ok := loaded["security"]
	if !ok {
		t.Fatal("profile security not loaded")
	}
	if got, want := profile.Name, "security"; got != want {
		t.Fatalf("Name = %q, want %q", got, want)
	}
	if got, want := profile.Prompt, "review security issues"; got != want {
		t.Fatalf("Prompt = %q, want %q", got, want)
	}
}

func TestLoadAllUsesCapabilitySpecificPromptsWhenPresent(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(profilesDir, "review.md"), []byte("review prompt"), 0o644); err != nil {
		t.Fatalf("write review.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "ask.md"), []byte("ask prompt"), 0o644); err != nil {
		t.Fatalf("write ask.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "summarize.md"), []byte("summarize prompt"), 0o644); err != nil {
		t.Fatalf("write summarize.md: %v", err)
	}

	loaded, err := LoadAll(filepath.Join(root, "profiles"))
	if err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}

	profile := loaded["default"]
	if got, want := profile.ReviewPrompt, "review prompt"; got != want {
		t.Fatalf("ReviewPrompt = %q, want %q", got, want)
	}
	if got, want := profile.AskPrompt, "ask prompt"; got != want {
		t.Fatalf("AskPrompt = %q, want %q", got, want)
	}
	if got, want := profile.SummarizePrompt, "summarize prompt"; got != want {
		t.Fatalf("SummarizePrompt = %q, want %q", got, want)
	}
}

func TestLoadAllFallsBackToPromptForMissingCapabilityFiles(t *testing.T) {
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

	loaded, err := LoadAll(filepath.Join(root, "profiles"))
	if err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}

	profile := loaded["default"]
	for name, got := range map[string]string{
		"review":    profile.ReviewPrompt,
		"ask":       profile.AskPrompt,
		"summarize": profile.SummarizePrompt,
	} {
		if want := "shared prompt"; got != want {
			t.Fatalf("%s prompt = %q, want %q", name, got, want)
		}
	}
}
