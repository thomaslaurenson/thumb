package cmd

import (
	"testing"

	"github.com/thomaslaurenson/thumb/internal/entry"
)

func TestShow(t *testing.T) {
	t.Parallel()
	files := map[string]string{
		entryPath("2026-10-24"): "* yesterday's work\n",
		entryPath(testToday):    "* today's work\n",
		// Dated after today, such as an item migrated forward, so held back
		entryPath("2026-10-26"): "- tomorrow's item\n",
	}
	runTests(t, []cmdTest{
		{
			name:    "the latest entry the site would publish",
			files:   files,
			args:    []string{"show"},
			wantOut: []string{"A's .plan for Oct 25, 2026", "* today's work"},
			notOut:  []string{"tomorrow's item"},
		},
		{
			name:    "the date given",
			files:   files,
			args:    []string{"show", "2026-10-24"},
			wantOut: []string{"A's .plan for Oct 24, 2026", "* yesterday's work"},
		},
		{
			name:    "a date with no entry",
			files:   files,
			args:    []string{"show", "2026-10-01"},
			wantErr: true,
		},
		{
			name:  "a date that does not exist",
			args:  []string{"show", "2026-02-30"},
			errIs: entry.ErrBadDate,
		},
		{
			name:    "nothing published yet",
			files:   map[string]string{entryPath("2026-10-26"): "- tomorrow's item\n"},
			args:    []string{"show"},
			wantErr: true,
		},
	})
}
