package main

import (
	"context"
	"bufio"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/cryolitia/gitea-ai-bot/internal/config"
	"github.com/cryolitia/gitea-ai-bot/internal/core"
	"github.com/cryolitia/gitea-ai-bot/internal/executor"
	innerexec "github.com/cryolitia/gitea-ai-bot/internal/executor/opencode"
	"github.com/cryolitia/gitea-ai-bot/internal/loader"
	"github.com/cryolitia/gitea-ai-bot/internal/logging"
	platformadapter "github.com/cryolitia/gitea-ai-bot/internal/platform"
	"github.com/cryolitia/gitea-ai-bot/internal/platform/gitea"
	githubplatform "github.com/cryolitia/gitea-ai-bot/internal/platform/github"
	"github.com/cryolitia/gitea-ai-bot/internal/publish"
	"github.com/cryolitia/gitea-ai-bot/internal/review"
	"github.com/cryolitia/gitea-ai-bot/internal/server"
	"github.com/cryolitia/gitea-ai-bot/internal/workspace"
)

func main() {
	var daemon bool
	var listen string
	var configRoot string
	var profilesDir string
	var instance string
	var platformName string
	var baseURL string
	var owner string
	var repo string
	var pr int
	var headSHA string
	var profile string
	var provider string
	var model string
	var tokenEnv string
	var publishFlag bool
	var dryRun bool
	var triggerType string
	var eventName string
	var command string

	flag.BoolVar(&daemon, "daemon", false, "run webhook server")
	flag.StringVar(&listen, "listen", ":8080", "daemon listen address")
	flag.StringVar(&configRoot, "config", "./config", "configuration root")
	flag.StringVar(&profilesDir, "profiles-dir", "", "profiles directory for oneshot mode")
	flag.StringVar(&instance, "instance", "", "instance key")
	flag.StringVar(&platformName, "platform", "", "platform name for oneshot mode")
	flag.StringVar(&baseURL, "base-url", "", "platform API base URL for oneshot mode")
	flag.StringVar(&owner, "owner", "", "repository owner")
	flag.StringVar(&repo, "repo", "", "repository name")
	flag.IntVar(&pr, "pr", 0, "pull request number")
	flag.StringVar(&headSHA, "head-sha", "", "pull request head sha")
	flag.StringVar(&profile, "profile", "", "profile name")
	flag.StringVar(&provider, "provider", "", "opencode model provider for oneshot mode")
	flag.StringVar(&model, "model", "", "opencode model name for oneshot mode")
	flag.StringVar(&tokenEnv, "token-env", "", "environment variable holding platform token for oneshot mode")
	flag.BoolVar(&publishFlag, "publish", false, "publish review back to platform")
	flag.BoolVar(&dryRun, "dry-run", false, "skip publishing")
	flag.StringVar(&triggerType, "trigger-type", "command", "trigger type")
	flag.StringVar(&eventName, "event-name", "", "event name")
	flag.StringVar(&command, "command", "", "raw command text")
	flag.Parse()
	if err := loadDotEnv(".env"); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	logger, closeLogger, err := logging.New(logging.Options{Output: os.Stderr, Level: "INFO"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = closeLogger() }()
	slog.SetDefault(logger)

	multiAdapter := platformadapter.MultiAdapter{Gitea: gitea.Adapter{}, GitHub: githubplatform.Adapter{}}
	publisher := publish.Publisher{Platform: multiAdapter}

	if daemon {
		loaded, err := loader.New(configRoot)
		if err != nil {
			exitErr(logger, err)
		}
		if instance == "" {
			exitErr(logger, fmt.Errorf("--instance is required in daemon mode"))
		}
		platformName, err := loaded.PlatformForInstance(instance)
		if err != nil {
			exitErr(logger, err)
		}
		if platformName != "gitea" {
			exitErr(logger, fmt.Errorf("daemon mode currently supports only gitea instances"))
		}
		reviewer := review.Service{
			Loader:    loaded,
			Platform:  multiAdapter,
			Workspace: workspace.Manager{CacheRoot: "./.cache/repos", WorkRoot: "./.worktrees", Logger: logger},
			Executor:  executor.Runner{Executor: innerexec.Executor{}, TempDir: os.TempDir()},
			Progress:  progressLogger(logger),
		}
		logger.Info("starting daemon", slog.String("listen", listen), slog.String("instance", instance))
		handler := server.Handler{InstanceKey: instance, Adapter: gitea.Adapter{}, Loader: loaded, Reviewer: reviewer, Publisher: publisher}
		exitErr(logger, http.ListenAndServe(listen, handler))
	}

	if profilesDir == "" {
		profilesDir = configRoot + "/profiles"
	}
	if instance == "" {
		instance = "oneshot"
	}
	if platformName == "" || owner == "" || repo == "" || pr == 0 || provider == "" || model == "" {
		exitErr(logger, fmt.Errorf("--platform, --owner, --repo, --pr, --provider, and --model are required in oneshot mode"))
	}
	baseURL = resolveBaseURL(platformName, baseURL)
	if platformName == "github" && !dryRun {
		exitErr(logger, fmt.Errorf("github support is currently limited to oneshot dry-run mode"))
	}
	token := ""
	if tokenEnv != "" {
		token = os.Getenv(tokenEnv)
		if token == "" {
			exitErr(logger, fmt.Errorf("environment variable %s is not set", tokenEnv))
		}
	}
	oneShotLoader, err := loader.NewOneShot(profilesDir, config.EffectiveRepositoryConfig{
		InstanceKey: instance,
		Owner:       owner,
		Repo:        repo,
		Platform:    platformName,
		BaseURL:     baseURL,
		Auth:        config.ResolvedAuth{Token: token},
		Model:       config.ModelDefinition{Provider: provider, Model: model},
		Config: config.Config{
			DefaultProfile:          defaultProfileName(profile),
			EnabledProfiles:         []string{defaultProfileName(profile)},
			PublishSummaryComment:   publishFlag,
			PublishInlineComments:   publishFlag,
			InlineCommentLimit:      5,
			InlineFallbackToGeneral: true,
		},
	})
	if err != nil {
		exitErr(logger, err)
	}
	reviewer := review.Service{
		Loader:    oneShotLoader,
		Platform:  multiAdapter,
		Workspace: workspace.Manager{CacheRoot: "./.cache/repos", WorkRoot: "./.worktrees", Logger: logger},
		Executor:  executor.Runner{Executor: innerexec.Executor{}, TempDir: os.TempDir()},
		Progress:  progressLogger(logger),
	}

	req := core.ReviewRequest{
		InstanceKey:  instance,
		Owner:        owner,
		Repo:         repo,
		PRNumber:     pr,
		HeadSHA:      headSHA,
		DeliveryPath: "oneshot",
		TriggerType:  triggerType,
		EventName:    eventName,
		CommandText:  command,
		ProfileName:  profile,
		Publish:      publishFlag,
		DryRun:       dryRun,
	}

	result, err := reviewer.Execute(context.Background(), req)
	if err != nil {
		exitErr(logger, err)
	}
	effective, _, err := oneShotLoader.Load(instance, owner, repo, profile)
	if err != nil {
		exitErr(logger, err)
	}
	if err := publisher.Publish(context.Background(), effective, req, result); err != nil {
		exitErr(logger, err)
	}
	printResult(logger, result)
}


func printResult(logger *slog.Logger, result core.ReviewResult) {
	logger.Info("review finished",
		slog.String("review_action", result.ReviewAction),
		slog.String("summary", result.Summary),
		slog.Int("general_count", len(result.GeneralComments)),
		slog.Int("inline_count", len(result.InlineFindings)),
	)
	for _, finding := range result.InlineFindings {
		logger.Info("inline finding",
			slog.String("path", finding.Position.Path),
			slog.Int("start_line", finding.Position.StartLine),
			slog.Int("end_line", finding.Position.EndLine),
			slog.String("title", finding.Title),
			slog.String("body", finding.Body),
		)
	}
	if len(result.Warnings) > 0 {
		logger.Warn("review warnings", slog.Any("warnings", result.Warnings))
	}
}

func exitErr(logger *slog.Logger, err error) {
	if err == nil {
		return
	}
	logger.Error("command failed", slog.String("error", err.Error()))
	os.Exit(1)
}

func progressLogger(logger *slog.Logger) func(string) {
	return func(message string) {
		logger.Info("progress", slog.String("message", message))
	}
}

func resolveBaseURL(platformName, configured string) string {
	if configured != "" {
		return configured
	}
	switch strings.ToLower(platformName) {
	case "github":
		return "https://api.github.com"
	default:
		return ""
	}
}

func defaultProfileName(profile string) string {
	if profile == "" {
		return "default"
	}
	return profile
}

func parseRepoSpec(spec string) (platform string, owner string, repo string, ok bool) {
	parts := strings.SplitN(spec, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", "", false
	}
	repoParts := strings.SplitN(parts[1], "/", 2)
	if len(repoParts) != 2 || repoParts[0] == "" || repoParts[1] == "" {
		return "", "", "", false
	}
	return parts[0], repoParts[0], repoParts[1], true
}

func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return scanner.Err()
}
