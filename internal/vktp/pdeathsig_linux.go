//go:build linux

package vktp

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// setPdeathsig makes the kernel SIGKILL the vkturn child if this process dies (even
// on a crash/SIGKILL), so it never orphans and lingers holding its 127.0.0.1:9000
// listener - which would make the next launch panic with "address already in use".
func setPdeathsig(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
}

// reapStaleVkturn SIGKILLs any lingering vkturn process before spawning a fresh one,
// so a disconnect race or a prior crash that orphaned vkturn (still holding
// 127.0.0.1:9000) can never make the new launch fail with "address already in use".
func reapStaleVkturn(binaryPath string) {
	want := strings.TrimSuffix(filepath.Base(binaryPath), " (deleted)")
	if want == "" {
		return
	}
	self := os.Getpid()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}
		target, err := os.Readlink("/proc/" + e.Name() + "/exe")
		if err != nil {
			continue
		}
		if strings.TrimSuffix(filepath.Base(target), " (deleted)") == want {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}
