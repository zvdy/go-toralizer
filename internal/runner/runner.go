// Package runner executes commands with their network traffic directed
// through a SOCKS5 proxy such as Tor.
package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/zvdy/go-toralizer/internal/tor"
)

// ErrTimeout is returned (wrapped) when the command exceeds Options.Timeout.
var ErrTimeout = errors.New("command timed out")

type Mode string

const (
	ModeAuto     Mode = "auto"
	ModeEnv      Mode = "env"
	ModeTorsocks Mode = "torsocks"
)

const (
	defaultProxy = "socks5h://127.0.0.1:9050"
	killDelay    = 5 * time.Second
)

var proxyVars = []string{
	"ALL_PROXY", "all_proxy",
	"HTTP_PROXY", "http_proxy",
	"HTTPS_PROXY", "https_proxy",
}

var noProxyVars = []string{"NO_PROXY", "no_proxy"}

type Options struct {
	// Argv is executed directly without a shell. Exactly one of Argv or Shell must be set.
	Argv []string
	// Shell is a command line executed via "sh -c".
	Shell string
	// Proxy is the SOCKS5 proxy, as a URL or host:port. Defaults to socks5h://127.0.0.1:9050.
	Proxy      string
	Mode       Mode
	Timeout    time.Duration
	OutputPath string
	Verbose    bool
	Stdout     io.Writer
	Stderr     io.Writer
}

// Run executes the command described by opts and returns the child's exit code.
// A non-zero exit code is not an error; errors report launch, timeout and I/O failures.
func Run(ctx context.Context, opts Options) (int, error) {
	if (len(opts.Argv) == 0) == (opts.Shell == "") {
		return 0, errors.New("exactly one of argv or shell command must be set")
	}
	stdout, stderr := opts.Stdout, opts.Stderr
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}

	proxyURL, host, port, err := parseProxy(opts.Proxy)
	if err != nil {
		return 0, err
	}

	logger := slog.New(slog.DiscardHandler)
	if opts.Verbose {
		logger = slog.New(slog.NewTextHandler(stderr, nil))
	}

	args := opts.Argv
	if opts.Shell != "" {
		args = []string{"sh", "-c", opts.Shell}
	}

	mode, args, extra, err := resolveMode(opts.Mode, args, host, port, stderr)
	if err != nil {
		return 0, err
	}
	env := buildEnv(scrubTorsocks(os.Environ()), append(proxyEnv(proxyURL), extra...))

	logger.Info("running command", "program", args[0], "args", len(args)-1, "mode", string(mode), "proxy", proxyURL)

	if opts.OutputPath != "" {
		f, err := openOutput(opts.OutputPath)
		if err != nil {
			return 0, err
		}
		defer func() { _ = f.Close() }()
		stdout = io.MultiWriter(stdout, f)
		stderr = io.MultiWriter(stderr, f)
	}

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext( //nolint:gosec // running the user-supplied command is the purpose of this package
		ctx, args[0], args[1:]...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	isolate(cmd)
	cmd.WaitDelay = killDelay

	err = cmd.Run()
	if err != nil {
		if ctxErr := ctx.Err(); errors.Is(ctxErr, context.DeadlineExceeded) && opts.Timeout > 0 {
			return exitCode(cmd), fmt.Errorf("%w after %s", ErrTimeout, opts.Timeout)
		} else if ctxErr != nil {
			return exitCode(cmd), ctxErr
		}
	}
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return 0, nil
	case errors.As(err, &exitErr):
		return exitCode(cmd), nil
	default:
		return 0, fmt.Errorf("run %s: %w", args[0], err)
	}
}

func parseProxy(raw string) (proxyURL, host, port string, err error) {
	if raw == "" {
		raw = defaultProxy
	}
	norm, err := tor.NormalizeProxy(raw)
	if err != nil {
		return "", "", "", err
	}
	hostport := strings.TrimPrefix(norm, "socks5h://")
	host, port, err = net.SplitHostPort(hostport)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid proxy %q: %w", raw, err)
	}
	return norm, host, port, nil
}

// resolveMode returns the effective mode, the final argv and extra env entries.
func resolveMode(mode Mode, args []string, host, port string, warn io.Writer) (Mode, []string, []string, error) {
	switch mode {
	case "", ModeAuto, ModeEnv, ModeTorsocks:
	default:
		return "", nil, nil, fmt.Errorf("unknown mode %q", mode)
	}
	if mode == "" {
		mode = ModeAuto
	}

	var torsocks string
	if mode != ModeEnv {
		p, err := exec.LookPath("torsocks")
		switch {
		case err == nil:
			torsocks = p
		case mode == ModeTorsocks:
			return "", nil, nil, fmt.Errorf("torsocks mode requested but torsocks not found: %w", err)
		default:
			_, _ = fmt.Fprintln(warn, "warning: torsocks not found; falling back to environment proxy variables, programs that ignore them may bypass Tor")
		}
	}
	if torsocks == "" {
		return ModeEnv, args, nil, nil
	}
	extra := []string{"TORSOCKS_TOR_ADDRESS=" + host, "TORSOCKS_TOR_PORT=" + port}
	return ModeTorsocks, append([]string{torsocks}, args...), extra, nil
}

func proxyEnv(proxyURL string) []string {
	env := make([]string, 0, len(proxyVars)+len(noProxyVars))
	for _, k := range proxyVars {
		env = append(env, k+"="+proxyURL)
	}
	for _, k := range noProxyVars {
		env = append(env, k+"=")
	}
	return env
}

// scrubTorsocks drops inherited torsocks settings, which could redirect traffic
// away from the configured proxy.
func scrubTorsocks(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if !strings.HasPrefix(kv, "TORSOCKS_") {
			out = append(out, kv)
		}
	}
	return out
}

// buildEnv layers overrides on base; overrides replace existing entries by key.
func buildEnv(base, overrides []string) []string {
	replaced := make(map[string]struct{}, len(overrides))
	for _, kv := range overrides {
		k, _, _ := strings.Cut(kv, "=")
		replaced[k] = struct{}{}
	}
	out := make([]string, 0, len(base)+len(overrides))
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		if _, ok := replaced[k]; !ok {
			out = append(out, kv)
		}
	}
	return append(out, overrides...)
}

func openOutput(path string) (*os.File, error) {
	// O_TRUNC is deferred until the descriptor is verified, so a swapped-in
	// device or symlink is never truncated.
	f, err := os.OpenFile(path, //nolint:gosec // path is user-chosen output file
		os.O_WRONLY|os.O_CREATE|openNoFollow, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open output file: %w", err)
	}
	fi, err := f.Stat()
	if err == nil && !fi.Mode().IsRegular() {
		err = fmt.Errorf("output path %q is not a regular file", path)
	}
	if err == nil {
		err = f.Truncate(0)
	}
	if err == nil {
		err = f.Chmod(0o600)
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func exitCode(cmd *exec.Cmd) int {
	ps := cmd.ProcessState
	if ps == nil {
		return -1
	}
	if ws, ok := ps.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return ps.ExitCode()
}
