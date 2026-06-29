package triggers

import "testing"

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		commandType string
		profile     string
		question    string
		commandText string
	}{
		{name: "review default profile", body: "/review", commandType: "review", profile: "default", commandText: "/review"},
		{name: "review explicit profile", body: "/review security", commandType: "review", profile: "security", commandText: "/review security"},
		{name: "ask default profile", body: "/ask why this change", commandType: "ask", profile: "default", question: "why this change", commandText: "/ask why this change"},
		{name: "ask explicit profile", body: "/ask security why this change", commandType: "ask", profile: "security", question: "why this change", commandText: "/ask security why this change"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, ok := ParseCommand(tt.body, "default", []string{"default", "security"})
			if !ok {
				t.Fatal("ParseCommand() ok = false, want true")
			}
			if got, want := command.CommandType, tt.commandType; got != want {
				t.Fatalf("CommandType = %q, want %q", got, want)
			}
			if got, want := command.ProfileName, tt.profile; got != want {
				t.Fatalf("ProfileName = %q, want %q", got, want)
			}
			if got, want := command.Question, tt.question; got != want {
				t.Fatalf("Question = %q, want %q", got, want)
			}
			if got, want := command.Raw, tt.commandText; got != want {
				t.Fatalf("Raw = %q, want %q", got, want)
			}
		})
	}
}

func TestParseCommandRejectsUnsupportedCommands(t *testing.T) {
	if _, ok := ParseCommand("hello there", "default", []string{"default", "security"}); ok {
		t.Fatal("ParseCommand() ok = true, want false")
	}
}

func TestParseCommandRejectsAskWithoutQuestion(t *testing.T) {
	if _, ok := ParseCommand("/ask", "default", []string{"default", "security"}); ok {
		t.Fatal("ParseCommand() ok = true, want false")
	}
}
