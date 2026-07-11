//go:build !windows

package services

import "os/exec"

// hideWindow is a no-op off Windows (there is no console window to hide).
func hideWindow(cmd *exec.Cmd) {}
