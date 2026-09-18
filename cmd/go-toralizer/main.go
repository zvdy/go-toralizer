// Command go-toralizer runs commands with their traffic routed through the Tor
// network.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v3"

	"github.com/zvdy/go-toralizer/internal/app"
)

func main() {
	os.Exit(run())
}

func run() int {
	// Ctrl-C and SIGTERM cancel the context, which stops the wrapped command
	// rather than leaving it orphaned.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd := app.New(app.Options{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})

	err := cmd.Run(ctx, os.Args)
	if err == nil {
		return app.ExitOK
	}

	var exitErr cli.ExitCoder
	if errors.As(err, &exitErr) {
		if msg := exitErr.Error(); msg != "" {
			fmt.Fprintf(os.Stderr, "%s: %s\n", app.Name, msg)
		}
		return exitErr.ExitCode()
	}

	fmt.Fprintf(os.Stderr, "%s: %v\n", app.Name, err)
	return app.ExitFailure
}
