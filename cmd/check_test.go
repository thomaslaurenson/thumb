package cmd

import (
	"errors"
	"testing"

	"github.com/thomaslaurenson/thumb/internal/store"
)

// wantExitCode asserts the command set exit code code through ExitCodeError.
func wantExitCode(code int) func(*testing.T, string, string, error) {
	return func(t *testing.T, _, _ string, err error) {
		t.Helper()
		var ec *ExitCodeError
		if !errors.As(err, &ec) || ec.Code != code {
			t.Errorf("error = %v, want ExitCodeError with code %d", err, code)
		}
	}
}

func TestCheck(t *testing.T) {
	t.Parallel()
	runTests(t, []cmdTest{
		{
			name:       "a valid repository",
			files:      map[string]string{entryPath(testToday): "* a\n", entryPath("2026-10-24"): "* b\n"},
			args:       []string{"check"},
			wantErrOut: []string{"[*] Checked 2 entries"},
			emptyOut:   true,
		},
		{
			name:       "a warning does not fail",
			files:      map[string]string{entryPath(testToday): "*fixed\n"},
			args:       []string{"check"},
			wantErrOut: []string{"[!] ", "warning: line 1 has no space after its marker", "[*] Checked 1 entry"},
		},
		{
			name:       "a bad project line fails",
			files:      map[string]string{store.ProjectName: "lowercase keyword\n"},
			args:       []string{"check"},
			wantErrOut: []string{"error: line 1: expected an uppercase KEYWORD"},
			wantErr:    true,
			check:      wantExitCode(1),
		},
		{
			name:       "a bad thumb.conf fails",
			files:      map[string]string{store.ConfigName: testConf + "bogus=1\n"},
			args:       []string{"check"},
			wantErrOut: []string{"error: line 5: unknown key"},
			wantErr:    true,
			check:      wantExitCode(1),
		},
		{
			name:    "an argument",
			args:    []string{"check", "extra"},
			wantErr: true,
		},
	})
}
