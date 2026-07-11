//go:build windows

package services

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps a spawned console helper (the powershell audio player) from flashing a
// window: HideWindow plus CREATE_NO_WINDOW so no console is allocated at all.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
