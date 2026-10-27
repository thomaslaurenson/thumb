package cmd

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thomaslaurenson/thumb/internal/site"
	"github.com/thomaslaurenson/thumb/internal/store"
)

// planContains asserts the .plan built under dir, relative to the root, holds
// want.
func planContains(dir, want string) func(*testing.T, string, string, error) {
	return func(t *testing.T, root, _ string, _ error) {
		t.Helper()
		if got := readFile(t, filepath.Join(root, dir, ".plan")); !strings.Contains(got, want) {
			t.Errorf(".plan = %q, want it to contain %q", got, want)
		}
	}
}

func TestBuild(t *testing.T) {
	t.Parallel()
	files := map[string]string{entryPath(testToday): "* built\n"}
	// 11:30 UTC on 25 October: a UTC runner says the 25th, Auckland the 26th
	utcRunner := func(_ *testing.T, env *Env, _ string) {
		env.Now = func() time.Time { return time.Date(2026, 10, 25, 11, 30, 0, 0, time.UTC) }
	}
	twoDays := map[string]string{entryPath("2026-10-25"): "* a\n", entryPath("2026-10-26"): "* b\n"}
	runTests(t, []cmdTest{
		{
			name:       "into build/ by default",
			files:      files,
			args:       []string{"build"},
			wantErrOut: []string{"[*] Built 1 entry into"},
			emptyOut:   true,
			check:      planContains("build", "* built"),
		},
		{
			name:  "a relative DIR is taken from the working directory",
			files: files,
			args:  []string{"build", "site"},
			setup: func(_ *testing.T, env *Env, root string) {
				env.Getwd = func() (string, error) { return filepath.Join(root, "work"), nil }
			},
			check: planContains(filepath.Join("work", "site"), "* built"),
		},
		{
			name:  "an absolute DIR",
			files: files,
			args:  []string{"build", "{root}/out"},
			check: planContains("out", "* built"),
		},
		{
			name:       "entries dated after today are held back",
			files:      map[string]string{entryPath(testToday): "* built\n", entryPath("2026-10-26"): "- migrated\n"},
			args:       []string{"build"},
			wantErrOut: []string{"[*] Built 1 entry", "[*] Held back 1 entry dated after " + testToday},
		},
		{
			name:  "the runner's date without a timezone",
			files: twoDays,
			args:  []string{"build"},
			setup: utcRunner,
			check: planContains("build", "Updated: 2026-10-25"),
		},
		{
			name:  "the author's date with one",
			files: map[string]string{store.ConfigName: testConf + "timezone=Pacific/Auckland\n", entryPath("2026-10-25"): "* a\n", entryPath("2026-10-26"): "* b\n"},
			args:  []string{"build"},
			setup: utcRunner,
			check: planContains("build", "Updated: 2026-10-26"),
		},
		{
			name:    "refuses the repository root",
			args:    []string{"build", "{root}"},
			wantErr: true,
		},
		{
			name:    "refuses the plans folder",
			files:   files,
			args:    []string{"build", "{root}/plans"},
			wantErr: true,
		},
		{
			name:  "refuses a folder that is not a previous build",
			files: map[string]string{"notes/keep.txt": "mine\n"},
			args:  []string{"build", "{root}/notes"},
			errIs: site.ErrNotBuild,
			wantFiles: map[string]string{
				"notes/keep.txt": "mine\n",
			},
		},
		{
			name:       "a failed check builds nothing",
			files:      map[string]string{store.ProjectName: "lowercase keyword\n"},
			args:       []string{"build"},
			wantErr:    true,
			wantErrOut: []string{"[!] "},
			check: func(t *testing.T, root, _ string, _ error) {
				assertAbsent(t, filepath.Join(root, "build"))
			},
		},
		{
			name:    "more than one DIR",
			args:    []string{"build", "a", "b"},
			wantErr: true,
		},
	})
}
