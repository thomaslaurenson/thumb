package cmd

import (
	"path/filepath"
	"testing"

	"github.com/thomaslaurenson/thumb/internal/store"
)

func TestProject(t *testing.T) {
	t.Parallel()
	runTests(t, []cmdTest{
		{
			name:    "the items, without comments or invalid lines",
			files:   map[string]string{store.ProjectName: "# now\nBUILDING thumb\nnot an item\nREADING Snow Crash\n"},
			args:    []string{"project"},
			wantOut: []string{"BUILDING thumb\nREADING Snow Crash\n"},
			notOut:  []string{"# now", "not an item"},
		},
		{
			name:     "no project file prints nothing",
			args:     []string{"project"},
			emptyOut: true,
		},
		{
			name:    "an argument",
			args:    []string{"project", "extra"},
			wantErr: true,
		},
	})
}

func TestProjectEdit(t *testing.T) {
	t.Parallel()
	runTests(t, []cmdTest{
		{
			name:      "creates the project file for the editor",
			args:      []string{"project", "edit"},
			setup:     withEditor("true"),
			wantFiles: map[string]string{store.ProjectName: ""},
		},
		{
			name:      "keeps what is there",
			files:     map[string]string{store.ProjectName: "BUILDING thumb\n"},
			args:      []string{"project", "edit"},
			setup:     withEditor("true"),
			wantFiles: map[string]string{store.ProjectName: "BUILDING thumb\n"},
		},
		{
			name:    "the editor fails",
			args:    []string{"project", "edit"},
			setup:   withEditor("false"),
			wantErr: true,
		},
		{
			name:  "no editor set creates nothing",
			args:  []string{"project", "edit"},
			errIs: errNoEditor,
			check: func(t *testing.T, root, _ string, _ error) {
				assertAbsent(t, filepath.Join(root, store.ProjectName))
			},
		},
	})
}
