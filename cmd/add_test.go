package cmd

import "testing"

// TestAdd covers done, todo, idea and block, which share one constructor and
// differ only in the marker they write.
func TestAdd(t *testing.T) {
	t.Parallel()
	today := entryPath(testToday)
	runTests(t, []cmdTest{
		{
			name:       "done",
			args:       []string{"done", "fixed", "rcon"},
			wantErrOut: []string{"[+] " + testToday + ": * fixed rcon"},
			wantFiles:  map[string]string{today: "* fixed rcon\n"},
		},
		{
			name:       "todo",
			args:       []string{"todo", "write", "docs"},
			wantErrOut: []string{"[+] " + testToday + ": - write docs"},
			wantFiles:  map[string]string{today: "- write docs\n"},
		},
		{
			name:      "idea",
			args:      []string{"idea", "a", "theme"},
			wantFiles: map[string]string{today: "? a theme\n"},
		},
		{
			name:      "block",
			args:      []string{"block", "waiting", "on", "DNS"},
			wantFiles: map[string]string{today: "! waiting on DNS\n"},
		},
		{
			name:      "appends to an existing entry",
			files:     map[string]string{today: "Some prose.\n* first\n"},
			args:      []string{"done", "second"},
			wantFiles: map[string]string{today: "Some prose.\n* first\n* second\n"},
		},
		{
			name:      "flag-like words after the text stay text",
			args:      []string{"todo", "add", "a", "--date", "flag"},
			wantFiles: map[string]string{today: "- add a --date flag\n"},
		},
		{
			name:      "a short flag-like word stays text",
			args:      []string{"done", "ran", "ls", "-la"},
			wantFiles: map[string]string{today: "* ran ls -la\n"},
		},
		{
			name:      "text starting with a dash after --",
			args:      []string{"idea", "--", "-v", "for", "verbose"},
			wantFiles: map[string]string{today: "? -v for verbose\n"},
		},
		{
			name:      "--dir before the text",
			args:      []string{"todo", "--dir", "{root}", "write", "docs"},
			setup:     func(_ *testing.T, env *Env, _ string) { env.Dir = "" },
			wantFiles: map[string]string{today: "- write docs\n"},
		},
		{
			name:    "no text",
			args:    []string{"todo"},
			wantErr: true,
		},
		{
			name:    "blank text",
			args:    []string{"todo", "  "},
			wantErr: true,
		},
		{
			name:    "text over two lines",
			args:    []string{"todo", "one\ntwo"},
			wantErr: true,
		},
	})
}
