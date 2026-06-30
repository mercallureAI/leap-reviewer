package triggers

import "strings"

type Command struct {
	Raw         string
	CommandType string
	ProfileName string
	Question     string
}

func ParseCommand(body, defaultProfile string, enabledProfiles []string) (Command, bool) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return Command{}, false
	}

	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return Command{}, false
	}

	switch parts[0] {
	case "/review":
		profile := defaultProfile
		if len(parts) > 1 {
			profile = parts[1]
		}
		return Command{
			Raw:         strings.Join(parts, " "),
			CommandType: "review",
			ProfileName: profile,
		}, true
	case "/ask":
		if len(parts) < 2 {
			return Command{}, false
		}

		profile := defaultProfile
		questionStart := 1
		if len(parts) > 2 && contains(enabledProfiles, parts[1]) {
			profile = parts[1]
			questionStart = 2
		}
		question := strings.TrimSpace(strings.Join(parts[questionStart:], " "))
		if question == "" {
			return Command{}, false
		}
		return Command{
			Raw:         strings.Join(parts, " "),
			CommandType: "ask",
			ProfileName: profile,
			Question:    question,
		}, true
	case "/summarize":
		profile := defaultProfile
		if len(parts) > 1 {
			profile = parts[1]
		}
		return Command{
			Raw:         strings.Join(parts, " "),
			CommandType: "summarize",
			ProfileName: profile,
		}, true
	default:
		return Command{}, false
	}
}

func ParseReviewCommand(body, defaultProfile string, enabledProfiles []string) (Command, bool) {
	command, ok := ParseCommand(body, defaultProfile, enabledProfiles)
	if !ok || command.CommandType != "review" {
		return Command{}, false
	}
	return command, true
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
