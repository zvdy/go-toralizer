//go:build unix

package runner

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/term"
)

const openNoFollow = syscall.O_NOFOLLOW

// isolate puts the child in its own process group so that cancellation also
// reaches grandchildren spawned by "sh -c" pipelines.
func isolate(cmd *exec.Cmd) {
	attr := &syscall.SysProcAttr{Setpgid: true}
	if term.IsTerminal(int(os.Stdin.Fd())) { //nolint:gosec // fd values fit in int
		// A background process group would receive SIGTTIN on terminal reads.
		attr.Foreground = true
		attr.Ctty = int(os.Stdin.Fd()) //nolint:gosec // fd values fit in int
	}
	cmd.SysProcAttr = attr
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
}
