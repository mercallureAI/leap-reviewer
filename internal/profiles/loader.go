package profiles

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Definition struct {
	Name          string
	Target        string `yaml:"target"`
	Language      string `yaml:"language"`
	InlineEnabled bool   `yaml:"inline_enabled"`
	InlineLimit   int    `yaml:"inline_limit"`
	Prompt          string
	ReviewPrompt    string
	AskPrompt       string
	SummarizePrompt string
}

func LoadAll(root string) (map[string]Definition, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	loaded := make(map[string]Definition, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		definition, err := loadOne(filepath.Join(root, name), name)
		if err != nil {
			return nil, err
		}
		loaded[name] = definition
	}

	return loaded, nil
}

func SortedNames(defs map[string]Definition) []string {
	names := make([]string, 0, len(defs))
	for name := range defs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func loadOne(dir, name string) (Definition, error) {
	definitionBytes, err := os.ReadFile(filepath.Join(dir, "profile.yaml"))
	if err != nil {
		return Definition{}, err
	}
	promptBytes, err := os.ReadFile(filepath.Join(dir, "prompt.md"))
	if err != nil {
		return Definition{}, err
	}
	reviewPromptBytes, err := readOptionalPrompt(filepath.Join(dir, "review.md"))
	if err != nil {
		return Definition{}, err
	}
	askPromptBytes, err := readOptionalPrompt(filepath.Join(dir, "ask.md"))
	if err != nil {
		return Definition{}, err
	}
	summarizePromptBytes, err := readOptionalPrompt(filepath.Join(dir, "summarize.md"))
	if err != nil {
		return Definition{}, err
	}

	var definition Definition
	dec := yaml.NewDecoder(strings.NewReader(string(definitionBytes)))
	dec.KnownFields(true)
	if err := dec.Decode(&definition); err != nil {
		return Definition{}, fmt.Errorf("decode profile %s: %w", name, err)
	}

	definition.Name = name
	definition.Prompt = string(promptBytes)
	definition.ReviewPrompt = choosePrompt(reviewPromptBytes, promptBytes)
	definition.AskPrompt = choosePrompt(askPromptBytes, promptBytes)
	definition.SummarizePrompt = choosePrompt(summarizePromptBytes, promptBytes)
	return definition, nil
}

func readOptionalPrompt(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

func choosePrompt(capability []byte, fallback []byte) string {
	if len(capability) > 0 {
		return string(capability)
	}
	return string(fallback)
}
