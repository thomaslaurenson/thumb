// Package tui is the interactive terminal interface: browse entries and mark
// items with vim keys, type lines in place, migrate open items between days,
// and keep the project list. Changes stay in memory until the user quits.
package tui

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/thomaslaurenson/thumb/internal/entry"
	"github.com/thomaslaurenson/thumb/internal/project"
	"github.com/thomaslaurenson/thumb/internal/store"
)

// Options is what the interface needs from the command that starts it.
type Options struct {
	Store *store.Store
	// Today is the local date, YYYY-MM-DD.
	Today string
	// Now reads the clock the status bar shows.
	Now func() time.Time
	In  io.Reader
	Out io.Writer
}

type viewKind int

const (
	viewDay viewKind = iota
	viewOpen
	viewArchive
	viewProject
)

// viewNames name the views in the status bar, lowercase like tmux windows.
var viewNames = []string{"day", "open", "archive", "project"}

const viewCount = viewKind(4)

type mode int

const (
	modeNormal mode = iota
	// modeInsert is typing on a line in place.
	modeInsert
	// modeInput is a one-line prompt in the footer, for the filter.
	modeInput
	// modeMigrate picks the day a line migrates to, in a panel above the
	// status bar.
	modeMigrate
	modeHelp
	modeRollover
)

// insertState describes the line being typed in modeInsert.
type insertState struct {
	// date is the entry being typed in, or "" for the project file
	date string
	// line is the one-based line being typed
	line int
	// fresh marks a line opened for this insert. Leaving it as it started
	// removes it again rather than keeping an empty marker.
	fresh   bool
	prefill string
	// continues makes enter open the next line below, in the day and project
	// views; elsewhere enter just keeps the line
	continues bool
	// changed records whether the session has kept anything, so a session
	// that changed nothing leaves no undo step behind
	changed bool
}

// row is one selectable line of the current view. For entry lines it names
// the entry and one-based line; archive rows carry only the date; project
// rows carry only the line.
type row struct {
	date string
	line int
}

// snapshot is the state an undo returns to. A nil entry means the entry did
// not exist before the change.
type snapshot struct {
	entries      map[string]*entry.Entry
	project      *project.Project
	projectDirty bool
	dates        []string
	view         viewKind
	day          string
	cursor       int
}

var _ tea.Model = (*model)(nil)

type model struct {
	st    *store.Store
	today string
	now   func() time.Time
	// name is the repository's folder name, shown where tmux shows the session
	name string

	entries      map[string]*entry.Entry
	dates        []string
	project      *project.Project
	dirty        map[string]bool
	projectDirty bool

	view viewKind
	// prevView is the view before this one, flagged "-" as tmux flags the
	// last window, or -1 before the first switch
	prevView viewKind
	day      string
	cursor   int
	offset   int
	rows     []row
	filter   string

	mode  mode
	input textinput.Model
	// lineStyles are the input's own styles, for typing a line in the body;
	// a footer prompt swaps in promptStyles instead
	lineStyles textinput.Styles
	// target is the line being migrated and pick the day the picker is on.
	// pickTyping is set while that day is typed rather than stepped to.
	target     row
	pick       string
	pickTyping bool
	ins        insertState

	keys        keymap
	insertKeys  keymap
	migrateKeys keymap
	// pending is a key sequence typed so far, such as "d" waiting for "dd"
	pending    string
	helpOffset int

	rolloverFrom string
	rollover     []entry.Ref
	oneByOne     bool

	undo   []snapshot
	status string
	width  int
	height int
}

// SaveError reports changes the interface could not save as it closed.
type SaveError struct{ Err error }

// Error says the save on exit failed, and why.
func (e *SaveError) Error() string { return "save on exit: " + e.Err.Error() }

// Unwrap returns the error the save failed with.
func (e *SaveError) Unwrap() error { return e.Err }

// Run loads the repository and runs the interface until the user quits. A
// failed save as it closes is reported as a *SaveError.
func Run(ctx context.Context, o Options) error {
	m, err := newModel(o)
	if err != nil {
		return err
	}
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(o.In), tea.WithOutput(o.Out))
	_, err = p.Run()
	return errors.Join(err, m.saveOnExit())
}

// saveOnExit keeps what the program still holds when it ends without a quit
// key, such as when the terminal hangs up over SSH. Every quit key saves first,
// so after one there is nothing left to save.
func (m *model) saveOnExit() error {
	if m.mode == modeInsert {
		m.insertEsc()
	}
	if !m.unsaved() {
		return nil
	}
	if err := m.save(); err != nil {
		return &SaveError{Err: err}
	}
	return nil
}

func newModel(o Options) (*model, error) {
	all, err := o.Store.LoadAll()
	if err != nil {
		return nil, err
	}
	proj, err := o.Store.LoadProject()
	if err != nil {
		return nil, err
	}
	m := &model{
		st:       o.Store,
		today:    o.Today,
		now:      o.Now,
		name:     filepath.Base(o.Store.Root),
		entries:  map[string]*entry.Entry{},
		project:  proj,
		dirty:    map[string]bool{},
		prevView: -1,
		day:      o.Today,
		width:    80,
		height:   24,

		keys:        newKeymap(normalBindings()),
		insertKeys:  newKeymap(insertBindings()),
		migrateKeys: newKeymap(migrateBindings()),
	}
	for _, e := range all {
		m.entries[e.Date] = e
		m.dates = append(m.dates, e.Date)
	}
	m.input = textinput.New()
	m.lineStyles = m.input.Styles()
	m.offerRollover()
	m.refresh()
	return m, nil
}

// offerRollover starts the rollover prompt when today has no content yet and
// the most recent earlier entry has open items.
func (m *model) offerRollover() {
	if e := m.entries[m.today]; e != nil && !e.Empty() {
		return
	}
	for i := len(m.dates) - 1; i >= 0; i-- {
		d := m.dates[i]
		if d >= m.today {
			continue
		}
		items := m.entries[d].OpenItems()
		if len(items) == 0 {
			return
		}
		m.rolloverFrom = d
		for _, it := range items {
			m.rollover = append(m.rollover, it.Ref)
		}
		m.mode = modeRollover
		return
	}
}

// Init starts the status bar clock; the repository is already loaded.
func (m *model) Init() tea.Cmd {
	return m.tick()
}

// clockMsg arrives when the minute changes, to redraw the status bar clock.
type clockMsg struct{}

// tick waits for the next minute on the clock, so the bar's time turns over
// when tmux's does rather than up to a minute late.
func (m *model) tick() tea.Cmd {
	now := m.now()
	next := now.Truncate(time.Minute).Add(time.Minute)
	return tea.Tick(next.Sub(now), func(time.Time) tea.Msg { return clockMsg{} })
}

// Update handles a resize, the clock, or a key, which goes to the handler for
// the current mode.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clamp()
		if m.mode == modeInsert {
			m.input.SetWidth(m.insertWidth())
		}
		return m, nil
	case clockMsg:
		return m, m.tick()
	case tea.KeyPressMsg:
		switch m.mode {
		case modeInsert:
			return m.updateInsert(msg)
		case modeInput:
			return m.updateInput(msg)
		case modeMigrate:
			return m.updateMigrate(msg)
		case modeHelp:
			return m.updateHelp(msg)
		case modeRollover:
			return m.updateRollover(msg)
		default:
			return m.updateNormal(msg)
		}
	}
	if m.mode == modeInput || m.mode == modeInsert {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *model) updateNormal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.status = ""
	b, next := m.keys.lookup(m.pending, msg.String())
	m.pending = next
	if b == nil {
		return m, nil
	}
	return m, b.run(m)
}

// updateInsert sends enter, esc and ctrl+c to their bindings and every other
// key to the line being typed.
func (m *model) updateInsert(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if b, _ := m.insertKeys.lookup("", msg.String()); b != nil {
		return m, b.run(m)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// updateHelp scrolls the keys screen; any other key closes it.
func (m *model) updateHelp(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.helpOffset++
	case "k", "up":
		m.helpOffset--
	case "ctrl+d":
		m.helpOffset += m.bodyHeight() / 2
	case "ctrl+u":
		m.helpOffset -= m.bodyHeight() / 2
	default:
		m.mode = modeNormal
		return m, nil
	}
	m.helpOffset = min(max(m.helpOffset, 0), max(len(m.helpLines())-m.bodyHeight(), 0))
	return m, nil
}

func (m *model) showKeys() {
	m.mode = modeHelp
	m.helpOffset = 0
}

func (m *model) saveAndQuit() tea.Cmd {
	if err := m.save(); err != nil {
		m.status = "Save failed, not quitting: " + err.Error()
		return nil
	}
	return tea.Quit
}

func (m *model) updateRollover(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if !m.oneByOne {
		switch key {
		case "a":
			for len(m.rollover) > 0 {
				m.rolloverOne()
			}
			m.endRollover()
		case "o":
			m.oneByOne = true
		case "n", "esc", "q":
			m.endRollover()
		}
		return m, nil
	}
	switch key {
	case "y":
		m.rolloverOne()
	case "n":
		m.rollover = m.rollover[1:]
	case "q", "esc":
		m.rollover = nil
	}
	if len(m.rollover) == 0 {
		m.endRollover()
	}
	return m, nil
}

// rolloverOne migrates the first offered item into today. Line numbers in
// the source are unaffected, since a migrated line stays where it was.
func (m *model) rolloverOne() {
	ref := m.rollover[0]
	m.rollover = m.rollover[1:]
	m.pushEntries(ref.Date, m.today)
	if err := entry.Migrate(m.entries[ref.Date], ref.Line, m.ensure(m.today)); err != nil {
		m.status = err.Error()
		return
	}
	m.dirty[ref.Date] = true
	m.dirty[m.today] = true
}

func (m *model) endRollover() {
	m.mode = modeNormal
	m.rollover = nil
	m.oneByOne = false
	m.showDay(m.today, 0)
}

// updateInput types into the filter prompt, filtering as each key lands, so
// enter and esc both just close it.
func (m *model) updateInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter":
		m.mode = modeNormal
		m.input.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.filter = m.input.Value()
	m.refresh()
	return m, cmd
}

// startFilter opens the filter prompt over the status bar, as tmux opens its
// command prompt.
func (m *model) startFilter() {
	m.mode = modeInput
	m.input.SetStyles(promptStyles)
	m.input.Prompt = "> "
	m.input.Placeholder = "filter"
	m.input.SetWidth(max(m.width-4, 10))
	m.input.SetValue(m.filter)
	m.input.CursorEnd()
	m.input.Focus()
}

// updateMigrate runs the migrate picker's keys, or sends keys to the day
// being typed.
func (m *model) updateMigrate(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.status = ""
	if m.pickTyping {
		return m.updatePickTyping(msg)
	}
	if b, _ := m.migrateKeys.lookup("", msg.String()); b != nil {
		return m, b.run(m)
	}
	return m, nil
}

// updatePickTyping types the day to migrate to. The picker follows the typed
// day whenever it is a valid date, so the week and the words above it show
// where the line will go before enter is pressed.
func (m *model) updatePickTyping(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.stopPickTyping()
		return m, nil
	case "enter":
		value := strings.TrimSpace(m.input.Value())
		if !entry.ValidDate(value) {
			m.status = "Not a valid date: " + value
			return m, nil
		}
		m.pick = value
		m.stopPickTyping()
		m.confirmPick()
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if value := strings.TrimSpace(m.input.Value()); entry.ValidDate(value) {
		m.pick = value
	}
	return m, cmd
}

// dayLayout is the ISO form every date in thumb takes.
const dayLayout = "2006-01-02"

// addDays returns date moved by n days, or date itself if it does not parse.
func addDays(date string, n int) string {
	t, err := time.Parse(dayLayout, date)
	if err != nil {
		return date
	}
	return t.AddDate(0, 0, n).Format(dayLayout)
}

func (m *model) tomorrow() string {
	return addDays(m.today, 1)
}
