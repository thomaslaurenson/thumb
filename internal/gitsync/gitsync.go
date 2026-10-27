// Package gitsync publishes a data repository with git: it commits the
// entries and project file, pulls with rebase, then pushes.
package gitsync

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// ErrNotRepo reports a data folder that is not inside a git repository.
var ErrNotRepo = errors.New("not a git repository")

// Result says what a sync did.
type Result struct {
	// Committed is true when there were changes to commit.
	Committed bool
}

// Sync stages paths, relative to root, commits them with message if anything
// changed, then pulls with rebase and pushes. A path that does not exist is
// skipped. Git's own output goes to out.
//
// Only paths are staged, never "git add -A" over the whole tree, so a
// half-edited thumb.conf or a stray build folder is never published by a
// routine sync.
func Sync(ctx context.Context, root string, paths []string, message string, out io.Writer) (Result, error) {
	var r Result
	if err := run(ctx, root, io.Discard, "rev-parse", "--git-dir"); err != nil {
		// Only a git that ran and refused means "not a repository"; a git that
		// could not start at all is reported as itself
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return r, fmt.Errorf("%q: %w", root, ErrNotRepo)
		}
		return r, err
	}

	var present []string
	for _, p := range paths {
		if _, err := os.Stat(filepath.Join(root, p)); err == nil {
			present = append(present, p)
		}
	}
	if len(present) > 0 {
		args := append([]string{"add", "-A", "--"}, present...)
		if err := run(ctx, root, out, args...); err != nil {
			return r, err
		}
	}

	staged, err := hasStaged(ctx, root)
	if err != nil {
		return r, err
	}
	if staged {
		if err := run(ctx, root, out, "commit", "--quiet", "-m", message); err != nil {
			return r, err
		}
		r.Committed = true
	}
	if err := run(ctx, root, out, "pull", "--rebase", "--autostash", "--quiet"); err != nil {
		return r, err
	}
	if err := run(ctx, root, out, "push", "--quiet"); err != nil {
		return r, err
	}
	return r, nil
}

// hasStaged asks git whether the index differs from HEAD. git diff --quiet
// exits 1 for "there are differences", so that exit code is an answer here
// rather than a failure; any other failure is reported.
func hasStaged(ctx context.Context, root string) (bool, error) {
	c := exec.CommandContext(ctx, "git", "-C", root, "diff", "--cached", "--quiet")
	var stderr bytes.Buffer
	c.Stderr = &stderr
	err := c.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return false, nil
	case errors.As(err, &exitErr) && exitErr.ExitCode() == 1:
		return true, nil
	default:
		return false, fmt.Errorf("git diff --cached: %w: %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
}

func run(ctx context.Context, root string, out io.Writer, args ...string) error {
	c := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	c.Stdout = out
	c.Stderr = out
	if err := c.Run(); err != nil {
		return fmt.Errorf("git %s: %w", args[0], err)
	}
	return nil
}
