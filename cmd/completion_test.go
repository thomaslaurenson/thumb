package cmd

import (
	"fmt"
	"testing"

	"github.com/spf13/cobra"
)

// Cobra's hidden __complete command prints one candidate per line, then the
// directive as ":<number>".
var (
	noFileComp = fmt.Sprintf(":%d\n", cobra.ShellCompDirectiveNoFileComp)
	compError  = fmt.Sprintf(":%d\n", cobra.ShellCompDirectiveError)
)

func TestCompletion(t *testing.T) {
	t.Parallel()
	files := map[string]string{entryPath("2026-10-24"): "* done\n- write docs\n"}
	runTests(t, []cmdTest{
		{
			name:    "dates for a DATE argument, today included",
			files:   files,
			args:    []string{"__complete", "show", ""},
			wantOut: []string{testToday + "\n", "2026-10-24\n", noFileComp},
		},
		{
			name:    "dates filtered by what is typed",
			files:   files,
			args:    []string{"__complete", "create", "2026-10-24"},
			wantOut: []string{"2026-10-24\n"},
			notOut:  []string{testToday},
		},
		{
			name:    "open items for a REF argument, described by their text",
			files:   files,
			args:    []string{"__complete", "resolve", ""},
			wantOut: []string{"2026-10-24:2\twrite docs\n", noFileComp},
			notOut:  []string{"2026-10-24:1"},
		},
		{
			name:    "dates for migrate's second argument",
			files:   files,
			args:    []string{"__complete", "migrate", "2026-10-24:2", ""},
			wantOut: []string{testToday + "\n", noFileComp},
		},
		{
			name:    "nothing after the last argument",
			files:   files,
			args:    []string{"__complete", "resolve", "2026-10-24:2", ""},
			wantOut: []string{noFileComp},
			notOut:  []string{"2026-10-24"},
		},
		{
			name: "no repository reports an error directive",
			args: []string{"__complete", "show", ""},
			setup: func(_ *testing.T, env *Env, root string) {
				env.Dir = root + "/typo"
			},
			wantOut: []string{compError},
		},
		{
			name:    "the completion command is kept",
			args:    []string{"completion", "bash"},
			wantOut: []string{"bash completion"},
		},
	})
}
