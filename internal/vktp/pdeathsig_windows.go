//go:build windows

package vktp

import (
	"os/exec"
	"syscall"
)

// CREATE_NO_WINDOW keeps the console-subsystem vkturn child from allocating a console,
// which would otherwise flash a black window every launch under the GUI (windowsgui) app.
const createNoWindow = 0x08000000

func setPdeathsig(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

// reapStaleVkturn is a no-op on Windows (the Linux /proc scan does not apply).
func reapStaleVkturn(binaryPath string) {}
