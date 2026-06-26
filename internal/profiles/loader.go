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
	Prompt        string
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

	var definition Definition
	dec := yaml.NewDecoder(strings.NewReader(string(definitionBytes)))
	dec.KnownFields(true)
	if err := dec.Decode(&definition); err != nil {
		return Definition{}, fmt.Errorf("decode profile %s: %w", name, err)
	}

	definition.Name = name
	definition.Prompt = string(promptBytes)
	return definition, nil
}
