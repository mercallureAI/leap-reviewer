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
