package xray

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
)

// LocalProcess is a bin/xray run child started unprivileged for proxy-only mode (no TUN,
// so no elevation needed - it only opens local socks/http listeners).
type LocalProcess struct {
	cmd *exec.Cmd
	dir string
}

type localWrapper struct {
	ConfigPath string `json:"configPath"`
}

// StartLocal writes the config to a scratch dir and spawns bin/xray run with no tun name,
// so the helper's route init is skipped and the process stays unprivileged.
func StartLocal(xrayBin, configJSON string) (*LocalProcess, error) {
	dir, err := os.MkdirTemp("", "wingsv-xray-proxy-")
	if err != nil {
		return nil, err
	}
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(configJSON), 0o600); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	wb, err := json.Marshal(&localWrapper{ConfigPath: cfgPath})
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	runPath := filepath.Join(dir, "run.json")
	if err := os.WriteFile(runPath, wb, 0o600); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	cmd := exec.Command(xrayBin, "run", "-config", runPath)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	hideWindow(cmd)
	if err := cmd.Start(); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	return &LocalProcess{cmd: cmd, dir: dir}, nil
}

// Stop kills the process and removes its scratch dir.
func (p *LocalProcess) Stop() {
	if p == nil {
		return
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		_ = p.cmd.Wait()
	}
	if p.dir != "" {
		_ = os.RemoveAll(p.dir)
	}
}
