package entry

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrWrongMarker reports a line whose marker does not allow the change.
	ErrWrongMarker = errors.New("marker does not allow this change")
	// ErrSameDay reports a migration to the entry the item is already in.
	ErrSameDay = errors.New("already in that entry")
)

// Resolve marks open line n as done later.
func (e *Entry) Resolve(n int) error {
	return e.remark(n, Later, string(Todo)+string(Blocked))
}

// Drop marks open or idea line n as dropped.
func (e *Entry) Drop(n int) error {
	return e.remark(n, Dropped, string(Todo)+string(Blocked)+string(Idea))
}

// remark changes line n's marker to m, provided its current marker is one of
// allowed.
func (e *Entry) remark(n int, m byte, allowed string) error {
	l, err := e.Line(n)
	if err != nil {
		return err
	}
	if !l.IsPlan() {
		return fmt.Errorf("line %d of %q: %w", n, e.Date, ErrNotPlanLine)
	}
	if !strings.ContainsRune(allowed, rune(l.Marker)) {
		return fmt.Errorf("line %d of %q is %q, need one of %q: %w", n, e.Date, l.Marker, allowed, ErrWrongMarker)
	}
	return e.SetMarker(n, m)
}

// Migrate moves open or idea line n of from into to, Bullet Journal style:
// the original stays as a moved line naming the target date, and a copy with
// the original marker is appended to to.
func Migrate(from *Entry, n int, to *Entry) error {
	if from.Date == to.Date {
		return fmt.Errorf("line %d of %q: %w", n, from.Date, ErrSameDay)
	}
	l, err := from.Line(n)
	if err != nil {
		return err
	}
	if !l.IsPlan() {
		return fmt.Errorf("line %d of %q: %w", n, from.Date, ErrNotPlanLine)
	}
	if !IsOpen(l.Marker) && l.Marker != Idea {
		return fmt.Errorf("line %d of %q is %q, need an open item or idea: %w", n, from.Date, l.Marker, ErrWrongMarker)
	}
	to.Append(Line{Marker: l.Marker, Text: l.Text})
	return from.Set(n, Line{Marker: Moved, Text: l.Text, Target: to.Date})
}
