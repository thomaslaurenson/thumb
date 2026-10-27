package cmd

import (
	"path/filepath"
	"testing"

	"github.com/thomaslaurenson/thumb/internal/entry"
)

// withEditor sets VISUAL to command, such as "true", which exits at once
// without touching the file it is given.
func withEditor(command string) func(*testing.T, *Env, string) {
	return func(_ *testing.T, env *Env, _ string) { env.Visual = command }
}

func TestEdit(t *testing.T) {
	t.Parallel()
	runTests(t, []cmdTest{
		{
			name:      "creates today's entry for the editor",
			args:      []string{"edit"},
			setup:     withEditor("true"),
			wantFiles: map[string]string{entryPath(testToday): ""},
		},
		{
			name:      "opens the date given and keeps what is there",
			files:     map[string]string{entryPath("2026-10-24"): "* kept\n"},
			args:      []string{"edit", "2026-10-24"},
			setup:     withEditor("true"),
			wantFiles: map[string]string{entryPath("2026-10-24"): "* kept\n"},
		},
		{
			name:      "EDITOR when VISUAL is unset",
			args:      []string{"edit"},
			setup:     func(_ *testing.T, env *Env, _ string) { env.Editor = "true" },
			wantFiles: map[string]string{entryPath(testToday): ""},
		},
		{
			name:    "the editor fails",
			args:    []string{"edit"},
			setup:   withEditor("false"),
			wantErr: true,
		},
		{
			name:  "no editor set creates nothing",
			args:  []string{"edit"},
			errIs: errNoEditor,
			check: func(t *testing.T, root, _ string, _ error) {
				assertAbsent(t, filepath.Join(root, entryPath(testToday)))
			},
		},
		{
			name:  "a date that does not exist",
			args:  []string{"edit", "2026-02-30"},
			setup: withEditor("true"),
			errIs: entry.ErrBadDate,
		},
	})
}
