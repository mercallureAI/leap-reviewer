package workspace

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cryolitia/gitea-ai-bot/internal/review"
)

var repoLocks sync.Map

type Manager struct {
	CacheRoot string
	WorkRoot  string
	Logger    *slog.Logger
	GitRunner func(context.Context, string, ...string) error
}

func (m Manager) Prepare(ctx context.Context, req review.PrepareRequest) (review.PreparedWorkspace, error) {
	unlock := lockRepo(req.InstanceKey, req.Owner, req.Repo)
	defer unlock()

	m.log(ctx, "preparing repository workspace", slog.String("owner", req.Owner), slog.String("repo", req.Repo), slog.String("head_sha", req.HeadSHA), slog.String("head_ref", req.HeadRef))
	cacheDir := filepath.Join(m.CacheRoot, req.InstanceKey, req.Owner, req.Repo)
	workDir := filepath.Join(m.WorkRoot, req.InstanceKey, req.Owner, req.Repo, sanitizeHead(req.HeadSHA))

	if err := os.MkdirAll(filepath.Dir(cacheDir), 0o755); err != nil {
		return review.PreparedWorkspace{}, err
	}
	if err := os.MkdirAll(filepath.Dir(workDir), 0o755); err != nil {
		return review.PreparedWorkspace{}, err
	}
	unlockFile, err := acquireFileLock(ctx, cacheDir+".lock")
	if err != nil {
		return review.PreparedWorkspace{}, err
	}
	defer unlockFile()

	if _, err := os.Stat(cacheDir); err != nil {
		if os.IsNotExist(err) {
			m.log(ctx, "initializing repository cache", slog.String("cache_dir", cacheDir))
			if err := m.runGit(ctx, "", "clone", "--no-checkout", "--depth", "1", req.RepoURL, cacheDir); err != nil {
				return review.PreparedWorkspace{}, err
			}
			if target := fetchTarget(req); target != "" {
				m.log(ctx, "fetching target into cache", slog.String("target", target))
				if err := m.runGit(ctx, cacheDir, "fetch", "--depth", "1", "origin", target); err != nil {
					return review.PreparedWorkspace{}, err
				}
			}
		} else {
			return review.PreparedWorkspace{}, err
		}
	} else {
		m.log(ctx, "refreshing repository cache", slog.String("cache_dir", cacheDir), slog.String("target", fetchTarget(req)))
		if err := m.runGit(ctx, cacheDir, "fetch", "--depth", "1", "origin", fetchTarget(req)); err != nil {
			return review.PreparedWorkspace{}, err
		}
	}

	_ = os.RemoveAll(workDir)
	m.log(ctx, "creating isolated worktree", slog.String("work_dir", workDir))
	if err := m.runGit(ctx, "", "clone", cacheDir, workDir); err != nil {
		return review.PreparedWorkspace{}, err
	}
	if err := m.runGit(ctx, workDir, "remote", "set-url", "origin", req.RepoURL); err != nil {
		return review.PreparedWorkspace{}, err
	}
	if target := fetchTarget(req); target != "" {
		m.log(ctx, "fetching target into isolated worktree", slog.String("target", target))
		if err := m.runGit(ctx, workDir, "fetch", "--depth", "1", "origin", target); err != nil {
			return review.PreparedWorkspace{}, err
		}
	}
	m.log(ctx, "checking out requested revision", slog.String("head_sha", req.HeadSHA))
	if err := m.runGit(ctx, workDir, "checkout", req.HeadSHA); err != nil {
		return review.PreparedWorkspace{}, err
	}

	return review.PreparedWorkspace{Path: workDir}, nil
}

func (m Manager) runGit(ctx context.Context, dir string, args ...string) error {
	if m.GitRunner != nil {
		return m.GitRunner(ctx, dir, args...)
	}
	return runGit(ctx, dir, args...)
}

func (m Manager) log(ctx context.Context, message string, args ...any) {
	if m.Logger != nil {
		m.Logger.InfoContext(ctx, message, args...)
	}
}

func runGit(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, string(output))
	}
	return nil
}

func sanitizeHead(head string) string {
	if head == "" {
		return "latest"
	}
	return strings.ReplaceAll(head, string(filepath.Separator), "_")
}

func fetchTarget(req review.PrepareRequest) string {
	if req.HeadRef != "" {
		return req.HeadRef
	}
	return req.HeadSHA
}

func lockRepo(instanceKey, owner, repo string) func() {
	key := instanceKey + "/" + owner + "/" + repo
	value, _ := repoLocks.LoadOrStore(key, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func acquireFileLock(ctx context.Context, lockPath string) (func(), error) {
	for {
		file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			return func() {
				_ = file.Close()
				_ = os.Remove(lockPath)
			}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}
