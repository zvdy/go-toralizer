// Package app wires the go-toralizer command-line interface together.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/zvdy/go-toralizer/internal/buildinfo"
	"github.com/zvdy/go-toralizer/internal/runner"
	"github.com/zvdy/go-toralizer/internal/tor"
)

// Name is the binary name shown in help output.
const Name = "go-toralizer"

// Dependencies are the side-effecting operations the commands rely on. They are
// fields so that tests can substitute them; New fills in any left nil with the
// real implementation.
type Dependencies struct {
	// Probe reports whether a Tor SOCKS proxy is listening.
	Probe func(ctx context.Context, client *tor.Client) error

	// Verify confirms that traffic through the proxy exits over Tor.
	Verify func(ctx context.Context, client *tor.Client) (tor.Status, error)

	// Run executes the wrapped command.
	Run func(ctx context.Context, cfg runner.Config) (runner.Result, error)

	// NewNym requests a fresh Tor circuit.
	NewNym func(ctx context.Context, client *tor.ControlClient) error
}

func (d *Dependencies) applyDefaults() {
	if d.Probe == nil {
		d.Probe = func(ctx context.Context, c *tor.Client) error { return c.Probe(ctx) }
	}
	if d.Verify == nil {
		d.Verify = func(ctx context.Context, c *tor.Client) (tor.Status, error) { return c.Verify(ctx) }
	}
	if d.Run == nil {
		d.Run = runner.Run
	}
	if d.NewNym == nil {
		d.NewNym = func(ctx context.Context, c *tor.ControlClient) error { return c.NewNym(ctx) }
	}
}

// Options configures the CLI.
type Options struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// Build is reported by `version` and `--version`. The zero value is filled
	// in from buildinfo.Get.
	Build buildinfo.Info

	// Env is the base environment handed to wrapped commands. Nil means the
	// process environment.
	Env []string

	Dependencies Dependencies
}

func (o *Options) applyDefaults() {
	if o.Stdin == nil {
		o.Stdin = os.Stdin
	}
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
	if o.Build == (buildinfo.Info{}) {
		o.Build = buildinfo.Get()
	}
	o.Dependencies.applyDefaults()
}

// New builds the root command.
func New(opts Options) *cli.Command {
	opts.applyDefaults()
	a := &application{opts: opts}

	root := &cli.Command{
		Name:        Name,
		Usage:       "run commands with their traffic routed through the Tor network",
		Version:     opts.Build.Short(),
		Description: rootDescription,
		Reader:      opts.Stdin,
		// Errors are rendered and turned into an exit status by the caller, so
		// that every failure takes the same path.
		ExitErrHandler:        func(context.Context, *cli.Command, error) {},
		Writer:                opts.Stdout,
		ErrWriter:             opts.Stderr,
		EnableShellCompletion: true,
		Flags:                 globalFlags(),
		Commands: []*cli.Command{
			a.execCommand(),
			a.checkCommand(),
			a.newnymCommand(),
			a.versionCommand(),
		},
	}

	// urfave/cli looks OnUsageError up on the command being run rather than
	// inheriting it from the parent, so every command gets its own.
	root.OnUsageError = onUsageError
	for _, sub := range root.Commands {
		sub.OnUsageError = onUsageError
	}
	return root
}

// onUsageError reports a rejected flag or argument with ExitUsage, keeping
// "these arguments are wrong" distinguishable from "the command failed".
func onUsageError(_ context.Context, _ *cli.Command, err error, _ bool) error {
	return cli.Exit(err.Error(), ExitUsage)
}

const rootDescription = `go-toralizer runs a command with the proxy environment variables that make
Tor-aware programs (curl, wget, git, most language HTTP clients, ...) send their
traffic through your local Tor SOCKS proxy.

It is not a sandbox. Programs that ignore the proxy environment, or that open
raw sockets, are not affected -- use torsocks or a transparent proxy if you need
enforcement rather than cooperation. DNS is routed through Tor by exporting
socks5h:// URLs, so hostnames are resolved by the exit relay rather than leaking
to your local resolver.`

type application struct {
	opts Options
}

func globalFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    "proxy",
			Aliases: []string{"p"},
			Usage:   "Tor SOCKS proxy `ADDRESS` (socks5://host:port)",
			Value:   tor.DefaultSOCKSAddress,
			Sources: cli.EnvVars("TORALIZER_PROXY"),
		},
		&cli.DurationFlag{
			Name:    "dial-timeout",
			Usage:   "how long to wait for the proxy to accept a connection, as a `DURATION`",
			Value:   tor.DefaultDialTimeout,
			Sources: cli.EnvVars("TORALIZER_DIAL_TIMEOUT"),
		},
		&cli.DurationFlag{
			Name:    "check-timeout",
			Usage:   "how long to wait for the connectivity check, as a `DURATION`",
			Value:   tor.DefaultCheckTimeout,
			Sources: cli.EnvVars("TORALIZER_CHECK_TIMEOUT"),
		},
		&cli.StringFlag{
			Name:    "check-url",
			Usage:   "`URL` queried to confirm traffic exits over Tor",
			Value:   tor.DefaultCheckURL,
			Sources: cli.EnvVars("TORALIZER_CHECK_URL"),
		},
		&cli.BoolFlag{
			Name:    "verbose",
			Aliases: []string{"V"},
			Usage:   "print progress and diagnostics on stderr",
			Sources: cli.EnvVars("TORALIZER_VERBOSE"),
		},
	}
}

// torClient builds a tor.Client from the global flags.
func (a *application) torClient(cmd *cli.Command) (*tor.Client, error) {
	p, err := tor.ParseProxy(cmd.String("proxy"))
	if err != nil {
		return nil, cli.Exit(err.Error(), ExitUsage)
	}
	return &tor.Client{
		Proxy:        p,
		DialTimeout:  cmd.Duration("dial-timeout"),
		CheckTimeout: cmd.Duration("check-timeout"),
		CheckURL:     cmd.String("check-url"),
	}, nil
}

func (a *application) verbosef(cmd *cli.Command, format string, args ...any) {
	if !cmd.Bool("verbose") {
		return
	}
	fmt.Fprintf(a.opts.Stderr, Name+": "+format+"\n", args...)
}

// ensureTor probes the proxy and, unless skipVerify is set, confirms that
// traffic through it actually exits over Tor.
func (a *application) ensureTor(ctx context.Context, cmd *cli.Command, client *tor.Client, skipVerify bool) error {
	a.verbosef(cmd, "probing tor socks proxy at %s", client.Proxy)
	if err := a.opts.Dependencies.Probe(ctx, client); err != nil {
		return cli.Exit(err.Error(), ExitProxyUnreachable)
	}

	if skipVerify {
		a.verbosef(cmd, "skipping connectivity check (--skip-verify)")
		return nil
	}

	a.verbosef(cmd, "verifying tor connectivity via %s", client.CheckURL)
	status, err := a.opts.Dependencies.Verify(ctx, client)
	switch {
	case errors.Is(err, tor.ErrNotTor):
		return cli.Exit(err.Error(), ExitNotTor)
	case err != nil:
		return cli.Exit(err.Error(), ExitFailure)
	}
	a.verbosef(cmd, "tor exit address is %s", status.IP)
	return nil
}

func (a *application) versionCommand() *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "print build information",
		Action: func(_ context.Context, _ *cli.Command) error {
			_, err := fmt.Fprintln(a.opts.Stdout, a.opts.Build.String())
			return err
		},
	}
}

func (a *application) checkCommand() *cli.Command {
	return &cli.Command{
		Name:  "check",
		Usage: "verify that the Tor proxy is reachable and traffic exits over Tor",
		Description: "Probes the SOCKS proxy, then makes a request through it and reports the\n" +
			"exit address. Exits " + itoa(ExitProxyUnreachable) + " if the proxy is unreachable and " +
			itoa(ExitNotTor) + " if traffic is\nnot leaving over Tor.",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			client, err := a.torClient(cmd)
			if err != nil {
				return err
			}

			a.verbosef(cmd, "probing tor socks proxy at %s", client.Proxy)
			if probeErr := a.opts.Dependencies.Probe(ctx, client); probeErr != nil {
				return cli.Exit(probeErr.Error(), ExitProxyUnreachable)
			}
			fmt.Fprintf(a.opts.Stdout, "proxy:  %s (reachable)\n", client.Proxy)

			status, err := a.opts.Dependencies.Verify(ctx, client)
			switch {
			case errors.Is(err, tor.ErrNotTor):
				fmt.Fprintf(a.opts.Stdout, "tor:    no (exit address %s)\n", status.IP)
				return cli.Exit(err.Error(), ExitNotTor)
			case err != nil:
				return cli.Exit(err.Error(), ExitFailure)
			}
			fmt.Fprintf(a.opts.Stdout, "tor:    yes\nexit:   %s\n", status.IP)
			return nil
		},
	}
}

func (a *application) newnymCommand() *cli.Command {
	return &cli.Command{
		Name:    "newnym",
		Aliases: []string{"renew"},
		Usage:   "request a fresh Tor circuit via the control port",
		Description: "Sends SIGNAL NEWNYM to Tor's control port, so subsequent connections use a\n" +
			"new circuit and a different exit relay. Requires ControlPort to be enabled in\n" +
			"your torrc.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "control-address",
				Usage:   "Tor control port `ADDRESS` (host:port)",
				Value:   tor.DefaultControlAddress,
				Sources: cli.EnvVars("TORALIZER_CONTROL_ADDRESS"),
			},
			&cli.StringFlag{
				Name:    "control-password",
				Usage:   "`PASSWORD` for the control port, if one is configured",
				Sources: cli.EnvVars("TORALIZER_CONTROL_PASSWORD"),
			},
			&cli.DurationFlag{
				Name:  "control-timeout",
				Usage: "how long to wait for the control port, as a `DURATION`",
				Value: tor.DefaultControlTimeout,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			control := &tor.ControlClient{
				Address:  cmd.String("control-address"),
				Password: cmd.String("control-password"),
				Timeout:  cmd.Duration("control-timeout"),
			}
			a.verbosef(cmd, "requesting new circuit via %s", control.Address)
			if err := a.opts.Dependencies.NewNym(ctx, control); err != nil {
				return cli.Exit(err.Error(), ExitControl)
			}
			fmt.Fprintln(a.opts.Stdout, "requested a new Tor circuit")
			return nil
		},
	}
}

func (a *application) execCommand() *cli.Command {
	return &cli.Command{
		Name:      "exec",
		Aliases:   []string{"run"},
		Usage:     "run a command with its traffic routed through Tor",
		ArgsUsage: "[--] PROGRAM [ARG...]",
		Description: "Either pass a program and its arguments directly:\n\n" +
			"    " + Name + " exec -- curl https://check.torproject.org/api/ip\n\n" +
			"or pass a shell command line with --command:\n\n" +
			"    " + Name + " exec --command \"curl -s https://example.com | head\"\n\n" +
			"The argument form is preferred: it involves no shell, so nothing is word-split\n" +
			"or expanded on the way to the program. The command's own exit status is passed\n" +
			"through unchanged.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "command",
				Aliases: []string{"c"},
				Usage:   "shell `COMMAND` line to run (alternative to passing arguments)",
			},
			&cli.DurationFlag{
				Name:    "timeout",
				Aliases: []string{"t"},
				Usage:   "abort the command after this `DURATION` (0 disables)",
				Value:   30 * time.Second,
				Sources: cli.EnvVars("TORALIZER_TIMEOUT"),
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "also write the command's output to `FILE` (mode 0600)",
			},
			&cli.BoolFlag{
				Name:    "quiet",
				Aliases: []string{"q"},
				Usage:   "suppress the command's output on this terminal",
			},
			&cli.StringSliceFlag{
				Name:  "shell",
				Usage: "interpreter used for --command, as repeated `ARG` values",
			},
			&cli.BoolFlag{
				Name:    "skip-verify",
				Usage:   "only check that the proxy is listening, do not confirm the exit is Tor",
				Sources: cli.EnvVars("TORALIZER_SKIP_VERIFY"),
			},
		},
		Action: a.runExec,
	}
}

func (a *application) runExec(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args().Slice()
	command := cmd.String("command")

	switch {
	case command == "" && len(args) == 0:
		return cli.Exit("no command given: pass a program after -- or use --command", ExitUsage)
	case command != "" && len(args) > 0:
		return cli.Exit("pass either --command or a program and arguments, not both", ExitUsage)
	}

	client, err := a.torClient(cmd)
	if err != nil {
		return err
	}
	if err = a.ensureTor(ctx, cmd, client, cmd.Bool("skip-verify")); err != nil {
		return err
	}

	cfg := runner.Config{
		Args:       args,
		Command:    command,
		Shell:      cmd.StringSlice("shell"),
		Timeout:    cmd.Duration("timeout"),
		Env:        a.opts.Env,
		ExtraEnv:   ProxyEnv(client.Proxy),
		Stdin:      a.opts.Stdin,
		Stdout:     a.opts.Stdout,
		Stderr:     a.opts.Stderr,
		OutputFile: cmd.String("output"),
	}
	if cmd.Bool("quiet") {
		cfg.Stdout, cfg.Stderr = nil, nil
	}

	if argv, argvErr := cfg.Argv(); argvErr == nil {
		a.verbosef(cmd, "running %q through %s", argv, client.Proxy)
	}

	result, err := a.opts.Dependencies.Run(ctx, cfg)
	a.verbosef(cmd, "command finished in %s with status %d", result.Duration.Round(time.Millisecond), result.ExitCode)

	if cfg.OutputFile != "" && err == nil {
		fmt.Fprintf(a.opts.Stderr, "%s: output written to %s\n", Name, cfg.OutputFile)
	}

	var timeoutErr *runner.TimeoutError
	var exitErr *runner.ExitError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &timeoutErr):
		return cli.Exit(timeoutErr.Error(), ExitTimeout)
	case errors.As(err, &exitErr):
		// The command ran; its status is the meaningful result, so report it
		// without an additional error message of our own.
		return cli.Exit("", exitErr.Code)
	case errors.Is(err, runner.ErrNoCommand):
		return cli.Exit(err.Error(), ExitUsage)
	default:
		return cli.Exit(err.Error(), ExitFailure)
	}
}

// ProxyEnv returns the environment variables that make proxy-aware programs
// route through p. Both the upper- and lower-case spellings are set, because
// different tools look for different ones.
func ProxyEnv(p tor.Proxy) map[string]string {
	url := p.EnvURL()
	return map[string]string{
		"ALL_PROXY":   url,
		"all_proxy":   url,
		"HTTP_PROXY":  url,
		"http_proxy":  url,
		"HTTPS_PROXY": url,
		"https_proxy": url,
		"TORALIZER":   "1",
	}
}

func itoa(i int) string { return strconv.Itoa(i) }
