package project

import (
	"slices"
	"testing"
)

func TestParseItem(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		in     string
		want   Item
		wantOK bool
	}{
		{name: "item", in: "BUILDING Security utilities", want: Item{Keyword: "BUILDING", Text: "Security utilities"}, wantOK: true},
		{name: "tab and extra spaces", in: "PLAYING\t  Torchlight II  ", want: Item{Keyword: "PLAYING", Text: "Torchlight II"}, wantOK: true},
		{name: "digits and dashes in keyword", in: "C2-SETUP lab", want: Item{Keyword: "C2-SETUP", Text: "lab"}, wantOK: true},
		{name: "lowercase keyword", in: "building x", wantOK: false},
		{name: "keyword only", in: "EATING", wantOK: false},
		{name: "comment", in: "# note", wantOK: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ParseItem(tc.in)
			if ok != tc.wantOK || got != tc.want {
				t.Errorf("ParseItem(%q) = %+v, %v, want %+v, %v", tc.in, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestProject(t *testing.T) {
	t.Parallel()
	p := Parse([]byte("# what I am up to\r\n\r\nBUILDING thumb\r\nreading books\r\nEATING Pho\r\n"))
	if want := []string{"# what I am up to", "", "BUILDING thumb", "reading books", "EATING Pho"}; !slices.Equal(p.Lines, want) {
		t.Errorf("Lines = %q, want %q", p.Lines, want)
	}
	items := p.Items()
	if len(items) != 2 || items[1].Keyword != "EATING" {
		t.Errorf("Items = %+v", items)
	}
	if probs := p.Problems(); len(probs) != 1 || probs[0] != "line 4: expected an uppercase KEYWORD then a description" {
		t.Errorf("Problems = %q", probs)
	}
	if got := string(Normalised(items)); got != "BUILDING thumb\nEATING Pho\n" {
		t.Errorf("Normalised = %q", got)
	}
}
