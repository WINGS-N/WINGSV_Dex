package nethelper

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
)

// xrayRunWrapper is the run json bin/xray consumes: it points at the real xray config and
// carries the TUN name/priority the helper's route init uses.
type xrayRunWrapper struct {
	TunName     string `json:"tunName,omitempty"`
	TunPriority int    `json:"tunPriority,omitempty"`
	EnableIPv6  bool   `json:"enableIPv6,omitempty"`
	DatDir      string `json:"datDir,omitempty"`
	ConfigPath  string `json:"configPath,omitempty"`
}

// xrayChild is a running bin/xray process plus its scratch dir.
type xrayChild struct {
	cmd *exec.Cmd
	dir string
}

// startXray writes the xray config and run wrapper to a private scratch dir and spawns
// bin/xray run. The child inherits the helper's privileges (it must create the TUN and set
// routes). Its stdout/stderr go to the helper's stderr; stdout here is the JSON reply
// channel and must not be polluted.
func startXray(xrayBin, configJSON, tunName, datDir string, enableIPv6 bool) (*xrayChild, error) {
	dir, err := os.MkdirTemp("", "wingsv-xray-")
	if err != nil {
		return nil, err
	}
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(configJSON), 0o600); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	wrapper := xrayRunWrapper{
		TunName:     tunName,
		TunPriority: 20,
		EnableIPv6:  enableIPv6,
		DatDir:      datDir,
		ConfigPath:  cfgPath,
	}
	wb, err := json.Marshal(&wrapper)
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
	setXrayPdeathsig(cmd)
	if err := cmd.Start(); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	return &xrayChild{cmd: cmd, dir: dir}, nil
}

// stop kills the xray child and removes its scratch dir. Killing the process closes the
// TUN, which drops the routes bound to it.
func (x *xrayChild) stop() {
	if x == nil {
		return
	}
	if x.cmd != nil && x.cmd.Process != nil {
		_ = x.cmd.Process.Kill()
		_ = x.cmd.Wait()
	}
	if x.dir != "" {
		_ = os.RemoveAll(x.dir)
	}
}
