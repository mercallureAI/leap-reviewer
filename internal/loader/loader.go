package loader

import (
	"fmt"
	"path/filepath"

	"github.com/cryolitia/gitea-ai-bot/internal/config"
	"github.com/cryolitia/gitea-ai-bot/internal/profiles"
)

type Loader struct {
	config   *config.File
	profiles map[string]profiles.Definition
}

func New(root string) (*Loader, error) {
	configFile, err := config.Load(filepath.Join(root, "config.yaml"))
	if err != nil {
		return nil, err
	}
	loadedProfiles, err := profiles.LoadAll(filepath.Join(root, "profiles"))
	if err != nil {
		return nil, err
	}
	return &Loader{config: configFile, profiles: loadedProfiles}, nil
}

func (l *Loader) Load(instanceKey, owner, repo, profile string) (config.EffectiveRepositoryConfig, profiles.Definition, error) {
	effective, err := l.config.EffectiveRepositoryConfig(instanceKey, owner, repo)
	if err != nil {
		return config.EffectiveRepositoryConfig{}, profiles.Definition{}, err
	}
	name := profile
	if name == "" {
		name = effective.Config.DefaultProfile
	}
	profileDef, ok := l.profiles[name]
	if !ok {
		return config.EffectiveRepositoryConfig{}, profiles.Definition{}, fmt.Errorf("profile %q not found", name)
	}
	if !contains(effective.Config.EnabledProfiles, name) {
		return config.EffectiveRepositoryConfig{}, profiles.Definition{}, fmt.Errorf("profile %q is not enabled", name)
	}
	return effective, profileDef, nil
}

func (l *Loader) PlatformForInstance(instanceKey string) (string, error) {
	instance, ok := l.config.Instances[instanceKey]
	if !ok {
		return "", fmt.Errorf("instance %q not found", instanceKey)
	}
	return instance.Platform, nil
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
