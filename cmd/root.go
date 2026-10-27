// Package cmd wires the thumb command line: argument parsing, the process
// environment and output streams. The work itself lives under internal/.
package cmd

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/thomaslaurenson/thumb/internal/config"
	"github.com/thomaslaurenson/thumb/internal/store"
	"github.com/thomaslaurenson/thumb/internal/tui"
)

// Env holds the facts about the process that main reads once and passes in,
// so nothing below reads the environment or the clock itself.
type Env struct {
	// Dir is THUMB_DIR, the data repository, or empty to search upwards.
	Dir string
	// Visual and Editor are VISUAL and EDITOR.
	Visual string
	Editor string
	// Getwd returns the working directory.
	Getwd func() (string, error)
	// Now returns the current local time.
	Now func() time.Time
	// In is standard input, handed to the editor and the interface.
	In io.Reader
}

// ExitCodeError is returned by a command that has already produced its output
// and needs to set the process exit code itself.
type ExitCodeError struct{ Code int }

func (e *ExitCodeError) Error() string { return "" }

// App holds the dependencies shared by every subcommand.
type App struct {
	env Env
	dir string
}

// NewRootCmd builds the command tree, writing output to out and errw. Run
// bare, thumb opens the interactive interface.
func NewRootCmd(out, errw io.Writer, env Env) *cobra.Command {
	a := &App{env: env}
	root := &cobra.Command{
		Use:   "thumb",
		Short: "Keep a daily .plan and a project list, and publish them as a site",
		Long: "thumb keeps a daily .plan file in the style of the 1990s finger plans,\n" +
			"plus a project list, and publishes both as a small static site.\n\n" +
			"Run with no command to open the interactive interface.",
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       Version,
		Args:          cobra.NoArgs,
		RunE:          a.runTUI,
	}
	root.SetOut(out)
	root.SetErr(errw)
	root.PersistentFlags().StringVar(&a.dir, "dir", "", "data repository (default THUMB_DIR, or the nearest folder with a thumb.conf)")

	root.AddCommand(
		a.newCreateCmd(),
		a.newEditCmd(),
		a.newAddCmd("done", '*', "add a done line to today's entry"),
		a.newAddCmd("todo", '-', "add a to do line to today's entry"),
		a.newAddCmd("idea", '?', "add an idea to today's entry"),
		a.newAddCmd("block", '!', "add a blocked line to today's entry"),
		a.newOpenCmd(),
		a.newRemarkCmd("resolve", "mark an open item done later (+)"),
		a.newRemarkCmd("drop", "mark an open item or idea dropped (~)"),
		a.newMigrateCmd(),
		a.newListCmd(),
		a.newShowCmd(),
		a.newProjectCmd(),
		a.newCheckCmd(),
		a.newBuildCmd(),
		a.newServeCmd(),
		a.newSyncCmd(),
		newVersionCmd(),
	)
	return root
}

// store opens the data repository: --dir, then THUMB_DIR, then the nearest
// folder above the working directory holding a thumb.conf. A folder named by
// --dir or THUMB_DIR must hold a thumb.conf itself.
func (a *App) store() (*store.Store, error) {
	if a.dir != "" {
		st, err := store.Open(a.dir)
		if err != nil {
			return nil, fmt.Errorf("--dir %w", err)
		}
		return st, nil
	}
	if a.env.Dir != "" {
		st, err := store.Open(a.env.Dir)
		if err != nil {
			return nil, fmt.Errorf("THUMB_DIR %w", err)
		}
		return st, nil
	}
	wd, err := a.env.Getwd()
	if err != nil {
		return nil, err
	}
	root, err := store.Find(wd)
	if err != nil {
		return nil, fmt.Errorf("%w; run thumb inside your plans repository or set THUMB_DIR", err)
	}
	return store.New(root), nil
}

// abs resolves path against the working directory Env reports, rather than
// the process's own, so tests can run in parallel from anywhere.
func (a *App) abs(path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	wd, err := a.env.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, path), nil
}

func (a *App) today() string {
	return a.env.Now().Format("2006-01-02")
}

// publishDay is the author's date for deciding what is published, taken in
// thumb.conf's timezone so a build on a UTC runner agrees with the author.
func (a *App) publishDay(c config.Config) (string, error) {
	return c.Today(a.env.Now())
}

// editor returns the editor command split into words, so a setting such as
// "code --wait" works.
func (a *App) editor() []string {
	e := a.env.Visual
	if e == "" {
		e = a.env.Editor
	}
	return strings.Fields(e)
}

func (a *App) runTUI(cmd *cobra.Command, _ []string) error {
	st, err := a.store()
	if err != nil {
		return err
	}
	return tui.Run(cmd.Context(), tui.Options{
		Store: st,
		Today: a.today(),
		Now:   a.env.Now,
		In:    a.env.In,
		Out:   cmd.OutOrStdout(),
	})
}
