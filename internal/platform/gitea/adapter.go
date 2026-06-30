package gitea

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cryolitia/gitea-ai-bot/internal/config"
	"github.com/cryolitia/gitea-ai-bot/internal/core"
	"github.com/cryolitia/gitea-ai-bot/internal/review"
)

type Adapter struct {
	Client *http.Client
}

type pullRequestResponse struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Head  struct {
		SHA  string `json:"sha"`
		Ref  string `json:"ref"`
		Repo struct {
			CloneURL string `json:"clone_url"`
		} `json:"repo"`
	} `json:"head"`
}

type fileResponse struct {
	Filename string `json:"filename"`
	Patch    string `json:"patch"`
}

type issueResponse struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type repoResponse struct {
	DefaultBranch string `json:"default_branch"`
	CloneURL      string `json:"clone_url"`
}

type branchResponse struct {
	Name   string `json:"name"`
	Commit struct {
		ID string `json:"id"`
	} `json:"commit"`
}

type createReviewComment struct {
	Path       string `json:"path"`
	Body       string `json:"body"`
	NewLineNum int    `json:"new_position,omitempty"`
	OldLineNum int    `json:"old_position,omitempty"`
}

type createReviewRequest struct {
	Body     string                `json:"body,omitempty"`
	State    string                `json:"state,omitempty"`
	CommitID string                `json:"commit_id,omitempty"`
	Comments []createReviewComment `json:"comments,omitempty"`
}

type createCommentRequest struct {
	Body string `json:"body"`
}

func (a Adapter) GetPullRequestContext(ctx context.Context, effective config.EffectiveRepositoryConfig, req core.ReviewRequest) (review.PullRequestContext, error) {
	prPath := fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d", effective.Owner, effective.Repo, req.PRNumber)
	filesPath := prPath + "/files"

	var pr pullRequestResponse
	if err := a.getJSON(ctx, effective, prPath, &pr); err != nil {
		return review.PullRequestContext{}, err
	}
	var files []fileResponse
	if err := a.getJSON(ctx, effective, filesPath, &files); err != nil {
		return review.PullRequestContext{}, err
	}

	changed := make([]review.ChangedFile, 0, len(files))
	for _, file := range files {
		changed = append(changed, review.ChangedFile{Path: file.Filename, Patch: file.Patch})
	}

	return review.PullRequestContext{
		Title:        pr.Title,
		Body:         pr.Body,
		CloneURL:     pr.Head.Repo.CloneURL,
		HeadSHA:      pr.Head.SHA,
		HeadRef:      pr.Head.Ref,
		FilesChanged: changed,
	}, nil
}

func (a Adapter) GetAskContext(ctx context.Context, effective config.EffectiveRepositoryConfig, req core.ReviewRequest) (review.PullRequestContext, error) {
	if req.IssueNumber > 0 && req.PRNumber == 0 {
		issuePath := fmt.Sprintf("/api/v1/repos/%s/%s/issues/%d", effective.Owner, effective.Repo, req.IssueNumber)
		repoPath := fmt.Sprintf("/api/v1/repos/%s/%s", effective.Owner, effective.Repo)
		var issue issueResponse
		if err := a.getJSON(ctx, effective, issuePath, &issue); err != nil {
			return review.PullRequestContext{}, err
		}
		var repo repoResponse
		if err := a.getJSON(ctx, effective, repoPath, &repo); err != nil {
			return review.PullRequestContext{}, err
		}
		branchPath := fmt.Sprintf("/api/v1/repos/%s/%s/branches/%s", effective.Owner, effective.Repo, repo.DefaultBranch)
		var branch branchResponse
		if err := a.getJSON(ctx, effective, branchPath, &branch); err != nil {
			return review.PullRequestContext{}, err
		}
		return review.PullRequestContext{Title: issue.Title, Body: issue.Body, CloneURL: repo.CloneURL, HeadSHA: branch.Commit.ID, HeadRef: branch.Name}, nil
	}
	return a.GetPullRequestContext(ctx, effective, req)
}

func (a Adapter) PublishReview(ctx context.Context, effective config.EffectiveRepositoryConfig, req core.ReviewRequest, result core.ReviewResult) error {
	state := mapReviewAction(result.ReviewAction)
	comments := make([]createReviewComment, 0, len(result.InlineFindings))
	for _, finding := range result.InlineFindings {
		comments = append(comments, createReviewComment{
			Path:       finding.Position.Path,
			Body:       formatInlineBody(finding),
			NewLineNum: finding.Position.StartLine,
		})
	}
	body := result.Summary
	if len(result.GeneralComments) > 0 {
		for _, comment := range result.GeneralComments {
			body += "\n\n## " + comment.Title + "\n" + comment.Body
		}
	}

	payload := createReviewRequest{Body: body, State: state, CommitID: req.HeadSHA, Comments: comments}
	return a.postJSON(ctx, effective, fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d/reviews", effective.Owner, effective.Repo, req.PRNumber), payload)
}

func (a Adapter) PublishComment(ctx context.Context, effective config.EffectiveRepositoryConfig, req core.ReviewRequest, body string) error {
	payload := createCommentRequest{Body: body}
	return a.postJSON(ctx, effective, fmt.Sprintf("/api/v1/repos/%s/%s/issues/%d/comments", effective.Owner, effective.Repo, issueOrPullNumber(req)), payload)
}

func (a Adapter) UpdatePullRequestBody(ctx context.Context, effective config.EffectiveRepositoryConfig, req core.ReviewRequest, body string) error {
	payload := createCommentRequest{Body: body}
	return a.patchJSON(ctx, effective, fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d", effective.Owner, effective.Repo, req.PRNumber), payload)
}

func VerifyWebhookSignature(secret string, body []byte, signature string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func (a Adapter) getJSON(ctx context.Context, effective config.EffectiveRepositoryConfig, endpoint string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(effective.BaseURL, endpoint), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+effective.Auth.Token)
	resp, err := a.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gitea GET %s failed: %s: %s", endpoint, resp.Status, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

func (a Adapter) postJSON(ctx context.Context, effective config.EffectiveRepositoryConfig, endpoint string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, joinURL(effective.BaseURL, endpoint), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+effective.Auth.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gitea POST %s failed: %s: %s", endpoint, resp.Status, string(body))
	}
	return nil
}

func (a Adapter) patchJSON(ctx context.Context, effective config.EffectiveRepositoryConfig, endpoint string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, joinURL(effective.BaseURL, endpoint), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+effective.Auth.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gitea PATCH %s failed: %s: %s", endpoint, resp.Status, string(body))
	}
	return nil
}

func (a Adapter) httpClient() *http.Client {
	if a.Client != nil {
		return a.Client
	}
	return http.DefaultClient
}

func joinURL(baseURL, endpoint string) string {
	if strings.HasPrefix(endpoint, "/") {
		return strings.TrimRight(baseURL, "/") + endpoint
	}
	return strings.TrimRight(baseURL, "/") + "/" + endpoint
}

func mapReviewAction(action string) string {
	switch action {
	case "approve":
		return "APPROVE"
	case "request_changes":
		return "REJECT"
	default:
		return "COMMENT"
	}
}

func formatInlineBody(finding core.InlineFinding) string {
	var body string
	if finding.Title == "" {
		body = finding.Body
	} else {
		body = finding.Title + "\n\n" + finding.Body
	}
	if finding.Position.EndLine > finding.Position.StartLine && finding.Position.StartLine > 0 {
		body = fmt.Sprintf("Lines %d-%d\n\n%s", finding.Position.StartLine, finding.Position.EndLine, body)
	}
	return body
}

func issueOrPullNumber(req core.ReviewRequest) int {
	if req.IssueNumber > 0 {
		return req.IssueNumber
	}
	return req.PRNumber
}
