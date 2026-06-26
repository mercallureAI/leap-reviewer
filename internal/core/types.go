package core

type ReviewRequest struct {
	InstanceKey      string
	Platform         string
	Owner            string
	Repo             string
	PRNumber         int
	HeadSHA          string
	DeliveryPath     string
	TriggerType      string
	EventName        string
	CommandText      string
	TriggerUser      string
	TriggerCommentID int64
	ProfileName      string
	Publish          bool
	DryRun           bool
}

type ReviewComment struct {
	Title    string                 `json:"title" yaml:"title"`
	Body     string                 `json:"body" yaml:"body"`
	Metadata map[string]interface{} `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

type InlinePosition struct {
	Path      string `json:"path" yaml:"path"`
	StartLine int    `json:"start_line" yaml:"start_line"`
	EndLine   int    `json:"end_line" yaml:"end_line"`
	StartSide string `json:"start_side,omitempty" yaml:"start_side,omitempty"`
	EndSide   string `json:"end_side,omitempty" yaml:"end_side,omitempty"`
}

type InlineFinding struct {
	Position InlinePosition         `json:"position" yaml:"position"`
	Title    string                 `json:"title" yaml:"title"`
	Body     string                 `json:"body" yaml:"body"`
	Metadata map[string]interface{} `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

type ReviewResult struct {
	ReviewAction    string          `json:"review_action" yaml:"review_action"`
	Summary         string          `json:"summary" yaml:"summary"`
	GeneralComments []ReviewComment `json:"general_comments" yaml:"general_comments"`
	InlineFindings  []InlineFinding `json:"inline_findings" yaml:"inline_findings"`
	Warnings        []string        `json:"warnings" yaml:"warnings"`
}
