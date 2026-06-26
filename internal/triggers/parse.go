package triggers

import "strings"

type ReviewCommand struct {
	Raw         string
	CommandType string
	ProfileName string
}

func ParseReviewCommand(body, defaultProfile string) (ReviewCommand, bool) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return ReviewCommand{}, false
	}

	parts := strings.Fields(trimmed)
	if len(parts) == 0 || parts[0] != "/review" {
		return ReviewCommand{}, false
	}

	profile := defaultProfile
	if len(parts) > 1 {
		profile = parts[1]
	}

	return ReviewCommand{
		Raw:         strings.Join(parts, " "),
		CommandType: "review",
		ProfileName: profile,
	}, true
}
