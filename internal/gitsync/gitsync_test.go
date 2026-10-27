package gitsync

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// git runs a git command for test setup, failing the test on error.
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

// newClone makes a bare remote with one commit and returns a clone of it with
// a committer identity configured.
func newClone(t *testing.T) (clone, remote string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	base := t.TempDir()
	remote = filepath.Join(base, "remote.git")
	seed := filepath.Join(base, "seed")
	clone = filepath.Join(base, "clone")
	git(t, base, "init", "--quiet", "--bare", "--initial-branch=main", remote)
	git(t, base, "init", "--quiet", "--initial-branch=main", seed)
	if err := os.WriteFile(filepath.Join(seed, "thumb.conf"), []byte("title=T\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, seed, "add", "thumb.conf")
	git(t, seed, "commit", "--quiet", "-m", "Seeded")
	git(t, seed, "push", "--quiet", remote, "main")
	git(t, base, "clone", "--quiet", remote, clone)
	git(t, clone, "config", "user.name", "Test")
	git(t, clone, "config", "user.email", "test@example.com")
	return clone, remote
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// syncPaths are the paths thumb stages, as cmd passes them.
var syncPaths = []string{"plans", "project"}

func TestSyncCommitsAndPushes(t *testing.T) {
	t.Parallel()
	clone, remote := newClone(t)
	write(t, filepath.Join(clone, "plans", "2026", "2026-10-24.plan"), "* synced\n")
	write(t, filepath.Join(clone, "project"), "BUILDING thumb\n")
	write(t, filepath.Join(clone, "thumb.conf"), "title=edited but not published\n")

	r, err := Sync(t.Context(), clone, syncPaths, "Updated plans for 2026-10-24", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Committed {
		t.Error("expected a commit")
	}
	if got := git(t, remote, "log", "-1", "--format=%s", "main"); got != "Updated plans for 2026-10-24" {
		t.Errorf("remote head message = %q", got)
	}
	files := git(t, remote, "show", "--name-only", "--format=", "main")
	if !strings.Contains(files, "plans/2026/2026-10-24.plan") || !strings.Contains(files, "project") {
		t.Errorf("pushed commit files = %q", files)
	}
	if strings.Contains(files, "thumb.conf") {
		t.Error("thumb.conf should not be staged by a sync")
	}
}

func TestSyncWithNothingToCommit(t *testing.T) {
	t.Parallel()
	clone, _ := newClone(t)
	r, err := Sync(t.Context(), clone, syncPaths, "unused", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if r.Committed {
		t.Error("nothing changed, so nothing should be committed")
	}
}

func TestSyncOutsideRepo(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	_, err := Sync(t.Context(), t.TempDir(), syncPaths, "x", io.Discard)
	if !errors.Is(err, ErrNotRepo) {
		t.Errorf("Sync error = %v, want ErrNotRepo", err)
	}
}

func TestSyncWhenGitCannotRun(t *testing.T) {
	t.Parallel()
	clone, _ := newClone(t)
	// A cancelled context stops git before it starts, the same failure as a
	// git missing from PATH, which a parallel test cannot arrange
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := Sync(ctx, clone, syncPaths, "x", io.Discard)
	if errors.Is(err, ErrNotRepo) {
		t.Errorf("Sync error = %v, want the failure itself rather than ErrNotRepo", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Sync error = %v, want context.Canceled", err)
	}
}
