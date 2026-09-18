// Package runner executes a child process with the environment needed to send
// its traffic through a proxy, and reports how it exited.
package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"time"
)

// ErrNoCommand is returned when neither Command nor Args is set.
var ErrNoCommand = errors.New("no command to run")

// outputFileMode keeps captured output readable only by its owner: it can hold
// whatever the command fetched over Tor.
const outputFileMode os.FileMode = 0o600

// killGrace is how long a cancelled child is given to exit after its interrupt
// before it is killed outright.
const killGrace = 5 * time.Second

// Config describes one command execution.
type Config struct {
	// Args runs a program directly, without a shell. Preferred over Command:
	// arguments reach the program exactly as given, with no quoting or
	// expansion in between.
	Args []string

	// Command is a shell command line, run through Shell. Ignored when Args is
	// set.
	Command string

	// Shell is the interpreter used for Command. Zero means DefaultShell.
	Shell []string

	// Timeout bounds the run. Zero or negative means no timeout.
	Timeout time.Duration

	// Env is the base environment. Zero means os.Environ().
	Env []string

	// ExtraEnv is applied on top of Env, replacing any existing entry with the
	// same name.
	ExtraEnv map[string]string

	// Stdin, Stdout and Stderr are wired to the child. Nil Stdout/Stderr
	// discard the corresponding stream.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// OutputFile also receives stdout and stderr, interleaved. The file is
	// created with owner-only permissions and truncated if it exists.
	OutputFile string
}

// Result describes how the command finished.
type Result struct {
	// ExitCode is the child's exit status, or -1 if it never started or was
	// killed by a signal.
	ExitCode int

	// TimedOut reports that the command was stopped because Timeout elapsed.
	TimedOut bool

	// Duration is the wall-clock time the command ran for.
	Duration time.Duration
}

// DefaultShell returns the interpreter used for shell commands on this platform.
func DefaultShell() []string {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/C"}
	}
	return []string{"sh", "-c"}
}

// ExitError reports a command that ran to completion but exited non-zero.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("command exited with status %d", e.Code)
}

func (e *ExitError) Unwrap() error { return e.Err }

// TimeoutError reports a command stopped because its timeout elapsed.
type TimeoutError struct {
	Timeout time.Duration
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("command timed out after %s", e.Timeout)
}

func (e *TimeoutError) Is(target error) bool { return target == context.DeadlineExceeded }

// Argv returns the exact program and arguments Run would execute.
func (c Config) Argv() ([]string, error) {
	if len(c.Args) > 0 {
		if strings.TrimSpace(c.Args[0]) == "" {
			return nil, fmt.Errorf("%w: program name is empty", ErrNoCommand)
		}
		return slices.Clone(c.Args), nil
	}
	if strings.TrimSpace(c.Command) == "" {
		return nil, ErrNoCommand
	}
	shell := c.Shell
	if len(shell) == 0 {
		shell = DefaultShell()
	}
	return append(slices.Clone(shell), c.Command), nil
}

// Run executes the command described by cfg.
//
// A non-zero exit status is reported as an *ExitError and an elapsed timeout as
// a *TimeoutError; in both cases the returned Result is still populated.
func Run(ctx context.Context, cfg Config) (Result, error) {
	argv, err := cfg.Argv()
	if err != nil {
		return Result{ExitCode: -1}, err
	}

	runCtx := ctx
	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	stdout, stderr := cfg.Stdout, cfg.Stderr
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	if cfg.OutputFile != "" {
		file, openErr := os.OpenFile(cfg.OutputFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, outputFileMode)
		if openErr != nil {
			return Result{ExitCode: -1}, fmt.Errorf("open output file %q: %w", cfg.OutputFile, openErr)
		}
		defer func() { _ = file.Close() }()
		// Stream rather than buffer: the command's output may be arbitrarily
		// large, and it should reach the file as it is produced.
		stdout = io.MultiWriter(stdout, file)
		stderr = io.MultiWriter(stderr, file)
	}

	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...) //nolint:gosec // running a user-supplied command is this tool's purpose
	cmd.Env = buildEnv(cfg.Env, cfg.ExtraEnv)
	cmd.Stdin = cfg.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	// On cancellation, ask the child to stop before killing it, and give it a
	// grace period so it can flush output and clean up.
	cmd.Cancel = func() error { return signalInterrupt(cmd) }
	cmd.WaitDelay = killGrace

	started := time.Now()
	err = cmd.Run()
	result := Result{ExitCode: -1, Duration: time.Since(started)}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if errors.Is(runCtx.Err(), context.DeadlineExceeded) && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.TimedOut = true
		return result, &TimeoutError{Timeout: cfg.Timeout}
	}
	if err == nil {
		return result, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return result, &ExitError{Code: result.ExitCode, Err: err}
	}
	return result, fmt.Errorf("running %q: %w", argv[0], err)
}

// buildEnv layers extra over base, replacing same-named entries rather than
// appending duplicates.
func buildEnv(base []string, extra map[string]string) []string {
	if base == nil {
		base = os.Environ()
	}
	if len(extra) == 0 {
		return slices.Clone(base)
	}

	env := make([]string, 0, len(base)+len(extra))
	for _, entry := range base {
		name, _, found := strings.Cut(entry, "=")
		if found {
			if _, overridden := extra[name]; overridden {
				continue
			}
		}
		env = append(env, entry)
	}
	for name, value := range extra {
		env = append(env, name+"="+value)
	}
	slices.Sort(env)
	return env
}
