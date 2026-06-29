package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadBuildsEffectiveRepositoryConfigWithSeparateModels(t *testing.T) {
	t.Setenv("GITEA_TOKEN_DEFAULT", "instance-token")
	t.Setenv("GITEA_WEBHOOK_SECRET_DEFAULT", "instance-secret")
	t.Setenv("GITEA_TOKEN_TEAM", "owner-token")
	t.Setenv("GITEA_WEBHOOK_SECRET_TEAM", "owner-secret")

	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}

	content := `
models:
  fast-review:
    provider: openai
    model: gpt-4.1-mini
  deep-ask:
    provider: openai
    model: gpt-5.4
instances:
  corp-gitea:
    platform: gitea
    base_url: https://gitea.example.com
    auth:
      token_env: GITEA_TOKEN_DEFAULT
      webhook_secret_env: GITEA_WEBHOOK_SECRET_DEFAULT
    config:
      review_model: fast-review
      ask_model: deep-ask
      auto_review_enabled: false
      command_review_enabled: true
      allowed_auto_events: []
      allowed_commands: [review, ask]
      default_profile: default
      enabled_profiles: [default]
      publish_summary_comment: true
      publish_inline_comments: true
      inline_comment_limit: 5
      inline_fallback_to_general: true
      opencode_timeout_seconds: 420
      ignore_draft_prs: true
      ignore_bot_prs: true
      ignore_bot_comments: true
      dry_run_default: false
    owners:
      backend-team:
        auth:
          token_env: GITEA_TOKEN_TEAM
          webhook_secret_env: GITEA_WEBHOOK_SECRET_TEAM
        config:
          auto_review_enabled: true
          allowed_auto_events: [pull_request.opened, pull_request.synchronize]
          enabled_profiles: [default, security]
        repos:
          payment-service:
            config:
              review_model: fast-review
              ask_model: deep-ask
              publish_inline_comments: false
              inline_comment_limit: 3
`

	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	effective, err := loaded.EffectiveRepositoryConfig("corp-gitea", "backend-team", "payment-service")
	if err != nil {
		t.Fatalf("EffectiveRepositoryConfig() error = %v", err)
	}

	if effective.Config.AutoReviewEnabled != true {
		t.Fatalf("AutoReviewEnabled = %v, want true", effective.Config.AutoReviewEnabled)
	}
	if effective.Config.PublishInlineComments != false {
		t.Fatalf("PublishInlineComments = %v, want false", effective.Config.PublishInlineComments)
	}
	if effective.Config.InlineCommentLimit != 3 {
		t.Fatalf("InlineCommentLimit = %d, want 3", effective.Config.InlineCommentLimit)
	}
	if got, want := effective.Auth.Token, "owner-token"; got != want {
		t.Fatalf("auth token = %q, want %q", got, want)
	}
	if got, want := effective.ReviewModel.Provider, "openai"; got != want {
		t.Fatalf("review model provider = %q, want %q", got, want)
	}
	if got, want := effective.ReviewModel.Model, "gpt-4.1-mini"; got != want {
		t.Fatalf("review model name = %q, want %q", got, want)
	}
	if got, want := effective.AskModel.Model, "gpt-5.4"; got != want {
		t.Fatalf("ask model name = %q, want %q", got, want)
	}
	if got, want := len(effective.Config.EnabledProfiles), 2; got != want {
		t.Fatalf("enabled profiles len = %d, want %d", got, want)
	}
	if got, want := effective.Config.OpencodeTimeoutSeconds, 420; got != want {
		t.Fatalf("OpencodeTimeoutSeconds = %d, want %d", got, want)
	}
}

func TestDefaultConfigAllowsReviewAndAskCommands(t *testing.T) {
	if got, want := defaultConfig().AllowedCommands, []string{"review", "ask"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("AllowedCommands = %#v, want %#v", got, want)
	}
}

func TestDefaultConfigSetsOpencodeTimeout(t *testing.T) {
	if got, want := defaultConfig().OpencodeTimeoutSeconds, 300; got != want {
		t.Fatalf("OpencodeTimeoutSeconds = %d, want %d", got, want)
	}
}
