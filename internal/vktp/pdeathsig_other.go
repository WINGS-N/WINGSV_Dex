//go:build !linux && !windows

package vktp

import "os/exec"

// setPdeathsig is a no-op off Linux (Pdeathsig is Linux-only).
func setPdeathsig(cmd *exec.Cmd) {}

// reapStaleVkturn is a no-op off Linux (scans /proc).
func reapStaleVkturn(binaryPath string) {}
