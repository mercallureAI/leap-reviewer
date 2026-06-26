package resultparser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFileReadsStructuredReviewResult(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "result.json")
	content := `{
  "review_action": "comment",
  "summary": "Summary",
  "general_comments": [{"title": "Design", "body": "General note"}],
  "inline_findings": [{"path": "main.go", "line": 12, "side": "RIGHT", "title": "Bug", "body": "Inline note"}],
  "warnings": ["trimmed"]
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write result file: %v", err)
	}

	result, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if got, want := result.ReviewAction, "comment"; got != want {
		t.Fatalf("ReviewAction = %q, want %q", got, want)
	}
	if got, want := len(result.InlineFindings), 1; got != want {
		t.Fatalf("InlineFindings len = %d, want %d", got, want)
	}
}

func TestParseFileNormalizesOpencodeInlineFindingShape(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "result.json")
	content := `{
  "review_action": "request_changes",
  "summary": "Summary",
  "general_comments": [],
  "inline_findings": [{"file": "pkgs/test.nix", "line_start": 53, "line_end": 68, "severity": "high", "body": "Inline note"}],
  "warnings": []
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write result file: %v", err)
	}

	result, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if got, want := result.InlineFindings[0].Position.Path, "pkgs/test.nix"; got != want {
		t.Fatalf("Position.Path = %q, want %q", got, want)
	}
	if got, want := result.InlineFindings[0].Position.StartLine, 53; got != want {
		t.Fatalf("Position.StartLine = %d, want %d", got, want)
	}
	if got, want := result.InlineFindings[0].Position.EndLine, 68; got != want {
		t.Fatalf("Position.EndLine = %d, want %d", got, want)
	}
	if got, want := result.InlineFindings[0].Body, "Inline note"; got != want {
		t.Fatalf("Body = %q, want %q", got, want)
	}
}

func TestParseFileNormalizesStringGeneralComments(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "result.json")
	content := `{
  "review_action": "comment",
  "summary": "Summary",
  "general_comments": ["plain general comment"],
  "inline_findings": [],
  "warnings": []
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write result file: %v", err)
	}

	result, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if got, want := len(result.GeneralComments), 1; got != want {
		t.Fatalf("GeneralComments len = %d, want %d", got, want)
	}
	if got, want := result.GeneralComments[0].Body, "plain general comment"; got != want {
		t.Fatalf("GeneralComments[0].Body = %q, want %q", got, want)
	}
}
