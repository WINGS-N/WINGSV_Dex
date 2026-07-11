//go:build !linux

package nethelper

import "os/exec"

func setXrayPdeathsig(cmd *exec.Cmd) {}
