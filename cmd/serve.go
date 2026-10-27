package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/thomaslaurenson/thumb/internal/preview"
	"github.com/thomaslaurenson/thumb/internal/store"
)

// pollInterval is how often serve looks for a saved change. Polling needs no
// file-watching dependency, and a plans repository is small enough to look
// over twice a second without anyone noticing.
const pollInterval = 500 * time.Millisecond

// defaultPort nods to finger's own port, 79, which needs root to bind.
const defaultPort = 7979

func (a *App) newServeCmd() *cobra.Command {
	var port int
	c := &cobra.Command{
		Use:   "serve",
		Short: "Preview the site on localhost, rebuilding and reloading on each save",
		Long: "Serve the site on 127.0.0.1 without writing build/. Saving thumb.conf, the\n" +
			"project file or an entry rebuilds it and reloads open pages. A save that\n" +
			"fails the check is reported, and the last good site stays up.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.serve(cmd, port)
		},
	}
	c.Flags().IntVarP(&port, "port", "p", defaultPort, "port to listen on")
	return c
}

func (a *App) serve(cmd *cobra.Command, port int) error {
	ctx := cmd.Context()
	st, err := a.store()
	if err != nil {
		return err
	}
	watched := []string{filepath.Join(st.Root, store.ConfigName), st.ProjectPath(), st.PlansDir()}
	// Stamp before rendering, so a save landing in between is seen next poll
	stamp, err := preview.Stamp(watched...)
	if err != nil {
		return err
	}
	r, err := a.render(cmd, st)
	if errors.Is(err, errCheckFailed) {
		return fmt.Errorf("%w, nothing served", err)
	}
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("%w; choose another with --port", err)
	}
	site := preview.New(r.files)
	srv := &http.Server{
		Handler:           site,
		ReadHeaderTimeout: 10 * time.Second,
		// Requests share the command's context, so Ctrl-C also ends the open
		// reload streams, which would otherwise hold Shutdown until its timeout
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	fmt.Fprintf(cmd.ErrOrStderr(), "[*] Serving %s at http://%s/ (Ctrl-C to stop)\n", entries(r.published), ln.Addr())
	r.reportHeld(cmd)

	tick := time.NewTicker(pollInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return shutdown(ctx, srv, served)
		case err := <-served:
			return err
		case <-tick.C:
		}
		now, err := preview.Stamp(watched...)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "[!] %v\n", err)
			continue
		}
		if now == stamp {
			continue
		}
		stamp = now
		r, err := a.render(cmd, st)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "[!] %v, still serving the last good site\n", err)
			continue
		}
		if site.Set(r.files) {
			fmt.Fprintf(cmd.ErrOrStderr(), "[~] Rebuilt %s\n", entries(r.published))
			r.reportHeld(cmd)
		}
	}
}

// shutdown stops srv once ctx is cancelled, waits for its Serve to return,
// and reports the cancellation so main exits as interrupted.
func shutdown(ctx context.Context, srv *http.Server, served <-chan error) error {
	// ctx is already cancelled, so shutting down needs a context of its own
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	err := srv.Shutdown(sctx)
	<-served
	if err != nil {
		return err
	}
	return ctx.Err()
}
