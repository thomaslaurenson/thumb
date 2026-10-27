package cmd

import "testing"

func TestOpen(t *testing.T) {
	t.Parallel()
	runTests(t, []cmdTest{
		{
			name: "to do and blocked lines with their refs",
			files: map[string]string{
				entryPath("2026-10-24"): "* done\n- write docs\n",
				entryPath(testToday):    "? an idea\n! waiting on DNS\n",
			},
			args:    []string{"open"},
			wantOut: []string{"2026-10-24:2", "- write docs", testToday + ":2", "! waiting on DNS"},
			notOut:  []string{"* done", "an idea"},
		},
		{
			name:     "nothing open prints nothing",
			files:    map[string]string{entryPath(testToday): "* all done\n"},
			args:     []string{"open"},
			emptyOut: true,
		},
		{
			name:    "an argument",
			args:    []string{"open", "extra"},
			wantErr: true,
		},
	})
}
