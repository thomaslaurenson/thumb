package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/thomaslaurenson/thumb/internal/entry"
	"github.com/thomaslaurenson/thumb/internal/project"
)

// The look is tmux's default: a black on green status bar, and black on
// yellow for messages, prompts and the selected row, as tmux draws its
// messages and copy-mode selection. Colours are the 16 ANSI ones, so they
// follow the terminal's theme, as tmux's do, rather than assuming a
// background.
var (
	styleBar     = lipgloss.NewStyle().Background(lipgloss.Color("2")).Foreground(lipgloss.Color("0"))
	styleMessage = lipgloss.NewStyle().Background(lipgloss.Color("3")).Foreground(lipgloss.Color("0"))
	styleCursor  = lipgloss.NewStyle().Background(lipgloss.Color("3")).Foreground(lipgloss.Color("0"))
	styleFaint   = lipgloss.NewStyle().Faint(true)
	styleBold    = lipgloss.NewStyle().Bold(true)
	styleWarn    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	markerStyles = map[byte]lipgloss.Style{
		entry.Done:    lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		entry.Later:   lipgloss.NewStyle().Foreground(lipgloss.Color("6")),
		entry.Todo:    lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		entry.Dropped: lipgloss.NewStyle().Faint(true).Strikethrough(true),
		entry.Blocked: lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true),
		entry.Idea:    lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
		entry.Moved:   lipgloss.NewStyle().Faint(true),
	}
)

// promptStyles draw a footer prompt over the status bar in the message style,
// as tmux draws its command prompt.
var promptStyles = func() textinput.Styles {
	state := textinput.StyleState{
		Text:        styleMessage,
		Prompt:      styleMessage,
		Placeholder: styleMessage.Faint(true),
		Suggestion:  styleMessage.Faint(true),
	}
	return textinput.Styles{
		Focused: state,
		Blurred: state,
		Cursor:  textinput.CursorStyle{Color: lipgloss.Color("0"), Shape: tea.CursorBlock, Blink: true},
	}
}()

// View draws the body for the current mode, the hint line, then the status
// bar along the bottom.
func (m *model) View() tea.View {
	var body string
	switch m.mode {
	case modeHelp:
		body = m.keysBody()
	case modeRollover:
		body = m.rolloverBody()
	default:
		body = m.body()
	}
	v := tea.NewView(body + "\n" + m.hintLine() + "\n" + m.statusLine())
	v.AltScreen = true
	return v
}

// statusLine is the bottom line. A prompt or a message takes it over in the
// message style, as tmux's command prompt and messages do; otherwise it is the
// status bar. The migrate picker shows its messages in its panel instead, so
// the day being viewed stays in sight while picking another.
func (m *model) statusLine() string {
	switch {
	case m.mode == modeInput:
		v := m.input.View()
		return m.fit(v + styleMessage.Render(strings.Repeat(" ", max(m.width-lipgloss.Width(v), 0))))
	case m.status != "" && m.mode != modeMigrate:
		return m.fill(styleMessage, m.status)
	}
	return m.fill(styleBar, m.statusBar())
}

// statusBar lays out the bar as tmux's default status line: the repository's
// folder on the left where tmux shows the session, the views as windows
// centred between the ends, and the time and date on the right. The current
// view is flagged "*" and the one before it "-", and the day view is named for
// the day it shows, as tmux renames a window for the program running in it.
func (m *model) statusBar() string {
	left := "[" + m.name + "] "
	var windows []string
	for i, name := range viewNames {
		v := viewKind(i)
		if v == viewDay {
			name = m.day
		}
		flag := " "
		switch v {
		case m.view:
			flag = "*"
		case m.prevView:
			flag = "-"
		}
		windows = append(windows, fmt.Sprintf("%d:%s%s", i+1, name, flag))
	}
	centre := strings.Join(windows, " ")
	right := m.now().Format("15:04 02-Jan-06")
	if m.unsaved() {
		right = "[unsaved] " + right
	}

	lw, cw, rw := lipgloss.Width(left), lipgloss.Width(centre), lipgloss.Width(right)
	if lw+cw+rw > m.width {
		// Too narrow for all three: keep the views, which change, over the clock
		right, rw = "", 0
	}
	gap := max(m.width-lw-cw-rw, 0)
	return left + strings.Repeat(" ", gap/2) + centre + strings.Repeat(" ", gap-gap/2) + right
}

func (m *model) body() string {
	lines := make([]string, 0, m.bodyHeight())
	if len(m.rows) == 0 {
		lines = append(lines, styleFaint.Render(m.emptyText()))
	}
	end := min(m.offset+m.bodyHeight(), len(m.rows))
	for i := m.offset; i < end; i++ {
		text := m.renderRow(m.rows[i])
		switch {
		case i == m.cursor && m.mode == modeInsert:
			text = m.input.View()
			if m.view == viewOpen {
				r := m.rows[i]
				text = styleFaint.Render(fmt.Sprintf("%-14s ", fmt.Sprintf("%s:%d", r.date, r.line))) + text
			}
		case i == m.cursor:
			text = styleCursor.Render(m.fitPlain(m.rowPlain(m.rows[i])))
		}
		lines = append(lines, m.fit(text))
	}
	for len(lines) < m.bodyHeight() {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (m *model) emptyText() string {
	switch {
	case m.filter != "":
		return "Nothing matches the filter. esc clears it."
	case m.view == viewDay:
		return "No entry for " + m.day + " yet. o adds a line, ? lists every key."
	case m.view == viewOpen:
		return "Nothing open."
	case m.view == viewProject:
		return "No project lines. o adds one, such as: BUILDING thumb"
	default:
		return "No entries yet."
	}
}

// rowPlain is the unstyled form of a row, used under the cursor so the
// highlight covers the whole line evenly.
func (m *model) rowPlain(r row) string {
	if m.view == viewOpen {
		return fmt.Sprintf("%-14s %s", fmt.Sprintf("%s:%d", r.date, r.line), m.rowText(r))
	}
	return m.rowText(r)
}

func (m *model) renderRow(r row) string {
	switch m.view {
	case viewProject:
		if it, ok := project.ParseItem(m.rowText(r)); ok {
			return styleBold.Render(it.Keyword) + " " + it.Text
		}
		return styleFaint.Render(m.rowText(r))
	case viewArchive:
		e := m.entries[r.date]
		if e.Empty() {
			return r.date + "  " + styleFaint.Render("(empty)")
		}
		t := e.Tally()
		return styleBold.Render(r.date) + "  " + t.String() + "  " + styleFaint.Render(t.FirstDone)
	case viewOpen:
		ref := fmt.Sprintf("%-14s ", fmt.Sprintf("%s:%d", r.date, r.line))
		return styleFaint.Render(ref) + renderLine(m.rowText(r))
	default:
		return renderLine(m.rowText(r))
	}
}

// renderLine colours a line by its marker; prose is left plain.
func renderLine(s string) string {
	l := entry.ParseLine(s)
	if !l.IsPlan() {
		return s
	}
	return markerStyles[l.Marker].Render(s)
}

// keysLabelWidth is the width of the key column on the keys screen.
const keysLabelWidth = 15

// keysSection renders one section of the keys screen: a heading, then a row
// per binding with a label. The marker in a marking key is drawn as it looks
// in a plan, so the section doubles as the legend for the file format.
func keysSection(sec section, list []binding) []string {
	lines := []string{styleBold.Render(sectionNames[sec])}
	for _, b := range list {
		if b.section != sec || b.label == "" {
			continue
		}
		label := b.label
		if style, ok := markerStyles[b.marker]; ok && b.marker != 0 {
			label = strings.Replace(b.label, string(b.marker), style.Render(string(b.marker)), 1)
		}
		pad := strings.Repeat(" ", max(keysLabelWidth-len(b.label), 1))
		lines = append(lines, "  "+label+pad+b.desc)
	}
	return lines
}

// helpLines lays out the keys screen: two columns when the terminal is wide
// enough to hold them side by side, otherwise one column to scroll.
func (m *model) helpLines() []string {
	list := m.helpBindings()
	stack := func(secs ...section) []string {
		var out []string
		for i, sec := range secs {
			if i > 0 {
				out = append(out, "")
			}
			out = append(out, keysSection(sec, list)...)
		}
		return out
	}
	left := stack(sectionMove, sectionWrite, sectionDays, sectionTyping)
	right := stack(sectionMark, sectionMigrate, sectionOther)
	footnote := []string{
		"",
		styleFaint.Render("A plan line is a marker, a space, then text."),
		styleFaint.Render("Any other line is prose."),
	}

	leftWidth := 0
	for _, l := range left {
		leftWidth = max(leftWidth, lipgloss.Width(l))
	}
	rightWidth := 0
	for _, l := range right {
		rightWidth = max(rightWidth, lipgloss.Width(l))
	}
	const gap = 4
	if leftWidth+gap+rightWidth > m.width {
		out := append(left, "")
		out = append(out, right...)
		return append(out, footnote...)
	}
	var out []string
	for i := range max(len(left), len(right)) {
		var l, r string
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		out = append(out, l+strings.Repeat(" ", leftWidth+gap-lipgloss.Width(l))+r)
	}
	return append(out, footnote...)
}

// keysBody is the visible part of the keys screen at the current scroll.
func (m *model) keysBody() string {
	lines := m.helpLines()
	start := min(m.helpOffset, max(len(lines)-m.bodyHeight(), 0))
	end := min(start+m.bodyHeight(), len(lines))
	return m.pad(slices.Clone(lines[start:end]))
}

func (m *model) rolloverBody() string {
	var lines []string
	if !m.oneByOne {
		lines = append(lines,
			fmt.Sprintf("%s has %d open item(s) and today has no entry yet.", m.rolloverFrom, len(m.rollover)),
			"",
			"  a  migrate all of them to today",
			"  o  go through them one by one",
			"  n  not now",
		)
		return m.pad(lines)
	}
	ref := m.rollover[0]
	lines = append(lines,
		fmt.Sprintf("Migrate to today? (%d left)", len(m.rollover)),
		"",
		"  "+renderLine(m.entries[ref.Date].Lines[ref.Line-1]),
		"",
		"  y  migrate    n  skip    q  stop",
	)
	return m.pad(lines)
}

// normalHint is the hint line while browsing. It is written out rather than
// built from the keymap, so a test checks that every key it names is bound.
const normalHint = "? keys  o add  i edit  dd delete  space done  r mark  u undo  ZZ quit"

// markHint takes the hint line's place after r, while thumb waits for the
// marker to set. It is written out too, and a test checks it against the keymap.
const markHint = "* done  + later  - to do  ~ dropped  ! blocked  ? idea  > migrate  esc cancels"

// markHintLine draws markHint with each marker as it looks in a plan, so the
// hint doubles as the legend, as the keys screen does.
func markHintLine() string {
	parts := strings.Split(markHint, "  ")
	for i, p := range parts {
		if style, ok := markerStyles[p[0]]; ok && len(p) > 1 && p[1] == ' ' {
			parts[i] = style.Render(p[:1]) + styleFaint.Render(p[1:])
		} else {
			parts[i] = styleFaint.Render(p)
		}
	}
	return strings.Join(parts, "  ")
}

// migratePanelHeight is how many lines the migrate picker's panel takes in
// place of the hint line.
const migratePanelHeight = 3

// migrateHint is the last line of the migrate picker's panel. Like normalHint
// it is written out, and a test checks every key it names is bound.
const migrateHint = "h l day  j k week  t today  tab entry day  i type  enter migrate  esc cancel"

// migratePanel is the migrate picker: the move in words, the week around the
// picked day, then the keys. It sits above the status bar, which stays, so
// the day being viewed and today's date are in sight while picking.
func (m *model) migratePanel() string {
	from := m.target.date
	move := markerStyles[entry.Moved].Render(">") + styleFaint.Render(" from ") + dayWords(from, m.today) + styleFaint.Render("  to  ")
	if m.pickTyping {
		move += m.input.View() + "  "
	}
	picked := styleBold.Render(dayWords(m.pick, m.today))
	switch {
	case m.status != "":
		move += styleWarn.Render(m.status)
	case m.pickTyping && strings.TrimSpace(m.input.Value()) != m.pick:
		// Part of a date, so there is no day to name yet
		move += styleFaint.Render("YYYY-MM-DD")
	case m.pick == from:
		move += picked + "  " + styleWarn.Render("same day, pick another")
	default:
		move += picked + styleFaint.Render(", "+relativeDay(m.pick, m.today))
	}
	keys := migrateHint
	if m.pickTyping {
		keys = "enter migrate  esc stop typing"
	}
	return m.fit(move) + "\n" + m.fit(m.weekStrip()) + "\n" + styleFaint.Render(m.fitPlain(keys))
}

// weekStrip is the week, Monday first, around the picked day. The picked day
// is drawn as the selected row is, the line's own day faint, today bold, and a
// day that already has an entry underlined.
func (m *model) weekStrip() string {
	t, err := time.Parse(dayLayout, m.pick)
	if err != nil {
		return ""
	}
	monday := t.AddDate(0, 0, -((int(t.Weekday()) + 6) % 7))
	cells := make([]string, 7)
	for i := range cells {
		day := monday.AddDate(0, 0, i)
		date := day.Format(dayLayout)
		style := lipgloss.NewStyle()
		if e := m.entries[date]; e != nil && !e.Empty() {
			style = style.Underline(true)
		}
		if date == m.today {
			style = style.Bold(true)
		}
		if date == m.target.date {
			style = style.Faint(true)
		}
		if date == m.pick {
			style = style.Inherit(styleCursor)
		}
		cells[i] = style.Render(day.Format("Mon 02"))
	}
	return "  " + strings.Join(cells, "  ")
}

// dayWords writes a date as it is said, such as "Wed 28 Oct", adding the year
// when it is not the same as today's.
func dayWords(date, today string) string {
	t, err := time.Parse(dayLayout, date)
	if err != nil {
		return date
	}
	if date[:4] != today[:4] {
		return t.Format("Mon 2 Jan 2006")
	}
	return t.Format("Mon 2 Jan")
}

// relativeDay says how far date is from today: today, tomorrow, yesterday,
// in 5 days or 3 days ago.
func relativeDay(date, today string) string {
	d, err1 := time.Parse(dayLayout, date)
	t, err2 := time.Parse(dayLayout, today)
	if err1 != nil || err2 != nil {
		return ""
	}
	// Both parse as UTC midnight, so the gap is whole days
	switch n := int(d.Sub(t).Hours() / 24); {
	case n == 0:
		return "today"
	case n == 1:
		return "tomorrow"
	case n == -1:
		return "yesterday"
	case n > 1:
		return fmt.Sprintf("in %d days", n)
	default:
		return fmt.Sprintf("%d days ago", -n)
	}
}

// hintLine sits above the status bar, where vim's command line sits inside a
// tmux pane: the keys for the current mode, and any unfinished key sequence at
// the right. The migrate picker's panel takes its place.
func (m *model) hintLine() string {
	switch m.mode {
	case modeInput:
		return styleFaint.Render(m.fitPlain("enter accepts  esc cancels"))
	case modeMigrate:
		return m.migratePanel()
	case modeInsert:
		hint := "  enter next line  esc done"
		if !m.ins.continues {
			hint = "  enter or esc keep"
		}
		return m.fit(styleBold.Render("-- INSERT --") + styleFaint.Render(hint))
	case modeHelp:
		return styleFaint.Render(m.fitPlain("j k scroll  any other key closes"))
	case modeRollover:
		return ""
	}
	hint := styleFaint.Render(normalHint)
	switch {
	case m.pending == "r":
		hint = markHintLine()
	case m.filter != "":
		hint = styleFaint.Render("filter: " + m.filter + "  (esc clears)")
	}
	if m.pending != "" {
		// The keys of an unfinished sequence, such as the first d of dd, sit at
		// the right as vim shows them
		width := max(m.width-len(m.pending)-1, 0)
		return lipgloss.NewStyle().Inline(true).Width(width).MaxWidth(width).Render(hint) + " " + m.pending
	}
	return m.fit(hint)
}

// pad fills a block of lines to the body height so the footer stays put.
func (m *model) pad(lines []string) string {
	for i := range lines {
		lines[i] = m.fit(lines[i])
	}
	for len(lines) < m.bodyHeight() {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// fit cuts a styled line to the terminal width.
func (m *model) fit(s string) string {
	return lipgloss.NewStyle().Inline(true).MaxWidth(m.width).Render(s)
}

// fitPlain cuts and pads a plain line to the full width, so a highlight spans
// the terminal.
func (m *model) fitPlain(s string) string {
	return m.fill(lipgloss.NewStyle(), s)
}

// fill cuts and pads a plain line to the full width in style, so a bar spans
// the terminal.
func (m *model) fill(style lipgloss.Style, s string) string {
	return style.Inline(true).Width(m.width).MaxWidth(m.width).Render(s)
}
