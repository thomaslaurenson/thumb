package site

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrNotBuild reports an output folder that holds something other than a
// previous build, which Write refuses to replace.
var ErrNotBuild = errors.New("refusing to replace a folder that is not a previous build")

// Write replaces the folder out with files. The new site is written beside
// it first and moved into place only once complete, so a failed build never
// leaves a half-written site.
//
// Replacing out deletes it, so only an empty folder or a previous build,
// recognised by its index.html and .plan, may be replaced. A typo such as
// "thumb build ~/Documents" stops here instead.
func Write(out string, files map[string][]byte) (err error) {
	out = filepath.Clean(out)
	if filepath.Dir(out) == out {
		return fmt.Errorf("%q: %w", out, ErrNotBuild)
	}
	if err := checkReplaceable(out); err != nil {
		return err
	}
	parent := filepath.Dir(out)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create %q: %w", parent, err)
	}
	tmp, err := os.MkdirTemp(parent, filepath.Base(out)+".tmp.*")
	if err != nil {
		return fmt.Errorf("create build folder: %w", err)
	}
	defer func() {
		if err != nil {
			os.RemoveAll(tmp)
		}
	}()
	if err = os.Chmod(tmp, 0o755); err != nil {
		return fmt.Errorf("create build folder: %w", err)
	}
	for name, data := range files {
		path := filepath.Join(tmp, filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
		if err = os.WriteFile(path, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	if err = os.RemoveAll(out); err != nil {
		return fmt.Errorf("replace %q: %w", out, err)
	}
	if err = os.Rename(tmp, out); err != nil {
		return fmt.Errorf("replace %q: %w", out, err)
	}
	return nil
}

func checkReplaceable(out string) error {
	items, err := os.ReadDir(out)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && len(items) == 0) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %q: %w", out, err)
	}
	for _, marker := range []string{"index.html", ".plan"} {
		if _, err := os.Stat(filepath.Join(out, marker)); err != nil {
			return fmt.Errorf("%q: %w", out, ErrNotBuild)
		}
	}
	return nil
}
