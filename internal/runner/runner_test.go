package runner

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigArgv(t *testing.T) {
	t.Parallel()

	t.Run("arguments are used verbatim", func(t *testing.T) {
		t.Parallel()

		argv, err := Config{Args: []string{"curl", "-s", "https://example.com"}}.Argv()
		require.NoError(t, err)
		assert.Equal(t, []string{"curl", "-s", "https://example.com"}, argv)
	})

	t.Run("arguments win over a shell command", func(t *testing.T) {
		t.Parallel()

		argv, err := Config{Args: []string{"echo", "hi"}, Command: "rm -rf /"}.Argv()
		require.NoError(t, err)
		assert.Equal(t, []string{"echo", "hi"}, argv)
	})

	t.Run("a command is wrapped in the default shell", func(t *testing.T) {
		t.Parallel()

		argv, err := Config{Command: "echo hi"}.Argv()
		require.NoError(t, err)
		assert.Equal(t, append(DefaultShell(), "echo hi"), argv)
	})

	t.Run("a custom shell is honoured", func(t *testing.T) {
		t.Parallel()

		argv, err := Config{Command: "echo hi", Shell: []string{"bash", "-o", "pipefail", "-c"}}.Argv()
		require.NoError(t, err)
		assert.Equal(t, []string{"bash", "-o", "pipefail", "-c", "echo hi"}, argv)
	})

	t.Run("the caller's slices are not aliased", func(t *testing.T) {
		t.Parallel()

		shell := []string{"sh", "-c"}
		cfg := Config{Command: "echo hi", Shell: shell}
		_, err := cfg.Argv()
		require.NoError(t, err)
		assert.Equal(t, []string{"sh", "-c"}, shell, "Argv must not append into the caller's slice")
	})

	for _, tt := range []struct {
		name string
		cfg  Config
	}{
		{name: "nothing to run", cfg: Config{}},
		{name: "blank command", cfg: Config{Command: "   "}},
		{name: "blank program name", cfg: Config{Args: []string{"  "}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := tt.cfg.Argv()
			require.ErrorIs(t, err, ErrNoCommand)
		})
	}
}

func TestDefaultShell(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		assert.Equal(t, []string{"cmd", "/C"}, DefaultShell())
		return
	}
	assert.Equal(t, []string{"sh", "-c"}, DefaultShell())
}

func TestRunSuccess(t *testing.T) {
	t.Parallel()
	requireHelper(t)

	var stdout, stderr bytes.Buffer
	cfg := helperConfig("both")
	cfg.Stdout = &stdout
	cfg.Stderr = &stderr

	result, err := Run(t.Context(), cfg)
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.False(t, result.TimedOut)
	assert.Positive(t, result.Duration)
	assert.Equal(t, "to-stdout", stdout.String())
	assert.Equal(t, "to-stderr", stderr.String())
}

func TestRunPropagatesExitStatus(t *testing.T) {
	t.Parallel()
	requireHelper(t)

	result, err := Run(t.Context(), helperConfig("exit", "42"))

	var exitErr *ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 42, exitErr.Code)
	assert.Equal(t, 42, result.ExitCode)
	assert.Contains(t, err.Error(), "42")
}

func TestRunReportsAMissingProgram(t *testing.T) {
	t.Parallel()

	result, err := Run(t.Context(), Config{
		Args: []string{filepath.Join(t.TempDir(), "definitely-not-a-program")},
	})
	require.Error(t, err)

	var exitErr *ExitError
	assert.NotErrorAs(t, err, &exitErr, "a program that never started is not an exit status")
	assert.Equal(t, -1, result.ExitCode)
}

func TestRunTimesOut(t *testing.T) {
	t.Parallel()
	requireHelper(t)

	cfg := helperConfig("sleep", "30s")
	cfg.Timeout = 200 * time.Millisecond

	started := time.Now()
	result, err := Run(t.Context(), cfg)
	elapsed := time.Since(started)

	var timeoutErr *TimeoutError
	require.ErrorAs(t, err, &timeoutErr)
	assert.Equal(t, 200*time.Millisecond, timeoutErr.Timeout)
	assert.True(t, result.TimedOut)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, elapsed, 20*time.Second, "the child should be stopped, not waited out")
}

func TestRunStopsWhenTheContextIsCancelled(t *testing.T) {
	t.Parallel()
	requireHelper(t)

	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	result, err := Run(ctx, helperConfig("sleep", "30s"))
	require.Error(t, err)
	assert.False(t, result.TimedOut, "cancellation is not a timeout")
	assert.Less(t, result.Duration, 20*time.Second)
}

func TestRunZeroTimeoutMeansNoLimit(t *testing.T) {
	t.Parallel()
	requireHelper(t)

	cfg := helperConfig("sleep", "50ms")
	cfg.Timeout = 0

	result, err := Run(t.Context(), cfg)
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
}

func TestRunWritesAnOutputFile(t *testing.T) {
	t.Parallel()
	requireHelper(t)

	path := filepath.Join(t.TempDir(), "output.txt")
	var stdout bytes.Buffer
	cfg := helperConfig("both")
	cfg.Stdout = &stdout
	cfg.OutputFile = path

	_, err := Run(t.Context(), cfg)
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(content), "to-stdout")
	assert.Contains(t, string(content), "to-stderr")
	assert.Equal(t, "to-stdout", stdout.String(), "the terminal stream is kept as well")

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, outputFileMode, info.Mode().Perm(),
			"captured output may be sensitive and must not be world-readable")
	}
}

func TestRunTruncatesAnExistingOutputFile(t *testing.T) {
	t.Parallel()
	requireHelper(t)

	path := filepath.Join(t.TempDir(), "output.txt")
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("stale", 100)), 0o600))

	cfg := helperConfig("echo", "fresh")
	cfg.OutputFile = path
	_, err := Run(t.Context(), cfg)
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "fresh", string(content))
}

func TestRunReportsAnUnwritableOutputFile(t *testing.T) {
	t.Parallel()
	requireHelper(t)

	cfg := helperConfig("echo", "hi")
	cfg.OutputFile = filepath.Join(t.TempDir(), "missing-dir", "output.txt")

	result, err := Run(t.Context(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open output file")
	assert.Equal(t, -1, result.ExitCode)
}

func TestRunForwardsStdin(t *testing.T) {
	t.Parallel()
	requireHelper(t)

	var stdout bytes.Buffer
	cfg := helperConfig("cat")
	cfg.Stdin = strings.NewReader("piped input")
	cfg.Stdout = &stdout

	_, err := Run(t.Context(), cfg)
	require.NoError(t, err)
	assert.Equal(t, "piped input", stdout.String())
}

func TestRunAppliesExtraEnv(t *testing.T) {
	t.Parallel()
	requireHelper(t)

	var stdout bytes.Buffer
	cfg := helperConfig("env", "ALL_PROXY")
	cfg.ExtraEnv = map[string]string{"ALL_PROXY": "socks5h://127.0.0.1:9050"}
	cfg.Stdout = &stdout

	_, err := Run(t.Context(), cfg)
	require.NoError(t, err)
	assert.Equal(t, "socks5h://127.0.0.1:9050", stdout.String())
}

func TestRunNoCommand(t *testing.T) {
	t.Parallel()

	result, err := Run(t.Context(), Config{})
	require.ErrorIs(t, err, ErrNoCommand)
	assert.Equal(t, -1, result.ExitCode)
}

func TestBuildEnv(t *testing.T) {
	t.Parallel()

	t.Run("extra entries replace rather than shadow", func(t *testing.T) {
		t.Parallel()

		env := buildEnv([]string{"PATH=/bin", "ALL_PROXY=http://old", "HOME=/root"},
			map[string]string{"ALL_PROXY": "socks5h://new"})

		assert.Equal(t, []string{"ALL_PROXY=socks5h://new", "HOME=/root", "PATH=/bin"}, env)
	})

	t.Run("entries without a separator are preserved", func(t *testing.T) {
		t.Parallel()

		env := buildEnv([]string{"WEIRD"}, map[string]string{"A": "1"})
		assert.Contains(t, env, "WEIRD")
		assert.Contains(t, env, "A=1")
	})

	t.Run("the base slice is not modified", func(t *testing.T) {
		t.Parallel()

		base := []string{"B=2", "A=1"}
		buildEnv(base, map[string]string{"C": "3"})
		assert.Equal(t, []string{"B=2", "A=1"}, base)
	})
}

func TestErrorTypes(t *testing.T) {
	t.Parallel()

	inner := errors.New("boom")
	exitErr := &ExitError{Code: 3, Err: inner}
	assert.Equal(t, "command exited with status 3", exitErr.Error())
	require.ErrorIs(t, exitErr, inner)

	timeoutErr := &TimeoutError{Timeout: 5 * time.Second}
	assert.Equal(t, "command timed out after 5s", timeoutErr.Error())
	require.ErrorIs(t, timeoutErr, context.DeadlineExceeded)
}
