package nethelper

import (
	"os"
	"os/exec"
)

// byedpiChild is a running ciadpi process the helper owns.
type byedpiChild struct {
	cmd *exec.Cmd
}

// startByeDPI spawns ciadpi. The caller (Linux) then moves it into the bypass cgroup so
// its upstream egress skips the tunnel. Output goes to the helper's stderr.
func startByeDPI(bin string, args []string) (*byedpiChild, error) {
	cmd := exec.Command(bin, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	setXrayPdeathsig(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &byedpiChild{cmd: cmd}, nil
}

func (b *byedpiChild) stop() {
	if b == nil || b.cmd == nil || b.cmd.Process == nil {
		return
	}
	_ = b.cmd.Process.Kill()
	_ = b.cmd.Wait()
}
