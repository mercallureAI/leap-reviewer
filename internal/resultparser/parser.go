package resultparser

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cryolitia/gitea-ai-bot/internal/core"
)

type rawReviewResult struct {
	ReviewAction    string              `json:"review_action"`
	Summary         string              `json:"summary"`
	GeneralComments []rawGeneralComment `json:"general_comments"`
	InlineFindings  []rawInlineFinding  `json:"inline_findings"`
	Warnings        []string            `json:"warnings"`
}

type rawGeneralComment struct {
	Title string
	Body  string
}

func (r *rawGeneralComment) UnmarshalJSON(data []byte) error {
	var asString string
	if err := json.Unmarshal(data, &asString); err == nil {
		r.Body = asString
		return nil
	}

	var asObject struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := json.Unmarshal(data, &asObject); err != nil {
		return fmt.Errorf("decode general comment: %w", err)
	}
	r.Title = asObject.Title
	r.Body = asObject.Body
	return nil
}

type rawInlineFinding struct {
	Path      string `json:"path"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	LineStart int    `json:"line_start"`
	Side      string `json:"side"`
	LineEnd   int    `json:"line_end"`
	StartSide string `json:"start_side"`
	EndSide   string `json:"end_side"`
	Title     string `json:"title"`
	Body      string `json:"body"`
}

func ParseFile(path string) (core.ReviewResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return core.ReviewResult{}, err
	}

	var raw rawReviewResult
	if err := json.Unmarshal(data, &raw); err != nil {
		return core.ReviewResult{}, err
	}

	result := core.ReviewResult{
		ReviewAction:    raw.ReviewAction,
		Summary:         raw.Summary,
		GeneralComments: make([]core.ReviewComment, 0, len(raw.GeneralComments)),
		Warnings:        raw.Warnings,
		InlineFindings:  make([]core.InlineFinding, 0, len(raw.InlineFindings)),
	}
	for _, comment := range raw.GeneralComments {
		result.GeneralComments = append(result.GeneralComments, core.ReviewComment{Title: comment.Title, Body: comment.Body})
	}
	for _, finding := range raw.InlineFindings {
		path := finding.Path
		if path == "" {
			path = finding.File
		}
		startLine := finding.LineStart
		if startLine == 0 {
			startLine = finding.Line
		}
		endLine := finding.LineEnd
		if endLine == 0 {
			endLine = finding.Line
		}
		if endLine == 0 {
			endLine = startLine
		}
		startSide := finding.StartSide
		if startSide == "" {
			startSide = finding.Side
		}
		endSide := finding.EndSide
		if endSide == "" {
			endSide = finding.Side
		}
		result.InlineFindings = append(result.InlineFindings, core.InlineFinding{
			Position: core.InlinePosition{
				Path:      path,
				StartLine: startLine,
				EndLine:   endLine,
				StartSide: startSide,
				EndSide:   endSide,
			},
			Title: finding.Title,
			Body:  finding.Body,
		})
	}

	return result, nil
}
