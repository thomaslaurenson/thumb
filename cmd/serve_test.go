package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thomaslaurenson/thumb/internal/store"
)

// lockedBuffer is a bytes.Buffer the command's goroutine can write while the
// test reads it.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

var _ io.Writer = (*lockedBuffer)(nil)

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

var servingURL = regexp.MustCompile(`http://127\.0\.0\.1:[0-9]+`)

// eventually polls cond until it holds, failing the test after five seconds.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func get(t *testing.T, url string) string {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// startServe runs thumb with args against root in the background. The
// returned stop cancels it and returns its error, and also runs when the test
// ends, so a failing test never leaves the server running.
func startServe(t *testing.T, root string, errOut io.Writer, args ...string) (stop func() error) {
	t.Helper()
	env := Env{
		Dir:   root,
		Getwd: func() (string, error) { return root, nil },
		Now:   func() time.Time { return testNow },
	}
	c := NewRootCmd(io.Discard, errOut, env)
	c.SetArgs(args)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	done := make(chan error, 1)
	go func() { done <- c.ExecuteContext(ctx) }()
	stop = sync.OnceValue(func() error {
		cancel()
		return <-done
	})
	t.Cleanup(func() { _ = stop() })
	return stop
}

func TestServe(t *testing.T) {
	t.Parallel()
	runTests(t, []cmdTest{
		{
			name:       "a failed check serves nothing",
			files:      map[string]string{store.ProjectName: "lowercase keyword\n"},
			args:       []string{"serve", "--port", "0"},
			errIs:      errCheckFailed,
			wantErrOut: []string{"[!] "},
		},
		{
			name:    "takes no arguments",
			args:    []string{"serve", "build"},
			wantErr: true,
		},
	})
}

func TestServePortInUse(t *testing.T) {
	t.Parallel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	stop := startServe(t, newRoot(t, nil), io.Discard, "serve", "--port", port)
	if err := stop(); err == nil || errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want the port in use reported", err)
	}
}

func TestServeRebuildsOnSave(t *testing.T) {
	t.Parallel()
	root := newRoot(t, map[string]string{entryPath(testToday): "* first\n"})
	var errOut lockedBuffer
	stop := startServe(t, root, &errOut, "serve", "--port", "0")

	var base string
	eventually(t, "the serving line", func() bool {
		base = servingURL.FindString(errOut.String())
		return base != ""
	})
	if got := get(t, base+"/.plan"); !strings.Contains(got, "* first") {
		t.Fatalf(".plan = %q, want the first entry", got)
	}
	if got := get(t, base+"/"); !strings.Contains(got, "EventSource") {
		t.Error("home page lacks the reload script")
	}

	writeFile(t, filepath.Join(root, entryPath(testToday)), "* second\n")
	eventually(t, "the rebuilt .plan", func() bool {
		return strings.Contains(get(t, base+"/.plan"), "* second")
	})
	if !strings.Contains(errOut.String(), "[~] Rebuilt 1 entry") {
		t.Errorf("stderr = %q, want the rebuild reported", errOut.String())
	}

	writeFile(t, filepath.Join(root, store.ProjectName), "lowercase keyword\n")
	eventually(t, "the failed check", func() bool {
		return strings.Contains(errOut.String(), "still serving the last good site")
	})
	if got := get(t, base+"/.plan"); !strings.Contains(got, "* second") {
		t.Errorf(".plan = %q after a failed check, want the last good site", got)
	}

	if err := stop(); !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled so main exits as interrupted", err)
	}
}
