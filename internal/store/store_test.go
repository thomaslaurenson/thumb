package store

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/thomaslaurenson/thumb/internal/entry"
	"github.com/thomaslaurenson/thumb/internal/project"
)

const conf = "title=Test .plan\nauthor=Test User\nlogin=tester\nbase_url=https://plan.example.com/\n"

// newRepo writes files, keyed by path relative to the root, into a fresh
// repository with a valid thumb.conf.
func newRepo(t *testing.T, files map[string]string) *Store {
	t.Helper()
	root := t.TempDir()
	files[ConfigName] = conf
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return New(root)
}

func TestFind(t *testing.T) {
	t.Parallel()
	s := newRepo(t, map[string]string{"plans/2026/2026-10-24.plan": "* a\n"})
	got, err := Find(filepath.Join(s.Root, "plans", "2026"))
	if err != nil {
		t.Fatal(err)
	}
	if got != s.Root {
		t.Errorf("Find = %q, want %q", got, s.Root)
	}
	if _, err := Find(t.TempDir()); !errors.Is(err, ErrNoRoot) {
		t.Errorf("Find outside a repo error = %v, want ErrNoRoot", err)
	}
}

func TestOpen(t *testing.T) {
	t.Parallel()
	repo := newRepo(t, map[string]string{})
	confDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(confDir, ConfigName), 0o755); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		root    string
		wantErr error
	}{
		{name: "repository", root: repo.Root},
		{name: "folder without thumb.conf", root: t.TempDir(), wantErr: ErrNoConfig},
		{name: "missing folder", root: filepath.Join(t.TempDir(), "typo"), wantErr: ErrNoConfig},
		{name: "thumb.conf is a folder", root: confDir, wantErr: ErrNoConfig},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, err := Open(tc.root)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Open error = %v, want %v", err, tc.wantErr)
			}
			if err == nil && s.Root != tc.root {
				t.Errorf("Open root = %q, want %q", s.Root, tc.root)
			}
		})
	}
}

func TestDates(t *testing.T) {
	t.Parallel()
	s := newRepo(t, map[string]string{
		"plans/2026/2026-10-24.plan": "* a\n",
		"plans/2025/2025-12-31.plan": "* b\n",
		"plans/2026/2026-10-20.plan": "",
		"plans/2026/notes.plan":      "x\n",
		"plans/2025/2026-01-01.plan": "wrong folder\n",
	})
	got, err := s.Dates()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2025-12-31", "2026-10-20", "2026-10-24"}
	if !slices.Equal(got, want) {
		t.Errorf("Dates = %v, want %v", got, want)
	}
}

func TestDatesWithoutPlansFolder(t *testing.T) {
	t.Parallel()
	got, err := newRepo(t, map[string]string{}).Dates()
	if err != nil || got != nil {
		t.Errorf("Dates = %v, %v, want nil, nil", got, err)
	}
}

func TestLoadMissingIsEmpty(t *testing.T) {
	t.Parallel()
	e, err := newRepo(t, map[string]string{}).Load("2026-10-24")
	if err != nil {
		t.Fatal(err)
	}
	if !e.Empty() || e.Date != "2026-10-24" {
		t.Errorf("Load of a missing day = %+v", e)
	}
}

func TestSaveCreatesYearFolderAndKeepsMode(t *testing.T) {
	t.Parallel()
	s := newRepo(t, map[string]string{"plans/2026/2026-10-20.plan": "- a\n"})
	path := s.EntryPath("2026-10-20")
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	e, err := s.Load("2026-10-20")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Resolve(1); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(e); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want 0640", info.Mode().Perm())
	}

	fresh := entry.New("2027-01-01")
	fresh.Append(entry.Line{Marker: entry.Done, Text: "new year"})
	if err := s.Save(fresh); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(s.EntryPath("2027-01-01"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "* new year\n" {
		t.Errorf("saved = %q", data)
	}
}

func TestSaveLeavesNoTemporaryFiles(t *testing.T) {
	t.Parallel()
	s := newRepo(t, map[string]string{"plans/2026/2026-10-20.plan": "- a\n"})
	e, err := s.Load("2026-10-20")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save(e); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(filepath.Join(s.PlansDir(), "2026"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Errorf("year folder holds %d files, want 1", len(files))
	}
}

func TestProjectRoundTrip(t *testing.T) {
	t.Parallel()
	s := newRepo(t, map[string]string{})
	p, err := s.LoadProject()
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Lines) != 0 {
		t.Errorf("missing project should be empty, got %q", p.Lines)
	}
	p = project.Parse([]byte("# now\nBUILDING thumb\n"))
	if err := s.SaveProject(p); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadProject()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Lines, p.Lines) {
		t.Errorf("project = %q, want %q", got.Lines, p.Lines)
	}
}

func TestCheck(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		files      map[string]string
		wantFailed bool
		wantMsg    string
	}{
		{name: "valid tree", files: map[string]string{"plans/2026/2026-10-24.plan": "* a\n"}},
		{name: "no plans folder", files: map[string]string{}},
		{name: "future date is fine", files: map[string]string{"plans/2099/2099-01-01.plan": "* a\n"}},
		{name: "hidden swap file ignored", files: map[string]string{"plans/2026/.2026-10-24.plan.swp": "x"}},
		{name: "bad name", files: map[string]string{"plans/2026/notes.plan": "x\n"}, wantFailed: true, wantMsg: "file name is not YYYY-MM-DD.plan"},
		{name: "impossible date", files: map[string]string{"plans/2026/2026-02-30.plan": "x\n"}, wantFailed: true, wantMsg: "2026-02-30 is not a real date"},
		{name: "wrong year folder", files: map[string]string{"plans/2025/2026-01-01.plan": "x\n"}, wantFailed: true, wantMsg: "entry belongs in the 2026 folder, not 2025"},
		{name: "stray file", files: map[string]string{"plans/README.md": "x\n"}, wantFailed: true, wantMsg: "not a YYYY folder or a .plan file inside one"},
		{name: "ambiguous line warns only", files: map[string]string{"plans/2026/2026-10-24.plan": "*fixed\n"}, wantMsg: "line 1 has no space after its marker, read as prose"},
		{name: "bad project line", files: map[string]string{"project": "building lowercase\n"}, wantFailed: true, wantMsg: "line 1: expected an uppercase KEYWORD then a description"},
		{name: "project comments allowed", files: map[string]string{"project": "# status\n\nBUILDING thumb\n"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r, err := newRepo(t, tc.files).Check()
			if err != nil {
				t.Fatal(err)
			}
			if r.Failed() != tc.wantFailed {
				t.Errorf("Failed = %v, want %v: %+v", r.Failed(), tc.wantFailed, r.Problems)
			}
			if tc.wantMsg == "" {
				if len(r.Problems) != 0 {
					t.Errorf("unexpected problems: %+v", r.Problems)
				}
				return
			}
			if !slices.ContainsFunc(r.Problems, func(p Problem) bool { return p.Message == tc.wantMsg }) {
				t.Errorf("no problem %q in %+v", tc.wantMsg, r.Problems)
			}
		})
	}
}

func TestCheckBadConfig(t *testing.T) {
	t.Parallel()
	s := newRepo(t, map[string]string{})
	if err := os.WriteFile(filepath.Join(s.Root, ConfigName), []byte("title=x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := s.Check()
	if err != nil {
		t.Fatal(err)
	}
	if !r.Failed() {
		t.Error("a config missing keys should fail the check")
	}
	// The problem carries the path, so the message must not repeat it
	for _, p := range r.Problems {
		if strings.Contains(p.Message, s.Root) {
			t.Errorf("problem message %q repeats the path %q", p.Message, p.Path)
		}
	}
}

func TestFindSkipsConfigFolder(t *testing.T) {
	t.Parallel()
	start := filepath.Join(t.TempDir(), ConfigName, "sub")
	if err := os.MkdirAll(start, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Find(start); !errors.Is(err, ErrNoRoot) {
		t.Errorf("Find below a thumb.conf folder error = %v, want ErrNoRoot", err)
	}
}

func TestCreateProject(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{name: "creates a missing file", files: map[string]string{}, want: ""},
		{name: "keeps an existing file", files: map[string]string{ProjectName: "BUILDING thumb\n"}, want: "BUILDING thumb\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newRepo(t, tc.files)
			if err := s.CreateProject(); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(s.ProjectPath())
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != tc.want {
				t.Errorf("project = %q, want %q", b, tc.want)
			}
		})
	}
}
