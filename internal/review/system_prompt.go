package review

import (
	_ "embed"
	"path/filepath"
	"strings"
)

//go:embed prompts/system.md
var systemPrompt string

//go:embed prompts/instructions.md
var instructionPrompt string

func embeddedSystemPrompt() string {
	return strings.TrimSpace(systemPrompt)
}

func embeddedInstructionPrompt() string {
	return strings.TrimSpace(instructionPrompt)
}

func systemPromptSourcePath() string {
	return filepath.Join("internal", "review", "prompts", "system.md")
}

func instructionPromptSourcePath() string {
	return filepath.Join("internal", "review", "prompts", "instructions.md")
}
