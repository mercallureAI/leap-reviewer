package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	askservice "github.com/cryolitia/gitea-ai-bot/internal/ask"
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

type daemonCommand struct {
	Listen string `default:":8080" help:"daemon listen address"`
	Config string `default:"./config" help:"configuration root"`
	Instance string `required:"" help:"instance key"`
}

type reviewCommand struct {
	Config      string `default:"./config" help:"configuration root"`
	ProfilesDir string `help:"profiles directory for oneshot mode"`
	Instance    string `help:"instance key"`
	Platform    string `required:"" help:"platform name for oneshot mode"`
	BaseURL     string `help:"platform API base URL for oneshot mode"`
	Owner       string `required:"" help:"repository owner"`
	Repo        string `required:"" help:"repository name"`
	PR          int    `required:"" help:"pull request number"`
	HeadSHA     string `help:"pull request head sha"`
	Profile     string `help:"profile name"`
	Provider    string `required:"" help:"opencode model provider for oneshot mode"`
	Model       string `required:"" help:"opencode model name for oneshot mode"`
	TimeoutSeconds int `default:"300" help:"opencode timeout in seconds"`
	TokenEnv    string `help:"environment variable holding platform token for oneshot mode"`
	Publish     bool   `help:"publish review back to platform"`
	DryRun      bool   `help:"skip publishing"`
	TriggerType string `default:"command" help:"trigger type"`
	EventName   string `help:"event name"`
	Command     string `help:"raw command text"`
}

type askCommand struct {
	Config      string `default:"./config" help:"configuration root"`
	ProfilesDir string `help:"profiles directory for oneshot mode"`
	Instance    string `help:"instance key"`
	Platform    string `required:"" help:"platform name for oneshot mode"`
	BaseURL     string `help:"platform API base URL for oneshot mode"`
	Owner       string `required:"" help:"repository owner"`
	Repo        string `required:"" help:"repository name"`
	PR          int    `required:"" help:"pull request number"`
	HeadSHA     string `help:"pull request head sha"`
	Profile     string `help:"profile name"`
	Provider    string `required:"" help:"opencode model provider for oneshot mode"`
	Model       string `required:"" help:"opencode model name for oneshot mode"`
	TimeoutSeconds int `default:"300" help:"opencode timeout in seconds"`
	Question    string `required:"" help:"question for the model"`
	TokenEnv    string `help:"environment variable holding platform token for oneshot mode"`
	Publish     bool   `help:"publish answer back to platform"`
	DryRun      bool   `help:"skip publishing"`
	TriggerType string `default:"command" help:"trigger type"`
	EventName   string `help:"event name"`
}

type cli struct {
	Daemon daemonCommand `cmd:"" help:"run webhook server"`
	Review reviewCommand `cmd:"" help:"run one-shot review"`
	Ask    askCommand    `cmd:"" help:"ask a question about a pull request"`
}

func main() {
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

	parsed, command, err := parseCLI(os.Args[1:])
	if err != nil {
		exitErr(logger, err)
	}

	multiAdapter := platformadapter.MultiAdapter{Gitea: gitea.Adapter{}, GitHub: githubplatform.Adapter{}}
	publisher := publish.Publisher{Platform: multiAdapter}

	switch command {
	case "daemon":
		exitErr(logger, runDaemon(logger, multiAdapter, publisher, parsed.Daemon))
	case "review":
		exitErr(logger, runReview(logger, multiAdapter, publisher, parsed.Review))
	case "ask":
		exitErr(logger, runAsk(logger, multiAdapter, parsed.Ask))
	default:
		exitErr(logger, fmt.Errorf("unsupported command %q", command))
	}
}

func parseCLI(args []string) (cli, string, error) {
	parsed := cli{}
	parser, err := kong.New(&parsed, kong.Name("review-bot"), kong.UsageOnError())
	if err != nil {
		return cli{}, "", err
	}
	ctx, err := parser.Parse(args)
	if err != nil {
		return cli{}, "", err
	}
	command := ctx.Command()
	if strings.HasPrefix(command, "daemon") {
		return parsed, "daemon", nil
	}
	if strings.HasPrefix(command, "review") {
		return parsed, "review", nil
	}
	if strings.HasPrefix(command, "ask") {
		return parsed, "ask", nil
	}
	return parsed, command, nil
}

func runDaemon(logger *slog.Logger, multiAdapter platformadapter.MultiAdapter, publisher publish.Publisher, cmd daemonCommand) error {
	loaded, err := loader.New(cmd.Config)
	if err != nil {
		return err
	}
	platformName, err := loaded.PlatformForInstance(cmd.Instance)
	if err != nil {
		return err
	}
	if platformName != "gitea" {
		return fmt.Errorf("daemon mode currently supports only gitea instances")
	}
	reviewer := review.Service{
		Loader:    loaded,
		Platform:  multiAdapter,
		Workspace: workspace.Manager{CacheRoot: "./.cache/repos", WorkRoot: "./.worktrees", Logger: logger},
		Executor:  executor.Runner{Executor: innerexec.Executor{}, TempDir: os.TempDir()},
		Progress:  progressLogger(logger),
	}
	askReviewer := askservice.Service{
		Loader:    loaded,
		Platform:  multiAdapter,
		Workspace: workspace.Manager{CacheRoot: "./.cache/repos", WorkRoot: "./.worktrees", Logger: logger},
		Executor:  executor.Runner{Executor: innerexec.Executor{}, TempDir: os.TempDir()},
		Progress:  progressLogger(logger),
	}
	logger.Info("starting daemon", slog.String("listen", cmd.Listen), slog.String("instance", cmd.Instance))
	handler := server.Handler{InstanceKey: cmd.Instance, Adapter: gitea.Adapter{}, Loader: loaded, Reviewer: reviewer, Publisher: publisher, AskReviewer: askReviewer, CommentPublisher: multiAdapter}
	return http.ListenAndServe(cmd.Listen, handler)
}

func runReview(logger *slog.Logger, multiAdapter platformadapter.MultiAdapter, publisher publish.Publisher, cmd reviewCommand) error {
	oneShotLoader, req, err := buildReviewOneShotLoader(cmd)
	if err != nil {
		return err
	}
	reviewer := review.Service{
		Loader:    oneShotLoader,
		Platform:  multiAdapter,
		Workspace: workspace.Manager{CacheRoot: "./.cache/repos", WorkRoot: "./.worktrees", Logger: logger},
		Executor:  executor.Runner{Executor: innerexec.Executor{}, TempDir: os.TempDir()},
		Progress:  progressLogger(logger),
	}
	result, err := reviewer.Execute(context.Background(), req)
	if err != nil {
		return err
	}
	effective, _, err := oneShotLoader.Load(req.InstanceKey, req.Owner, req.Repo, req.ProfileName)
	if err != nil {
		return err
	}
	if err := publisher.Publish(context.Background(), effective, req, result); err != nil {
		return err
	}
	printResult(logger, result)
	return nil
}

func runAsk(logger *slog.Logger, multiAdapter platformadapter.MultiAdapter, cmd askCommand) error {
	oneShotLoader, req, err := buildAskOneShotLoader(cmd)
	if err != nil {
		return err
	}
	askReviewer := askservice.Service{
		Loader:    oneShotLoader,
		Platform:  multiAdapter,
		Workspace: workspace.Manager{CacheRoot: "./.cache/repos", WorkRoot: "./.worktrees", Logger: logger},
		Executor:  executor.Runner{Executor: innerexec.Executor{}, TempDir: os.TempDir()},
		Progress:  progressLogger(logger),
	}
	result, err := askReviewer.Execute(context.Background(), req)
	if err != nil {
		return err
	}
	effective, _, err := oneShotLoader.Load(req.InstanceKey, req.Owner, req.Repo, req.ProfileName)
	if err != nil {
		return err
	}
	if req.Publish && !req.DryRun {
		if err := multiAdapter.PublishComment(context.Background(), effective, req, formatAskComment("cli", req.QuestionText, result.Answer)); err != nil {
			return err
		}
	}
	printAskResult(logger, result)
	return nil
}

func buildReviewOneShotLoader(cmd reviewCommand) (*loader.OneShotLoader, core.ReviewRequest, error) {
	oneShotLoader, instance, baseURL, token, err := buildOneShotLoader(cmd.Config, cmd.ProfilesDir, cmd.Instance, cmd.Platform, cmd.BaseURL, cmd.Owner, cmd.Repo, cmd.Profile, cmd.Provider, cmd.Model, cmd.TimeoutSeconds, cmd.TokenEnv, cmd.Publish)
	if err != nil {
		return nil, core.ReviewRequest{}, err
	}
	req := core.ReviewRequest{
		InstanceKey:  instance,
		Owner:        cmd.Owner,
		Repo:         cmd.Repo,
		PRNumber:     cmd.PR,
		HeadSHA:      cmd.HeadSHA,
		DeliveryPath: "oneshot",
		TriggerType:  cmd.TriggerType,
		EventName:    cmd.EventName,
		CommandText:  cmd.Command,
		ProfileName:  cmd.Profile,
		Publish:      cmd.Publish,
		DryRun:       cmd.DryRun,
	}
	_ = baseURL
	_ = token
	return oneShotLoader, req, nil
}

func buildAskOneShotLoader(cmd askCommand) (*loader.OneShotLoader, core.ReviewRequest, error) {
	oneShotLoader, instance, _, _, err := buildOneShotLoader(cmd.Config, cmd.ProfilesDir, cmd.Instance, cmd.Platform, cmd.BaseURL, cmd.Owner, cmd.Repo, cmd.Profile, cmd.Provider, cmd.Model, cmd.TimeoutSeconds, cmd.TokenEnv, cmd.Publish)
	if err != nil {
		return nil, core.ReviewRequest{}, err
	}
	req := core.ReviewRequest{
		InstanceKey:  instance,
		Owner:        cmd.Owner,
		Repo:         cmd.Repo,
		PRNumber:     cmd.PR,
		HeadSHA:      cmd.HeadSHA,
		DeliveryPath: "oneshot",
		TriggerType:  cmd.TriggerType,
		EventName:    cmd.EventName,
		CommandText:  "/ask " + cmd.Question,
		QuestionText: cmd.Question,
		ProfileName:  cmd.Profile,
		Publish:      cmd.Publish,
		DryRun:       cmd.DryRun,
	}
	return oneShotLoader, req, nil
}

func buildOneShotLoader(configRoot, profilesDir, instance, platformName, baseURL, owner, repo, profile, provider, model string, timeoutSeconds int, tokenEnv string, publishFlag bool) (*loader.OneShotLoader, string, string, string, error) {
	if profilesDir == "" {
		profilesDir = configRoot + "/profiles"
	}
	if instance == "" {
		instance = "oneshot"
	}
	baseURL = resolveBaseURL(platformName, baseURL)
	if platformName == "github" && publishFlag {
		return nil, "", "", "", fmt.Errorf("github support is currently limited to oneshot dry-run mode")
	}
	token := ""
	if tokenEnv != "" {
		token = os.Getenv(tokenEnv)
		if token == "" {
			return nil, "", "", "", fmt.Errorf("environment variable %s is not set", tokenEnv)
		}
	}
	oneShotLoader, err := loader.NewOneShot(profilesDir, config.EffectiveRepositoryConfig{
		InstanceKey: instance,
		Owner:       owner,
		Repo:        repo,
		Platform:    platformName,
		BaseURL:     baseURL,
		Auth:        config.ResolvedAuth{Token: token},
		ReviewModel: config.ModelDefinition{Provider: provider, Model: model},
		AskModel:    config.ModelDefinition{Provider: provider, Model: model},
		Config: config.Config{
			DefaultProfile:          defaultProfileName(profile),
			EnabledProfiles:         []string{defaultProfileName(profile)},
			OpencodeTimeoutSeconds:  timeoutSeconds,
			PublishSummaryComment:   publishFlag,
			PublishInlineComments:   publishFlag,
			InlineCommentLimit:      5,
			InlineFallbackToGeneral: true,
		},
	})
	if err != nil {
		return nil, "", "", "", err
	}
	return oneShotLoader, instance, baseURL, token, nil
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

func printAskResult(logger *slog.Logger, result core.AskResult) {
	logger.Info("ask finished", slog.String("answer", result.Answer))
	if len(result.Warnings) > 0 {
		logger.Warn("ask warnings", slog.Any("warnings", result.Warnings))
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

func formatAskComment(user, question, answer string) string {
	return fmt.Sprintf("@%s\n\n问题：\n%s\n\n回答：\n%s", user, question, answer)
}
