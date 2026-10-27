// Package store reads and writes a data repository: thumb.conf at the root,
// the project file beside it, and one plans/YYYY/YYYY-MM-DD.plan file per day.
package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/thomaslaurenson/thumb/internal/config"
	"github.com/thomaslaurenson/thumb/internal/entry"
	"github.com/thomaslaurenson/thumb/internal/project"
)

// File and folder names inside a data repository.
const (
	ConfigName  = "thumb.conf"
	ProjectName = "project"
	PlansName   = "plans"
)

var (
	// ErrNoRoot reports that no folder above the start holds a thumb.conf.
	ErrNoRoot = errors.New("no thumb.conf found in this folder or any above it")
	// ErrNoConfig reports a folder named as the data repository that holds no
	// thumb.conf.
	ErrNoConfig = errors.New("no thumb.conf in this folder")
)

var yearPattern = regexp.MustCompile(`^[0-9]{4}$`)

// Find walks up from start to the first folder holding a thumb.conf.
func Find(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if info, err := os.Stat(filepath.Join(dir, ConfigName)); err == nil && !info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%q: %w", start, ErrNoRoot)
		}
		dir = parent
	}
}

// Store is a data repository rooted at Root.
type Store struct {
	Root string
}

// New returns the store rooted at root, without checking that it is one.
func New(root string) *Store {
	return &Store{Root: root}
}

// Open returns the store rooted at root, which must hold a thumb.conf.
//
// A missing folder is not treated as an empty repository: Save creates folders
// as it goes, so a mistyped root would otherwise collect a new plans tree of
// its own and every listing of it would come back empty without complaint.
func Open(root string) (*Store, error) {
	info, err := os.Stat(filepath.Join(root, ConfigName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%q: %w", root, ErrNoConfig)
	}
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", root, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%q: %w", root, ErrNoConfig)
	}
	return New(root), nil
}

// PlansDir is the folder the entries live in.
func (s *Store) PlansDir() string {
	return filepath.Join(s.Root, PlansName)
}

// EntryPath is where the entry for date lives, whether or not it exists.
func (s *Store) EntryPath(date string) string {
	return filepath.Join(s.PlansDir(), date[:4], date+".plan")
}

// Config reads thumb.conf.
func (s *Store) Config() (config.Config, error) {
	return config.Load(filepath.Join(s.Root, ConfigName))
}

// Dates lists the dates that have an entry file, oldest first. Files whose
// names are not real dates are left out; Check reports them.
func (s *Store) Dates() ([]string, error) {
	years, err := os.ReadDir(s.PlansDir())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read plans: %w", err)
	}
	var dates []string
	for _, y := range years {
		if !y.IsDir() || !yearPattern.MatchString(y.Name()) {
			continue
		}
		files, err := os.ReadDir(filepath.Join(s.PlansDir(), y.Name()))
		if err != nil {
			return nil, fmt.Errorf("read plans: %w", err)
		}
		for _, f := range files {
			date, ok := strings.CutSuffix(f.Name(), ".plan")
			if ok && !f.IsDir() && entry.ValidDate(date) && date[:4] == y.Name() {
				dates = append(dates, date)
			}
		}
	}
	slices.Sort(dates)
	return dates, nil
}

// Exists reports whether date has an entry file.
func (s *Store) Exists(date string) bool {
	_, err := os.Stat(s.EntryPath(date))
	return err == nil
}

// Load reads the entry for date. A date with no file gives an empty entry.
func (s *Store) Load(date string) (*entry.Entry, error) {
	data, err := os.ReadFile(s.EntryPath(date))
	if errors.Is(err, fs.ErrNotExist) {
		return entry.New(date), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read entry %s: %w", date, err)
	}
	return entry.Parse(date, data), nil
}

// LoadAll reads every entry, oldest first.
func (s *Store) LoadAll() ([]*entry.Entry, error) {
	dates, err := s.Dates()
	if err != nil {
		return nil, err
	}
	entries := make([]*entry.Entry, 0, len(dates))
	for _, d := range dates {
		e, err := s.Load(d)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// Save writes an entry, creating its year folder if needed.
func (s *Store) Save(e *entry.Entry) error {
	path := s.EntryPath(e.Date)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create year folder: %w", err)
	}
	return writeAtomic(path, e.Bytes())
}

// ProjectPath is where the project file lives, whether or not it exists.
func (s *Store) ProjectPath() string {
	return filepath.Join(s.Root, ProjectName)
}

// CreateProject creates an empty project file if there is none, so an editor
// can open it. An existing file is left untouched.
func (s *Store) CreateProject() error {
	f, err := os.OpenFile(s.ProjectPath(), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create project file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("create project file: %w", err)
	}
	return nil
}

// LoadProject reads the project file. A missing file gives an empty project.
func (s *Store) LoadProject() (*project.Project, error) {
	data, err := os.ReadFile(s.ProjectPath())
	if errors.Is(err, fs.ErrNotExist) {
		return &project.Project{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read project: %w", err)
	}
	return project.Parse(data), nil
}

// SaveProject writes the project file.
func (s *Store) SaveProject(p *project.Project) error {
	return writeAtomic(s.ProjectPath(), p.Bytes())
}

// writeAtomic replaces path with data through a temporary file in the same
// folder, so an interrupted write never leaves a truncated file. An existing
// file keeps its permissions; CreateTemp alone would make it 0600.
func writeAtomic(path string, data []byte) (err error) {
	mode := fs.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	defer func() {
		if err != nil {
			os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write %q: %w", path, err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	if err = os.Chmod(tmp.Name(), mode); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}
