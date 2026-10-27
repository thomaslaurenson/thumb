package cmd

import (
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// filterByPrefix keeps the candidates starting with prefix.
func filterByPrefix(candidates []string, prefix string) []string {
	return slices.DeleteFunc(candidates, func(s string) bool {
		return !strings.HasPrefix(s, prefix)
	})
}

// dates lists today and every entry's date, newest first.
func (a *App) dates() ([]string, error) {
	st, err := a.store()
	if err != nil {
		return nil, err
	}
	all, err := st.Dates()
	if err != nil {
		return nil, err
	}
	slices.Reverse(all)
	if !slices.Contains(all, a.today()) {
		all = append([]string{a.today()}, all...)
	}
	return all, nil
}

// completeDates offers entry dates for a [DATE] argument.
func (a *App) completeDates(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	d, err := a.dates()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	return filterByPrefix(d, toComplete), cobra.ShellCompDirectiveNoFileComp
}

// completeRefs offers the DATE:LINE refs of open items, described by their
// text, for a REF argument.
func (a *App) completeRefs(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	st, err := a.store()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	entries, err := st.LoadAll()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var refs []string
	for _, e := range entries {
		for _, it := range e.OpenItems() {
			if strings.HasPrefix(it.Ref.String(), toComplete) {
				refs = append(refs, it.Ref.String()+"\t"+it.Line.Text)
			}
		}
	}
	return refs, cobra.ShellCompDirectiveNoFileComp
}

// completeMigrate offers refs for the first argument and dates for the second.
func (a *App) completeMigrate(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return a.completeRefs(cmd, args, toComplete)
	}
	return a.completeDates(cmd, args[1:], toComplete)
}
