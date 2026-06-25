//go:build !windows

package cli

import (
	"os/exec"
	"syscall"
)

// setDaemonSysProcAttr detaches the child daemon from the parent's process group
// so it keeps running after the REPL (parent) exits.
func setDaemonSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
}
