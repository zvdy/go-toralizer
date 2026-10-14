package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/zvdy/go-toralizer/internal/runner"
	"github.com/zvdy/go-toralizer/internal/tor"
)

var version = "dev"

type exitError struct{ code int }

func (e exitError) Error() string { return fmt.Sprintf("command exited with status %d", e.code) }

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := newApp(tor.NewChecker()).Run(ctx, os.Args)
	var ee exitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &ee):
		return ee.code
	default:
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
}

func newApp(checker tor.Checker) *cli.Command {
	return &cli.Command{
		Name:    "go-toralizer",
		Usage:   "run commands through the Tor network",
		Version: version,
		Commands: []*cli.Command{
			{
				Name:      "exec",
				Usage:     "execute a command through Tor",
				ArgsUsage: "[-- command [args...]]",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "command", Aliases: []string{"c"}, Usage: "shell command line (alternative to argv after --)"},
					&cli.StringFlag{Name: "proxy", Aliases: []string{"p"}, Value: "socks5h://127.0.0.1:9050", Usage: "Tor SOCKS5 proxy"},
					&cli.DurationFlag{Name: "timeout", Aliases: []string{"t"}, Value: 30 * time.Second, Usage: "command timeout (0 disables)"},
					&cli.StringFlag{Name: "output", Aliases: []string{"o"}, Usage: "also write output to this file"},
					&cli.StringFlag{Name: "mode", Value: string(runner.ModeAuto), Usage: "routing mode: auto, env or torsocks"},
					&cli.BoolFlag{Name: "skip-verify", Usage: "skip the check.torproject.org verification"},
					&cli.BoolFlag{Name: "verbose", Aliases: []string{"v"}, Usage: "log execution details to stderr"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return execAction(ctx, cmd, checker)
				},
			},
		},
	}
}

func execAction(ctx context.Context, cmd *cli.Command, checker tor.Checker) error {
	shell := cmd.String("command")
	argv := cmd.Args().Slice()
	if (shell == "") == (len(argv) == 0) {
		return errors.New("provide either --command or a command after --")
	}

	proxy, err := tor.NormalizeProxy(cmd.String("proxy"))
	if err != nil {
		return err
	}
	if err := checker.Reachable(ctx, proxy); err != nil {
		return fmt.Errorf("tor proxy is not reachable on %s: %w", proxy, err)
	}
	if !cmd.Bool("skip-verify") {
		if err := checker.Verify(ctx, proxy); err != nil {
			return fmt.Errorf("tor verification failed: %w", err)
		}
	}

	if cmd.Bool("skip-verify") {
		_, _ = fmt.Fprintln(cmd.ErrWriter, "warning: skipping Tor verification; exit through Tor is not confirmed")
	}

	code, err := runner.Run(ctx, runner.Options{
		Argv:       argv,
		Shell:      shell,
		Proxy:      proxy,
		Mode:       runner.Mode(cmd.String("mode")),
		Timeout:    cmd.Duration("timeout"),
		OutputPath: cmd.String("output"),
		Verbose:    cmd.Bool("verbose"),
	})
	if errors.Is(err, runner.ErrTimeout) {
		fmt.Fprintln(os.Stderr, "error:", err)
		return exitError{code: 124}
	}
	if err != nil {
		return err
	}
	if code != 0 {
		return exitError{code: code}
	}
	return nil
}
