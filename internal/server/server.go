package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cryolitia/gitea-ai-bot/internal/config"
	"github.com/cryolitia/gitea-ai-bot/internal/core"
	"github.com/cryolitia/gitea-ai-bot/internal/platform/gitea"
	"github.com/cryolitia/gitea-ai-bot/internal/profiles"
	summarizeservice "github.com/cryolitia/gitea-ai-bot/internal/summarize"
	"github.com/cryolitia/gitea-ai-bot/internal/triggers"
)

type Reviewer interface {
	Execute(context.Context, core.ReviewRequest) (core.ReviewResult, error)
}

type Publisher interface {
	Publish(context.Context, config.EffectiveRepositoryConfig, core.ReviewRequest, core.ReviewResult) error
}

type Loader interface {
	Load(instanceKey, owner, repo, profile string) (config.EffectiveRepositoryConfig, any, error)
}

type Handler struct {
	InstanceKey string
	Adapter     gitea.Adapter
	Loader      interface {
		Load(instanceKey, owner, repo, profile string) (config.EffectiveRepositoryConfig, profiles.Definition, error)
	}
	Reviewer interface {
		Execute(context.Context, core.ReviewRequest) (core.ReviewResult, error)
	}
	Publisher interface {
		Publish(context.Context, config.EffectiveRepositoryConfig, core.ReviewRequest, core.ReviewResult) error
	}
	AskReviewer interface {
		Execute(context.Context, core.ReviewRequest) (core.AskResult, error)
	}
	CommentPublisher interface {
		PublishComment(context.Context, config.EffectiveRepositoryConfig, core.ReviewRequest, string) error
	}
	SummarizeReviewer interface {
		Execute(context.Context, core.ReviewRequest) (core.SummarizeResult, error)
	}
	BodyUpdater interface {
		UpdatePullRequestBody(context.Context, config.EffectiveRepositoryConfig, core.ReviewRequest, string) error
	}
	PermissionChecker interface {
		GetRepositoryPermission(context.Context, config.EffectiveRepositoryConfig, string) (core.RepositoryPermission, error)
	}
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	event := r.Header.Get("X-Gitea-Event")
	var req core.ReviewRequest
	var handled bool
	var effective config.EffectiveRepositoryConfig
	switch event {
	case "issue_comment":
		var issueEvent webhookIssueComment
		if err = json.Unmarshal(body, &issueEvent); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if issueEvent.Action != "created" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		effective, _, err = h.Loader.Load(h.InstanceKey, issueEvent.Repository.Owner.UserName, issueEvent.Repository.Name, "")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !gitea.VerifyWebhookSignature(effective.Auth.WebhookSecret, body, r.Header.Get("X-Gitea-Signature")) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		req, handled, err = ParseIssueCommentTrigger(body, effective.Config.DefaultProfile, effective.Config.EnabledProfiles)
	case "pull_request":
		req, handled, err = ParsePullRequestTrigger(body)
	default:
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !handled {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	req.InstanceKey = h.InstanceKey
	req.DeliveryPath = "daemon"

	if event != "issue_comment" {
		effective, _, err = h.Loader.Load(h.InstanceKey, req.Owner, req.Repo, req.ProfileName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !gitea.VerifyWebhookSignature(effective.Auth.WebhookSecret, body, r.Header.Get("X-Gitea-Signature")) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	}
	if req.TriggerType == "event" && !allowsAutoEvent(effective.Config, req.EventName) {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if req.TriggerType == "command" && !allowsCommand(effective.Config, commandName(req.CommandText)) {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if commandName(req.CommandText) == "summarize" {
		result, err := h.SummarizeReviewer.Execute(r.Context(), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if result.AlreadySummarized {
			if h.CommentPublisher != nil {
				_ = h.CommentPublisher.PublishComment(r.Context(), effective, req, formatSummarizeAlreadySummarized(req.TriggerUser, result.Source))
			}
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if !canUpdatePullRequestBody(req.TriggerUser, result.OriginalAuthor, r.Context(), effective, h.PermissionChecker) {
			if err := h.CommentPublisher.PublishComment(r.Context(), effective, req, formatSummarizePermissionDenied(req.TriggerUser, result.Body)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusAccepted)
			return
		}
		body := summarizeservice.BuildPublishedBody(result.OriginalBody, req.TriggerUser, result.Body)
		if err := h.BodyUpdater.UpdatePullRequestBody(r.Context(), effective, req, body); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if req.QuestionText != "" {
		result, err := h.AskReviewer.Execute(r.Context(), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := h.CommentPublisher.PublishComment(r.Context(), effective, req, formatAskComment(req.TriggerUser, req.QuestionText, result.Answer)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}

	result, err := h.Reviewer.Execute(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.Publisher.Publish(r.Context(), effective, req, result); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func canUpdatePullRequestBody(triggerUser, originalAuthor string, ctx context.Context, effective config.EffectiveRepositoryConfig, checker interface {
	GetRepositoryPermission(context.Context, config.EffectiveRepositoryConfig, string) (core.RepositoryPermission, error)
}) bool {
	if triggerUser != "" && triggerUser == originalAuthor {
		return true
	}
	if checker == nil {
		return false
	}
	permission, err := checker.GetRepositoryPermission(ctx, effective, triggerUser)
	if err != nil {
		return false
	}
	return permission.Admin || permission.Push
}

func formatSummarizePermissionDenied(user, body string) string {
	return fmt.Sprintf("@%s 你当前没有修改这个 PR 描述的权限，下面是可手动采用的建议正文。\n\n---\n\n%s", user, body)
}

func formatSummarizeAlreadySummarized(user, source string) string {
	return fmt.Sprintf("@%s 这个 PR 已经由 %s 总结过了。", user, source)
}

type webhookIssueComment struct {
	Action  string `json:"action"`
	IsPull  bool   `json:"is_pull"`
	Comment struct {
		Body string `json:"body"`
		ID   int64  `json:"id"`
	} `json:"comment"`
	Repository struct {
		Owner struct {
			UserName string `json:"username"`
		} `json:"owner"`
		Name string `json:"name"`
	} `json:"repository"`
	Issue struct {
		Number int `json:"number"`
	} `json:"issue"`
	Sender struct {
		UserName string `json:"username"`
	} `json:"sender"`
}

type webhookPullRequest struct {
	Action     string `json:"action"`
	Number     int    `json:"number"`
	Repository struct {
		Owner struct {
			UserName string `json:"username"`
		} `json:"owner"`
		Name string `json:"name"`
	} `json:"repository"`
	PullRequest struct {
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
	} `json:"pull_request"`
	Sender struct {
		UserName string `json:"username"`
	} `json:"sender"`
}

func ParseIssueCommentTrigger(payload []byte, defaultProfile string, enabledProfiles []string) (core.ReviewRequest, bool, error) {
	var event webhookIssueComment
	if err := json.Unmarshal(payload, &event); err != nil {
		return core.ReviewRequest{}, false, err
	}
	if event.Action != "created" {
		return core.ReviewRequest{}, false, nil
	}
	command, ok := triggers.ParseCommand(event.Comment.Body, defaultProfile, enabledProfiles)
	if !ok {
		return core.ReviewRequest{}, false, nil
	}
	if !event.IsPull && command.CommandType != "ask" {
		return core.ReviewRequest{}, false, nil
	}
	prNumber := 0
	issueNumber := event.Issue.Number
	if event.IsPull {
		prNumber = event.Issue.Number
		issueNumber = 0
	}
	return core.ReviewRequest{
		TriggerType:      "command",
		CommandText:      command.Raw,
		QuestionText:     command.Question,
		ProfileName:      command.ProfileName,
		Owner:            event.Repository.Owner.UserName,
		Repo:             event.Repository.Name,
		PRNumber:         prNumber,
		IssueNumber:      issueNumber,
		TriggerUser:      event.Sender.UserName,
		TriggerCommentID: event.Comment.ID,
		Publish:          true,
	}, true, nil
}

func commandName(commandText string) string {
	trimmed := strings.TrimSpace(commandText)
	if trimmed == "" {
		return ""
	}
	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimPrefix(parts[0], "/")
}

func formatAskComment(user, question, answer string) string {
	return fmt.Sprintf("@%s\n\n问题：\n%s\n\n回答：\n%s", user, question, answer)
}

func ParsePullRequestTrigger(payload []byte) (core.ReviewRequest, bool, error) {
	var event webhookPullRequest
	if err := json.Unmarshal(payload, &event); err != nil {
		return core.ReviewRequest{}, false, err
	}
	action := strings.ToLower(event.Action)
	if action != "opened" && action != "synchronize" && action != "reopened" {
		return core.ReviewRequest{}, false, nil
	}
	return core.ReviewRequest{
		TriggerType: "event",
		EventName:   BuildAutoEventName("pull_request", action),
		Owner:       event.Repository.Owner.UserName,
		Repo:        event.Repository.Name,
		PRNumber:    event.Number,
		HeadSHA:     event.PullRequest.Head.SHA,
		TriggerUser: event.Sender.UserName,
		Publish:     true,
	}, true, nil
}

func BuildAutoEventName(event, action string) string {
	if event == "pull_request" {
		return fmt.Sprintf("pull_request.%s", strings.ToLower(action))
	}
	return event
}

func allowsAutoEvent(cfg config.Config, eventName string) bool {
	if !cfg.AutoReviewEnabled {
		return false
	}
	for _, allowed := range cfg.AllowedAutoEvents {
		if allowed == eventName {
			return true
		}
	}
	return false
}

func allowsCommand(cfg config.Config, command string) bool {
	if !cfg.CommandReviewEnabled {
		return false
	}
	for _, allowed := range cfg.AllowedCommands {
		if allowed == command {
			return true
		}
	}
	return false
}
