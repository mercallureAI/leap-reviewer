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

func (a Adapter) PublishReview(context.Context, config.EffectiveRepositoryConfig, core.ReviewRequest, core.ReviewResult) error {
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
