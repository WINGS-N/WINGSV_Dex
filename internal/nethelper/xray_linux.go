//go:build linux

package nethelper

import (
	"os/exec"
	"syscall"
)

// setXrayPdeathsig makes the kernel SIGKILL the xray child if the helper dies, so a
// crashed helper never orphans xray still holding the TUN device.
func setXrayPdeathsig(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
}
