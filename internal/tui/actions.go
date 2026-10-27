package tui

import (
	"maps"
	"slices"
	"strings"

	"github.com/thomaslaurenson/thumb/internal/entry"
	"github.com/thomaslaurenson/thumb/internal/project"
)

// refresh rebuilds the rows of the current view and keeps the cursor valid.
func (m *model) refresh() {
	m.rows = m.buildRows()
	m.clamp()
}

func (m *model) buildRows() []row {
	var rows []row
	switch m.view {
	case viewDay:
		if e := m.entries[m.day]; e != nil {
			for i := range e.Lines {
				rows = append(rows, row{date: m.day, line: i + 1})
			}
		}
	case viewOpen:
		for _, d := range m.dates {
			for _, it := range m.entries[d].OpenItems() {
				rows = append(rows, row{date: d, line: it.Ref.Line})
			}
		}
	case viewArchive:
		for i := len(m.dates) - 1; i >= 0; i-- {
			rows = append(rows, row{date: m.dates[i]})
		}
	case viewProject:
		for i := range m.project.Lines {
			rows = append(rows, row{line: i + 1})
		}
	}
	if m.filter == "" {
		return rows
	}
	needle := strings.ToLower(m.filter)
	return slices.DeleteFunc(rows, func(r row) bool {
		return !strings.Contains(strings.ToLower(m.rowText(r)), needle)
	})
}

// rowText is the plain text a row shows, which filtering searches.
func (m *model) rowText(r row) string {
	switch {
	case m.view == viewProject:
		return m.project.Lines[r.line-1]
	case r.line == 0:
		e := m.entries[r.date]
		t := e.Tally()
		if e.Empty() {
			return r.date + "  (empty)"
		}
		return r.date + "  " + t.String() + "  " + t.FirstDone
	default:
		return m.entries[r.date].Lines[r.line-1]
	}
}

// bodyHeight leaves the bottom lines for the status bar and, above it, the
// hint line or the migrate picker's panel.
func (m *model) bodyHeight() int {
	footer := 2
	if m.mode == modeMigrate {
		footer = 1 + migratePanelHeight
	}
	return max(m.height-footer, 1)
}

func (m *model) clamp() {
	m.cursor = min(max(m.cursor, 0), max(len(m.rows)-1, 0))
	h := m.bodyHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	m.offset = max(m.offset, 0)
}

func (m *model) move(n int) {
	m.cursor += n
	m.clamp()
}

// setView changes view, remembering the one it leaves for the status bar.
func (m *model) setView(v viewKind) {
	if v != m.view {
		m.prevView = m.view
		m.view = v
	}
}

func (m *model) switchView(v viewKind) {
	if v == m.view {
		return
	}
	m.setView(v)
	m.cursor, m.offset = 0, 0
	m.filter = ""
	m.refresh()
}

// showDay switches to the day view of date with the cursor on line.
func (m *model) showDay(date string, line int) {
	m.setView(viewDay)
	m.day = date
	m.filter = ""
	m.refresh()
	m.cursor = max(line-1, 0)
	m.clamp()
}

// stepDay moves the day view to the previous or next day that has an entry,
// with today always reachable.
func (m *model) stepDay(dir int) {
	if m.view != viewDay {
		return
	}
	days := slices.Clone(m.dates)
	if !slices.Contains(days, m.today) {
		days = append(days, m.today)
		slices.Sort(days)
	}
	if !slices.Contains(days, m.day) {
		days = append(days, m.day)
		slices.Sort(days)
	}
	i := slices.Index(days, m.day) + dir
	if i < 0 || i >= len(days) {
		return
	}
	m.showDay(days[i], 0)
}

func (m *model) selected() (row, bool) {
	if len(m.rows) == 0 {
		return row{}, false
	}
	return m.rows[m.cursor], true
}

// selectedLine returns the entry line under the cursor, for the views whose
// rows are entry lines.
func (m *model) selectedLine() (row, bool) {
	r, ok := m.selected()
	if !ok || (m.view != viewDay && m.view != viewOpen) {
		return row{}, false
	}
	return r, true
}

func (m *model) open() {
	r, ok := m.selected()
	if !ok {
		return
	}
	switch m.view {
	case viewArchive:
		m.showDay(r.date, 0)
	case viewOpen:
		m.showDay(r.date, r.line)
	}
}

// ensure returns the entry for date, creating an empty one if needed.
func (m *model) ensure(date string) *entry.Entry {
	if e := m.entries[date]; e != nil {
		return e
	}
	e := entry.New(date)
	m.entries[date] = e
	m.dates = append(m.dates, date)
	slices.Sort(m.dates)
	return e
}

// pushEntries records the named entries before a change, for undo.
func (m *model) pushEntries(dates ...string) {
	s := m.baseSnapshot()
	s.entries = map[string]*entry.Entry{}
	for _, d := range dates {
		if e := m.entries[d]; e != nil {
			s.entries[d] = e.Clone()
		} else {
			s.entries[d] = nil
		}
	}
	m.undo = append(m.undo, s)
}

// pushProject records the project before a change, for undo.
func (m *model) pushProject() {
	s := m.baseSnapshot()
	s.project = m.project.Clone()
	s.projectDirty = m.projectDirty
	m.undo = append(m.undo, s)
}

func (m *model) baseSnapshot() snapshot {
	return snapshot{dates: slices.Clone(m.dates), view: m.view, day: m.day, cursor: m.cursor}
}

func (m *model) undoLast() {
	if len(m.undo) == 0 {
		m.status = "Nothing to undo"
		return
	}
	s := m.undo[len(m.undo)-1]
	m.undo = m.undo[:len(m.undo)-1]
	m.restore(s, true)
	m.status = "Undone"
}

// restore puts back the state a snapshot recorded. markDirty flags what it
// restored as unsaved; rolling back a session that changed nothing leaves
// the flags as they were.
func (m *model) restore(s snapshot, markDirty bool) {
	for d, e := range s.entries {
		if e == nil {
			delete(m.entries, d)
			delete(m.dirty, d)
			continue
		}
		m.entries[d] = e
		if markDirty {
			m.dirty[d] = true
		}
	}
	if s.project != nil {
		m.project = s.project
		if markDirty {
			m.projectDirty = true
		}
	}
	m.dates = s.dates
	m.setView(s.view)
	m.day = s.day
	m.refresh()
	m.cursor = s.cursor
	m.clamp()
}

func (m *model) setMarker(marker byte) {
	r, ok := m.selectedLine()
	if !ok {
		return
	}
	e := m.entries[r.date]
	if entry.IsBlank(e.Lines[r.line-1]) {
		m.status = "Blank line, nothing to mark"
		return
	}
	if marker == entry.Moved {
		m.startMigrate()
		return
	}
	// A new marker would drop the target date, leaving the item on both days
	if entry.ParseLine(e.Lines[r.line-1]).Marker == entry.Moved {
		m.status = "Moved line, nothing to mark"
		return
	}
	m.pushEntries(r.date)
	if err := e.SetMarker(r.line, marker); err != nil {
		m.status = err.Error()
		return
	}
	m.dirty[r.date] = true
	m.refresh()
}

// toggleDone marks the selected item done: done that day on today or a later
// day, done later on an earlier one. A done item goes back to to do.
func (m *model) toggleDone() {
	r, ok := m.selectedLine()
	if !ok {
		return
	}
	switch entry.ParseLine(m.entries[r.date].Lines[r.line-1]).Marker {
	case entry.Done, entry.Later:
		m.setMarker(entry.Todo)
	default:
		if r.date < m.today {
			m.setMarker(entry.Later)
		} else {
			m.setMarker(entry.Done)
		}
	}
}

// insertNew opens a fresh line to type on, below the cursor when below is 1
// and above it when 0. From the open and archive views the line goes at the
// end of today, and the day view opens there so the line can be seen.
func (m *model) insertNew(below int) {
	n := 1
	if r, ok := m.selected(); ok {
		n = r.line + below
	}
	switch m.view {
	case viewProject:
		m.startInsert("")
		m.openLine(n, true, "")
	case viewDay:
		m.startInsert(m.day)
		m.openLine(n, true, "- ")
	default:
		m.showDay(m.today, 0)
		n = 1
		if e := m.entries[m.today]; e != nil {
			n = len(e.Lines) + 1
		}
		m.startInsert(m.today)
		m.openLine(n, true, "- ")
	}
}

// insertEdit starts typing on the selected line, with the cursor at the start
// of its text, after any marker, or at its end.
func (m *model) insertEdit(atStart bool) {
	r, ok := m.selected()
	if !ok || m.view == viewArchive {
		return
	}
	m.startInsert(r.date)
	m.openLine(r.line, false, "")
	if !atStart {
		return
	}
	if l := entry.ParseLine(m.input.Value()); l.IsPlan() && r.date != "" {
		m.input.SetCursor(2)
	} else {
		m.input.CursorStart()
	}
}

// startInsert begins a typing session on date, or on the project for "". The
// whole session is one undo step, however many lines it types.
func (m *model) startInsert(date string) {
	if date == "" {
		m.pushProject()
	} else {
		m.pushEntries(date)
	}
	m.ins = insertState{date: date, continues: m.view == viewDay || m.view == viewProject}
	m.mode = modeInsert
	m.input.SetStyles(m.lineStyles)
	m.input.Prompt = ""
	m.input.Placeholder = ""
	m.input.SetWidth(m.insertWidth())
	m.input.Focus()
}

// openLine puts the cursor on line n to type on it. A fresh line is inserted
// there first, holding prefill, so the row exists before anything is typed.
func (m *model) openLine(n int, fresh bool, prefill string) {
	if fresh {
		n = min(n, m.lineCount(m.ins.date)+1)
		if m.ins.date == "" {
			m.project.Lines = slices.Insert(m.project.Lines, n-1, prefill)
		} else {
			m.ensure(m.ins.date).Insert(n, prefill)
		}
	}
	m.ins.line, m.ins.fresh, m.ins.prefill = n, fresh, prefill
	m.focus(m.ins.date, n)
	m.input.SetValue(m.lineText(m.ins.date, n))
	m.input.CursorEnd()
}

func (m *model) lineCount(date string) int {
	if date == "" {
		return len(m.project.Lines)
	}
	if e := m.entries[date]; e != nil {
		return len(e.Lines)
	}
	return 0
}

func (m *model) lineText(date string, n int) string {
	if date == "" {
		return m.project.Lines[n-1]
	}
	return m.entries[date].Lines[n-1]
}

// focus clears any filter and puts the cursor on line n of date, so the line
// being typed is always on screen.
func (m *model) focus(date string, n int) {
	m.filter = ""
	m.refresh()
	for i, r := range m.rows {
		if r.date == date && r.line == n {
			m.cursor = i
			break
		}
	}
	m.clamp()
}

// insertWidth leaves room for the ref the open view prints before each line.
func (m *model) insertWidth() int {
	w := m.width - 1
	if m.view == viewOpen {
		w -= 15
	}
	return max(w, 10)
}

// keepLine writes the typed value into the line. A project line has to be an
// item, a comment or blank, so anything else stays open to be corrected.
func (m *model) keepLine(value string) bool {
	date, n := m.ins.date, m.ins.line
	if date == "" {
		if t := strings.TrimSpace(value); t != "" && !strings.HasPrefix(t, "#") {
			if _, ok := project.ParseItem(value); !ok {
				m.status = "Expected an uppercase KEYWORD then a description; esc throws it away"
				return false
			}
		}
	}
	if !m.ins.fresh && value == m.lineText(date, n) {
		return true
	}
	if date == "" {
		m.project.Lines[n-1] = value
		m.projectDirty = true
	} else {
		m.entries[date].Lines[n-1] = value
		m.dirty[date] = true
	}
	m.ins.changed = true
	return true
}

// dropFresh removes a fresh line that was never kept.
func (m *model) dropFresh() {
	if !m.ins.fresh {
		return
	}
	n := m.ins.line
	if m.ins.date == "" {
		m.project.Lines = slices.Delete(m.project.Lines, n-1, n)
	} else if err := m.entries[m.ins.date].Delete(n); err != nil {
		m.status = err.Error()
	}
	m.ins.line = max(n-1, 1)
}

// insertEnter keeps the line and opens the next one below with the same
// marker, so a list is typed one line after another. Enter on a fresh line
// still holding only its marker ends typing instead, without keeping it.
func (m *model) insertEnter() {
	value := m.input.Value()
	if m.ins.fresh && value == m.ins.prefill && value != "" {
		m.dropFresh()
		m.endInsert()
		return
	}
	if !m.keepLine(value) {
		return
	}
	if !m.ins.continues {
		m.endInsert()
		return
	}
	prefill := ""
	if l := entry.ParseLine(value); l.IsPlan() && m.ins.date != "" {
		prefill = string(l.Marker) + " "
		if l.Marker == entry.Moved {
			prefill = string(entry.Todo) + " "
		}
	}
	m.openLine(m.ins.line+1, true, prefill)
}

// insertEsc keeps the line and stops typing, unless it is a fresh line left
// as it started. A line that cannot be kept is thrown away, so esc always
// stops typing, as it does in vim.
func (m *model) insertEsc() {
	value := m.input.Value()
	switch {
	case m.ins.fresh && value == m.ins.prefill:
		m.dropFresh()
	case !m.keepLine(value):
		m.dropFresh()
		m.status = "Not kept: a project line is an uppercase KEYWORD then a description"
	}
	m.endInsert()
}

// endInsert returns to browsing. A session that kept nothing is rolled back
// silently, so it leaves no undo step and no unsaved marker behind.
func (m *model) endInsert() {
	m.mode = modeNormal
	m.input.Blur()
	if !m.ins.changed {
		s := m.undo[len(m.undo)-1]
		m.undo = m.undo[:len(m.undo)-1]
		m.restore(s, false)
		return
	}
	m.focus(m.ins.date, min(m.ins.line, m.lineCount(m.ins.date)))
}

func (m *model) deleteLine() {
	r, ok := m.selected()
	if !ok || m.view == viewArchive {
		return
	}
	if m.view == viewProject {
		m.pushProject()
		m.project.Lines = slices.Delete(m.project.Lines, r.line-1, r.line)
		m.projectDirty = true
		m.refresh()
		return
	}
	m.pushEntries(r.date)
	if err := m.entries[r.date].Delete(r.line); err != nil {
		m.status = err.Error()
		return
	}
	m.dirty[r.date] = true
	m.refresh()
}

// startMigrate opens the migrate picker for the selected line, on today, or on
// tomorrow when the line is already on today.
func (m *model) startMigrate() {
	r, ok := m.selectedLine()
	if !ok {
		return
	}
	l := entry.ParseLine(m.entries[r.date].Lines[r.line-1])
	if !entry.IsOpen(l.Marker) && l.Marker != entry.Idea {
		m.status = "Only open items and ideas can be migrated"
		return
	}
	m.target = r
	m.pick = m.today
	if r.date == m.today {
		m.pick = m.tomorrow()
	}
	m.mode = modeMigrate
	// The panel is taller than the hint line, so keep the line in view
	m.clamp()
}

// pickEntry moves the picker to the next day after it that has an entry, or
// the one before it for a negative dir, passing over the line's own day.
func (m *model) pickEntry(dir int) {
	for i := range m.dates {
		d := m.dates[i]
		if dir < 0 {
			d = m.dates[len(m.dates)-1-i]
		}
		if d == m.target.date || m.entries[d].Empty() {
			continue
		}
		if (dir > 0 && d > m.pick) || (dir < 0 && d < m.pick) {
			m.pick = d
			return
		}
	}
}

// typePick swaps stepping for typing the day, starting from the picked one.
func (m *model) typePick() {
	m.pickTyping = true
	m.input.SetStyles(m.lineStyles)
	m.input.Prompt = ""
	m.input.Placeholder = "YYYY-MM-DD"
	m.input.SetWidth(len(dayLayout))
	m.input.SetValue(m.pick)
	m.input.CursorEnd()
	m.input.Focus()
}

func (m *model) stopPickTyping() {
	m.pickTyping = false
	m.input.Blur()
}

// confirmPick migrates the line to the picked day. The line's own day is
// refused, as the panel says while it is picked.
func (m *model) confirmPick() {
	if m.pick == m.target.date {
		return
	}
	m.mode = modeNormal
	m.migrate(m.target, m.pick)
}

func (m *model) cancelPick() {
	m.mode = modeNormal
	m.clamp()
}

func (m *model) migrate(r row, date string) {
	m.pushEntries(r.date, date)
	if err := entry.Migrate(m.entries[r.date], r.line, m.ensure(date)); err != nil {
		m.undo = m.undo[:len(m.undo)-1]
		m.status = err.Error()
		m.refresh()
		return
	}
	m.dirty[r.date] = true
	m.dirty[date] = true
	m.status = "Moved to " + date
	m.refresh()
}

// save writes every changed entry and the project. An entry that is still
// empty and was never on disk is skipped, so browsing never litters the
// plans tree with empty files.
func (m *model) save() error {
	for _, d := range slices.Sorted(maps.Keys(m.dirty)) {
		e := m.entries[d]
		if e == nil || (e.Empty() && !m.st.Exists(d)) {
			delete(m.dirty, d)
			continue
		}
		if err := m.st.Save(e); err != nil {
			return err
		}
		delete(m.dirty, d)
	}
	if m.projectDirty {
		if err := m.st.SaveProject(m.project); err != nil {
			return err
		}
		m.projectDirty = false
	}
	return nil
}

func (m *model) unsaved() bool {
	return len(m.dirty) > 0 || m.projectDirty
}
