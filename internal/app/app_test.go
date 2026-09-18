package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/zvdy/go-toralizer/internal/buildinfo"
	"github.com/zvdy/go-toralizer/internal/runner"
	"github.com/zvdy/go-toralizer/internal/tor"
)

// harness runs the CLI with stubbed dependencies and records what they saw.
type harness struct {
	stdout bytes.Buffer
	stderr bytes.Buffer

	probeErr  error
	status    tor.Status
	verifyErr error
	newNymErr error

	runResult runner.Result
	runErr    error

	probeCalls  int
	verifyCalls int

	ranConfig  *runner.Config
	ranArgv    []string
	newNymCall *tor.ControlClient
}

func newHarness() *harness {
	return &harness{status: tor.Status{IsTor: true, IP: "185.220.101.1"}}
}

func (h *harness) run(t *testing.T, args ...string) error {
	t.Helper()

	cmd := New(Options{
		Stdin:  strings.NewReader(""),
		Stdout: &h.stdout,
		Stderr: &h.stderr,
		Build:  buildinfo.Info{Version: "v1.2.3", GoVersion: "go1.27.0", Platform: "linux/amd64"},
		Env:    []string{"PATH=/usr/bin"},
		Dependencies: Dependencies{
			Probe: func(context.Context, *tor.Client) error {
				h.probeCalls++
				return h.probeErr
			},
			Verify: func(context.Context, *tor.Client) (tor.Status, error) {
				h.verifyCalls++
				return h.status, h.verifyErr
			},
			Run: func(_ context.Context, cfg runner.Config) (runner.Result, error) {
				h.ranConfig = &cfg
				if argv, err := cfg.Argv(); err == nil {
					h.ranArgv = argv
				}
				return h.runResult, h.runErr
			},
			NewNym: func(_ context.Context, c *tor.ControlClient) error {
				h.newNymCall = c
				return h.newNymErr
			},
		},
	})

	return cmd.Run(t.Context(), append([]string{"go-toralizer"}, args...))
}

// requireExit asserts that err carries the given exit code and message fragment.
func requireExit(t *testing.T, err error, code int, contains string) {
	t.Helper()

	var exitErr cli.ExitCoder
	require.ErrorAs(t, err, &exitErr, "expected an exit-coded error, got %v", err)
	assert.Equal(t, code, exitErr.ExitCode())
	if contains != "" {
		assert.Contains(t, err.Error(), contains)
	}
}

func TestExecPreflight(t *testing.T) {
	t.Parallel()

	t.Run("refuses to run when the proxy is unreachable", func(t *testing.T) {
		t.Parallel()

		h := newHarness()
		h.probeErr = fmt.Errorf("%w on 127.0.0.1:9050", tor.ErrProxyUnreachable)

		err := h.run(t, "exec", "--", "echo", "test")
		requireExit(t, err, ExitProxyUnreachable, "tor proxy is not reachable")
		assert.Nil(t, h.ranConfig, "the command must not run without a proxy")
	})

	t.Run("refuses to run when traffic would not go through tor", func(t *testing.T) {
		t.Parallel()

		h := newHarness()
		h.status = tor.Status{IsTor: false, IP: "203.0.113.7"}
		h.verifyErr = fmt.Errorf("%w: exit address 203.0.113.7 is not a tor relay", tor.ErrNotTor)

		err := h.run(t, "exec", "--", "echo", "test")
		requireExit(t, err, ExitNotTor, "not being routed through tor")
		assert.Nil(t, h.ranConfig)
	})

	t.Run("reports a check that could not complete", func(t *testing.T) {
		t.Parallel()

		h := newHarness()
		h.verifyErr = fmt.Errorf("%w: request failed", tor.ErrCheckFailed)

		err := h.run(t, "exec", "--", "echo", "test")
		requireExit(t, err, ExitFailure, "tor connectivity check failed")
	})

	t.Run("skip-verify probes but does not check the exit", func(t *testing.T) {
		t.Parallel()

		h := newHarness()
		err := h.run(t, "exec", "--skip-verify", "--", "echo", "test")
		require.NoError(t, err)
		assert.Equal(t, 1, h.probeCalls)
		assert.Equal(t, 0, h.verifyCalls)
	})
}

func TestExecArguments(t *testing.T) {
	t.Parallel()

	t.Run("runs a program and its arguments without a shell", func(t *testing.T) {
		t.Parallel()

		h := newHarness()
		require.NoError(t, h.run(t, "exec", "--", "curl", "-s", "https://example.com"))
		assert.Equal(t, []string{"curl", "-s", "https://example.com"}, h.ranArgv)
	})

	t.Run("runs a shell command line", func(t *testing.T) {
		t.Parallel()

		h := newHarness()
		require.NoError(t, h.run(t, "exec", "--command", "echo test | tr a-z A-Z"))
		assert.Equal(t, append(runner.DefaultShell(), "echo test | tr a-z A-Z"), h.ranArgv)
	})

	t.Run("honours a custom shell", func(t *testing.T) {
		t.Parallel()

		h := newHarness()
		require.NoError(t, h.run(t, "exec", "--shell", "bash", "--shell", "-c", "--command", "echo test"))
		assert.Equal(t, []string{"bash", "-c", "echo test"}, h.ranArgv)
	})

	t.Run("rejects having no command at all", func(t *testing.T) {
		t.Parallel()

		h := newHarness()
		err := h.run(t, "exec")
		requireExit(t, err, ExitUsage, "no command given")
		assert.Equal(t, 0, h.probeCalls, "usage errors must be caught before touching the network")
	})

	t.Run("rejects mixing --command with arguments", func(t *testing.T) {
		t.Parallel()

		h := newHarness()
		err := h.run(t, "exec", "--command", "echo a", "--", "echo", "b")
		requireExit(t, err, ExitUsage, "not both")
	})

	t.Run("rejects an unparsable timeout", func(t *testing.T) {
		t.Parallel()

		h := newHarness()
		err := h.run(t, "exec", "--command", "echo test", "--timeout", "invalid")
		requireExit(t, err, ExitUsage, "timeout")
	})

	t.Run("rejects an unknown flag", func(t *testing.T) {
		t.Parallel()

		h := newHarness()
		err := h.run(t, "exec", "--command", "echo test", "--nonsense")
		requireExit(t, err, ExitUsage, "nonsense")
		assert.Equal(t, 0, h.probeCalls)
	})

	t.Run("rejects an invalid proxy address", func(t *testing.T) {
		t.Parallel()

		h := newHarness()
		err := h.run(t, "--proxy", "http://127.0.0.1:8080", "exec", "--command", "echo test")
		requireExit(t, err, ExitUsage, "unsupported scheme")
	})
}

func TestExecPassesProxyEnvironmentToTheCommand(t *testing.T) {
	t.Parallel()

	h := newHarness()
	require.NoError(t, h.run(t, "--proxy", "socks5://127.0.0.1:9150", "exec", "--", "curl", "https://example.com"))
	require.NotNil(t, h.ranConfig)

	want := "socks5h://127.0.0.1:9150"
	for _, name := range []string{"ALL_PROXY", "all_proxy", "HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy"} {
		assert.Equal(t, want, h.ranConfig.ExtraEnv[name], "%s should point at the Tor proxy", name)
	}
	assert.Equal(t, "1", h.ranConfig.ExtraEnv["TORALIZER"])
	assert.Equal(t, []string{"PATH=/usr/bin"}, h.ranConfig.Env)
}

func TestExecResults(t *testing.T) {
	t.Parallel()

	t.Run("passes the command's exit status through", func(t *testing.T) {
		t.Parallel()

		h := newHarness()
		h.runResult = runner.Result{ExitCode: 7}
		h.runErr = &runner.ExitError{Code: 7}

		err := h.run(t, "exec", "--", "false")
		requireExit(t, err, 7, "")
	})

	t.Run("reports a timeout with its own exit code", func(t *testing.T) {
		t.Parallel()

		h := newHarness()
		h.runResult = runner.Result{ExitCode: -1, TimedOut: true}
		h.runErr = &runner.TimeoutError{Timeout: time.Second}

		err := h.run(t, "exec", "--timeout", "1s", "--", "sleep", "30")
		requireExit(t, err, ExitTimeout, "command timed out after 1s")
	})

	t.Run("reports a command that could not start", func(t *testing.T) {
		t.Parallel()

		h := newHarness()
		h.runResult = runner.Result{ExitCode: -1}
		h.runErr = errors.New("running \"nope\": executable file not found in $PATH")

		err := h.run(t, "exec", "--", "nope")
		requireExit(t, err, ExitFailure, "executable file not found")
	})

	t.Run("quiet detaches the command's output from the terminal", func(t *testing.T) {
		t.Parallel()

		h := newHarness()
		require.NoError(t, h.run(t, "exec", "--quiet", "--", "echo", "test"))
		require.NotNil(t, h.ranConfig)
		assert.Nil(t, h.ranConfig.Stdout)
		assert.Nil(t, h.ranConfig.Stderr)
	})

	t.Run("output passes the destination through and says where it went", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "result.txt")
		h := newHarness()
		require.NoError(t, h.run(t, "exec", "--output", path, "--", "echo", "test"))
		require.NotNil(t, h.ranConfig)
		assert.Equal(t, path, h.ranConfig.OutputFile)
		assert.Contains(t, h.stderr.String(), "output written to "+path)
	})
}

func TestExecTimeoutFlag(t *testing.T) {
	t.Parallel()

	t.Run("defaults to thirty seconds", func(t *testing.T) {
		t.Parallel()

		h := newHarness()
		require.NoError(t, h.run(t, "exec", "--", "echo", "test"))
		assert.Equal(t, 30*time.Second, h.ranConfig.Timeout)
	})

	t.Run("zero disables the limit", func(t *testing.T) {
		t.Parallel()

		h := newHarness()
		require.NoError(t, h.run(t, "exec", "--timeout", "0", "--", "sleep", "60"))
		assert.Equal(t, time.Duration(0), h.ranConfig.Timeout)
	})
}

func TestVerboseGoesToStderr(t *testing.T) {
	t.Parallel()

	h := newHarness()
	require.NoError(t, h.run(t, "--verbose", "exec", "--", "echo", "test"))

	assert.Contains(t, h.stderr.String(), "probing tor socks proxy")
	assert.Contains(t, h.stderr.String(), "tor exit address is 185.220.101.1")
	assert.NotContains(t, h.stdout.String(), "probing tor socks proxy",
		"diagnostics must not contaminate the command's own output")
}

func TestCheckCommand(t *testing.T) {
	t.Parallel()

	t.Run("reports a working tor connection", func(t *testing.T) {
		t.Parallel()

		h := newHarness()
		require.NoError(t, h.run(t, "check"))
		assert.Contains(t, h.stdout.String(), "proxy:  socks5://127.0.0.1:9050 (reachable)")
		assert.Contains(t, h.stdout.String(), "tor:    yes")
		assert.Contains(t, h.stdout.String(), "exit:   185.220.101.1")
	})

	t.Run("reports an unreachable proxy", func(t *testing.T) {
		t.Parallel()

		h := newHarness()
		h.probeErr = tor.ErrProxyUnreachable

		err := h.run(t, "check")
		requireExit(t, err, ExitProxyUnreachable, "")
		assert.Equal(t, 0, h.verifyCalls)
	})

	t.Run("reports traffic that does not exit over tor", func(t *testing.T) {
		t.Parallel()

		h := newHarness()
		h.status = tor.Status{IsTor: false, IP: "203.0.113.7"}
		h.verifyErr = tor.ErrNotTor

		err := h.run(t, "check")
		requireExit(t, err, ExitNotTor, "")
		assert.Contains(t, h.stdout.String(), "tor:    no (exit address 203.0.113.7)")
	})
}

func TestNewNymCommand(t *testing.T) {
	t.Parallel()

	t.Run("requests a new circuit", func(t *testing.T) {
		t.Parallel()

		h := newHarness()
		require.NoError(t, h.run(t, "newnym"))
		require.NotNil(t, h.newNymCall)
		assert.Equal(t, tor.DefaultControlAddress, h.newNymCall.Address)
		assert.Contains(t, h.stdout.String(), "requested a new Tor circuit")
	})

	t.Run("passes control port settings through", func(t *testing.T) {
		t.Parallel()

		h := newHarness()
		require.NoError(t, h.run(t, "newnym",
			"--control-address", "127.0.0.1:9151",
			"--control-password", "secret",
			"--control-timeout", "3s"))

		require.NotNil(t, h.newNymCall)
		assert.Equal(t, "127.0.0.1:9151", h.newNymCall.Address)
		assert.Equal(t, "secret", h.newNymCall.Password)
		assert.Equal(t, 3*time.Second, h.newNymCall.Timeout)
	})

	t.Run("reports a control port failure", func(t *testing.T) {
		t.Parallel()

		h := newHarness()
		h.newNymErr = fmt.Errorf("%w: authenticating: 515", tor.ErrControl)

		err := h.run(t, "newnym")
		requireExit(t, err, ExitControl, "tor control port error")
	})
}

func TestVersionCommand(t *testing.T) {
	t.Parallel()

	h := newHarness()
	require.NoError(t, h.run(t, "version"))
	assert.Contains(t, h.stdout.String(), "v1.2.3")
	assert.Contains(t, h.stdout.String(), "go1.27.0")
	assert.Contains(t, h.stdout.String(), "linux/amd64")
}

func TestEnvironmentVariablesConfigureFlags(t *testing.T) {
	// Not parallel: it sets process-wide environment.
	t.Setenv("TORALIZER_PROXY", "socks5://127.0.0.1:9150")
	t.Setenv("TORALIZER_TIMEOUT", "2s")

	h := newHarness()
	require.NoError(t, h.run(t, "exec", "--", "echo", "test"))
	require.NotNil(t, h.ranConfig)
	assert.Equal(t, 2*time.Second, h.ranConfig.Timeout)
	assert.Equal(t, "socks5h://127.0.0.1:9150", h.ranConfig.ExtraEnv["ALL_PROXY"])
}

func TestProxyEnv(t *testing.T) {
	t.Parallel()

	p, err := tor.ParseProxy("socks5://127.0.0.1:9050")
	require.NoError(t, err)

	env := ProxyEnv(p)
	assert.Equal(t, "socks5h://127.0.0.1:9050", env["ALL_PROXY"],
		"socks5h keeps DNS resolution inside Tor")
	assert.Len(t, env, 7)
}

func TestOptionsDefaultsAreUsable(t *testing.T) {
	t.Parallel()

	// New must not panic or require every field to be provided.
	cmd := New(Options{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	require.NotNil(t, cmd)
	assert.Equal(t, Name, cmd.Name)
	assert.NotEmpty(t, cmd.Version)

	names := make([]string, 0, len(cmd.Commands))
	for _, sub := range cmd.Commands {
		names = append(names, sub.Name)
	}
	assert.Subset(t, names, []string{"exec", "check", "newnym", "version"})
}

func TestHelpIsAvailableWithoutTor(t *testing.T) {
	t.Parallel()

	h := newHarness()
	require.NoError(t, h.run(t, "help"))
	assert.Contains(t, h.stdout.String(), "exec")
	assert.Equal(t, 0, h.probeCalls)
}

func TestMain(m *testing.M) {
	// Keep ambient proxy settings from leaking into the flag defaults.
	for _, name := range []string{"TORALIZER_PROXY", "TORALIZER_TIMEOUT", "TORALIZER_VERBOSE", "TORALIZER_SKIP_VERIFY"} {
		_ = os.Unsetenv(name)
	}
	os.Exit(m.Run())
}
