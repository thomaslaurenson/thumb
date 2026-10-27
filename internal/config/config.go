// Package config reads thumb.conf, the site settings for a data repository.
//
// The file is key=value lines parsed as data. Nothing in it is ever run.
package config

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

var (
	// ErrBadLine reports a line that is not key=value.
	ErrBadLine = errors.New("expected key=value")
	// ErrUnknownKey reports a key thumb does not use.
	ErrUnknownKey = errors.New("unknown key")
	// ErrMissingKey reports a required key that is absent or empty.
	ErrMissingKey = errors.New("required key missing")
	// ErrBadURL reports a base_url that is not an http or https URL.
	ErrBadURL = errors.New("base_url must be an http or https URL")
	// ErrBadTimezone reports a timezone that is not an IANA zone name.
	ErrBadTimezone = errors.New("timezone must be an IANA name such as Pacific/Auckland")
)

// dateLayout is the YYYY-MM-DD form entry dates take.
const dateLayout = "2006-01-02"

var urlPattern = regexp.MustCompile(`^https?://[^/\s]+(/\S*)?$`)

// Config holds the site settings.
type Config struct {
	Title       string
	Author      string
	Login       string
	BaseURL     string
	Description string
	// Timezone is the author's IANA time zone, or empty for the local zone of
	// whichever machine runs thumb.
	Timezone string
}

// Today returns the author's date at now as YYYY-MM-DD: in Timezone when it is
// set, otherwise in now's own location.
//
// A build in CI runs on a UTC clock, which can be a day behind or ahead of the
// author. Timezone is what lets that build agree with the author about which
// day it is.
func (c Config) Today(now time.Time) (string, error) {
	if c.Timezone == "" {
		return now.Format(dateLayout), nil
	}
	loc, err := loadLocation(c.Timezone)
	if err != nil {
		return "", err
	}
	return now.In(loc).Format(dateLayout), nil
}

// loadLocation resolves an IANA zone name. time.LoadLocation also accepts ""
// and "Local", which mean UTC and the machine's own zone, so both are rejected
// here: either would quietly move the author's day to the build machine's.
func loadLocation(name string) (*time.Location, error) {
	if name == "" || name == "Local" {
		return nil, fmt.Errorf("%q: %w", name, ErrBadTimezone)
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("%q: %w", name, ErrBadTimezone)
	}
	return loc, nil
}

// Load reads and validates the file at path.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	c, err := Parse(data)
	if err != nil {
		return Config{}, fmt.Errorf("%q: %w", path, err)
	}
	return c, nil
}

// Parse reads and validates config content.
func Parse(data []byte) (Config, error) {
	var c Config
	sc := bufio.NewScanner(bytes.NewReader(data))
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSuffix(sc.Text(), "\r")
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return Config{}, fmt.Errorf("line %d: %w", n, ErrBadLine)
		}
		switch key {
		case "title":
			c.Title = value
		case "author":
			c.Author = value
		case "login":
			c.Login = value
		case "base_url":
			c.BaseURL = strings.TrimRight(value, "/")
		case "description":
			c.Description = value
		case "timezone":
			if value == "" {
				c.Timezone = ""
				continue
			}
			if _, err := loadLocation(value); err != nil {
				return Config{}, fmt.Errorf("line %d: %w", n, err)
			}
			c.Timezone = value
		default:
			return Config{}, fmt.Errorf("line %d: %w: %q", n, ErrUnknownKey, key)
		}
	}
	if err := sc.Err(); err != nil {
		return Config{}, err
	}
	for _, f := range []struct{ key, value string }{
		{"title", c.Title}, {"author", c.Author}, {"login", c.Login},
	} {
		if f.value == "" {
			return Config{}, fmt.Errorf("%w: %s", ErrMissingKey, f.key)
		}
	}
	if !urlPattern.MatchString(c.BaseURL) {
		return Config{}, ErrBadURL
	}
	return c, nil
}
