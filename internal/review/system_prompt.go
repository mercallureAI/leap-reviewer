package review

import (
	_ "embed"
	"path/filepath"
	"strings"
)

//go:embed prompts/system.md
var systemPrompt string

func embeddedSystemPrompt() string {
	return strings.TrimSpace(systemPrompt)
}

func systemPromptSourcePath() string {
	return filepath.Join("internal", "review", "prompts", "system.md")
}
