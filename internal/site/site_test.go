package site

import (
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thomaslaurenson/thumb/internal/config"
	"github.com/thomaslaurenson/thumb/internal/entry"
	"github.com/thomaslaurenson/thumb/internal/project"
)

var testConfig = config.Config{
	Title: "Test .plan", Author: "Test User", Login: "tester",
	BaseURL: "https://plan.example.com", Description: "A test plan",
}

func testOptions() Options {
	return Options{
		Config: testConfig,
		Entries: []*entry.Entry{
			entry.Parse("2025-12-31", []byte("* wrapped up the year\n\nQuiet day.\n")),
			entry.Parse("2026-10-20", []byte("* breadcrumb\n+ permalinks\n- plain text output\n! chroma test\n? tags\n~ js toggle\n> docs -> 2026-10-24\n")),
			entry.Parse("2026-10-22", []byte("\n")),
			entry.Parse("2026-10-24", []byte("* marker legend\n- emoji set\n- docs\n\nSpent the morning reading.\nIt holds up.\n")),
		},
		Project: []project.Item{{Keyword: "BUILDING", Text: "thumb"}, {Keyword: "EATING", Text: "Pho"}},
		Version: "0.0.0",
	}
}

func render(t *testing.T, o Options) map[string]string {
	t.Helper()
	files, err := Render(o)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for k, v := range files {
		out[k] = string(v)
	}
	return out
}

func TestRenderFileSet(t *testing.T) {
	t.Parallel()
	files := render(t, testOptions())
	for _, name := range []string{
		"index.html", "archive/index.html", "markers/index.html", "404.html", "atom.xml", ".plan", "plan.txt",
		".project", "project.txt", "2025-12-31/index.html", "2026-10-20/index.html", "2026-10-24/index.html",
	} {
		if _, ok := files[name]; !ok {
			t.Errorf("missing %s", name)
		}
	}
	if _, ok := files["2026-10-22/index.html"]; ok {
		t.Error("empty entry should have no page")
	}
}

func TestRenderPlanLeavesOutProject(t *testing.T) {
	t.Parallel()
	files := render(t, testOptions())
	for _, name := range []string{".plan", "plan.txt"} {
		if strings.Contains(files[name], "BUILDING") {
			t.Errorf("%s holds the project: %q", name, files[name])
		}
	}
	for _, name := range []string{".project", "project.txt"} {
		if !strings.Contains(files[name], "BUILDING") {
			t.Errorf("%s lacks the project: %q", name, files[name])
		}
	}
}

func TestRenderPages(t *testing.T) {
	t.Parallel()
	files := render(t, testOptions())
	tests := []struct {
		name string
		file string
		want []string
		not  []string
	}{
		{
			name: "home shows finger header, project, open items and latest entry",
			file: "index.html",
			want: []string{
				"<dt>Plan</dt><dd>3 entries, 4 open</dd>",
				"<dt>BUILDING</dt><dd>thumb</dd>",
				`plain text output <a href="2026-10-20/">2026-10-20</a>`,
				`<h2><a href="2026-10-24/">`,
				"<p>Spent the morning reading.\nIt holds up.</p>",
				`<a href="./" aria-current="page">Latest</a>`,
				`<a href="markers/">Markers</a>`,
			},
			not: []string{"<script", "ZgotmplZ", "markers-heading"},
		},
		{
			name: "moved line links to its target and stays out of open items",
			file: "2026-10-20/index.html",
			want: []string{
				`<li data-status="moved">docs <small>moved to <a href="../2026-10-24/">2026-10-24</a></small></li>`,
				`<a href="../2025-12-31/">older - 2025-12-31</a>`,
				`<a href="../2026-10-24/">newer - 2026-10-24</a>`,
				`<link rel="canonical" href="https://plan.example.com/2026-10-20/">`,
			},
		},
		{
			name: "archive opens only the newest year",
			file: "archive/index.html",
			want: []string{
				"<details open>\n<summary>2026 (2)</summary>",
				"<details>\n<summary>2025 (1)</summary>",
				"2 done, 2 open, 1 idea, 1 dropped, 1 moved - breadcrumb",
			},
		},
		{
			name: "markers page has its own nav entry",
			file: "markers/index.html",
			want: []string{
				`<a href="../markers/" aria-current="page">Markers</a>`,
				`<link rel="canonical" href="https://plan.example.com/markers/">`,
			},
		},
		{
			name: "404 links from the root and is not indexed",
			file: "404.html",
			want: []string{`<meta name="robots" content="noindex">`, `href="/archive/"`},
			not:  []string{`rel="canonical"`},
		},
		{
			name: "one inline stylesheet and an inline icon",
			file: "index.html",
			want: []string{"<style>", `rel="icon" type="image/svg+xml" href="data:image/svg`},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := files[tc.file]
			for _, w := range tc.want {
				if !strings.Contains(body, w) {
					t.Errorf("%s lacks %q", tc.file, w)
				}
			}
			for _, n := range tc.not {
				if strings.Contains(body, n) {
					t.Errorf("%s contains %q", tc.file, n)
				}
			}
			if c := strings.Count(body, "<style>"); c != 1 {
				t.Errorf("%s has %d style blocks, want 1", tc.file, c)
			}
		})
	}
}

func TestRenderOpenCount(t *testing.T) {
	t.Parallel()
	files := render(t, testOptions())
	// Two open on 2026-10-20 and two on 2026-10-24; the moved line is not open
	if !strings.Contains(files["index.html"], "4 open") {
		t.Error("expected 4 open items on the home page")
	}
}

func TestRenderEscapes(t *testing.T) {
	t.Parallel()
	o := testOptions()
	o.Entries = []*entry.Entry{entry.Parse("2026-10-24", []byte("* fixed <mark> & \"quotes\"\n<script>alert(1)</script>\n"))}
	files := render(t, o)
	page := files["2026-10-24/index.html"]
	if strings.Contains(page, "<script>") || strings.Contains(page, "<mark>") {
		t.Error("entry markup was not escaped")
	}
	if !strings.Contains(page, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Error("escaped prose missing")
	}
	// The feed carries the page HTML as escaped text, so markup from the entry
	// ends up escaped twice and never as a live tag
	if strings.Contains(files["atom.xml"], "<script>") || !strings.Contains(files["atom.xml"], "&amp;lt;script&amp;gt;") {
		t.Error("feed content not escaped")
	}
}

func TestRenderNoEntries(t *testing.T) {
	t.Parallel()
	o := testOptions()
	o.Entries = nil
	o.Project = nil
	files := render(t, o)
	if !strings.Contains(files["index.html"], "No entries yet.") {
		t.Error("home should say there are no entries")
	}
	if !strings.HasSuffix(files[".plan"], "No Plan.\n") {
		t.Errorf(".plan = %q", files[".plan"])
	}
	if files["project.txt"] != "" {
		t.Errorf("project.txt = %q, want empty", files["project.txt"])
	}
}

func TestAtomIsWellFormed(t *testing.T) {
	t.Parallel()
	files := render(t, testOptions())
	var feed struct {
		Entries []struct {
			ID string `xml:"id"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal([]byte(files["atom.xml"]), &feed); err != nil {
		t.Fatalf("atom.xml does not parse: %v", err)
	}
	if len(feed.Entries) != 3 || feed.Entries[0].ID != "https://plan.example.com/2026-10-24/" {
		t.Errorf("feed entries = %+v", feed.Entries)
	}
}

func TestDotPlan(t *testing.T) {
	t.Parallel()
	o := testOptions()
	got := string(DotPlan(testConfig, o.Entries[3]))
	lines := strings.Split(got, "\n")
	checks := []struct {
		line int
		want string
	}{
		{0, "Login: tester"},
		{1, "Updated: 2026-10-24"},
		{2, "Plan:"},
		{4, "Test User's .plan for Oct 24, 2026"},
		{5, "----------------------------------"},
		{6, "* marker legend"},
	}
	for _, c := range checks {
		if !strings.HasPrefix(lines[c.line], c.want) {
			t.Errorf("line %d = %q, want prefix %q", c.line, lines[c.line], c.want)
		}
	}
	if !strings.HasSuffix(got, "It holds up.\n") {
		t.Errorf("trailing blank lines not trimmed: %q", got)
	}
}

func TestEntryTextPossessive(t *testing.T) {
	t.Parallel()
	c := testConfig
	c.Author = "James"
	got := string(EntryText(c, entry.Parse("2026-10-24", []byte("* a\n"))))
	if !strings.HasPrefix(got, "James' .plan for Oct 24, 2026\n") {
		t.Errorf("EntryText = %q", got)
	}
}

func TestWrite(t *testing.T) {
	t.Parallel()
	files := map[string][]byte{"index.html": []byte("x"), ".plan": []byte("p"), "a/index.html": []byte("y")}
	out := filepath.Join(t.TempDir(), "build")
	if err := Write(out, files); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "leftover"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Write(out, files); err != nil {
		t.Fatalf("replacing a previous build: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "leftover")); !errors.Is(err, os.ErrNotExist) {
		t.Error("leftover file survived a rebuild")
	}
	if b, err := os.ReadFile(filepath.Join(out, "a", "index.html")); err != nil || string(b) != "y" {
		t.Errorf("nested file = %q, %v", b, err)
	}
	matches, _ := filepath.Glob(out + ".tmp.*")
	if len(matches) != 0 {
		t.Errorf("temporary folders left: %v", matches)
	}
}

func TestWriteRefusesOtherFolders(t *testing.T) {
	t.Parallel()
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "precious.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Write(out, map[string][]byte{"index.html": nil}); !errors.Is(err, ErrNotBuild) {
		t.Errorf("Write error = %v, want ErrNotBuild", err)
	}
	if _, err := os.Stat(filepath.Join(out, "precious.txt")); err != nil {
		t.Error("existing file was removed")
	}
	if err := Write("/", nil); !errors.Is(err, ErrNotBuild) {
		t.Errorf("Write(/) error = %v, want ErrNotBuild", err)
	}
}

func TestEmbeddedTemplatesParse(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"home", "entry", "archive", "markers", "404"} {
		if pages[name].Lookup("main") == nil || pages[name].Lookup("body") == nil {
			t.Errorf("template set %s lacks main or body", name)
		}
	}
}

func TestMarkersPageCoversEveryMarker(t *testing.T) {
	t.Parallel()
	page := render(t, testOptions())["markers/index.html"]
	for _, m := range []byte(entry.Markers) {
		if want := `<li data-status="` + entry.Status(m) + `">`; !strings.Contains(page, want) {
			t.Errorf("markers page lacks %c: %q", m, want)
		}
	}
}

func TestSplit(t *testing.T) {
	t.Parallel()
	entries := testOptions().Entries
	dates := func(es []*entry.Entry) string {
		var d []string
		for _, e := range es {
			d = append(d, e.Date)
		}
		return strings.Join(d, " ")
	}
	tests := []struct {
		name          string
		today         string
		wantPublished string
		wantHeld      string
	}{
		{name: "no today publishes everything", wantPublished: "2025-12-31 2026-10-20 2026-10-24"},
		{name: "today itself is published", today: "2026-10-24", wantPublished: "2025-12-31 2026-10-20 2026-10-24"},
		{name: "later entries are held", today: "2026-10-23", wantPublished: "2025-12-31 2026-10-20", wantHeld: "2026-10-24"},
		{name: "empty entries are in neither", today: "2026-10-21", wantPublished: "2025-12-31 2026-10-20", wantHeld: "2026-10-24"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			published, held := Split(entries, tc.today)
			if got := dates(published); got != tc.wantPublished {
				t.Errorf("published = %q, want %q", got, tc.wantPublished)
			}
			if got := dates(held); got != tc.wantHeld {
				t.Errorf("held = %q, want %q", got, tc.wantHeld)
			}
		})
	}
}

func TestRenderHoldsFutureEntries(t *testing.T) {
	t.Parallel()
	o := testOptions()
	o.Today = "2026-10-23"
	files := render(t, o)
	if _, ok := files["2026-10-24/index.html"]; ok {
		t.Error("an entry after today has a page")
	}
	tests := []struct {
		file string
		want []string
		not  []string
	}{
		{
			file: "index.html",
			want: []string{"<dt>Updated</dt><dd><time datetime=\"2026-10-20\">", "<dt>Plan</dt><dd>2 entries, 2 open</dd>"},
			not:  []string{`href="2026-10-24/"`, "emoji set"},
		},
		{file: ".plan", want: []string{"Updated: 2026-10-20", "Test User's .plan for Oct 20, 2026"}},
		{file: "atom.xml", want: []string{"<updated>2026-10-20T00:00:00Z</updated>"}, not: []string{"/2026-10-24/"}},
		{file: "archive/index.html", not: []string{"2026-10-24"}},
		{
			// The moved line still names its target, unlinked until that day has a page
			file: "2026-10-20/index.html",
			want: []string{`docs <small>moved to 2026-10-24</small>`},
			not:  []string{`href="../2026-10-24/"`},
		},
	}
	for _, tc := range tests {
		body := files[tc.file]
		for _, w := range tc.want {
			if !strings.Contains(body, w) {
				t.Errorf("%s lacks %q", tc.file, w)
			}
		}
		for _, n := range tc.not {
			if strings.Contains(body, n) {
				t.Errorf("%s contains %q", tc.file, n)
			}
		}
	}
}
