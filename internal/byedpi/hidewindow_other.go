//go:build !windows

package byedpi

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
