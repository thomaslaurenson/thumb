package cmd

import (
	"testing"

	"github.com/thomaslaurenson/thumb/internal/entry"
)

func TestMigrate(t *testing.T) {
	t.Parallel()
	files := map[string]string{entryPath("2026-10-24"): "- write docs\n* shipped\n"}
	runTests(t, []cmdTest{
		{
			name:       "to today by default",
			files:      files,
			args:       []string{"migrate", "2026-10-24:1"},
			wantErrOut: []string{"[~] 2026-10-24:1: moved to " + testToday + ": write docs"},
			wantFiles: map[string]string{
				entryPath("2026-10-24"): "> write docs -> " + testToday + "\n* shipped\n",
				entryPath(testToday):    "- write docs\n",
			},
		},
		{
			name:  "to the date given",
			files: files,
			args:  []string{"migrate", "2026-10-24:1", "2026-11-01"},
			wantFiles: map[string]string{
				entryPath("2026-10-24"): "> write docs -> 2026-11-01\n* shipped\n",
				entryPath("2026-11-01"): "- write docs\n",
			},
		},
		{
			name:  "to the day it is already in",
			files: files,
			args:  []string{"migrate", "2026-10-24:1", "2026-10-24"},
			errIs: entry.ErrSameDay,
		},
		{
			name:  "a done line",
			files: files,
			args:  []string{"migrate", "2026-10-24:2"},
			errIs: entry.ErrWrongMarker,
		},
		{
			name:    "a day with no entry",
			args:    []string{"migrate", "2026-10-20:1"},
			wantErr: true,
		},
		{
			name:  "a target date that does not exist",
			files: files,
			args:  []string{"migrate", "2026-10-24:1", "2026-02-30"},
			errIs: entry.ErrBadDate,
		},
		{
			name:  "not a ref",
			args:  []string{"migrate", "2026-10-24"},
			errIs: entry.ErrBadRef,
		},
	})
}
