package cmd

import (
	"testing"

	"github.com/thomaslaurenson/thumb/internal/entry"
)

func TestCreate(t *testing.T) {
	t.Parallel()
	runTests(t, []cmdTest{
		{
			name:       "today by default",
			args:       []string{"create"},
			wantOut:    []string{entryPath(testToday)},
			wantErrOut: []string{"[+] Created"},
			wantFiles:  map[string]string{entryPath(testToday): ""},
		},
		{
			name:      "the date given",
			args:      []string{"create", "2026-11-01"},
			wantOut:   []string{entryPath("2026-11-01")},
			wantFiles: map[string]string{entryPath("2026-11-01"): ""},
		},
		{
			name:       "an existing entry is kept",
			files:      map[string]string{entryPath(testToday): "* kept\n"},
			args:       []string{"create"},
			wantOut:    []string{entryPath(testToday)},
			wantErrOut: []string{"[*] Exists"},
			wantFiles:  map[string]string{entryPath(testToday): "* kept\n"},
		},
		{
			name:  "a date that does not exist",
			args:  []string{"create", "2026-02-30"},
			errIs: entry.ErrBadDate,
		},
		{
			name:    "more than one date",
			args:    []string{"create", "2026-11-01", "2026-11-02"},
			wantErr: true,
		},
	})
}
