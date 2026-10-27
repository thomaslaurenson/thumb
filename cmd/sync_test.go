package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thomaslaurenson/thumb/internal/gitsync"
	"github.com/thomaslaurenson/thumb/internal/store"
)

// git runs a git command for test setup with a fixed identity and none of the
// developer's own configuration, failing the test on error.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	c.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// gitRepo makes root a repository that pushes to a bare remote, with commit
// paths committed and pushed, and returns the remote.
func gitRepo(t *testing.T, root string, commit ...string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	remote := filepath.Join(t.TempDir(), "remote.git")
	git(t, root, "init", "--quiet", "--bare", "--initial-branch=main", remote)
	git(t, root, "init", "--quiet", "--initial-branch=main")
	git(t, root, "config", "user.name", "Test")
	git(t, root, "config", "user.email", "test@example.com")
	git(t, root, append([]string{"add", "--"}, commit...)...)
	git(t, root, "commit", "--quiet", "-m", "Seeded")
	git(t, root, "remote", "add", "origin", remote)
	git(t, root, "push", "--quiet", "-u", "origin", "main")
	return remote
}

// remoteHead returns the subject of the commit at the head of the remote's
// main branch.
func remoteHead(t *testing.T, remote string) string {
	t.Helper()
	return git(t, remote, "log", "-1", "--format=%s", "main")
}

func TestSync(t *testing.T) {
	t.Parallel()
	entry := map[string]string{entryPath(testToday): "* synced\n"}
	runTests(t, []cmdTest{
		{
			name:       "commits and pushes the entries",
			files:      entry,
			args:       []string{"sync"},
			wantErrOut: []string{"[+] Committed and pushed"},
			emptyOut:   true,
			setup: func(t *testing.T, _ *Env, root string) {
				remote := gitRepo(t, root, store.ConfigName)
				t.Cleanup(func() {
					if got := remoteHead(t, remote); got != "Updated plans for "+testToday {
						t.Errorf("remote head = %q, want the sync commit", got)
					}
				})
			},
		},
		{
			name:       "nothing new to commit",
			files:      entry,
			args:       []string{"sync"},
			wantErrOut: []string{"[*] Nothing new to commit"},
			setup: func(t *testing.T, _ *Env, root string) {
				gitRepo(t, root, ".")
			},
		},
		{
			name:       "a failed check syncs nothing",
			files:      map[string]string{store.ProjectName: "lowercase keyword\n"},
			args:       []string{"sync"},
			wantErr:    true,
			wantErrOut: []string{"[!] "},
			setup: func(t *testing.T, _ *Env, root string) {
				remote := gitRepo(t, root, store.ConfigName)
				t.Cleanup(func() {
					if got := remoteHead(t, remote); got != "Seeded" {
						t.Errorf("remote head = %q, want nothing pushed", got)
					}
				})
			},
		},
		{
			name:  "outside a git repository",
			args:  []string{"sync"},
			errIs: gitsync.ErrNotRepo,
			setup: func(t *testing.T, _ *Env, _ string) {
				if _, err := exec.LookPath("git"); err != nil {
					t.Skip("git not installed")
				}
			},
		},
		{
			name:    "an argument",
			args:    []string{"sync", "extra"},
			wantErr: true,
		},
	})
}
