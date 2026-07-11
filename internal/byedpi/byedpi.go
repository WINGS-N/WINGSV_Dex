// Package byedpi builds the ciadpi argument list from settings and runs the local ByeDPI
// SOCKS proxy that xray chains its outbound through as a DPI-bypass front.
package byedpi

import (
	"os"
	"os/exec"
	"strconv"

	"github.com/WINGS-N/wingsv-dex/internal/config"
)

// desyncFlag maps a desync method name to the ciadpi flag that takes the split position.
var desyncFlag = map[string]string{
	"split":    "--split",
	"disorder": "--disorder",
	"oob":      "--oob",
	"disoob":   "--disoob",
	"fake":     "--fake",
}

// Args builds the ciadpi argument list for the given settings. When UseCommandSettings is
// set, the raw command tokens are used verbatim (after the fixed ip/port), letting power
// users pass ciadpi options directly.
func Args(b config.ByeDPISettings) []string {
	b = b.Normalized()
	args := []string{"--ip", b.ProxyIP, "--port", strconv.Itoa(b.ProxyPort)}
	if b.AuthEnabled && b.Username != "" {
		args = append(args, "--socks-user", b.Username, "--socks-pass", b.Password)
	}
	if b.UseCommandSettings {
		return append(args, tokenize(b.Command)...)
	}
	args = append(args, "--max-conn", strconv.Itoa(b.MaxConnections), "--buf-size", strconv.Itoa(b.BufferSize))
	if b.NoDomain {
		args = append(args, "--no-domain")
	}
	if b.TCPFastOpen {
		args = append(args, "--tfo")
	}
	if b.DefaultTTL > 0 {
		args = append(args, "--def-ttl", strconv.Itoa(b.DefaultTTL))
	}
	if flag, ok := desyncFlag[b.DesyncMethod]; ok {
		args = append(args, flag, strconv.Itoa(b.SplitPosition))
		if b.DesyncMethod == "fake" {
			args = append(args, "--ttl", strconv.Itoa(b.FakeTTL))
		}
	} else if b.DesyncMethod == "auto" {
		args = append(args, "--auto", "torst")
	}
	return args
}

// tokenize splits a raw command string on whitespace, honoring simple double quotes.
func tokenize(s string) []string {
	var out []string
	var cur []rune
	inQuote := false
	flush := func() {
		if len(cur) > 0 {
			out = append(out, string(cur))
			cur = cur[:0]
		}
	}
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case (r == ' ' || r == '\t') && !inQuote:
			flush()
		default:
			cur = append(cur, r)
		}
	}
	flush()
	return out
}

// Process is a running local ciadpi (used in proxy-only mode; vpn mode runs it via the
// net-helper so its egress can bypass the tunnel).
type Process struct {
	cmd *exec.Cmd
}

// Start spawns ciadpi with the settings-derived args. bin is the path to bin/byedpi.
func Start(bin string, b config.ByeDPISettings) (*Process, error) {
	cmd := exec.Command(bin, Args(b)...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	hideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Process{cmd: cmd}, nil
}

// Stop kills the process.
func (p *Process) Stop() {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return
	}
	_ = p.cmd.Process.Kill()
	_ = p.cmd.Wait()
}
