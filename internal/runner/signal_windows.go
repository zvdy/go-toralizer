//go:build windows

package runner

import "os/exec"

// signalInterrupt kills the child. Windows has no portable way to deliver an
// interrupt to another process, so os.Interrupt is unimplemented there.
func signalInterrupt(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}
