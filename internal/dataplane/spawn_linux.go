//go:build linux

package dataplane

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// startHelper launches the net-helper via pkexec (which prompts the user to authorize)
// and talks to it over the child's stdin/stdout.
func startHelper(exePath string) (*helper, error) {
	cmd := exec.Command("pkexec", exePath, "--net-helper")
	cmd.Stderr = os.Stderr // surface helper diagnostics in the app's log
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("dataplane: launch helper: %w", err)
	}
	return &helper{
		enc: json.NewEncoder(stdin),
		dec: json.NewDecoder(bufio.NewReader(stdout)),
		stop: func() {
			_ = stdin.Close()
			_ = cmd.Wait()
		},
	}, nil
}
