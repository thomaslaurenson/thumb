package cmd

import "testing"

func TestVersion(t *testing.T) {
	t.Parallel()
	runTests(t, []cmdTest{
		{
			name:    "prints the version",
			args:    []string{"version"},
			wantOut: []string{Version + "\n"},
		},
		{
			name: "needs no repository",
			args: []string{"version"},
			setup: func(_ *testing.T, env *Env, root string) {
				env.Dir = root + "/typo"
			},
			wantOut: []string{Version},
		},
		{
			name:    "an argument",
			args:    []string{"version", "extra"},
			wantErr: true,
		},
	})
}

func TestVersionFrom(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		injected string
		module   string
		want     string
	}{
		{name: "an injected version wins", injected: "1.2.3", module: "v9.9.9", want: "1.2.3"},
		{name: "go install of a tag", injected: devVersion, module: "v1.2.3", want: "1.2.3"},
		{name: "go install of a prerelease tag", injected: devVersion, module: "v1.2.3-rc.1", want: "1.2.3-rc.1"},
		{name: "a working tree", injected: devVersion, module: "(devel)", want: devVersion},
		{name: "no build info", injected: devVersion, module: "", want: devVersion},
		{name: "a dirty tree", injected: devVersion, module: "v1.2.3+dirty", want: devVersion},
		{name: "a pseudo-version with no tag", injected: devVersion, module: "v0.0.0-20261024093000-abcdef123456", want: devVersion},
		{name: "a pseudo-version above a tag", injected: devVersion, module: "v1.2.4-0.20261024093000-abcdef123456", want: devVersion},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := versionFrom(tc.injected, tc.module); got != tc.want {
				t.Errorf("versionFrom(%q, %q) = %q, want %q", tc.injected, tc.module, got, tc.want)
			}
		})
	}
}
