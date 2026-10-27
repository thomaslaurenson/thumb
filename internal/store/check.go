package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/thomaslaurenson/thumb/internal/config"
	"github.com/thomaslaurenson/thumb/internal/entry"
)

// Problem is one finding from Check.
type Problem struct {
	// Path is the file or folder the problem is in.
	Path string
	// Message describes the problem.
	Message string
	// Fatal marks an error rather than a warning.
	Fatal bool
}

// Report collects what Check found.
type Report struct {
	Problems []Problem
	// Entries counts the entry files checked.
	Entries int
}

// Failed reports whether any problem is an error.
func (r Report) Failed() bool {
	for _, p := range r.Problems {
		if p.Fatal {
			return true
		}
	}
	return false
}

// Check validates the whole repository: the config, the project file, the
// layout of the plans tree and every entry in it. Entries dated after today
// are fine: the build may run in a time zone behind the author's.
func (s *Store) Check() (Report, error) {
	var r Report
	add := func(path, msg string, fatal bool) {
		r.Problems = append(r.Problems, Problem{Path: path, Message: msg, Fatal: fatal})
	}

	// Parse rather than Config: Config's error already names the file, and the
	// problem carries the path itself
	confPath := filepath.Join(s.Root, ConfigName)
	data, err := os.ReadFile(confPath)
	if err != nil {
		return r, fmt.Errorf("read config: %w", err)
	}
	if _, err := config.Parse(data); err != nil {
		add(confPath, err.Error(), true)
	}

	p, err := s.LoadProject()
	if err != nil {
		return r, err
	}
	for _, msg := range p.Problems() {
		add(s.ProjectPath(), msg, true)
	}

	tops, err := os.ReadDir(s.PlansDir())
	if errors.Is(err, fs.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return r, fmt.Errorf("read plans: %w", err)
	}
	for _, top := range tops {
		path := filepath.Join(s.PlansDir(), top.Name())
		if isHidden(top.Name()) {
			continue
		}
		if !top.IsDir() || !yearPattern.MatchString(top.Name()) {
			add(path, "not a YYYY folder or a .plan file inside one", true)
			continue
		}
		files, err := os.ReadDir(path)
		if err != nil {
			return r, fmt.Errorf("read plans: %w", err)
		}
		for _, f := range files {
			if isHidden(f.Name()) {
				continue
			}
			fpath := filepath.Join(path, f.Name())
			date, ok := strings.CutSuffix(f.Name(), ".plan")
			switch {
			case f.IsDir() || !ok:
				add(fpath, "not a .plan file", true)
				continue
			case !datePattern(date):
				add(fpath, "file name is not YYYY-MM-DD.plan", true)
				continue
			case !entry.ValidDate(date):
				add(fpath, fmt.Sprintf("%s is not a real date", date), true)
				continue
			case date[:4] != top.Name():
				add(fpath, fmt.Sprintf("entry belongs in the %s folder, not %s", date[:4], top.Name()), true)
				continue
			}
			r.Entries++
			e, err := s.Load(date)
			if err != nil {
				return r, err
			}
			for _, w := range e.Warnings() {
				add(fpath, w, false)
			}
		}
	}
	return r, nil
}

// isHidden reports a dot file, such as an editor swap file, which Check skips.
func isHidden(name string) bool {
	return strings.HasPrefix(name, ".")
}

// datePattern reports whether s has the YYYY-MM-DD shape, real or not.
func datePattern(s string) bool {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	for i, c := range s {
		if i != 4 && i != 7 && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}
