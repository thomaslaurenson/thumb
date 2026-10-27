package preview

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testFiles() map[string][]byte {
	return map[string][]byte{
		"index.html":            []byte("<html><body><p>home</p></body></html>"),
		"404.html":              []byte("<html><body><p>lost</p></body></html>"),
		"2026-10-27/index.html": []byte("<html><body><p>day</p></body></html>"),
		".plan":                 []byte("plan\n"),
		"plan.txt":              []byte("plan\n"),
	}
}

func TestServeHTTP(t *testing.T) {
	t.Parallel()
	s := New(testFiles())
	tests := []struct {
		name   string
		method string
		target string
		status int
		// ctype is the Content-Type's expected prefix
		ctype    string
		body     string
		location string
		// reload is whether the body carries the reload script
		reload bool
	}{
		{name: "the root serves index.html", target: "/", status: 200, ctype: "text/html", body: "home", reload: true},
		{name: "a folder serves its index.html", target: "/2026-10-27/", status: 200, ctype: "text/html", body: "day", reload: true},
		{name: "a folder without its slash redirects", target: "/2026-10-27", status: 301, location: "/2026-10-27/"},
		{name: "a doubled slash redirects on the site", target: "//2026-10-27", status: 301, location: "/2026-10-27/"},
		{name: ".plan is sent as bytes, untouched", target: "/.plan", status: 200, ctype: "application/octet-stream", body: "plan\n"},
		{name: "plan.txt is text", target: "/plan.txt", status: 200, ctype: "text/plain", body: "plan\n"},
		{name: "a missing path gets 404.html", target: "/nope", status: 404, ctype: "text/html", body: "lost", reload: true},
		{name: "HEAD sends no body", method: http.MethodHead, target: "/", status: 200, ctype: "text/html"},
		{name: "POST is refused", method: http.MethodPost, target: "/", status: 405},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			method := tc.method
			if method == "" {
				method = http.MethodGet
			}
			rec := httptest.NewRecorder()
			s.ServeHTTP(rec, httptest.NewRequest(method, tc.target, nil))
			res := rec.Result()
			body := rec.Body.String()
			if res.StatusCode != tc.status {
				t.Fatalf("status = %d, want %d", res.StatusCode, tc.status)
			}
			if got := res.Header.Get("Location"); got != tc.location {
				t.Errorf("Location = %q, want %q", got, tc.location)
			}
			if tc.ctype == "" {
				return
			}
			if got := res.Header.Get("Content-Type"); !strings.HasPrefix(got, tc.ctype) {
				t.Errorf("Content-Type = %q, want prefix %q", got, tc.ctype)
			}
			if got := res.Header.Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
			if !tc.reload && body != tc.body {
				t.Errorf("body = %q, want %q", body, tc.body)
			}
			if tc.reload && (!strings.Contains(body, tc.body) || !strings.Contains(body, ReloadPath+`")`)) {
				t.Errorf("body = %q, want %q and the reload script", body, tc.body)
			}
			if tc.reload && !strings.HasSuffix(body, "</script>\n</body></html>") {
				t.Errorf("reload script is not just before </body>: %q", body)
			}
		})
	}
}

func TestSet(t *testing.T) {
	t.Parallel()
	s := New(testFiles())
	v0, _ := s.version()
	if s.Set(testFiles()) {
		t.Error("Set reported a change for the same files")
	}
	if v, _ := s.version(); v != v0 {
		t.Errorf("version = %q after an unchanged Set, want %q", v, v0)
	}
	files := testFiles()
	files["index.html"] = []byte("<html><body><p>new</p></body></html>")
	if !s.Set(files) {
		t.Error("Set reported no change for new files")
	}
	if v, _ := s.version(); v == v0 {
		t.Errorf("version = %q after a change, want a new one", v)
	}
	if got, _ := s.file("index.html"); !strings.Contains(string(got), "new") {
		t.Errorf("index.html = %q, want the new page", got)
	}
	if New(testFiles()).id == s.id {
		t.Error("two servers share an id, so a restart would not reload")
	}
}

// readEvent returns the data of the next server-sent event.
func readEvent(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	var data string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read event: %v", err)
		}
		line = strings.TrimSuffix(line, "\n")
		if line == "" {
			return data
		}
		if d, ok := strings.CutPrefix(line, "data: "); ok {
			data = d
		}
	}
}

func TestReloadStream(t *testing.T) {
	t.Parallel()
	s := New(testFiles())
	srv := httptest.NewServer(s)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+ReloadPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if got := res.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", got)
	}
	events := bufio.NewReader(res.Body)
	first := readEvent(t, events)
	if first == "" {
		t.Fatal("first event carries no version")
	}
	files := testFiles()
	files["plan.txt"] = []byte("changed\n")
	s.Set(files)
	if next := readEvent(t, events); next == first {
		t.Errorf("version after Set = %q, want it to differ from %q", next, first)
	}
}

func TestStamp(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	conf := filepath.Join(dir, "thumb.conf")
	plans := filepath.Join(dir, "plans")
	project := filepath.Join(dir, "project")
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(conf, "title=T\n")
	write(filepath.Join(plans, "2026", "2026-10-26.plan"), "* a\n")
	stamp := func() string {
		t.Helper()
		s, err := Stamp(conf, project, plans)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	prev := stamp()
	if again := stamp(); again != prev {
		t.Fatalf("stamp changed with nothing saved:\n%s\n%s", prev, again)
	}
	// Each change differs in size, so the test does not depend on how finely
	// the filesystem records modification times
	steps := []struct {
		name   string
		change func()
	}{
		{"an entry saved", func() { write(filepath.Join(plans, "2026", "2026-10-26.plan"), "* a\n* b\n") }},
		{"an entry added", func() { write(filepath.Join(plans, "2026", "2026-10-27.plan"), "- c\n") }},
		{"an entry removed", func() {
			if err := os.Remove(filepath.Join(plans, "2026", "2026-10-27.plan")); err != nil {
				t.Fatal(err)
			}
		}},
		{"a missing file created", func() { write(project, "BUILDING thumb\n") }},
		{"thumb.conf saved", func() { write(conf, "title=Longer\n") }},
	}
	for _, step := range steps {
		step.change()
		if now := stamp(); now == prev {
			t.Errorf("%s: stamp unchanged", step.name)
		} else {
			prev = now
		}
	}
}
