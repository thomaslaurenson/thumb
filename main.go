// Command thumb keeps a daily .plan file and a project list in the style of
// the 1990s finger plans, and publishes them as a small static site.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	// The zone database goes into the binary, so the timezone key in thumb.conf
	// resolves on Windows and in minimal containers, which ship no zoneinfo
	_ "time/tzdata"

	"github.com/thomaslaurenson/thumb/cmd"
)

func main() {
	// SIGHUP is the terminal going away, such as a dropped SSH connection; the
	// interface saves on the way out rather than dying with unsaved changes
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	env := cmd.Env{
		Dir:    os.Getenv("THUMB_DIR"),
		Visual: os.Getenv("VISUAL"),
		Editor: os.Getenv("EDITOR"),
		Getwd:  os.Getwd,
		Now:    time.Now,
		In:     os.Stdin,
	}
	root := cmd.NewRootCmd(os.Stdout, os.Stderr, env)
	if err := root.ExecuteContext(ctx); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			os.Exit(130)
		}
		var ec *cmd.ExitCodeError
		if errors.As(err, &ec) {
			os.Exit(ec.Code)
		}
		fmt.Fprintf(os.Stderr, "[!] %v\n", err)
		os.Exit(1)
	}
}
