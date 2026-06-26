package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewWritesToConfiguredFile(t *testing.T) {
	root := t.TempDir()
	logPath := filepath.Join(root, "review-bot.log")

	logger, closeFn, err := New(Options{Output: io.Discard, FilePath: logPath, Level: "INFO"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer closeFn()

	logger.InfoContext(context.Background(), "progress message", slog.String("phase", "testing"))

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got := string(content); !strings.Contains(got, "progress message") {
		t.Fatalf("log file content = %q, want message", got)
	}
}
