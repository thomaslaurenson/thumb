// Package project reads and edits the project file: a short list of what its
// owner is working on, one KEYWORD and description per line. It is what finger
// showed under "Project:", and what the profile card renders.
package project

import (
	"fmt"
	"regexp"
	"strings"
)

// itemPattern is the line format the profile card depends on: an uppercase
// keyword, whitespace, then the description.
var itemPattern = regexp.MustCompile(`^([A-Z][A-Z0-9_-]*)[ \t]+(\S.*)$`)

// Item is one project line.
type Item struct {
	Keyword string
	Text    string
}

// String renders the item in its file form.
func (i Item) String() string {
	return i.Keyword + " " + i.Text
}

// ParseItem reads one line, reporting false for anything that is not an item.
func ParseItem(s string) (Item, bool) {
	m := itemPattern.FindStringSubmatch(strings.TrimRight(s, " \t"))
	if m == nil {
		return Item{}, false
	}
	return Item{Keyword: m[1], Text: m[2]}, true
}

// isSkipped reports whether a line is a blank or a # comment.
func isSkipped(s string) bool {
	t := strings.TrimSpace(s)
	return t == "" || strings.HasPrefix(t, "#")
}

// Project holds the file's lines, comments and blanks included, so an edit
// keeps them.
type Project struct {
	Lines []string
}

// Parse reads the file content, dropping carriage returns.
func Parse(data []byte) *Project {
	p := &Project{}
	text := strings.TrimSuffix(string(data), "\n")
	if text == "" {
		return p
	}
	for _, line := range strings.Split(text, "\n") {
		p.Lines = append(p.Lines, strings.TrimSuffix(line, "\r"))
	}
	return p
}

// Bytes renders the project as file content.
func (p *Project) Bytes() []byte {
	if len(p.Lines) == 0 {
		return nil
	}
	return []byte(strings.Join(p.Lines, "\n") + "\n")
}

// Clone returns a copy that shares nothing with p.
func (p *Project) Clone() *Project {
	return &Project{Lines: append([]string(nil), p.Lines...)}
}

// Items returns the valid items in file order.
func (p *Project) Items() []Item {
	var items []Item
	for _, s := range p.Lines {
		if it, ok := ParseItem(s); ok {
			items = append(items, it)
		}
	}
	return items
}

// Problems describes every line that is neither an item, a comment nor blank.
func (p *Project) Problems() []string {
	var probs []string
	for i, s := range p.Lines {
		if isSkipped(s) {
			continue
		}
		if _, ok := ParseItem(s); !ok {
			probs = append(probs, fmt.Sprintf("line %d: expected an uppercase KEYWORD then a description", i+1))
		}
	}
	return probs
}

// Normalised renders only the valid items, one per line, which is the form
// published for other programs to read.
func Normalised(items []Item) []byte {
	var b strings.Builder
	for _, it := range items {
		b.WriteString(it.String())
		b.WriteByte('\n')
	}
	return []byte(b.String())
}
