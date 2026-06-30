package github

import (
	"context"
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
		SHA string `json:"sha"`
	} `json:"commit"`
}

func (a Adapter) GetPullRequestContext(ctx context.Context, effective config.EffectiveRepositoryConfig, req core.ReviewRequest) (review.PullRequestContext, error) {
	prPath := fmt.Sprintf("/repos/%s/%s/pulls/%d", effective.Owner, effective.Repo, req.PRNumber)
	filesPath := fmt.Sprintf("%s/files?per_page=100", prPath)

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
		issuePath := fmt.Sprintf("/repos/%s/%s/issues/%d", effective.Owner, effective.Repo, req.IssueNumber)
		repoPath := fmt.Sprintf("/repos/%s/%s", effective.Owner, effective.Repo)
		var issue issueResponse
		if err := a.getJSON(ctx, effective, issuePath, &issue); err != nil {
			return review.PullRequestContext{}, err
		}
		var repo repoResponse
		if err := a.getJSON(ctx, effective, repoPath, &repo); err != nil {
			return review.PullRequestContext{}, err
		}
		branchPath := fmt.Sprintf("/repos/%s/%s/branches/%s", effective.Owner, effective.Repo, repo.DefaultBranch)
		var branch branchResponse
		if err := a.getJSON(ctx, effective, branchPath, &branch); err != nil {
			return review.PullRequestContext{}, err
		}
		return review.PullRequestContext{Title: issue.Title, Body: issue.Body, CloneURL: repo.CloneURL, HeadSHA: branch.Commit.SHA, HeadRef: branch.Name}, nil
	}
	return a.GetPullRequestContext(ctx, effective, req)
}

func (a Adapter) PublishReview(context.Context, config.EffectiveRepositoryConfig, core.ReviewRequest, core.ReviewResult) error {
	return nil
}

func (a Adapter) PublishComment(context.Context, config.EffectiveRepositoryConfig, core.ReviewRequest, string) error {
	return nil
}

func (a Adapter) UpdatePullRequestBody(context.Context, config.EffectiveRepositoryConfig, core.ReviewRequest, string) error {
	return nil
}

func (a Adapter) getJSON(ctx context.Context, effective config.EffectiveRepositoryConfig, endpoint string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(effective.BaseURL, endpoint), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if effective.Auth.Token != "" {
		req.Header.Set("Authorization", "Bearer "+effective.Auth.Token)
	}
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := a.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github GET %s failed: %s: %s", endpoint, resp.Status, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(dst)
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
