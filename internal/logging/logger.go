package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

type Options struct {
	Output   io.Writer
	FilePath string
	Level    string
}

func New(opts Options) (*slog.Logger, func() error, error) {
	output := opts.Output
	if output == nil {
		output = os.Stderr
	}

	closeFn := func() error { return nil }
	writer := output
	if opts.FilePath != "" {
		file, err := os.OpenFile(opts.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, nil, err
		}
		writer = io.MultiWriter(output, file)
		closeFn = file.Close
	}

	level := parseLevel(opts.Level)
	handler := slog.NewTextHandler(writer, &slog.HandlerOptions{Level: level})
	return slog.New(handler), closeFn, nil
}

func parseLevel(level string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
