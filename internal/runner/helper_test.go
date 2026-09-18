package runner

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// helperEnv makes the test binary behave as a small helper program instead of
// running the test suite. This keeps the runner tests free of assumptions about
// which shell or utilities exist on the host.
const helperEnv = "GO_TORALIZER_RUNNER_HELPER"

func TestMain(m *testing.M) {
	if mode := os.Getenv(helperEnv); mode != "" {
		os.Exit(helperMain(mode, os.Args[1:]))
	}
	os.Exit(m.Run())
}

// helperArgs returns the argv that re-runs this test binary as a helper.
func helperArgs(args ...string) []string {
	return append([]string{os.Args[0]}, args...)
}

// helperConfig returns a Config that runs the helper in the given mode.
func helperConfig(mode string, args ...string) Config {
	return Config{
		Args:     helperArgs(args...),
		Env:      append(os.Environ(), helperEnv+"="+mode),
		ExtraEnv: map[string]string{},
	}
}

func helperMain(mode string, args []string) int {
	switch mode {
	case "echo":
		fmt.Fprint(os.Stdout, joinArgs(args))
		return 0
	case "stderr":
		fmt.Fprint(os.Stderr, joinArgs(args))
		return 0
	case "both":
		fmt.Fprint(os.Stdout, "to-stdout")
		fmt.Fprint(os.Stderr, "to-stderr")
		return 0
	case "exit":
		code, err := strconv.Atoi(firstArg(args))
		if err != nil {
			return 97
		}
		return code
	case "sleep":
		d, err := time.ParseDuration(firstArg(args))
		if err != nil {
			return 97
		}
		time.Sleep(d)
		return 0
	case "env":
		fmt.Fprint(os.Stdout, os.Getenv(firstArg(args)))
		return 0
	case "cat":
		_, _ = io.Copy(os.Stdout, os.Stdin)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown helper mode %q\n", mode)
		return 98
	}
}

func joinArgs(args []string) string {
	return strings.Join(args, " ")
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// requireHelper skips tests when the test binary cannot re-exec itself, which
// happens when tests run from a cached package archive rather than a binary.
func requireHelper(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(os.Args[0]); err != nil {
		t.Skipf("test binary %q is not executable: %v", os.Args[0], err)
	}
}
