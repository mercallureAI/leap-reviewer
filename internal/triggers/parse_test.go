package triggers

import "testing"

func TestParseReviewCommand(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		profile     string
		commandText string
	}{
		{name: "default profile", body: "/review", profile: "default", commandText: "/review"},
		{name: "explicit profile", body: "/review security", profile: "security", commandText: "/review security"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, ok := ParseReviewCommand(tt.body, "default")
			if !ok {
				t.Fatal("ParseReviewCommand() ok = false, want true")
			}
			if got, want := command.ProfileName, tt.profile; got != want {
				t.Fatalf("ProfileName = %q, want %q", got, want)
			}
			if got, want := command.Raw, tt.commandText; got != want {
				t.Fatalf("Raw = %q, want %q", got, want)
			}
		})
	}
}

func TestParseReviewCommandRejectsUnsupportedCommands(t *testing.T) {
	if _, ok := ParseReviewCommand("hello there", "default"); ok {
		t.Fatal("ParseReviewCommand() ok = true, want false")
	}
}
