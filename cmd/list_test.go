package cmd

import (
	"strings"
	"testing"
)

func TestList(t *testing.T) {
	t.Parallel()
	runTests(t, []cmdTest{
		{
			name: "newest first with a tally",
			files: map[string]string{
				entryPath("2026-10-23"): "",
				entryPath("2026-10-24"): "* one\n* two\n- three\n",
				entryPath(testToday):    "? an idea\n",
			},
			args:    []string{"list"},
			wantOut: []string{"1 idea", "2 done, 1 open", "(empty)"},
			check: func(t *testing.T, _, stdout string, _ error) {
				a := strings.Index(stdout, testToday)
				b := strings.Index(stdout, "2026-10-24")
				c := strings.Index(stdout, "2026-10-23")
				if a < 0 || a > b || b > c {
					t.Errorf("stdout = %q, want the dates newest first", stdout)
				}
			},
		},
		{
			name:     "no entries prints nothing",
			args:     []string{"list"},
			emptyOut: true,
		},
		{
			name:    "an argument",
			args:    []string{"list", "extra"},
			wantErr: true,
		},
	})
}
