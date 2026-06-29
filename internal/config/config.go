package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type ModelDefinition struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
}

type AuthEnv struct {
	TokenEnv         string `yaml:"token_env"`
	WebhookSecretEnv string `yaml:"webhook_secret_env"`
}

type ResolvedAuth struct {
	Token         string
	WebhookSecret string
}

type Config struct {
	ReviewModel            string
	AskModel               string
	OpencodeTimeoutSeconds int
	AutoReviewEnabled      bool
	CommandReviewEnabled   bool
	AllowedAutoEvents      []string
	AllowedCommands        []string
	DefaultProfile         string
	EnabledProfiles        []string
	PublishSummaryComment   bool
	PublishInlineComments   bool
	InlineCommentLimit      int
	InlineFallbackToGeneral bool
	IgnoreDraftPRs          bool
	IgnoreBotPRs            bool
	IgnoreBotComments       bool
	DryRunDefault           bool
}

type configPatch struct {
	ReviewModel            *string  `yaml:"review_model"`
	AskModel               *string  `yaml:"ask_model"`
	OpencodeTimeoutSeconds *int     `yaml:"opencode_timeout_seconds"`
	AutoReviewEnabled      *bool    `yaml:"auto_review_enabled"`
	CommandReviewEnabled   *bool    `yaml:"command_review_enabled"`
	AllowedAutoEvents      []string `yaml:"allowed_auto_events"`
	AllowedCommands        []string `yaml:"allowed_commands"`
	DefaultProfile         *string  `yaml:"default_profile"`
	EnabledProfiles        []string `yaml:"enabled_profiles"`
	PublishSummaryComment   *bool    `yaml:"publish_summary_comment"`
	PublishInlineComments   *bool    `yaml:"publish_inline_comments"`
	InlineCommentLimit      *int     `yaml:"inline_comment_limit"`
	InlineFallbackToGeneral *bool    `yaml:"inline_fallback_to_general"`
	IgnoreDraftPRs          *bool    `yaml:"ignore_draft_prs"`
	IgnoreBotPRs            *bool    `yaml:"ignore_bot_prs"`
	IgnoreBotComments       *bool    `yaml:"ignore_bot_comments"`
	DryRunDefault           *bool    `yaml:"dry_run_default"`
}

type Repository struct {
	Auth   *AuthEnv    `yaml:"auth,omitempty"`
	Config configPatch `yaml:"config"`
}

type Owner struct {
	Auth   *AuthEnv              `yaml:"auth,omitempty"`
	Config configPatch           `yaml:"config"`
	Repos  map[string]Repository `yaml:"repos"`
}

type Instance struct {
	Platform string           `yaml:"platform"`
	BaseURL  string           `yaml:"base_url"`
	Auth     *AuthEnv         `yaml:"auth,omitempty"`
	Config   configPatch      `yaml:"config"`
	Owners   map[string]Owner `yaml:"owners"`
}

type File struct {
	Models    map[string]ModelDefinition `yaml:"models"`
	Instances map[string]Instance        `yaml:"instances"`
}

type EffectiveRepositoryConfig struct {
	InstanceKey string
	Owner       string
	Repo        string
	Platform    string
	BaseURL     string
	Auth        ResolvedAuth
	ReviewModel ModelDefinition
	AskModel    ModelDefinition
	Config      Config
}

func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var loaded File
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&loaded); err != nil {
		return nil, err
	}

	for instanceName, instance := range loaded.Instances {
		for ownerName, owner := range instance.Owners {
			for repoName, repo := range owner.Repos {
				if repo.Auth != nil {
					return nil, fmt.Errorf("repo %s/%s in instance %s cannot define auth", ownerName, repoName, instanceName)
				}
			}
		}
	}

	return &loaded, nil
}

func (f *File) EffectiveRepositoryConfig(instanceKey, ownerName, repoName string) (EffectiveRepositoryConfig, error) {
	instance, ok := f.Instances[instanceKey]
	if !ok {
		return EffectiveRepositoryConfig{}, fmt.Errorf("instance %q not found", instanceKey)
	}
	owner, ok := instance.Owners[ownerName]
	if !ok {
		return EffectiveRepositoryConfig{}, fmt.Errorf("owner %q not found", ownerName)
	}
	repo, ok := owner.Repos[repoName]
	if !ok {
		return EffectiveRepositoryConfig{}, fmt.Errorf("repo %q not found", repoName)
	}

	effectiveConfig := defaultConfig()
	effectiveConfig = mergeConfig(effectiveConfig, instance.Config)
	effectiveConfig = mergeConfig(effectiveConfig, owner.Config)
	effectiveConfig = mergeConfig(effectiveConfig, repo.Config)

	reviewModel, ok := f.Models[effectiveConfig.ReviewModel]
	if !ok {
		return EffectiveRepositoryConfig{}, fmt.Errorf("review model %q not found", effectiveConfig.ReviewModel)
	}
	askModel, ok := f.Models[effectiveConfig.AskModel]
	if !ok {
		return EffectiveRepositoryConfig{}, fmt.Errorf("ask model %q not found", effectiveConfig.AskModel)
	}

	auth := ResolvedAuth{}
	authEnv := instance.Auth
	if owner.Auth != nil {
		authEnv = owner.Auth
	}
	if authEnv != nil {
		var err error
		auth, err = resolveAuth(*authEnv)
		if err != nil {
			return EffectiveRepositoryConfig{}, err
		}
	}

	return EffectiveRepositoryConfig{
		InstanceKey: instanceKey,
		Owner:       ownerName,
		Repo:        repoName,
		Platform:    instance.Platform,
		BaseURL:     instance.BaseURL,
		Auth:        auth,
		ReviewModel: reviewModel,
		AskModel:    askModel,
		Config:      effectiveConfig,
	}, nil
}

func mergeConfig(base Config, override configPatch) Config {
	merged := base
	if override.ReviewModel != nil {
		merged.ReviewModel = *override.ReviewModel
	}
	if override.AskModel != nil {
		merged.AskModel = *override.AskModel
	}
	if override.OpencodeTimeoutSeconds != nil {
		merged.OpencodeTimeoutSeconds = *override.OpencodeTimeoutSeconds
	}
	if override.AutoReviewEnabled != nil {
		merged.AutoReviewEnabled = *override.AutoReviewEnabled
	}
	if override.CommandReviewEnabled != nil {
		merged.CommandReviewEnabled = *override.CommandReviewEnabled
	}
	if override.AllowedAutoEvents != nil {
		merged.AllowedAutoEvents = cloneStrings(override.AllowedAutoEvents)
	}
	if override.AllowedCommands != nil {
		merged.AllowedCommands = cloneStrings(override.AllowedCommands)
	}
	if override.DefaultProfile != nil {
		merged.DefaultProfile = *override.DefaultProfile
	}
	if override.EnabledProfiles != nil {
		merged.EnabledProfiles = cloneStrings(override.EnabledProfiles)
	}
	if override.PublishSummaryComment != nil {
		merged.PublishSummaryComment = *override.PublishSummaryComment
	}
	if override.PublishInlineComments != nil {
		merged.PublishInlineComments = *override.PublishInlineComments
	}
	if override.InlineCommentLimit != nil {
		merged.InlineCommentLimit = *override.InlineCommentLimit
	}
	if override.InlineFallbackToGeneral != nil {
		merged.InlineFallbackToGeneral = *override.InlineFallbackToGeneral
	}
	if override.IgnoreDraftPRs != nil {
		merged.IgnoreDraftPRs = *override.IgnoreDraftPRs
	}
	if override.IgnoreBotPRs != nil {
		merged.IgnoreBotPRs = *override.IgnoreBotPRs
	}
	if override.IgnoreBotComments != nil {
		merged.IgnoreBotComments = *override.IgnoreBotComments
	}
	if override.DryRunDefault != nil {
		merged.DryRunDefault = *override.DryRunDefault
	}
	return merged
}

func defaultConfig() Config {
	return Config{
		OpencodeTimeoutSeconds:  300,
		AllowedAutoEvents:       []string{},
		AllowedCommands:         []string{"review", "ask"},
		DefaultProfile:          "default",
		EnabledProfiles:         []string{"default"},
		PublishSummaryComment:   true,
		PublishInlineComments:   true,
		InlineCommentLimit:      5,
		InlineFallbackToGeneral: true,
		IgnoreDraftPRs:          true,
		IgnoreBotPRs:            true,
		IgnoreBotComments:       true,
	}
}

func cloneStrings(values []string) []string {
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func resolveAuth(auth AuthEnv) (ResolvedAuth, error) {
	token := ""
	if auth.TokenEnv != "" {
		token = os.Getenv(auth.TokenEnv)
		if token == "" {
			return ResolvedAuth{}, fmt.Errorf("environment variable %s is not set", auth.TokenEnv)
		}
	}
	secret := ""
	if auth.WebhookSecretEnv != "" {
		secret = os.Getenv(auth.WebhookSecretEnv)
		if secret == "" {
			return ResolvedAuth{}, fmt.Errorf("environment variable %s is not set", auth.WebhookSecretEnv)
		}
	}
	return ResolvedAuth{Token: token, WebhookSecret: secret}, nil
}
