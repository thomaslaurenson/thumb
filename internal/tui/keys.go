package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/thomaslaurenson/thumb/internal/entry"
)

// section groups bindings on the keys screen.
type section int

const (
	sectionMove section = iota
	sectionWrite
	sectionMark
	sectionDays
	sectionOther
	sectionTyping
	sectionMigrate
)

var sectionNames = map[section]string{
	sectionMove:    "Moving",
	sectionWrite:   "Writing",
	sectionMark:    "Marking",
	sectionDays:    "Days and views",
	sectionOther:   "Other",
	sectionTyping:  "While typing a line",
	sectionMigrate: "While migrating a line",
}

// binding is one row of the keymap. Each trigger is a key as
// tea.KeyPressMsg.String() spells it, such as "j", "ctrl+d" or "space"; a
// trigger of several keys separated by spaces, such as "d d", is a sequence
// typed one key after the other.
//
// The same table drives dispatch and the keys screen, so a key cannot work
// without being listed or be listed without working.
type binding struct {
	triggers []string
	// label is how the keys screen shows the keys. Empty hides the row, for
	// the second half of a pair that an earlier row already describes, or for
	// a key kept off the screen.
	label   string
	desc    string
	section section
	// marker colours the label as that plan marker, so the marking rows double
	// as the legend for the file format
	marker byte
	run    func(m *model) tea.Cmd
}

// keymap is a binding table indexed for dispatch.
type keymap struct {
	list []binding
	// byTrigger maps a whole trigger to its index in list
	byTrigger map[string]int
	// prefixes holds each leading part of a sequence, such as "d" for "d d",
	// so the reader knows to wait for the next key
	prefixes map[string]bool
}

func newKeymap(list []binding) keymap {
	k := keymap{list: list, byTrigger: map[string]int{}, prefixes: map[string]bool{}}
	for i, b := range list {
		for _, t := range b.triggers {
			k.byTrigger[t] = i
			parts := strings.Fields(t)
			for n := 1; n < len(parts); n++ {
				k.prefixes[strings.Join(parts[:n], " ")] = true
			}
		}
	}
	return k
}

// lookup resolves a key pressed after pending, the keys of a sequence typed so
// far. It returns the binding to run, or the new pending sequence when the key
// starts or continues one. A key that completes nothing clears the sequence
// and is dropped, as in vim, rather than being run on its own.
func (k keymap) lookup(pending, key string) (b *binding, next string) {
	trigger := key
	if pending != "" {
		trigger = pending + " " + key
	}
	if k.prefixes[trigger] {
		return nil, trigger
	}
	if i, ok := k.byTrigger[trigger]; ok {
		return &k.list[i], ""
	}
	return nil, ""
}

// do adapts an action with no command to a binding's run function.
func do(f func(m *model)) func(m *model) tea.Cmd {
	return func(m *model) tea.Cmd {
		f(m)
		return nil
	}
}

func mark(marker byte) func(m *model) tea.Cmd {
	return do(func(m *model) { m.setMarker(marker) })
}

// remark is r then a marker, vim's replace-character applied to the marker
// that starts a plan line.
func remark(marker byte, desc string) binding {
	return binding{triggers: []string{"r " + string(marker)}, label: "r" + string(marker), desc: desc,
		section: sectionMark, marker: marker, run: mark(marker)}
}

// viewKey is key after either window prefix, the views being windows: ctrl+a
// as in GNU screen or ctrl+b as in tmux. Whichever one a multiplexer keeps for
// itself, the other still reaches thumb.
func viewKey(key string) []string {
	return []string{"ctrl+a " + key, "ctrl+b " + key}
}

// normalBindings is the keymap for browsing. It follows vim, with one key for
// each job.
func normalBindings() []binding {
	return []binding{
		{triggers: []string{"j", "down"}, label: "j k, down up", desc: "move down or up", section: sectionMove, run: do(func(m *model) { m.move(1) })},
		{triggers: []string{"k", "up"}, section: sectionMove, run: do(func(m *model) { m.move(-1) })},
		{triggers: []string{"ctrl+d"}, label: "ctrl+d ctrl+u", desc: "half a page down or up", section: sectionMove, run: do(func(m *model) { m.move(m.bodyHeight() / 2) })},
		{triggers: []string{"ctrl+u"}, section: sectionMove, run: do(func(m *model) { m.move(-m.bodyHeight() / 2) })},
		{triggers: []string{"g g"}, label: "gg G", desc: "first or last line", section: sectionMove, run: do(func(m *model) { m.cursor = 0; m.clamp() })},
		{triggers: []string{"G"}, section: sectionMove, run: do(func(m *model) { m.cursor = len(m.rows) - 1; m.clamp() })},

		{triggers: []string{"o"}, label: "o O", desc: "add a line below or above", section: sectionWrite, run: do(func(m *model) { m.insertNew(1) })},
		{triggers: []string{"O"}, section: sectionWrite, run: do(func(m *model) { m.insertNew(0) })},
		{triggers: []string{"i"}, label: "i A", desc: "edit the line, at the start or the end", section: sectionWrite, run: do(func(m *model) { m.insertEdit(true) })},
		{triggers: []string{"A"}, section: sectionWrite, run: do(func(m *model) { m.insertEdit(false) })},
		{triggers: []string{"d d"}, label: "dd", desc: "delete the line", section: sectionWrite, run: do((*model).deleteLine)},

		{triggers: []string{"space"}, label: "space", desc: "done, * today or + older; again for to do", section: sectionMark, run: do((*model).toggleDone)},
		remark(entry.Done, "done that day"),
		remark(entry.Later, "done later"),
		remark(entry.Todo, "to do"),
		remark(entry.Dropped, "dropped"),
		remark(entry.Blocked, "blocked or important"),
		remark(entry.Idea, "idea or question"),
		remark(entry.Moved, "migrate to another day"),

		{triggers: []string{"h", "left"}, label: "h l, left right", desc: "previous or next day", section: sectionDays, run: do(func(m *model) { m.stepDay(-1) })},
		{triggers: []string{"l", "right"}, section: sectionDays, run: do(func(m *model) { m.stepDay(1) })},
		{triggers: []string{"t"}, label: "t", desc: "today", section: sectionDays, run: do(func(m *model) { m.showDay(m.today, 0) })},
		{triggers: []string{"enter"}, label: "enter", desc: "open the selected day", section: sectionDays, run: do((*model).open)},
		{triggers: viewKey("n"), label: "ctrl+a/b n p", desc: "next or previous view", section: sectionDays, run: do(func(m *model) { m.switchView((m.view + 1) % viewCount) })},
		{triggers: viewKey("p"), section: sectionDays, run: do(func(m *model) { m.switchView((m.view + viewCount - 1) % viewCount) })},
		{triggers: viewKey("1"), label: "ctrl+a/b 1-4", desc: "day, open, archive, project view", section: sectionDays, run: do(func(m *model) { m.switchView(viewDay) })},
		{triggers: viewKey("2"), section: sectionDays, run: do(func(m *model) { m.switchView(viewOpen) })},
		{triggers: viewKey("3"), section: sectionDays, run: do(func(m *model) { m.switchView(viewArchive) })},
		{triggers: viewKey("4"), section: sectionDays, run: do(func(m *model) { m.switchView(viewProject) })},

		{triggers: []string{"/"}, label: "/ esc", desc: "filter, clear the filter", section: sectionOther, run: do((*model).startFilter)},
		{triggers: []string{"esc"}, section: sectionOther, run: do(func(m *model) { m.filter = ""; m.refresh() })},
		{triggers: []string{"u"}, label: "u", desc: "undo", section: sectionOther, run: do((*model).undoLast)},
		{triggers: []string{"Z Z"}, label: "ZZ", desc: "save and quit", section: sectionOther, run: (*model).saveAndQuit},
		{triggers: []string{"?"}, label: "?", desc: "this screen", section: sectionOther, run: do((*model).showKeys)},
		// Unlisted, so there is always a way out for anyone who does not know ZZ
		{triggers: []string{"ctrl+c"}, section: sectionOther, run: (*model).saveAndQuit},
	}
}

// insertBindings is the keymap while typing a line. Every other key goes to
// the line itself.
func insertBindings() []binding {
	return []binding{
		{triggers: []string{"enter"}, label: "enter", desc: "keep it, start the next (twice stops)", section: sectionTyping, run: do((*model).insertEnter)},
		{triggers: []string{"esc"}, label: "esc ctrl+c", desc: "keep it and stop typing", section: sectionTyping, run: do((*model).insertEsc)},
		{triggers: []string{"ctrl+c"}, section: sectionTyping, run: do((*model).insertEsc)},
	}
}

// migrateBindings is the keymap of the migrate picker, which steps through
// days as a calendar does: h and l by the day, j and k by the week.
func migrateBindings() []binding {
	return []binding{
		{triggers: []string{"h", "left"}, label: "h l, left right", desc: "a day earlier or later", section: sectionMigrate, run: do(func(m *model) { m.pick = addDays(m.pick, -1) })},
		{triggers: []string{"l", "right"}, section: sectionMigrate, run: do(func(m *model) { m.pick = addDays(m.pick, 1) })},
		{triggers: []string{"j", "down"}, label: "j k, down up", desc: "a week later or earlier", section: sectionMigrate, run: do(func(m *model) { m.pick = addDays(m.pick, 7) })},
		{triggers: []string{"k", "up"}, section: sectionMigrate, run: do(func(m *model) { m.pick = addDays(m.pick, -7) })},
		{triggers: []string{"t"}, label: "t", desc: "today", section: sectionMigrate, run: do(func(m *model) { m.pick = m.today })},
		{triggers: []string{"tab"}, label: "tab shift+tab", desc: "next or previous day with an entry", section: sectionMigrate, run: do(func(m *model) { m.pickEntry(1) })},
		{triggers: []string{"shift+tab"}, section: sectionMigrate, run: do(func(m *model) { m.pickEntry(-1) })},
		{triggers: []string{"i"}, label: "i", desc: "type the day, esc to step again", section: sectionMigrate, run: do((*model).typePick)},
		{triggers: []string{"enter"}, label: "enter", desc: "migrate to the picked day", section: sectionMigrate, run: do((*model).confirmPick)},
		{triggers: []string{"esc"}, label: "esc ctrl+c", desc: "cancel", section: sectionMigrate, run: do((*model).cancelPick)},
		{triggers: []string{"ctrl+c"}, section: sectionMigrate, run: do((*model).cancelPick)},
	}
}

// helpBindings is every key the keys screen lists, in screen order.
func (m *model) helpBindings() []binding {
	list := append(append([]binding(nil), m.keys.list...), m.insertKeys.list...)
	return append(list, m.migrateKeys.list...)
}
