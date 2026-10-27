package entry

import (
	"errors"
	"slices"
	"testing"
)

func TestValidDate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "real date", in: "2026-10-24", want: true},
		{name: "leap day in a leap year", in: "2028-02-29", want: true},
		{name: "leading zeros 08 and 09", in: "2026-08-09", want: true},
		{name: "30 February", in: "2026-02-30", want: false},
		{name: "29 February in a common year", in: "2026-02-29", want: false},
		{name: "month 13", in: "2026-13-01", want: false},
		{name: "day zero", in: "2026-10-00", want: false},
		{name: "unpadded month", in: "2026-1-01", want: false},
		{name: "other format", in: "24/10/2026", want: false},
		{name: "trailing text", in: "2026-10-24; rm -rf /", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ValidDate(tc.in); got != tc.want {
				t.Errorf("ValidDate(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestLongDate(t *testing.T) {
	t.Parallel()
	if got := LongDate("2026-03-05"); got != "Mar 5, 2026" {
		t.Errorf("LongDate = %q, want %q", got, "Mar 5, 2026")
	}
}

func TestParseLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want Line
	}{
		{name: "done", in: "* fixed rcon", want: Line{Marker: Done, Text: "fixed rcon"}},
		{name: "every marker", in: "? tags", want: Line{Marker: Idea, Text: "tags"}},
		{name: "moved with target", in: "> docs -> 2026-10-25", want: Line{Marker: Moved, Text: "docs", Target: "2026-10-25"}},
		{name: "moved without target", in: "> docs", want: Line{Marker: Moved, Text: "docs"}},
		{name: "marker with no space is prose", in: "*fixed", want: Line{Text: "*fixed"}},
		{name: "indented marker is prose", in: "  * x", want: Line{Text: "  * x"}},
		{name: "divider is prose", in: "---", want: Line{Text: "---"}},
		{name: "lone marker is prose", in: "*", want: Line{Text: "*"}},
		{name: "plain prose", in: "Quiet day.", want: Line{Text: "Quiet day."}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ParseLine(tc.in)
			if got != tc.want {
				t.Errorf("ParseLine(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
			if got.String() != tc.in {
				t.Errorf("round trip of %q gave %q", tc.in, got.String())
			}
		})
	}
}

func TestParseRef(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		in      string
		want    Ref
		wantErr error
	}{
		{name: "valid", in: "2026-10-20:3", want: Ref{Date: "2026-10-20", Line: 3}},
		{name: "no line", in: "2026-10-20", wantErr: ErrBadRef},
		{name: "line zero", in: "2026-10-20:0", wantErr: ErrBadRef},
		{name: "impossible date", in: "2026-02-30:1", wantErr: ErrBadDate},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseRef(tc.in)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("ParseRef(%q) error = %v, want %v", tc.in, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("ParseRef(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseAndBytes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		in        string
		wantLines []string
		wantOut   string
	}{
		{name: "unix", in: "* a\n\nprose\n", wantLines: []string{"* a", "", "prose"}, wantOut: "* a\n\nprose\n"},
		{name: "crlf kept", in: "* a\r\n- b\r\n", wantLines: []string{"* a", "- b"}, wantOut: "* a\r\n- b\r\n"},
		{name: "missing final newline added", in: "* a", wantLines: []string{"* a"}, wantOut: "* a\n"},
		{name: "empty", in: "", wantLines: nil, wantOut: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := Parse("2026-10-24", []byte(tc.in))
			if !slices.Equal(e.Lines, tc.wantLines) {
				t.Errorf("Lines = %q, want %q", e.Lines, tc.wantLines)
			}
			if got := string(e.Bytes()); got != tc.wantOut {
				t.Errorf("Bytes = %q, want %q", got, tc.wantOut)
			}
		})
	}
}

func TestEmpty(t *testing.T) {
	t.Parallel()
	if !Parse("2026-10-24", []byte("\n  \n\n")).Empty() {
		t.Error("whitespace-only entry should be empty")
	}
	if Parse("2026-10-24", []byte("x\n")).Empty() {
		t.Error("entry with text should not be empty")
	}
}

func sample() *Entry {
	return Parse("2026-10-20", []byte(
		"* breadcrumb\n+ permalinks\n- plain text output\n! chroma test\n? tags\n~ js toggle\n> docs -> 2026-10-21\n"))
}

func TestTally(t *testing.T) {
	t.Parallel()
	got := sample().Tally()
	want := Tally{Done: 2, Open: 2, Ideas: 1, Dropped: 1, Moved: 1, FirstDone: "breadcrumb"}
	if got != want {
		t.Errorf("Tally = %+v, want %+v", got, want)
	}
	if s := got.String(); s != "2 done, 2 open, 1 idea, 1 dropped, 1 moved" {
		t.Errorf("Tally.String = %q", s)
	}
	if s := (Tally{}).String(); s != "notes" {
		t.Errorf("empty Tally.String = %q, want notes", s)
	}
}

func TestOpenItems(t *testing.T) {
	t.Parallel()
	items := sample().OpenItems()
	var refs []string
	for _, it := range items {
		refs = append(refs, it.Ref.String())
	}
	if want := []string{"2026-10-20:3", "2026-10-20:4"}; !slices.Equal(refs, want) {
		t.Errorf("open refs = %v, want %v", refs, want)
	}
}

func TestWarnings(t *testing.T) {
	t.Parallel()
	e := Parse("2026-10-24", []byte("*fixed\n---\n* fine\n-5 degrees\n"))
	if got := len(e.Warnings()); got != 2 {
		t.Errorf("got %d warnings, want 2: %v", got, e.Warnings())
	}
}

func TestEdits(t *testing.T) {
	t.Parallel()
	e := Parse("2026-10-24", []byte("one\n- two\n"))
	if err := e.SetMarker(1, Done); err != nil {
		t.Fatal(err)
	}
	e.Insert(2, "? middle")
	if err := e.Delete(2); err != nil {
		t.Fatal(err)
	}
	if want := []string{"* one", "- two"}; !slices.Equal(e.Lines, want) {
		t.Errorf("Lines = %q, want %q", e.Lines, want)
	}
	if err := e.Delete(9); !errors.Is(err, ErrNoLine) {
		t.Errorf("Delete(9) error = %v, want ErrNoLine", err)
	}
}

func TestResolveAndDrop(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		op      func(*Entry, int) error
		line    int
		want    string
		wantErr error
	}{
		{name: "resolve todo", op: (*Entry).Resolve, line: 3, want: "+ plain text output"},
		{name: "resolve blocked", op: (*Entry).Resolve, line: 4, want: "+ chroma test"},
		{name: "resolve done refused", op: (*Entry).Resolve, line: 1, wantErr: ErrWrongMarker},
		{name: "resolve idea refused", op: (*Entry).Resolve, line: 5, wantErr: ErrWrongMarker},
		{name: "drop idea", op: (*Entry).Drop, line: 5, want: "~ tags"},
		{name: "drop past the end", op: (*Entry).Drop, line: 99, wantErr: ErrNoLine},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := sample()
			err := tc.op(e, tc.line)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil && e.Lines[tc.line-1] != tc.want {
				t.Errorf("line = %q, want %q", e.Lines[tc.line-1], tc.want)
			}
		})
	}
	prose := Parse("2026-10-24", []byte("just prose\n"))
	if err := prose.Resolve(1); !errors.Is(err, ErrNotPlanLine) {
		t.Errorf("Resolve on prose error = %v, want ErrNotPlanLine", err)
	}
}

func TestMigrate(t *testing.T) {
	t.Parallel()
	from := sample()
	to := Parse("2026-10-25", []byte("* already here\n"))
	if err := Migrate(from, 4, to); err != nil {
		t.Fatal(err)
	}
	if got := from.Lines[3]; got != "> chroma test -> 2026-10-25" {
		t.Errorf("source line = %q", got)
	}
	if want := []string{"* already here", "! chroma test"}; !slices.Equal(to.Lines, want) {
		t.Errorf("target = %q, want %q", to.Lines, want)
	}

	if err := Migrate(sample(), 1, to); !errors.Is(err, ErrWrongMarker) {
		t.Errorf("migrating a done line error = %v, want ErrWrongMarker", err)
	}
	same := sample()
	if err := Migrate(same, 3, same); !errors.Is(err, ErrSameDay) {
		t.Errorf("migrating to the same day error = %v, want ErrSameDay", err)
	}
}
