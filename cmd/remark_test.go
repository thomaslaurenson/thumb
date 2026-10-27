package cmd

import (
	"testing"

	"github.com/thomaslaurenson/thumb/internal/entry"
)

// remarkFiles is an entry with one line of each kind resolve and drop treat
// differently.
var remarkFiles = map[string]string{
	entryPath("2026-10-24"): "- write docs\n! waiting on DNS\n? a theme\n* shipped\nSome prose.\n",
}

func TestResolve(t *testing.T) {
	t.Parallel()
	runTests(t, []cmdTest{
		{
			name:       "a to do line",
			files:      remarkFiles,
			args:       []string{"resolve", "2026-10-24:1"},
			wantErrOut: []string{"[~] 2026-10-24:1: - -> + write docs"},
			wantFiles: map[string]string{
				entryPath("2026-10-24"): "+ write docs\n! waiting on DNS\n? a theme\n* shipped\nSome prose.\n",
			},
		},
		{
			name:       "a blocked line",
			files:      remarkFiles,
			args:       []string{"resolve", "2026-10-24:2"},
			wantErrOut: []string{"! -> +"},
		},
		{
			name:  "an idea",
			files: remarkFiles,
			args:  []string{"resolve", "2026-10-24:3"},
			errIs: entry.ErrWrongMarker,
		},
		{
			name:  "a prose line",
			files: remarkFiles,
			args:  []string{"resolve", "2026-10-24:5"},
			errIs: entry.ErrNotPlanLine,
		},
		{
			name:  "a line past the end",
			files: remarkFiles,
			args:  []string{"resolve", "2026-10-24:9"},
			errIs: entry.ErrNoLine,
		},
		{
			name:    "a day with no entry",
			args:    []string{"resolve", "2026-10-24:1"},
			wantErr: true,
		},
		{
			name:  "not a ref",
			args:  []string{"resolve", "yesterday"},
			errIs: entry.ErrBadRef,
		},
		{
			name:    "no ref",
			args:    []string{"resolve"},
			wantErr: true,
		},
	})
}

func TestDrop(t *testing.T) {
	t.Parallel()
	runTests(t, []cmdTest{
		{
			name:       "a to do line",
			files:      remarkFiles,
			args:       []string{"drop", "2026-10-24:1"},
			wantErrOut: []string{"[~] 2026-10-24:1: - -> ~ write docs"},
			wantFiles: map[string]string{
				entryPath("2026-10-24"): "~ write docs\n! waiting on DNS\n? a theme\n* shipped\nSome prose.\n",
			},
		},
		{
			name:       "an idea",
			files:      remarkFiles,
			args:       []string{"drop", "2026-10-24:3"},
			wantErrOut: []string{"? -> ~"},
		},
		{
			name:  "a done line",
			files: remarkFiles,
			args:  []string{"drop", "2026-10-24:4"},
			errIs: entry.ErrWrongMarker,
		},
		{
			name:  "not a ref",
			args:  []string{"drop", "2026-10-24"},
			errIs: entry.ErrBadRef,
		},
	})
}
