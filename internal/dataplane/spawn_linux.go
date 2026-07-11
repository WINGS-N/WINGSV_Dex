//go:build linux

package dataplane

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func pkexecEnv(logw io.Writer) []string {
	env := os.Environ()
	current := os.Getenv("SHELL")
	if shellAllowed(current) {
		return env
	}
	for _, candidate := range []string{"/run/current-system/sw/bin/bash", "/run/current-system/sw/bin/sh", "/bin/bash", "/bin/sh"} {
		if shellAllowed(candidate) {
			logLine(logw, "dataplane: overriding SHELL for pkexec old=%q new=%q", current, candidate)
			return appendEnv(env, "SHELL", candidate)
		}
	}
	logLine(logw, "dataplane: unsetting invalid SHELL for pkexec old=%q", current)
	return removeEnv(env, "SHELL")
}

func shellAllowed(shell string) bool {
	if shell == "" {
		return false
	}
	raw, err := os.ReadFile("/etc/shells")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == shell {
			return true
		}
	}
	return false
}

func appendEnv(env []string, key string, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	for _, item := range env {
		if !strings.HasPrefix(item, prefix) {
			out = append(out, item)
		}
	}
	return append(out, prefix+value)
}

func removeEnv(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, item := range env {
		if !strings.HasPrefix(item, prefix) {
			out = append(out, item)
		}
	}
	return out
}

// startHelper launches the net-helper via pkexec (which prompts the user to authorize)
// and talks to it over the child's stdin/stdout.
func startHelper(exePath string, logw io.Writer) (*helper, error) {
	pkexecPath, err := exec.LookPath("pkexec")
	if err != nil {
		return nil, fmt.Errorf("dataplane: find pkexec: %w", err)
	}
	logLine(logw, "dataplane: launching helper pkexec=%s exe=%s path_entries=%d", pkexecPath, exePath, pathEntries(os.Getenv("PATH")))
	if pkexecPath != "/run/wrappers/bin/pkexec" {
		logLine(logw, "dataplane: warning: pkexec is not the NixOS setuid wrapper; check PATH if authorization fails")
	}
	if rel, err := filepath.Rel(os.Getenv("HOME"), exePath); err == nil && !strings.HasPrefix(rel, "..") {
		logLine(logw, "dataplane: helper binary is under HOME; polkit may require entering this user's password")
	}
	cmd := exec.Command(pkexecPath, exePath, "--net-helper")
	cmd.Env = pkexecEnv(logw)
	cmd.Stderr = stderrWriter(logw)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("dataplane: launch helper via %s: %w", pkexecPath, err)
	}
	logLine(logw, "dataplane: helper process started pid=%d", cmd.Process.Pid)
	return &helper{
		enc: json.NewEncoder(stdin),
		dec: json.NewDecoder(bufio.NewReader(stdout)),
		stop: func() {
			_ = stdin.Close()
			_ = cmd.Wait()
		},
	}, nil
}

func pathEntries(path string) int {
	if path == "" {
		return 0
	}
	return len(strings.Split(path, ":"))
}
