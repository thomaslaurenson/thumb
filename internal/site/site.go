// Package site renders a data repository as a static website, an Atom feed
// and the plain text files that finger would have shown.
//
// Links between pages are relative, so the site works under any base path.
// The 404 page is the exception: GitHub Pages serves it at whatever URL was
// missed, so it links from the root instead.
package site

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"slices"
	"strings"
	textTemplate "text/template"

	"github.com/thomaslaurenson/thumb/internal/config"
	"github.com/thomaslaurenson/thumb/internal/entry"
	"github.com/thomaslaurenson/thumb/internal/project"
)

// FeedSize is how many of the newest entries the Atom feed carries.
const FeedSize = 20

// favicon is a small asterisk icon, inlined so browsers never request
// /favicon.ico from a site that has none.
const favicon = "data:image/svg+xml;base64,PHN2ZyB4bWxucz0naHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmcnIHZpZXdCb3g9JzAgMCAzMiAzMic+PHJlY3Qgd2lkdGg9JzMyJyBoZWlnaHQ9JzMyJyByeD0nNCcgZmlsbD0nIzIyMicvPjx0ZXh0IHg9JzE2JyB5PScyNycgZm9udC1mYW1pbHk9J21vbm9zcGFjZScgZm9udC1zaXplPSczMCcgZm9udC13ZWlnaHQ9J2JvbGQnIHRleHQtYW5jaG9yPSdtaWRkbGUnIGZpbGw9JyNmZmYnPio8L3RleHQ+PC9zdmc+"

//go:embed templates assets
var embedded embed.FS

// pages holds one parsed template set per page, each the base layout plus
// that page's "main" block. A broken embedded template means the binary was
// built wrong, so parsing panics at start-up rather than at first use.
var pages = func() map[string]*template.Template {
	sets := map[string]*template.Template{}
	for _, name := range []string{"home", "entry", "archive", "markers", "404"} {
		sets[name] = template.Must(template.ParseFS(embedded, "templates/base.html", "templates/"+name+".html"))
	}
	return sets
}()

var stylesheet = func() string {
	b, err := fs.ReadFile(embedded, "assets/style.css")
	if err != nil {
		panic("embedded stylesheet missing: " + err.Error())
	}
	return string(b)
}()

var atom = textTemplate.Must(textTemplate.New("atom").Funcs(textTemplate.FuncMap{
	"xml": template.HTMLEscapeString,
}).Parse(atomTemplate))

const atomTemplate = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
<title>{{xml .Site.Title}}</title>
<link href="{{xml .Site.BaseURL}}/"/>
<link rel="self" href="{{xml .Site.BaseURL}}/atom.xml"/>
<id>{{xml .Site.BaseURL}}/</id>
<updated>{{.Updated}}T00:00:00Z</updated>
<author><name>{{xml .Site.Author}}</name></author>
{{- range .Entries}}
<entry>
<title>{{.Date}}</title>
<link href="{{xml $.Site.BaseURL}}/{{.Date}}/"/>
<id>{{xml $.Site.BaseURL}}/{{.Date}}/</id>
<updated>{{.Date}}T00:00:00Z</updated>
<content type="html">{{xml .HTML}}</content>
</entry>
{{- end}}
</feed>
`

// Options is everything a build needs.
type Options struct {
	Config config.Config
	// Entries is every entry, oldest first. Empty ones are skipped.
	Entries []*entry.Entry
	Project []project.Item
	Version string
	// Today is the author's date as YYYY-MM-DD. Entries after it are held back
	// until their day; empty publishes every entry.
	Today string
}

// Split separates the entries a build publishes from those it holds back,
// keeping each oldest first. Empty entries are in neither. An entry dated after
// today is held: it is usually an item migrated forward, and publishing it would
// make that one line the latest entry, the .plan and the head of the feed. An
// empty today holds nothing back.
func Split(entries []*entry.Entry, today string) (published, held []*entry.Entry) {
	for _, e := range entries {
		switch {
		case e.Empty():
		case today != "" && e.Date > today:
			held = append(held, e)
		default:
			published = append(published, e)
		}
	}
	return published, held
}

// page is the data every template sees.
type page struct {
	Root, Home, Title, Canonical, Description, Current string
	Site                                               config.Config
	Version                                            string
	CSS                                                template.CSS
	Favicon                                            template.URL
	Data                                               any
}

// block is a run of plan lines or a paragraph of prose.
type block struct {
	List []item
	Para []string
}

type item struct {
	Status, Text, Target, TargetHref, Date string
}

type entryView struct {
	Date         string
	Body         []block
	Older, Newer string
}

type homeData struct {
	Latest  *entryView
	Count   int
	Open    []item
	Project []project.Item
}

type archiveRow struct {
	Date, Tally, Summary string
}

type archiveYear struct {
	Year string
	Rows []archiveRow
}

type archiveData struct {
	Years []archiveYear
}

// Render builds every file of the site, keyed by its path in the output.
func Render(o Options) (map[string][]byte, error) {
	entries, _ := Split(o.Entries, o.Today)
	published := map[string]bool{}
	for _, e := range entries {
		published[e.Date] = true
	}
	newest := slices.Clone(entries)
	slices.Reverse(newest)

	files := map[string][]byte{}
	base := func(root, title, path, desc, current string, data any) page {
		p := page{
			Root: root, Home: root, Title: title, Description: desc, Current: current,
			Site: o.Config, Version: o.Version, Data: data,
			CSS: template.CSS(stylesheet), Favicon: template.URL(favicon),
		}
		if root == "" {
			p.Home = "./"
		}
		if path != "-" {
			p.Canonical = o.Config.BaseURL + "/" + path
		}
		return p
	}

	home := homeData{Count: len(entries), Project: o.Project}
	for _, e := range entries {
		for _, it := range e.OpenItems() {
			home.Open = append(home.Open, item{Status: entry.Status(it.Line.Marker), Text: it.Line.Text, Date: e.Date})
		}
	}
	if len(newest) > 0 {
		home.Latest = &entryView{Date: newest[0].Date, Body: blocks(newest[0], "", published)}
	}
	if err := execute(files, "index.html", "home", base("", o.Config.Title, "", o.Config.Description, "latest", home)); err != nil {
		return nil, err
	}

	for i, e := range entries {
		v := entryView{Date: e.Date, Body: blocks(e, "../", published)}
		if i > 0 {
			v.Older = entries[i-1].Date
		}
		if i < len(entries)-1 {
			v.Newer = entries[i+1].Date
		}
		p := base("../", e.Date+" | "+o.Config.Title, e.Date+"/", o.Config.Author+"'s plan for "+e.Date, "", v)
		if err := execute(files, e.Date+"/index.html", "entry", p); err != nil {
			return nil, err
		}
	}

	var archive archiveData
	for _, e := range newest {
		year := e.Date[:4]
		if len(archive.Years) == 0 || archive.Years[len(archive.Years)-1].Year != year {
			archive.Years = append(archive.Years, archiveYear{Year: year})
		}
		t := e.Tally()
		y := &archive.Years[len(archive.Years)-1]
		y.Rows = append(y.Rows, archiveRow{Date: e.Date, Tally: t.String(), Summary: t.FirstDone})
	}
	if err := execute(files, "archive/index.html", "archive", base("../", "Archive | "+o.Config.Title, "archive/", "Every entry in "+o.Config.Author+"'s plan", "archive", archive)); err != nil {
		return nil, err
	}

	if err := execute(files, "markers/index.html", "markers", base("../", "Markers | "+o.Config.Title, "markers/", "What the markers in "+o.Config.Author+"'s plan mean", "markers", nil)); err != nil {
		return nil, err
	}

	if err := execute(files, "404.html", "404", base("/", "Not found | "+o.Config.Title, "-", "Page not found", "", nil)); err != nil {
		return nil, err
	}

	feed, err := renderAtom(o, newest[:min(len(newest), FeedSize)], published)
	if err != nil {
		return nil, err
	}
	files["atom.xml"] = feed

	var latest *entry.Entry
	if len(newest) > 0 {
		latest = newest[0]
	}
	plan := DotPlan(o.Config, latest)
	files[".plan"] = plan
	files["plan.txt"] = plan
	files[".project"] = project.Normalised(o.Project)
	files["project.txt"] = project.Normalised(o.Project)
	return files, nil
}

func execute(files map[string][]byte, path, name string, p page) error {
	var b bytes.Buffer
	if err := pages[name].ExecuteTemplate(&b, "base", p); err != nil {
		return fmt.Errorf("render %s: %w", path, err)
	}
	files[path] = b.Bytes()
	return nil
}

// blocks groups an entry's lines into lists and paragraphs. A moved line
// links to its target only when that day has a page.
func blocks(e *entry.Entry, root string, published map[string]bool) []block {
	var out []block
	var cur *block
	for _, s := range e.Lines {
		l := entry.ParseLine(s)
		switch {
		case l.IsPlan():
			if cur == nil || cur.List == nil {
				out = append(out, block{List: []item{}})
				cur = &out[len(out)-1]
			}
			it := item{Status: entry.Status(l.Marker), Text: l.Text, Target: l.Target}
			if l.Target != "" && published[l.Target] {
				it.TargetHref = root + l.Target + "/"
			}
			cur.List = append(cur.List, it)
		case entry.IsBlank(s):
			cur = nil
		default:
			if cur == nil || cur.List != nil {
				out = append(out, block{})
				cur = &out[len(out)-1]
			}
			cur.Para = append(cur.Para, s)
		}
	}
	return out
}

func renderAtom(o Options, newest []*entry.Entry, published map[string]bool) ([]byte, error) {
	type feedEntry struct{ Date, HTML string }
	data := struct {
		Site    config.Config
		Updated string
		Entries []feedEntry
	}{Site: o.Config, Updated: "1970-01-01"}
	if len(newest) > 0 {
		data.Updated = newest[0].Date
	}
	for _, e := range newest {
		var b bytes.Buffer
		if err := pages["entry"].ExecuteTemplate(&b, "body", blocks(e, o.Config.BaseURL+"/", published)); err != nil {
			return nil, fmt.Errorf("render feed entry %s: %w", e.Date, err)
		}
		data.Entries = append(data.Entries, feedEntry{Date: e.Date, HTML: b.String()})
	}
	var b bytes.Buffer
	if err := atom.Execute(&b, data); err != nil {
		return nil, fmt.Errorf("render atom.xml: %w", err)
	}
	return b.Bytes(), nil
}

// EntryText renders one entry as plain text under a heading in the style of
// the 1990s id Software plans, with trailing blank lines removed.
func EntryText(c config.Config, e *entry.Entry) []byte {
	owner := c.Author + "'s"
	if strings.HasSuffix(c.Author, "s") {
		owner = c.Author + "'"
	}
	heading := fmt.Sprintf("%s .plan for %s", owner, entry.LongDate(e.Date))
	var b strings.Builder
	b.WriteString(heading + "\n" + strings.Repeat("-", len([]rune(heading))) + "\n")
	for _, l := range e.TrimmedLines() {
		b.WriteString(l + "\n")
	}
	return []byte(b.String())
}

// DotPlan renders the .plan file: a finger-style header, then the latest
// entry. latest may be nil. The project is published on its own as .project.
func DotPlan(c config.Config, latest *entry.Entry) []byte {
	updated := "never"
	if latest != nil {
		updated = latest.Date
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Login: %-28s Name: %s\n", c.Login, c.Author)
	fmt.Fprintf(&b, "Updated: %-26s Web: %s/\n", updated, c.BaseURL)
	b.WriteString("Plan:\n\n")
	if latest == nil {
		b.WriteString("No Plan.\n")
	} else {
		b.Write(EntryText(c, latest))
	}
	return []byte(b.String())
}
