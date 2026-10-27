package cmd

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/thomaslaurenson/thumb/internal/gitsync"
	"github.com/thomaslaurenson/thumb/internal/site"
	"github.com/thomaslaurenson/thumb/internal/store"
)

// check prints every problem Check finds to errw and reports whether any was
// an error, and how many entries it read.
func check(errw io.Writer, st *store.Store) (failed bool, entries int, err error) {
	r, err := st.Check()
	if err != nil {
		return false, 0, err
	}
	for _, p := range r.Problems {
		kind := "warning"
		if p.Fatal {
			kind = "error"
		}
		fmt.Fprintf(errw, "[!] %s: %s: %s\n", p.Path, kind, p.Message)
	}
	return r.Failed(), r.Entries, nil
}

func (a *App) newCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Validate thumb.conf, the project file and every entry",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := a.store()
			if err != nil {
				return err
			}
			failed, n, err := check(cmd.ErrOrStderr(), st)
			if err != nil {
				return err
			}
			if failed {
				return &ExitCodeError{Code: 1}
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "[*] Checked %s\n", entries(n))
			return nil
		},
	}
}

func entries(n int) string {
	if n == 1 {
		return "1 entry"
	}
	return fmt.Sprintf("%d entries", n)
}

func (a *App) newBuildCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "build [DIR]",
		Short: "Check, then generate the site into DIR, build/ by default",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := a.store()
			if err != nil {
				return err
			}
			out := filepath.Join(st.Root, "build")
			if len(args) == 1 {
				if out, err = a.abs(args[0]); err != nil {
					return err
				}
			}
			for _, guarded := range []string{st.Root, st.PlansDir()} {
				if filepath.Clean(out) == filepath.Clean(guarded) {
					return fmt.Errorf("refusing to build into %q", out)
				}
			}
			r, err := a.render(cmd.ErrOrStderr(), st)
			if errors.Is(err, errCheckFailed) {
				return fmt.Errorf("%w, nothing built", err)
			}
			if err != nil {
				return err
			}
			if err := site.Write(out, r.files); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "[*] Built %s into %s\n", entries(r.published), out)
			r.reportHeld(cmd.ErrOrStderr())
			return nil
		},
	}
}

// errCheckFailed reports a repository whose check found an error, which check
// has already printed.
var errCheckFailed = errors.New("check failed")

// rendered is a checked site, ready to write or serve.
type rendered struct {
	files map[string][]byte
	// published and held count the entries the site shows and the ones dated
	// after today that it leaves out
	published, held int
	today           string
}

// render checks the repository, printing any problems to errw, then renders
// its site as of the author's today.
func (a *App) render(errw io.Writer, st *store.Store) (rendered, error) {
	failed, _, err := check(errw, st)
	if err != nil {
		return rendered{}, err
	}
	if failed {
		return rendered{}, errCheckFailed
	}
	c, err := st.Config()
	if err != nil {
		return rendered{}, err
	}
	all, err := st.LoadAll()
	if err != nil {
		return rendered{}, err
	}
	p, err := st.LoadProject()
	if err != nil {
		return rendered{}, err
	}
	today, err := a.publishDay(c)
	if err != nil {
		return rendered{}, err
	}
	files, err := site.Render(site.Options{Config: c, Entries: all, Project: p.Items(), Version: Version, Today: today})
	if err != nil {
		return rendered{}, err
	}
	published, held := site.Split(all, today)
	return rendered{files: files, published: len(published), held: len(held), today: today}, nil
}

// reportHeld tells errw how many entries the site leaves out for being dated
// after today, if any.
func (r rendered) reportHeld(errw io.Writer) {
	if r.held > 0 {
		fmt.Fprintf(errw, "[*] Held back %s dated after %s\n", entries(r.held), r.today)
	}
}

func (a *App) newSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Commit the entries and project, pull with rebase, and push",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := a.store()
			if err != nil {
				return err
			}
			failed, _, err := check(cmd.ErrOrStderr(), st)
			if err != nil {
				return err
			}
			if failed {
				return fmt.Errorf("check failed, nothing synced")
			}
			paths := []string{store.PlansName, store.ProjectName}
			r, err := gitsync.Sync(cmd.Context(), st.Root, paths, "Updated plans for "+a.today(), cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			if r.Committed {
				fmt.Fprintln(cmd.ErrOrStderr(), "[+] Committed and pushed")
			} else {
				fmt.Fprintln(cmd.ErrOrStderr(), "[*] Nothing new to commit, pulled and pushed")
			}
			return nil
		},
	}
}
