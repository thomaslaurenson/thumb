// Package entry parses and edits one day of a plan: a plain text file in which
// a line starting with a marker and a space is a plan line and every other line
// is prose.
package entry

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Marker characters. Each is the first byte of a plan line.
const (
	Done    byte = '*'
	Later   byte = '+'
	Todo    byte = '-'
	Dropped byte = '~'
	Blocked byte = '!'
	Idea    byte = '?'
	Moved   byte = '>'
)

// Markers lists every marker character, in the order the legend shows them.
const Markers = "*+-~!?>"

// dateLayout is the ISO form every date in this tool takes.
const dateLayout = "2006-01-02"

var (
	// ErrNoLine reports a line number outside the entry.
	ErrNoLine = errors.New("no such line")
	// ErrNotPlanLine reports a prose line where a plan line was needed.
	ErrNotPlanLine = errors.New("not a plan line")
	// ErrBadRef reports a ref that is not DATE:LINE.
	ErrBadRef = errors.New("not a ref, expected DATE:LINE")
	// ErrBadDate reports a string that is not a real YYYY-MM-DD date.
	ErrBadDate = errors.New("not a valid date")
)

var (
	datePattern  = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
	refPattern   = regexp.MustCompile(`^([0-9]{4}-[0-9]{2}-[0-9]{2}):([0-9]+)$`)
	movedPattern = regexp.MustCompile(`^(.*) -> ([0-9]{4}-[0-9]{2}-[0-9]{2})$`)
	// A marker run straight into a word, such as "*fixed", reads as a typo for
	// a plan line. Dividers such as "---" are deliberately not matched.
	ambiguousPattern = regexp.MustCompile(`^[*+~!?>-][\p{L}\p{N}]`)
)

// ValidDate reports whether s is a real calendar date in YYYY-MM-DD form.
//
// The pattern pins the exact shape, which file names and refs rely on;
// time.Parse then rejects dates that do not exist, such as 2026-02-30.
func ValidDate(s string) bool {
	if !datePattern.MatchString(s) {
		return false
	}
	_, err := time.Parse(dateLayout, s)
	return err == nil
}

// LongDate formats a valid date for people, for example "Oct 24, 2026".
func LongDate(date string) string {
	t, err := time.Parse(dateLayout, date)
	if err != nil {
		return date
	}
	return t.Format("Jan 2, 2006")
}

// Status names the kind of plan line a marker starts, as used in the HTML.
func Status(m byte) string {
	switch m {
	case Done:
		return "done"
	case Later:
		return "later"
	case Todo:
		return "todo"
	case Dropped:
		return "dropped"
	case Blocked:
		return "blocked"
	case Idea:
		return "idea"
	case Moved:
		return "moved"
	default:
		return ""
	}
}

// IsMarker reports whether b is one of the marker characters.
func IsMarker(b byte) bool {
	return strings.IndexByte(Markers, b) >= 0
}

// IsOpen reports whether a marker means the item still needs doing.
func IsOpen(m byte) bool {
	return m == Todo || m == Blocked
}

// Line is one parsed line of an entry.
type Line struct {
	// Marker is the plan marker, or zero for a prose line.
	Marker byte
	// Text is the line without its marker, or the whole line for prose.
	Text string
	// Target is the date a moved item went to, for a Moved line that names one.
	Target string
}

// IsPlan reports whether the line is a plan line rather than prose.
func (l Line) IsPlan() bool {
	return l.Marker != 0
}

// String renders the line back to its text form.
func (l Line) String() string {
	if l.Marker == 0 {
		return l.Text
	}
	if l.Marker == Moved && l.Target != "" {
		return fmt.Sprintf("%c %s -> %s", l.Marker, l.Text, l.Target)
	}
	return fmt.Sprintf("%c %s", l.Marker, l.Text)
}

// ParseLine splits one line into its marker, text and, for a moved item, the
// date it moved to.
func ParseLine(s string) Line {
	if len(s) < 2 || s[1] != ' ' || !IsMarker(s[0]) {
		return Line{Text: s}
	}
	l := Line{Marker: s[0], Text: s[2:]}
	if l.Marker == Moved {
		if m := movedPattern.FindStringSubmatch(l.Text); m != nil {
			l.Text, l.Target = m[1], m[2]
		}
	}
	return l
}

// IsBlank reports whether a line has no visible characters.
func IsBlank(s string) bool {
	return strings.TrimSpace(s) == ""
}

// Ref points at one line of one entry, with a one-based line number.
type Ref struct {
	Date string
	Line int
}

// String renders the ref as DATE:LINE.
func (r Ref) String() string {
	return fmt.Sprintf("%s:%d", r.Date, r.Line)
}

// ParseRef reads a DATE:LINE ref.
func ParseRef(s string) (Ref, error) {
	m := refPattern.FindStringSubmatch(s)
	if m == nil {
		return Ref{}, fmt.Errorf("%q: %w", s, ErrBadRef)
	}
	if !ValidDate(m[1]) {
		return Ref{}, fmt.Errorf("%q: %w", m[1], ErrBadDate)
	}
	n, err := strconv.Atoi(m[2])
	if err != nil || n < 1 {
		return Ref{}, fmt.Errorf("%q: %w", s, ErrBadRef)
	}
	return Ref{Date: m[1], Line: n}, nil
}

// Entry is one day's file, held as its lines without line endings.
type Entry struct {
	Date  string
	Lines []string
	// crlf records that the file used Windows line endings, so a save writes
	// them back the same way.
	crlf bool
}

// New returns an empty entry for date.
func New(date string) *Entry {
	return &Entry{Date: date}
}

// Parse reads an entry's file content. Carriage returns are dropped from
// every line and remembered, so Bytes restores them.
func Parse(date string, data []byte) *Entry {
	e := &Entry{Date: date}
	if len(data) == 0 {
		return e
	}
	text := string(data)
	e.crlf = strings.Contains(text, "\r\n")
	text = strings.TrimSuffix(text, "\n")
	for _, line := range strings.Split(text, "\n") {
		e.Lines = append(e.Lines, strings.TrimSuffix(line, "\r"))
	}
	return e
}

// Bytes renders the entry as file content, ending with a newline.
func (e *Entry) Bytes() []byte {
	if len(e.Lines) == 0 {
		return nil
	}
	eol := "\n"
	if e.crlf {
		eol = "\r\n"
	}
	return []byte(strings.Join(e.Lines, eol) + eol)
}

// Clone returns a copy that shares nothing with e.
func (e *Entry) Clone() *Entry {
	c := *e
	c.Lines = append([]string(nil), e.Lines...)
	return &c
}

// Empty reports whether the entry has no visible content.
func (e *Entry) Empty() bool {
	for _, l := range e.Lines {
		if !IsBlank(l) {
			return false
		}
	}
	return true
}

// Line returns the parsed line n, counting from one.
func (e *Entry) Line(n int) (Line, error) {
	if n < 1 || n > len(e.Lines) {
		return Line{}, fmt.Errorf("line %d of %q: %w", n, e.Date, ErrNoLine)
	}
	return ParseLine(e.Lines[n-1]), nil
}

// TrimmedLines returns the lines with trailing blank lines removed.
func (e *Entry) TrimmedLines() []string {
	end := len(e.Lines)
	for end > 0 && IsBlank(e.Lines[end-1]) {
		end--
	}
	return e.Lines[:end]
}

// Tally counts an entry's plan lines by kind.
type Tally struct {
	Done, Open, Ideas, Dropped, Moved int
	// FirstDone is the text of the first done or done-later line.
	FirstDone string
}

// String renders the tally as a short phrase such as "3 done, 1 open".
func (t Tally) String() string {
	var parts []string
	add := func(n int, one, many string) {
		switch {
		case n == 1:
			parts = append(parts, "1 "+one)
		case n > 1:
			parts = append(parts, fmt.Sprintf("%d %s", n, many))
		}
	}
	add(t.Done, "done", "done")
	add(t.Open, "open", "open")
	add(t.Ideas, "idea", "ideas")
	add(t.Dropped, "dropped", "dropped")
	add(t.Moved, "moved", "moved")
	if len(parts) == 0 {
		return "notes"
	}
	return strings.Join(parts, ", ")
}

// Tally counts the entry's plan lines.
func (e *Entry) Tally() Tally {
	var t Tally
	for _, s := range e.Lines {
		l := ParseLine(s)
		switch l.Marker {
		case Done, Later:
			t.Done++
			if t.FirstDone == "" {
				t.FirstDone = l.Text
			}
		case Todo, Blocked:
			t.Open++
		case Idea:
			t.Ideas++
		case Dropped:
			t.Dropped++
		case Moved:
			t.Moved++
		}
	}
	return t
}

// Item is a plan line together with where it lives.
type Item struct {
	Ref  Ref
	Line Line
}

// OpenItems returns the entry's to do and blocked lines.
func (e *Entry) OpenItems() []Item {
	var items []Item
	for i, s := range e.Lines {
		l := ParseLine(s)
		if IsOpen(l.Marker) {
			items = append(items, Item{Ref: Ref{Date: e.Date, Line: i + 1}, Line: l})
		}
	}
	return items
}

// Warnings describes lines that look like plan lines but are read as prose.
func (e *Entry) Warnings() []string {
	var w []string
	for i, s := range e.Lines {
		if ambiguousPattern.MatchString(s) {
			w = append(w, fmt.Sprintf("line %d has no space after its marker, read as prose", i+1))
		}
	}
	return w
}

// Append adds a plan line at the end of the entry.
func (e *Entry) Append(l Line) {
	e.Lines = append(e.Lines, l.String())
}

// Insert puts a line before line n, or at the end when n is past the last.
func (e *Entry) Insert(n int, s string) {
	i := min(max(n-1, 0), len(e.Lines))
	e.Lines = append(e.Lines[:i], append([]string{s}, e.Lines[i:]...)...)
}

// Set replaces line n.
func (e *Entry) Set(n int, l Line) error {
	if n < 1 || n > len(e.Lines) {
		return fmt.Errorf("line %d of %q: %w", n, e.Date, ErrNoLine)
	}
	e.Lines[n-1] = l.String()
	return nil
}

// SetMarker changes line n's marker. A prose line becomes a plan line, and a
// marker of zero turns a plan line back into prose. Leaving the moved marker
// drops the line's target.
func (e *Entry) SetMarker(n int, m byte) error {
	l, err := e.Line(n)
	if err != nil {
		return err
	}
	l.Marker = m
	if m != Moved {
		l.Target = ""
	}
	return e.Set(n, l)
}

// Delete removes line n.
func (e *Entry) Delete(n int) error {
	if n < 1 || n > len(e.Lines) {
		return fmt.Errorf("line %d of %q: %w", n, e.Date, ErrNoLine)
	}
	e.Lines = append(e.Lines[:n-1], e.Lines[n:]...)
	return nil
}
