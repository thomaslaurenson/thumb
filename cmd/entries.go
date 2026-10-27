package cmd

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thomaslaurenson/thumb/internal/entry"
	"github.com/thomaslaurenson/thumb/internal/site"
)

// dateArg returns the date named by an optional argument, or today.
func (a *App) dateArg(args []string) (string, error) {
	if len(args) == 0 {
		return a.today(), nil
	}
	if !entry.ValidDate(args[0]) {
		return "", fmt.Errorf("%q: %w", args[0], entry.ErrBadDate)
	}
	return args[0], nil
}

func (a *App) newCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "create [DATE]",
		Short:             "Create an entry, today by default, and print its path; never overwrites",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: a.completeDates,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := a.store()
			if err != nil {
				return err
			}
			date, err := a.dateArg(args)
			if err != nil {
				return err
			}
			path := st.EntryPath(date)
			if st.Exists(date) {
				fmt.Fprintf(cmd.ErrOrStderr(), "[*] Exists: %s\n", path)
			} else {
				if err := st.Save(entry.New(date)); err != nil {
					return err
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "[+] Created: %s\n", path)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), path)
			return err
		},
	}
}

func (a *App) newEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "edit [DATE]",
		Short:             "Open an entry in $VISUAL or $EDITOR, creating it if needed",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: a.completeDates,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := a.store()
			if err != nil {
				return err
			}
			date, err := a.dateArg(args)
			if err != nil {
				return err
			}
			ed, err := a.requireEditor()
			if err != nil {
				return err
			}
			if !st.Exists(date) {
				if err := st.Save(entry.New(date)); err != nil {
					return err
				}
			}
			return a.runEditor(cmd, ed, st.EntryPath(date))
		},
	}
}

// errNoEditor reports that neither VISUAL nor EDITOR names an editor.
var errNoEditor = errors.New("set VISUAL or EDITOR to choose an editor")

// requireEditor returns the editor command, or errNoEditor when none is set,
// so a command can refuse before it creates the file it would open.
func (a *App) requireEditor() ([]string, error) {
	ed := a.editor()
	if len(ed) == 0 {
		return nil, errNoEditor
	}
	return ed, nil
}

// runEditor opens path in the editor ed with the terminal attached.
func (a *App) runEditor(cmd *cobra.Command, ed []string, path string) error {
	c := exec.CommandContext(cmd.Context(), ed[0], append(ed[1:], path)...)
	c.Stdin = a.env.In
	c.Stdout = cmd.OutOrStdout()
	c.Stderr = cmd.ErrOrStderr()
	if err := c.Run(); err != nil {
		return fmt.Errorf("run editor %q: %w", ed[0], err)
	}
	return nil
}

func (a *App) newAddCmd(name string, marker byte, short string) *cobra.Command {
	c := &cobra.Command{
		Use:   name + " TEXT...",
		Short: short,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			text := strings.Join(args, " ")
			if strings.TrimSpace(text) == "" {
				return fmt.Errorf("%s needs some text", name)
			}
			if strings.ContainsAny(text, "\r\n") {
				return fmt.Errorf("text must be a single line")
			}
			st, err := a.store()
			if err != nil {
				return err
			}
			date := a.today()
			e, err := st.Load(date)
			if err != nil {
				return err
			}
			l := entry.Line{Marker: marker, Text: text}
			e.Append(l)
			if err := st.Save(e); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "[+] %s: %s\n", date, l)
			return nil
		},
	}
	// Flags end at the first word of the text, so "thumb todo add a --date
	// flag" records the whole line. Flags before the text, such as --dir, still
	// parse; text that itself starts with a dash needs "--" in front of it.
	c.Flags().SetInterspersed(false)
	return c
}

func (a *App) newOpenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open",
		Short: "List unresolved - and ! lines with their DATE:LINE refs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := a.store()
			if err != nil {
				return err
			}
			entries, err := st.LoadAll()
			if err != nil {
				return err
			}
			for _, e := range entries {
				for _, it := range e.OpenItems() {
					fmt.Fprintf(cmd.OutOrStdout(), "%-14s %s\n", it.Ref, it.Line)
				}
			}
			return nil
		},
	}
}

func (a *App) newRemarkCmd(name, short string) *cobra.Command {
	return &cobra.Command{
		Use:               name + " REF",
		Short:             short + "; REF is DATE:LINE as printed by open",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: a.completeRefs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := entry.ParseRef(args[0])
			if err != nil {
				return err
			}
			st, err := a.store()
			if err != nil {
				return err
			}
			if !st.Exists(ref.Date) {
				return fmt.Errorf("no entry for %q", ref.Date)
			}
			e, err := st.Load(ref.Date)
			if err != nil {
				return err
			}
			before, err := e.Line(ref.Line)
			if err != nil {
				return err
			}
			op := e.Resolve
			if name == "drop" {
				op = e.Drop
			}
			if err := op(ref.Line); err != nil {
				return err
			}
			if err := st.Save(e); err != nil {
				return err
			}
			after, _ := e.Line(ref.Line)
			fmt.Fprintf(cmd.ErrOrStderr(), "[~] %s: %c -> %c %s\n", ref, before.Marker, after.Marker, after.Text)
			return nil
		},
	}
}

func (a *App) newMigrateCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "migrate REF [DATE]",
		Short:             "Move an open item or idea to another day, today by default, leaving a > line behind",
		Args:              cobra.RangeArgs(1, 2),
		ValidArgsFunction: a.completeMigrate,
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := entry.ParseRef(args[0])
			if err != nil {
				return err
			}
			target, err := a.dateArg(args[1:])
			if err != nil {
				return err
			}
			st, err := a.store()
			if err != nil {
				return err
			}
			if !st.Exists(ref.Date) {
				return fmt.Errorf("no entry for %q", ref.Date)
			}
			from, err := st.Load(ref.Date)
			if err != nil {
				return err
			}
			to, err := st.Load(target)
			if err != nil {
				return err
			}
			if err := entry.Migrate(from, ref.Line, to); err != nil {
				return err
			}
			// Write the copy first: if the second write fails the item exists
			// twice, which is visible, rather than not at all
			if err := st.Save(to); err != nil {
				return err
			}
			if err := st.Save(from); err != nil {
				return err
			}
			l, _ := from.Line(ref.Line)
			fmt.Fprintf(cmd.ErrOrStderr(), "[~] %s: moved to %s: %s\n", ref, target, l.Text)
			return nil
		},
	}
}

func (a *App) newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List entries, newest first, with a tally",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := a.store()
			if err != nil {
				return err
			}
			entries, err := st.LoadAll()
			if err != nil {
				return err
			}
			for i := len(entries) - 1; i >= 0; i-- {
				e := entries[i]
				tally := "(empty)"
				if !e.Empty() {
					tally = e.Tally().String()
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s\n", e.Date, tally)
			}
			return nil
		},
	}
}

func (a *App) newShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "show [DATE]",
		Short:             "Print an entry as plain text, by default the latest one the site would publish",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: a.completeDates,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := a.store()
			if err != nil {
				return err
			}
			c, err := st.Config()
			if err != nil {
				return err
			}
			var e *entry.Entry
			if len(args) == 1 {
				date, err := a.dateArg(args)
				if err != nil {
					return err
				}
				if !st.Exists(date) {
					return fmt.Errorf("no entry for %q", date)
				}
				if e, err = st.Load(date); err != nil {
					return err
				}
			} else {
				entries, err := st.LoadAll()
				if err != nil {
					return err
				}
				today, err := a.publishDay(c)
				if err != nil {
					return err
				}
				published, _ := site.Split(entries, today)
				if len(published) == 0 {
					return fmt.Errorf("no entries with content up to %s yet", today)
				}
				e = published[len(published)-1]
			}
			_, err = cmd.OutOrStdout().Write(site.EntryText(c, e))
			return err
		},
	}
}

func (a *App) newProjectCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "project",
		Short: "Print the project list",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := a.store()
			if err != nil {
				return err
			}
			p, err := st.LoadProject()
			if err != nil {
				return err
			}
			for _, it := range p.Items() {
				fmt.Fprintln(cmd.OutOrStdout(), it)
			}
			return nil
		},
	}
	c.AddCommand(&cobra.Command{
		Use:   "edit",
		Short: "Open the project file in $VISUAL or $EDITOR",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := a.store()
			if err != nil {
				return err
			}
			ed, err := a.requireEditor()
			if err != nil {
				return err
			}
			if err := st.CreateProject(); err != nil {
				return err
			}
			return a.runEditor(cmd, ed, st.ProjectPath())
		},
	})
	return c
}
