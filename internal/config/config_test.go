package config

import (
	"errors"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	t.Parallel()
	const base = "title=T\nauthor=A\nlogin=l\n"
	tests := []struct {
		name    string
		in      string
		want    Config
		wantErr error
	}{
		{
			name: "valid, trailing slash trimmed, comments skipped",
			in:   "# site\n" + base + "base_url=https://x.example/\ndescription=d\n",
			want: Config{Title: "T", Author: "A", Login: "l", BaseURL: "https://x.example", Description: "d"},
		},
		{
			name: "crlf",
			in:   "title=T\r\nauthor=A\r\nlogin=l\r\nbase_url=https://x.example\r\n",
			want: Config{Title: "T", Author: "A", Login: "l", BaseURL: "https://x.example"},
		},
		{
			name: "values are data, never run",
			in:   base + "base_url=https://x.example\ndescription=$(touch pwned)\n",
			want: Config{Title: "T", Author: "A", Login: "l", BaseURL: "https://x.example", Description: "$(touch pwned)"},
		},
		{name: "unknown key", in: base + "base_url=https://x.example\ncolour=red\n", wantErr: ErrUnknownKey},
		{name: "missing title", in: "author=A\nlogin=l\nbase_url=https://x.example\n", wantErr: ErrMissingKey},
		{name: "bad url", in: base + "base_url=plan.example.com\n", wantErr: ErrBadURL},
		{name: "bad line", in: "title=T\njust words\n", wantErr: ErrBadLine},
		{
			name: "timezone",
			in:   base + "base_url=https://x.example\ntimezone=Pacific/Auckland\n",
			want: Config{Title: "T", Author: "A", Login: "l", BaseURL: "https://x.example", Timezone: "Pacific/Auckland"},
		},
		{
			name: "empty timezone means unset",
			in:   base + "base_url=https://x.example\ntimezone=\n",
			want: Config{Title: "T", Author: "A", Login: "l", BaseURL: "https://x.example"},
		},
		{name: "unknown timezone", in: base + "base_url=https://x.example\ntimezone=Mars/Olympus\n", wantErr: ErrBadTimezone},
		{name: "local is not a zone name", in: base + "base_url=https://x.example\ntimezone=Local\n", wantErr: ErrBadTimezone},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := Parse([]byte(tc.in))
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Parse error = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("Parse = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestToday(t *testing.T) {
	t.Parallel()
	// 11:30 UTC on 25 October is already 26 October in Auckland (UTC+13)
	now := time.Date(2026, 10, 25, 11, 30, 0, 0, time.UTC)
	tests := []struct {
		name     string
		timezone string
		want     string
	}{
		{name: "unset uses the clock's own zone", want: "2026-10-25"},
		{name: "author ahead of the build", timezone: "Pacific/Auckland", want: "2026-10-26"},
		{name: "author behind the build", timezone: "America/Los_Angeles", want: "2026-10-25"},
		{name: "utc", timezone: "UTC", want: "2026-10-25"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := Config{Timezone: tc.timezone}.Today(now)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("Today = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTodayBadTimezone(t *testing.T) {
	t.Parallel()
	if _, err := (Config{Timezone: "Mars/Olympus"}).Today(time.Now()); !errors.Is(err, ErrBadTimezone) {
		t.Errorf("Today error = %v, want %v", err, ErrBadTimezone)
	}
}
