package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/thomaslaurenson/thumb/internal/entry"
	"github.com/thomaslaurenson/thumb/internal/store"
)

const today = "2026-10-25"

// testNow is 09:41 on today, for the status bar clock.
var testNow = time.Date(2026, 10, 25, 9, 41, 0, 0, time.Local)

// newTestModel builds a repository from files, keyed by path relative to the
// root, and opens the interface on it.
func newTestModel(t *testing.T, files map[string]string) (*model, *store.Store) {
	t.Helper()
	root := t.TempDir()
	files[store.ConfigName] = "title=T\nauthor=A\nlogin=l\nbase_url=https://x.example\n"
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	st := store.New(root)
	m, err := newModel(Options{Store: st, Today: today, Now: func() time.Time { return testNow }})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return m, st
}

// press sends keys in order. Named keys are written as they print, such as
// "enter" or "ctrl+c"; anything else is typed as text.
func press(m *model, keys ...string) {
	for _, k := range keys {
		m.Update(keyMsg(k))
	}
}

// typeText types s one character at a time into whatever has focus.
func typeText(m *model, s string) {
	for _, r := range s {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

// namedKeys are the keys keyMsg builds by name rather than as typed text.
var namedKeys = map[string]bool{
	"enter": true, "esc": true, "tab": true, "space": true, "backspace": true,
	"ctrl+a": true, "ctrl+b": true, "ctrl+u": true, "ctrl+c": true,
}

func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "ctrl+a", "ctrl+b", "ctrl+u":
		return tea.KeyPressMsg{Code: rune(k[len(k)-1]), Mod: tea.ModCtrl}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	default:
		r := []rune(k)[0]
		return tea.KeyPressMsg{Code: r, Text: k}
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

func TestMarkerKeys(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		keys []string
		want string
	}{
		{name: "r* marks done", keys: []string{"r", "*"}, want: "* thing\n"},
		{name: "r+ marks done later", keys: []string{"r", "+"}, want: "+ thing\n"},
		{name: "r~ drops", keys: []string{"r", "~"}, want: "~ thing\n"},
		{name: "r! blocks", keys: []string{"r", "!"}, want: "! thing\n"},
		{name: "r? marks an idea", keys: []string{"r", "?"}, want: "? thing\n"},
		{name: "r- marks to do", keys: []string{"r", "*", "r", "-"}, want: "- thing\n"},
		{name: "space toggles to done", keys: []string{"space"}, want: "* thing\n"},
		{name: "space twice toggles back", keys: []string{"space", "space"}, want: "- thing\n"},
		{name: "undo restores", keys: []string{"r", "*", "u"}, want: "- thing\n"},
		{name: "r then esc does nothing", keys: []string{"r", "esc"}, want: "- thing\n"},
		{name: "r then an unbound key does nothing", keys: []string{"r", "x"}, want: "- thing\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, st := newTestModel(t, map[string]string{"plans/2026/2026-10-25.plan": "- thing\n"})
			press(m, tc.keys...)
			if m.pending != "" {
				t.Errorf("pending = %q, want the sequence finished or dropped", m.pending)
			}
			if err := m.save(); err != nil {
				t.Fatal(err)
			}
			if got := readFile(t, st.EntryPath(today)); got != tc.want {
				t.Errorf("entry = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSpaceMarksDoneByDate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		date string
		line string
		want string
	}{
		{name: "today's item is done that day", date: today, line: "- thing", want: "* thing"},
		{name: "an older day's item is done later", date: "2026-10-20", line: "- thing", want: "+ thing"},
		{name: "a later day's item is done that day", date: "2026-10-26", line: "! thing", want: "* thing"},
		{name: "done goes back to to do", date: "2026-10-20", line: "* thing", want: "- thing"},
		{name: "done later goes back to to do", date: "2026-10-20", line: "+ thing", want: "- thing"},
		{name: "a moved line is left alone", date: "2026-10-20", line: "> thing -> 2026-10-25", want: "> thing -> 2026-10-25"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			files := map[string]string{"plans/2026/" + tc.date + ".plan": tc.line + "\n"}
			if tc.date != today {
				files["plans/2026/2026-10-25.plan"] = "* started\n"
			}
			m, _ := newTestModel(t, files)
			m.showDay(tc.date, 1)
			press(m, "space")
			if got := m.entries[tc.date].Lines[0]; got != tc.want {
				t.Errorf("line = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMovedLineKeepsItsMarker(t *testing.T) {
	t.Parallel()
	for _, keys := range [][]string{{"space"}, {"r", "-"}, {"r", "*"}, {"r", "~"}} {
		t.Run(strings.Join(keys, ""), func(t *testing.T) {
			t.Parallel()
			m, _ := newTestModel(t, map[string]string{"plans/2026/2026-10-25.plan": "> carry -> 2026-10-26\n"})
			press(m, keys...)
			if got := m.entries[today].Lines[0]; got != "> carry -> 2026-10-26" {
				t.Errorf("line = %q, want the moved line kept with its target", got)
			}
			if !strings.Contains(m.status, "Moved line") {
				t.Errorf("status = %q, want it to say why", m.status)
			}
		})
	}
}

func TestMarkingProseMakesAPlanLine(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t, map[string]string{"plans/2026/2026-10-25.plan": "a thought\n"})
	press(m, "r", "-")
	if got := m.entries[today].Lines[0]; got != "- a thought" {
		t.Errorf("line = %q", got)
	}
}

func TestAddEditDelete(t *testing.T) {
	t.Parallel()
	m, st := newTestModel(t, map[string]string{"plans/2026/2026-10-25.plan": "* one\n* two\n"})

	press(m, "o")
	typeText(m, "new")
	press(m, "esc")
	if got := strings.Join(m.entries[today].Lines, "|"); got != "* one|- new|* two" {
		t.Fatalf("after add = %q", got)
	}
	if m.cursor != 1 || m.mode != modeNormal {
		t.Errorf("cursor = %d mode = %v, want the cursor on the new line", m.cursor, m.mode)
	}

	press(m, "A", "ctrl+u")
	typeText(m, "? edited")
	press(m, "esc")
	press(m, "g", "g", "d", "d")
	press(m, "Z", "Z")
	if got := readFile(t, st.EntryPath(today)); got != "? edited\n* two\n" {
		t.Errorf("saved = %q", got)
	}
}

func TestTyping(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		start string
		keys  []string
		want  string
	}{
		{name: "o adds below with a to do marker", start: "* one\n", keys: []string{"o", "two", "esc"}, want: "* one|- two"},
		{name: "O adds above", start: "* one\n", keys: []string{"O", "zero", "esc"}, want: "- zero|* one"},
		{name: "o on an empty day", keys: []string{"o", "first", "esc"}, want: "- first"},
		{name: "enter starts the next line and twice stops", keys: []string{"o", "first", "enter", "second", "enter", "enter"}, want: "- first|- second"},
		{name: "the next line copies the marker", keys: []string{"o", "ctrl+u", "* did it", "enter", "and this", "esc"}, want: "* did it|* and this"},
		{name: "after prose the next line is prose", keys: []string{"o", "ctrl+u", "a thought", "enter", "more", "esc"}, want: "a thought|more"},
		{name: "an empty line between paragraphs is kept", keys: []string{"o", "ctrl+u", "one", "enter", "enter", "two", "esc"}, want: "one||two"},
		{name: "esc on an untouched new line keeps nothing", start: "* one\n", keys: []string{"o", "esc"}, want: "* one"},
		{name: "i edits from the start of the text", start: "- thing\n", keys: []string{"i", "big ", "esc"}, want: "- big thing"},
		{name: "i on prose edits from the start", start: "thing\n", keys: []string{"i", "a ", "esc"}, want: "a thing"},
		{name: "A edits from the end", start: "- thing\n", keys: []string{"A", "s", "esc"}, want: "- things"},
		{name: "ctrl+c keeps the line like esc", start: "- thing\n", keys: []string{"A", "s", "ctrl+c"}, want: "- things"},
		{name: "ctrl+c then u takes the session back", start: "- thing\n", keys: []string{"o", "lost", "enter", "also", "ctrl+c", "u"}, want: "- thing"},
		{name: "esc then dd removes just the last line", keys: []string{"o", "kept", "enter", "lost", "esc", "d", "d"}, want: "- kept"},
		{name: "keys type text while typing", keys: []string{"o", "q dd ? u", "esc"}, want: "- q dd ? u"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			files := map[string]string{}
			if tc.start != "" {
				files["plans/2026/2026-10-25.plan"] = tc.start
			}
			m, _ := newTestModel(t, files)
			for _, k := range tc.keys {
				if _, named := namedKeys[k]; named || len(k) == 1 && m.mode != modeInsert {
					press(m, k)
				} else {
					typeText(m, k)
				}
			}
			if m.mode != modeNormal {
				t.Fatalf("mode = %v, want normal", m.mode)
			}
			var lines []string
			if e := m.entries[today]; e != nil {
				lines = e.Lines
			}
			if got := strings.Join(lines, "|"); got != tc.want {
				t.Errorf("lines = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTypingSessionIsOneUndoStep(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t, map[string]string{"plans/2026/2026-10-25.plan": "* one\n"})
	press(m, "o")
	typeText(m, "two")
	press(m, "enter")
	typeText(m, "three")
	press(m, "esc", "u")
	if got := strings.Join(m.entries[today].Lines, "|"); got != "* one" {
		t.Errorf("after undo = %q", got)
	}
}

func TestTypingNothingLeavesNoTrace(t *testing.T) {
	t.Parallel()
	m, st := newTestModel(t, map[string]string{"plans/2026/2026-10-20.plan": "* x\n"})
	press(m, "o", "esc")
	if m.unsaved() || len(m.undo) != 0 {
		t.Errorf("unsaved = %v, undo steps = %d, want neither", m.unsaved(), len(m.undo))
	}
	press(m, "ctrl+a", "3")
	if m.view != viewArchive || len(m.rows) != 1 {
		t.Errorf("view = %v archive rows = %d, want only the existing day", m.view, len(m.rows))
	}
	press(m, "Z", "Z")
	if _, err := os.Stat(st.EntryPath(today)); !os.IsNotExist(err) {
		t.Error("an abandoned line created today's file")
	}
}

func TestTypingClearsTheFilter(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t, map[string]string{"plans/2026/2026-10-25.plan": "* apples\n* pears\n"})
	press(m, "/")
	typeText(m, "pears")
	press(m, "enter", "o")
	if m.filter != "" || len(m.rows) != 3 {
		t.Errorf("filter = %q rows = %d, want the whole day while typing", m.filter, len(m.rows))
	}
	if r := m.rows[m.cursor]; r.line != 3 {
		t.Errorf("cursor on line %d, want the new line 3", r.line)
	}
}

func TestKeySequences(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t, map[string]string{"plans/2026/2026-10-25.plan": "* a\n* b\n* c\n"})
	press(m, "d")
	if m.pending != "d" || !strings.Contains(m.View().Content, " d") {
		t.Errorf("pending = %q, want d shown while waiting", m.pending)
	}
	press(m, "j")
	if m.pending != "" || m.cursor != 0 || len(m.entries[today].Lines) != 3 {
		t.Errorf("an unfinished sequence should be dropped: pending %q cursor %d", m.pending, m.cursor)
	}
	press(m, "G", "g", "g")
	if m.cursor != 0 {
		t.Errorf("gg cursor = %d, want 0", m.cursor)
	}
	press(m, "down", "down", "up")
	if m.cursor != 1 {
		t.Errorf("arrow keys cursor = %d, want 1", m.cursor)
	}
	press(m, "g", "g")
	press(m, "j", "d", "d")
	if got := strings.Join(m.entries[today].Lines, "|"); got != "* a|* c" {
		t.Errorf("after dd = %q", got)
	}
}

func TestMigrate(t *testing.T) {
	t.Parallel()
	m, st := newTestModel(t, map[string]string{
		"plans/2026/2026-10-25.plan": "- carry me\n* done\n",
	})
	press(m, "r", ">")
	if m.mode != modeMigrate || m.pick != "2026-10-26" {
		t.Errorf("mode = %v pick = %q, want the picker on tomorrow", m.mode, m.pick)
	}
	press(m, "enter", "Z", "Z")
	if got := readFile(t, st.EntryPath(today)); got != "> carry me -> 2026-10-26\n* done\n" {
		t.Errorf("source = %q", got)
	}
	if got := readFile(t, st.EntryPath("2026-10-26")); got != "- carry me\n" {
		t.Errorf("target = %q", got)
	}
}

func TestMigrateRejectsBadDateAndDoneLines(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t, map[string]string{"plans/2026/2026-10-25.plan": "- a\n* b\n"})
	press(m, "r", ">", "i", "ctrl+u")
	typeText(m, "2026-02-30")
	press(m, "enter")
	if m.mode != modeMigrate || !m.pickTyping || !strings.Contains(m.hintLine(), "Not a valid date") {
		t.Errorf("bad date should keep typing with the complaint in the panel, panel %q", m.hintLine())
	}
	if got := m.statusLine(); !strings.Contains(got, "1:"+today) {
		t.Errorf("the complaint should leave the bar alone, got %q", got)
	}
	press(m, "esc", "esc", "j", "r", ">")
	if m.mode != modeNormal || !strings.Contains(m.status, "Only open items") {
		t.Errorf("done line should not migrate, status %q", m.status)
	}
}

func TestMigratePicker(t *testing.T) {
	t.Parallel()
	m, st := newTestModel(t, map[string]string{
		"plans/2026/2026-10-20.plan": "* earlier\n",
		"plans/2026/2026-10-25.plan": "- carry me\n",
		"plans/2026/2026-10-30.plan": "* later\n",
	})
	plain := func() string { return styleCode.ReplaceAllString(m.hintLine(), "") }
	press(m, "r", ">")
	for _, want := range []string{"> from Sun 25 Oct  to  Mon 26 Oct, tomorrow", "Mon 26  Tue 27", "Sun 01", "enter migrate"} {
		if got := plain(); !strings.Contains(got, want) {
			t.Errorf("panel = %q, want %q", got, want)
		}
	}
	if got := m.statusLine(); !strings.Contains(got, "1:"+today) || !strings.Contains(got, "09:41") {
		t.Errorf("the bar should stay while picking, got %q", got)
	}
	if lines := strings.Split(m.View().Content, "\n"); len(lines) != m.height {
		t.Errorf("view is %d lines on a %d line terminal", len(lines), m.height)
	}

	steps := []struct {
		keys []string
		want string
	}{
		{[]string{"l"}, "2026-10-27"},
		{[]string{"h", "left"}, "2026-10-25"},
		{[]string{"j"}, "2026-11-01"},
		{[]string{"k", "up"}, "2026-10-18"},
		{[]string{"t"}, "2026-10-25"},
		{[]string{"tab"}, "2026-10-30"},
		// shift+tab passes over the line's own day, which has an entry
		{[]string{"shift+tab"}, "2026-10-20"},
		{[]string{"shift+tab"}, "2026-10-20"},
	}
	for _, s := range steps {
		press(m, s.keys...)
		if m.pick != s.want {
			t.Errorf("after %v pick = %s, want %s", s.keys, m.pick, s.want)
		}
	}

	press(m, "t", "enter")
	if m.mode != modeMigrate || !strings.Contains(plain(), "same day, pick another") {
		t.Errorf("enter on the line's own day should stay in the picker, panel %q", plain())
	}

	press(m, "i", "ctrl+u")
	typeText(m, "2026-11-03")
	if m.pick != "2026-11-03" || !strings.Contains(plain(), "Tue 3 Nov, in 9 days") {
		t.Errorf("typing should move the picker: pick %s panel %q", m.pick, plain())
	}
	press(m, "enter", "Z", "Z")
	if got := readFile(t, st.EntryPath(today)); got != "> carry me -> 2026-11-03\n" {
		t.Errorf("source = %q", got)
	}
}

func TestMigratePickerCancels(t *testing.T) {
	t.Parallel()
	var lines []string
	for i := range 30 {
		lines = append(lines, fmt.Sprintf("- item %d", i))
	}
	m, _ := newTestModel(t, map[string]string{"plans/2026/2026-10-25.plan": strings.Join(lines, "\n") + "\n"})
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	press(m, "G", "r", ">")
	if m.cursor < m.offset || m.cursor >= m.offset+m.bodyHeight() {
		t.Errorf("cursor %d outside rows %d-%d, want the line kept in view above the panel", m.cursor, m.offset, m.offset+m.bodyHeight()-1)
	}
	press(m, "l", "esc")
	if m.mode != modeNormal || m.unsaved() || m.entries[today].Lines[29] != "- item 29" {
		t.Errorf("esc should leave the line alone: mode %v line %q", m.mode, m.entries[today].Lines[29])
	}
}

func TestOpenViewResolve(t *testing.T) {
	t.Parallel()
	m, st := newTestModel(t, map[string]string{
		"plans/2026/2026-10-20.plan": "* x\n- old item\n",
		"plans/2026/2026-10-25.plan": "* today\n",
	})
	press(m, "ctrl+a", "2")
	if len(m.rows) != 1 || m.rows[0].date != "2026-10-20" || m.rows[0].line != 2 {
		t.Fatalf("open rows = %+v", m.rows)
	}
	press(m, "space")
	if len(m.rows) != 0 {
		t.Errorf("resolved item should leave the open view")
	}
	press(m, "Z", "Z")
	if got := readFile(t, st.EntryPath("2026-10-20")); got != "* x\n+ old item\n" {
		t.Errorf("entry = %q", got)
	}
}

func TestArchiveEnterOpensDay(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t, map[string]string{
		"plans/2026/2026-10-20.plan": "* x\n",
		"plans/2026/2026-10-25.plan": "* y\n",
	})
	press(m, "ctrl+a", "3", "j", "enter")
	if m.view != viewDay || m.day != "2026-10-20" {
		t.Errorf("view = %v day = %s", m.view, m.day)
	}
	press(m, "l")
	if m.day != today {
		t.Errorf("next day = %s, want %s", m.day, today)
	}
	press(m, "left")
	if m.day != "2026-10-20" {
		t.Errorf("left day = %s, want 2026-10-20", m.day)
	}
	press(m, "right")
	if m.day != today {
		t.Errorf("right day = %s, want %s", m.day, today)
	}
}

func TestAddFromOpenViewGoesToToday(t *testing.T) {
	t.Parallel()
	m, st := newTestModel(t, map[string]string{"plans/2026/2026-10-20.plan": "- x\n"})
	press(m, "n", "ctrl+b", "2", "o")
	if m.view != viewDay || m.day != today {
		t.Fatalf("view = %v day = %s, want today's day view", m.view, m.day)
	}
	typeText(m, "captured")
	press(m, "esc", "Z", "Z")
	if got := readFile(t, st.EntryPath(today)); got != "- captured\n" {
		t.Errorf("today = %q", got)
	}
}

func TestOpenViewEditsInPlace(t *testing.T) {
	t.Parallel()
	m, st := newTestModel(t, map[string]string{"plans/2026/2026-10-20.plan": "* x\n- old item\n"})
	press(m, "n", "ctrl+a", "2", "A")
	typeText(m, ", reworded")
	press(m, "enter")
	if m.mode != modeNormal || m.view != viewOpen {
		t.Errorf("mode = %v view = %v, want enter to finish in the open view", m.mode, m.view)
	}
	press(m, "Z", "Z")
	if got := readFile(t, st.EntryPath("2026-10-20")); got != "* x\n- old item, reworded\n" {
		t.Errorf("entry = %q", got)
	}
}

func TestProjectView(t *testing.T) {
	t.Parallel()
	m, st := newTestModel(t, map[string]string{"project": "BUILDING thumb\n"})
	press(m, "ctrl+a", "4", "o")
	typeText(m, "lowercase bad")
	press(m, "enter")
	if m.mode != modeInsert || !strings.Contains(m.status, "KEYWORD") {
		t.Fatalf("an invalid project line should stay open, status %q", m.status)
	}
	press(m, "ctrl+u")
	typeText(m, "EATING Pho")
	press(m, "enter", "esc", "Z", "Z")
	if got := readFile(t, filepath.Join(st.Root, store.ProjectName)); got != "BUILDING thumb\nEATING Pho\n" {
		t.Errorf("project = %q", got)
	}
}

func TestProjectLineEscThrowsAwayWhatCannotBeKept(t *testing.T) {
	t.Parallel()
	for _, keys := range [][]string{{"o"}, {"A"}} {
		t.Run(keys[0], func(t *testing.T) {
			t.Parallel()
			m, _ := newTestModel(t, map[string]string{"project": "BUILDING thumb\n"})
			press(m, "ctrl+b", "4")
			press(m, keys...)
			press(m, "ctrl+u")
			typeText(m, "lowercase bad")
			press(m, "esc")
			if m.mode != modeNormal || !strings.Contains(m.status, "Not kept") {
				t.Errorf("mode = %v status = %q, want typing stopped and the line thrown away", m.mode, m.status)
			}
			if got := strings.Join(m.project.Lines, "|"); got != "BUILDING thumb" {
				t.Errorf("project = %q, want it unchanged", got)
			}
		})
	}
}

func TestRolloverAll(t *testing.T) {
	t.Parallel()
	m, st := newTestModel(t, map[string]string{"plans/2026/2026-10-24.plan": "- one\n* done\n! two\n"})
	if m.mode != modeRollover || len(m.rollover) != 2 {
		t.Fatalf("mode = %v, rollover = %v", m.mode, m.rollover)
	}
	press(m, "a", "Z", "Z")
	if got := readFile(t, st.EntryPath(today)); got != "- one\n! two\n" {
		t.Errorf("today = %q", got)
	}
	if got := readFile(t, st.EntryPath("2026-10-24")); got != "> one -> 2026-10-25\n* done\n> two -> 2026-10-25\n" {
		t.Errorf("yesterday = %q", got)
	}
}

func TestRolloverOneByOne(t *testing.T) {
	t.Parallel()
	m, st := newTestModel(t, map[string]string{"plans/2026/2026-10-24.plan": "- one\n- two\n"})
	press(m, "o", "n", "y", "Z", "Z")
	if got := readFile(t, st.EntryPath(today)); got != "- two\n" {
		t.Errorf("today = %q", got)
	}
}

func TestRolloverNotOfferedWhenTodayHasContent(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t, map[string]string{
		"plans/2026/2026-10-24.plan": "- one\n",
		"plans/2026/2026-10-25.plan": "* started\n",
	})
	if m.mode != modeNormal {
		t.Errorf("mode = %v, want normal", m.mode)
	}
}

func TestBrowsingCreatesNoFiles(t *testing.T) {
	t.Parallel()
	m, st := newTestModel(t, map[string]string{"plans/2026/2026-10-20.plan": "* x\n"})
	press(m, "h", "l", "t", "ctrl+a", "2", "ctrl+b", "3", "Z", "Z")
	if _, err := os.Stat(st.EntryPath(today)); !os.IsNotExist(err) {
		t.Errorf("quitting after browsing created %s", st.EntryPath(today))
	}
}

func TestFilter(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t, map[string]string{"plans/2026/2026-10-25.plan": "* apples\n* pears\n- apple pie\n"})
	press(m, "/")
	typeText(m, "APPLE")
	press(m, "enter")
	if len(m.rows) != 2 {
		t.Errorf("filtered rows = %d, want 2", len(m.rows))
	}
	press(m, "esc")
	if len(m.rows) != 3 {
		t.Errorf("rows after clearing = %d, want 3", len(m.rows))
	}
}

func TestViewRenders(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t, map[string]string{"plans/2026/2026-10-25.plan": "* done thing\n- todo thing\n\nprose\n"})
	out := m.View().Content
	for _, want := range []string{"[" + m.name + "]", "1:" + today + "*", "done thing", "? keys"} {
		if !strings.Contains(out, want) {
			t.Errorf("view lacks %q", want)
		}
	}
	if lines := strings.Split(out, "\n"); len(lines) != m.height {
		t.Errorf("view is %d lines on a %d line terminal", len(lines), m.height)
	}
	press(m, "o")
	if !strings.Contains(m.View().Content, "-- INSERT --") {
		t.Error("typing should show the insert footer")
	}
	press(m, "esc", "?")
	if !strings.Contains(m.View().Content, "done, * today or + older") {
		t.Error("keys screen missing")
	}
	m.Update(tea.WindowSizeMsg{Width: 10, Height: 3})
	_ = m.View()
}

func TestStatusBar(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t, map[string]string{"plans/2026/2026-10-25.plan": "- todo thing\n"})
	// Each bar is drawn as one styled run, so its text is contiguous between
	// the escape codes and Contains can find it
	bar := func() string {
		lines := strings.Split(m.View().Content, "\n")
		return lines[len(lines)-1]
	}

	got := bar()
	for _, want := range []string{"[" + m.name + "] ", "1:" + today + "* 2:open  3:archive  4:project ", "09:41 25-Oct-26"} {
		if !strings.Contains(got, want) {
			t.Errorf("bar = %q, want it to contain %q", got, want)
		}
	}
	if w := lipgloss.Width(got); w != m.width {
		t.Errorf("bar is %d wide, want the full %d", w, m.width)
	}

	press(m, "ctrl+a", "2")
	if got := bar(); !strings.Contains(got, "1:"+today+"- 2:open* ") {
		t.Errorf("after switching, bar = %q, want day flagged last and open current", got)
	}

	press(m, "space")
	if got := bar(); !strings.Contains(got, "[unsaved] 09:41") {
		t.Errorf("with a change, bar = %q, want [unsaved] before the clock", got)
	}

	// Undoing the mark leaves the line open, so it can migrate next
	press(m, "u")
	if got := bar(); !strings.Contains(got, "Undone") {
		t.Errorf("a message should take over the bar, got %q", got)
	}

	press(m, "r", ">")
	if got := bar(); !strings.Contains(got, "2:open*") {
		t.Errorf("the migrate picker should leave the bar alone, got %q", got)
	}

	m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	press(m, "esc")
	if got := bar(); !strings.Contains(got, "1:"+today) || strings.Contains(got, "09:41") {
		t.Errorf("a narrow bar = %q, want the views kept and the clock dropped", got)
	}
}

// styleCode matches the escape codes that colour text, so a test can read a
// screen as plain text.
var styleCode = regexp.MustCompile("\x1b\\[[0-9;:]*m")

func TestKeysScreen(t *testing.T) {
	t.Parallel()
	for _, width := range []int{200, 60} {
		t.Run(fmt.Sprintf("width %d", width), func(t *testing.T) {
			t.Parallel()
			m, _ := newTestModel(t, map[string]string{})
			m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
			screen := styleCode.ReplaceAllString(strings.Join(m.helpLines(), "\n"), "")
			for _, b := range m.helpBindings() {
				if b.label == "" {
					continue
				}
				if !strings.Contains(screen, b.label) || !strings.Contains(screen, b.desc) {
					t.Errorf("keys screen lacks %q %q", b.label, b.desc)
				}
			}
			for _, l := range m.helpLines() {
				if w := lipgloss.Width(l); w > width {
					t.Errorf("line is %d wide on a %d terminal: %q", w, width, l)
				}
			}
		})
	}
}

func TestKeysScreenScrollsAndCloses(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t, map[string]string{})
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	press(m, "?")
	if m.mode != modeHelp {
		t.Fatalf("mode = %v, want the keys screen", m.mode)
	}
	press(m, "j", "j")
	if m.helpOffset != 2 {
		t.Errorf("offset = %d, want 2", m.helpOffset)
	}
	press(m, "k", "k", "k")
	if m.helpOffset != 0 {
		t.Errorf("offset = %d, want it held at 0", m.helpOffset)
	}
	press(m, "down", "down", "up")
	if m.helpOffset != 1 {
		t.Errorf("arrow keys offset = %d, want 1", m.helpOffset)
	}
	press(m, "x")
	if m.mode != modeNormal {
		t.Error("any other key should close the keys screen")
	}
}

func TestEveryTriggerIsBoundOnce(t *testing.T) {
	t.Parallel()
	for name, list := range map[string][]binding{"normal": normalBindings(), "insert": insertBindings(), "migrate": migrateBindings()} {
		seen := map[string]bool{}
		for _, b := range list {
			if b.run == nil {
				t.Errorf("%s: %v has no action", name, b.triggers)
			}
			for _, tr := range b.triggers {
				if seen[tr] {
					t.Errorf("%s: %q is bound twice", name, tr)
				}
				seen[tr] = true
			}
		}
		k := newKeymap(list)
		for p := range k.prefixes {
			if _, ok := k.byTrigger[p]; ok {
				t.Errorf("%s: %q is both a key and the start of a sequence, so it could never run", name, p)
			}
		}
	}
}

func TestViewKeys(t *testing.T) {
	t.Parallel()
	for _, prefix := range []string{"ctrl+a", "ctrl+b"} {
		t.Run(prefix, func(t *testing.T) {
			t.Parallel()
			m, _ := newTestModel(t, map[string]string{"plans/2026/2026-10-25.plan": "* x\n"})
			for _, want := range []viewKind{viewOpen, viewArchive, viewProject, viewDay} {
				press(m, prefix, "n")
				if m.view != want {
					t.Fatalf("%s n view = %v, want %v", prefix, m.view, want)
				}
			}
			press(m, prefix, "p")
			if m.view != viewProject {
				t.Errorf("%s p view = %v, want the project view, wrapping round", prefix, m.view)
			}
			for i, want := range []viewKind{viewDay, viewOpen, viewArchive, viewProject} {
				press(m, prefix, fmt.Sprint(i+1))
				if m.view != want {
					t.Errorf("%s %d view = %v, want %v", prefix, i+1, m.view, want)
				}
			}
			press(m, prefix, "x")
			if m.view != viewProject || m.pending != "" {
				t.Errorf("%s then an unbound key: view = %v pending = %q, want nothing to happen", prefix, m.view, m.pending)
			}
		})
	}
}

func TestCtrlAWhileTypingStaysInTheLine(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t, map[string]string{"plans/2026/2026-10-25.plan": "* x\n"})
	press(m, "o")
	typeText(m, "abc")
	press(m, "ctrl+a")
	typeText(m, "n")
	if m.mode != modeInsert || m.view != viewDay || m.input.Value() != "n- abc" {
		t.Errorf("mode = %v view = %v value = %q, want ctrl+a to jump to the start of the line", m.mode, m.view, m.input.Value())
	}
}

func TestQuitKeysSave(t *testing.T) {
	t.Parallel()
	for _, keys := range [][]string{{"Z", "Z"}, {"ctrl+c"}} {
		t.Run(strings.Join(keys, ""), func(t *testing.T) {
			t.Parallel()
			m, st := newTestModel(t, map[string]string{"plans/2026/2026-10-25.plan": "- thing\n"})
			press(m, "space")
			var cmd tea.Cmd
			for _, k := range keys {
				_, cmd = m.Update(keyMsg(k))
			}
			if cmd == nil {
				t.Fatal("no command, want the program to quit")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Error("want the program to quit")
			}
			if got := readFile(t, st.EntryPath(today)); got != "* thing\n" {
				t.Errorf("entry = %q, want it saved", got)
			}
		})
	}
}

func TestSaveOnExitKeepsUnsavedWork(t *testing.T) {
	t.Parallel()
	m, st := newTestModel(t, map[string]string{"plans/2026/2026-10-25.plan": "- one\n"})
	press(m, "space", "o")
	typeText(m, "half typed")
	if err := m.saveOnExit(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, st.EntryPath(today)); got != "* one\n- half typed\n" {
		t.Errorf("entry = %q, want the mark and the line being typed kept", got)
	}
}

func TestHintKeysAreBound(t *testing.T) {
	t.Parallel()
	for hint, list := range map[string][]binding{normalHint: normalBindings(), migrateHint: migrateBindings()} {
		k := newKeymap(list)
		for _, pair := range strings.Split(hint, "  ") {
			key := strings.Fields(pair)[0]
			// A sequence such as dd is written run together in the hint
			_, whole := k.byTrigger[key]
			_, spelled := k.byTrigger[strings.Join(strings.Split(key, ""), " ")]
			if !whole && !spelled && !k.prefixes[key] {
				t.Errorf("the hint %q names %q, which is not bound", hint, key)
			}
		}
	}
}

func TestMarkHintListsEveryMarker(t *testing.T) {
	t.Parallel()
	k := newKeymap(normalBindings())
	var hinted string
	for _, pair := range strings.Split(markHint, "  ") {
		key := strings.Fields(pair)[0]
		if len(key) != 1 || !entry.IsMarker(key[0]) {
			continue
		}
		hinted += key
		if _, ok := k.byTrigger["r "+key]; !ok {
			t.Errorf("the mark hint names r%s, which is not bound", key)
		}
	}
	if hinted != entry.Markers {
		t.Errorf("the mark hint lists %q, want every marker %q", hinted, entry.Markers)
	}
}

func TestMarkHintShowsAfterR(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t, map[string]string{"plans/2026/2026-10-25.plan": "- a\n"})
	press(m, "r")
	if got := m.hintLine(); !strings.Contains(got, "migrate") || strings.Contains(got, "dd delete") {
		t.Errorf("hint after r = %q, want the markers in place of the normal keys", got)
	}
	press(m, "esc")
	if got := m.hintLine(); !strings.Contains(got, "dd delete") || m.entries[today].Lines[0] != "- a" {
		t.Errorf("after r esc: hint = %q line = %q, want the normal hint and the line unchanged", got, m.entries[today].Lines[0])
	}
}
