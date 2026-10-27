package cmd

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thomaslaurenson/thumb/internal/store"
)

// testToday is the date the test clock reads.
const testToday = "2026-10-25"

// testNow is 09:00 local time on testToday.
var testNow = time.Date(2026, 10, 25, 9, 0, 0, 0, time.Local)

const testConf = "title=T\nauthor=A\nlogin=l\nbase_url=https://x.example\n"

// cmdTest is one row of a command's functional test.
type cmdTest struct {
	name string
	// files seeds the repository beside a valid thumb.conf, keyed by path
	// relative to its root
	files map[string]string
	// args is the command line; "{root}" in an argument becomes the
	// repository's path
	args []string
	// setup runs after the files are written. By default THUMB_DIR names the
	// repository, the working directory is its root, the clock reads testNow
	// and no editor is set.
	setup   func(t *testing.T, env *Env, root string)
	wantOut []string
	// notOut lists text stdout must not contain
	notOut []string
	// emptyOut asserts a successful run printed nothing to stdout
	emptyOut   bool
	wantErrOut []string
	wantErr    bool
	// errIs is the sentinel the error must match, where it has one
	errIs error
	// wantFiles holds the exact content files must have afterwards, keyed by
	// path relative to the root
	wantFiles map[string]string
	// check makes any further assertion
	check func(t *testing.T, root, stdout string, err error)
}

// runTests runs each row as a parallel subtest against a fresh repository and
// a fresh command tree.
func runTests(t *testing.T, tests []cmdTest) {
	t.Helper()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := newRoot(t, tc.files)
			env := Env{
				Dir:   root,
				Getwd: func() (string, error) { return root, nil },
				Now:   func() time.Time { return testNow },
			}
			if tc.setup != nil {
				tc.setup(t, &env, root)
			}
			args := make([]string, len(tc.args))
			for i, a := range tc.args {
				args[i] = strings.ReplaceAll(a, "{root}", root)
			}

			out, errOut, err := run(t, env, args...)

			wantErr := tc.wantErr || tc.errIs != nil
			if (err != nil) != wantErr {
				t.Fatalf("%v: error = %v, want error %v; stderr %q", args, err, wantErr, errOut)
			}
			if tc.errIs != nil && !errors.Is(err, tc.errIs) {
				t.Errorf("error = %v, want %v", err, tc.errIs)
			}
			if err != nil && out != "" {
				t.Errorf("failed but wrote %q to stdout", out)
			}
			for _, w := range tc.wantOut {
				if !strings.Contains(out, w) {
					t.Errorf("stdout = %q, want it to contain %q", out, w)
				}
			}
			for _, w := range tc.notOut {
				if strings.Contains(out, w) {
					t.Errorf("stdout = %q, want it not to contain %q", out, w)
				}
			}
			if tc.emptyOut && out != "" {
				t.Errorf("stdout = %q, want nothing", out)
			}
			for _, w := range tc.wantErrOut {
				if !strings.Contains(errOut, w) {
					t.Errorf("stderr = %q, want it to contain %q", errOut, w)
				}
			}
			for name, want := range tc.wantFiles {
				if got := readFile(t, filepath.Join(root, name)); got != want {
					t.Errorf("%s = %q, want %q", name, got, want)
				}
			}
			if tc.check != nil {
				tc.check(t, root, out, err)
			}
		})
	}
}

// run executes thumb with env and returns both streams and the error.
func run(t *testing.T, env Env, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	root := NewRootCmd(&out, &errOut, env)
	root.SetArgs(args)
	err = root.Execute()
	return out.String(), errOut.String(), err
}

// newRoot writes a repository holding a valid thumb.conf and files, keyed by
// path relative to its root, and returns the root.
func newRoot(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, store.ConfigName), testConf)
	for name, body := range files {
		writeFile(t, filepath.Join(root, name), body)
	}
	return root
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// assertAbsent fails the test if path exists.
func assertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s exists, want it absent (stat error %v)", path, err)
	}
}

// entryPath is where the entry for date lives under a repository root.
func entryPath(date string) string {
	return filepath.Join("plans", date[:4], date+".plan")
}

func TestRoot(t *testing.T) {
	t.Parallel()
	seeded := map[string]string{entryPath(testToday): "* seeded\n"}
	noDir := func(_ *testing.T, env *Env, _ string) { env.Dir = "" }
	runTests(t, []cmdTest{
		{
			name:    "THUMB_DIR names the repository",
			files:   seeded,
			args:    []string{"list"},
			wantOut: []string{testToday},
		},
		{
			name:    "--dir names the repository",
			files:   seeded,
			args:    []string{"--dir", "{root}", "list"},
			setup:   noDir,
			wantOut: []string{testToday},
		},
		{
			name:  "--dir wins over THUMB_DIR",
			files: seeded,
			args:  []string{"--dir", "{root}", "list"},
			setup: func(t *testing.T, env *Env, _ string) {
				env.Dir = filepath.Join(t.TempDir(), "elsewhere")
			},
			wantOut: []string{testToday},
		},
		{
			name:  "found by walking up from the working directory",
			files: seeded,
			args:  []string{"list"},
			setup: func(_ *testing.T, env *Env, root string) {
				env.Dir = ""
				env.Getwd = func() (string, error) { return filepath.Join(root, "plans", "2026"), nil }
			},
			wantOut: []string{testToday},
		},
		{
			name:    "--version",
			args:    []string{"--version"},
			wantOut: []string{"thumb version " + Version},
		},
		{
			name:  "--dir must hold a thumb.conf",
			args:  []string{"--dir", "{root}/typo", "done", "x"},
			setup: noDir,
			errIs: store.ErrNoConfig,
			check: func(t *testing.T, root, _ string, _ error) {
				assertAbsent(t, filepath.Join(root, "typo"))
			},
		},
		{
			name: "THUMB_DIR must hold a thumb.conf",
			args: []string{"done", "x"},
			setup: func(_ *testing.T, env *Env, root string) {
				env.Dir = filepath.Join(root, "typo")
			},
			errIs: store.ErrNoConfig,
			check: func(t *testing.T, root, _ string, _ error) {
				assertAbsent(t, filepath.Join(root, "typo"))
			},
		},
		{
			name: "no repository above the working directory",
			args: []string{"list"},
			setup: func(t *testing.T, env *Env, _ string) {
				env.Dir = ""
				outside := t.TempDir()
				env.Getwd = func() (string, error) { return outside, nil }
			},
			errIs: store.ErrNoRoot,
		},
		{
			name:    "unknown command",
			args:    []string{"nope"},
			wantErr: true,
		},
	})
}
