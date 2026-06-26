package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"strings"
	"testing"
	"time"

	"github.com/cryolitia/gitea-ai-bot/internal/review"
)

func TestManagerPrepareClonesRepositoryIntoIsolatedDirectory(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "source")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	runGitCmd(t, repoDir, "init")
	runGitCmd(t, repoDir, "config", "user.email", "bot@example.com")
	runGitCmd(t, repoDir, "config", "user.name", "Bot")
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGitCmd(t, repoDir, "add", "README.md")
	runGitCmd(t, repoDir, "commit", "-m", "initial")
	headSHA := strings.TrimSpace(runGitCmd(t, repoDir, "rev-parse", "HEAD"))

	manager := Manager{CacheRoot: filepath.Join(root, "cache"), WorkRoot: filepath.Join(root, "work")}
	prepared, err := manager.Prepare(context.Background(), review.PrepareRequest{
		InstanceKey: "local",
		Owner:       "team",
		Repo:        "repo",
		HeadSHA:     headSHA,
		HeadRef:     "master",
		RepoURL:     repoDir,
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(prepared.Path, "README.md"))
	if err != nil {
		t.Fatalf("read cloned README: %v", err)
	}
	if got, want := string(content), "hello\n"; got != want {
		t.Fatalf("README content = %q, want %q", got, want)
	}
}

func TestManagerPrepareFetchesRequestedHeadWhenNotOnDefaultBranch(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "source")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	runGitCmd(t, repoDir, "init")
	runGitCmd(t, repoDir, "config", "user.email", "bot@example.com")
	runGitCmd(t, repoDir, "config", "user.name", "Bot")
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("main\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGitCmd(t, repoDir, "add", "README.md")
	runGitCmd(t, repoDir, "commit", "-m", "main")
	runGitCmd(t, repoDir, "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(repoDir, "feature.txt"), []byte("feature\n"), 0o644); err != nil {
		t.Fatalf("write feature.txt: %v", err)
	}
	runGitCmd(t, repoDir, "add", "feature.txt")
	runGitCmd(t, repoDir, "commit", "-m", "feature")
	headSHA := strings.TrimSpace(runGitCmd(t, repoDir, "rev-parse", "HEAD"))
	runGitCmd(t, repoDir, "checkout", "master")

	manager := Manager{CacheRoot: filepath.Join(root, "cache"), WorkRoot: filepath.Join(root, "work")}
	prepared, err := manager.Prepare(context.Background(), review.PrepareRequest{
		InstanceKey: "local",
		Owner:       "team",
		Repo:        "repo",
		HeadSHA:     headSHA,
		HeadRef:     "feature",
		RepoURL:     repoDir,
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(prepared.Path, "feature.txt"))
	if err != nil {
		t.Fatalf("read cloned feature.txt: %v", err)
	}
	if got, want := string(content), "feature\n"; got != want {
		t.Fatalf("feature content = %q, want %q", got, want)
	}
}

func TestManagerPrepareSerializesSameRepositoryCacheAccess(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(root, "cache", "local", "team", "repo")
	workRoot := filepath.Join(root, "work")

	runner := &concurrencyDetectingGitRunner{cacheDir: cacheDir}
	managerA := Manager{
		CacheRoot: cacheRoot(root),
		WorkRoot:  workRoot,
		GitRunner: runner.run,
	}
	managerB := Manager{
		CacheRoot: cacheRoot(root),
		WorkRoot:  workRoot,
		GitRunner: runner.run,
	}

	req := review.PrepareRequest{
		InstanceKey: "local",
		Owner:       "team",
		Repo:        "repo",
		HeadSHA:     "abc123",
		HeadRef:     "feature",
		RepoURL:     "/tmp/source.git",
	}

	errCh := make(chan error, 2)
	start := make(chan struct{})
	go func() {
		<-start
		_, err := managerA.Prepare(context.Background(), req)
		errCh <- err
	}()
	go func() {
		<-start
		_, err := managerB.Prepare(context.Background(), req)
		errCh <- err
	}()
	close(start)

	for range 2 {
		if err := <-errCh; err != nil {
			t.Fatalf("Prepare() error = %v, want nil", err)
		}
	}
	if runner.maxConcurrentCacheOps > 1 {
		t.Fatalf("maxConcurrentCacheOps = %d, want 1", runner.maxConcurrentCacheOps)
	}
}

func TestAcquireFileLockSerializesConcurrentCallers(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, "repo.lock")
	start := make(chan struct{})
	release := make(chan struct{})
	errCh := make(chan error, 1)

	unlock, err := acquireFileLock(context.Background(), lockPath)
	if err != nil {
		t.Fatalf("acquireFileLock() initial error = %v", err)
	}
	defer unlock()

	go func() {
		close(start)
		unlock, err := acquireFileLock(context.Background(), lockPath)
		if err == nil {
			unlock()
		}
		errCh <- err
		close(release)
	}()

	<-start
	select {
	case err := <-errCh:
		t.Fatalf("second acquireFileLock() completed too early with err=%v", err)
	case <-time.After(50 * time.Millisecond):
	}

	unlock()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("second acquireFileLock() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second acquireFileLock() did not complete after release")
	}
	<-release
}

func cacheRoot(root string) string {
	return filepath.Join(root, "cache")
}

type concurrencyDetectingGitRunner struct {
	mu                    sync.Mutex
	cacheDir              string
	activeCacheOps        int
	maxConcurrentCacheOps int
}

func (r *concurrencyDetectingGitRunner) run(_ context.Context, dir string, args ...string) error {
	if len(args) == 0 {
		return nil
	}
	switch args[0] {
	case "clone":
		target := args[len(args)-1]
		if err := os.MkdirAll(target, 0o755); err != nil {
			return err
		}
		if target == r.cacheDir {
			return r.withCacheCriticalSection(func() error { return nil })
		}
		return nil
	case "fetch":
		if dir == r.cacheDir {
			return r.withCacheCriticalSection(func() error { return nil })
		}
		return nil
	case "remote", "checkout":
		return nil
	default:
		return fmt.Errorf("unexpected git invocation in test: dir=%s args=%v", dir, args)
	}
}

func (r *concurrencyDetectingGitRunner) withCacheCriticalSection(fn func() error) error {
	r.mu.Lock()
	r.activeCacheOps++
	if r.activeCacheOps > r.maxConcurrentCacheOps {
		r.maxConcurrentCacheOps = r.activeCacheOps
	}
	concurrent := r.activeCacheOps
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		r.activeCacheOps--
		r.mu.Unlock()
	}()

	if concurrent > 1 {
		return fmt.Errorf("concurrent cache access detected")
	}
	time.Sleep(50 * time.Millisecond)
	return fn()
}
