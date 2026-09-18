//go:build !windows

package runner

import (
	"os"
	"os/exec"
)

// signalInterrupt asks the child to terminate politely.
func signalInterrupt(cmd *exec.Cmd) error {
	return cmd.Process.Signal(os.Interrupt)
}
