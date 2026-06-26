package loader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cryolitia/gitea-ai-bot/internal/config"
)

func TestNewOneShotLoaderLoadsProfilesWithoutConfigFile(t *testing.T) {
	root := t.TempDir()
	profilesDir := filepath.Join(root, "profiles", "default")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatalf("mkdir profiles: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "profile.yaml"), []byte("target: pull_request\nlanguage: zh-CN\ninline_enabled: true\ninline_limit: 10\n"), 0o644); err != nil {
		t.Fatalf("write profile.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "prompt.md"), []byte("review prompt"), 0o644); err != nil {
		t.Fatalf("write prompt.md: %v", err)
	}

	loader, err := NewOneShot(filepath.Join(root, "profiles"), config.EffectiveRepositoryConfig{
		InstanceKey: "oneshot",
		Owner:       "nixos",
		Repo:        "nixpkgs",
		Platform:    "github",
		BaseURL:     "https://api.github.com",
		Model:       config.ModelDefinition{Provider: "openai", Model: "gpt-5.4"},
		Config: config.Config{
			DefaultProfile:    "default",
			EnabledProfiles:   []string{"default"},
			InlineCommentLimit: 5,
		},
	})
	if err != nil {
		t.Fatalf("NewOneShot() error = %v", err)
	}

	effective, profile, err := loader.Load("oneshot", "nixos", "nixpkgs", "")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := effective.Platform, "github"; got != want {
		t.Fatalf("Platform = %q, want %q", got, want)
	}
	if got, want := profile.Name, "default"; got != want {
		t.Fatalf("profile name = %q, want %q", got, want)
	}
}
