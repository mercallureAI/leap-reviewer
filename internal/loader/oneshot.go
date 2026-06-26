package loader

import (
	"fmt"

	"github.com/cryolitia/gitea-ai-bot/internal/config"
	"github.com/cryolitia/gitea-ai-bot/internal/profiles"
)

type OneShotLoader struct {
	effective config.EffectiveRepositoryConfig
	profiles  map[string]profiles.Definition
}

func NewOneShot(profilesDir string, effective config.EffectiveRepositoryConfig) (*OneShotLoader, error) {
	loadedProfiles, err := profiles.LoadAll(profilesDir)
	if err != nil {
		return nil, err
	}
	return &OneShotLoader{effective: effective, profiles: loadedProfiles}, nil
}

func (l *OneShotLoader) Load(instanceKey, owner, repo, profile string) (config.EffectiveRepositoryConfig, profiles.Definition, error) {
	_ = instanceKey
	_ = owner
	_ = repo
	name := profile
	if name == "" {
		name = l.effective.Config.DefaultProfile
	}
	profileDef, ok := l.profiles[name]
	if !ok {
		return config.EffectiveRepositoryConfig{}, profiles.Definition{}, fmt.Errorf("profile %q not found", name)
	}
	if !contains(l.effective.Config.EnabledProfiles, name) {
		return config.EffectiveRepositoryConfig{}, profiles.Definition{}, fmt.Errorf("profile %q is not enabled", name)
	}
	return l.effective, profileDef, nil
}
