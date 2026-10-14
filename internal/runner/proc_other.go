//go:build !unix

package runner

import "os/exec"

const openNoFollow = 0

func isolate(cmd *exec.Cmd) {
	cmd.Cancel = func() error { return cmd.Process.Kill() }
}
