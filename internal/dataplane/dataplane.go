// Package dataplane drives the privileged net-helper from the main app. It launches an
// elevated copy of the app (`<exe> --net-helper`) and brings the WireGuard interface
// up/down through a small JSON control protocol. The elevation and transport are
// platform-specific (spawn_*.go): pkexec + stdio on Linux, UAC + named pipe on Windows.
// It never imports the privileged wg package - only the wire format is shared - so the
// unprivileged main process stays free of netlink / driver code.
package dataplane

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

// WGConfig mirrors wg.Config on the wire (matching json tags); the helper decodes it
// back into wg.Config.
type WGConfig struct {
	Interface     string   `json:"interface"`
	PrivateKey    string   `json:"privateKey"`
	Addresses     []string `json:"addresses"`
	MTU           int      `json:"mtu"`
	PeerPublicKey string   `json:"peerPublicKey"`
	PresharedKey  string   `json:"presharedKey"`
	PeerEndpoint  string   `json:"peerEndpoint"`
	AllowedIPs    []string `json:"allowedIps"`
	FwMark        int      `json:"fwmark"`
	Table         int      `json:"table"`
	Whitelist     bool     `json:"whitelist"` // app-routing whitelist mode inverts tunnel routing

	Amnezia bool   `json:"amnezia"`
	Jc      string `json:"jc"`
	Jmin    string `json:"jmin"`
	Jmax    string `json:"jmax"`
	S1      string `json:"s1"`
	S2      string `json:"s2"`
	S3      string `json:"s3"`
	S4      string `json:"s4"`
	H1      string `json:"h1"`
	H2      string `json:"h2"`
	H3      string `json:"h3"`
	H4      string `json:"h4"`
}

type command struct {
	Cmd           string    `json:"cmd"`
	ProtectSocket string    `json:"protectSocket,omitempty"`
	FwMark        int       `json:"fwmark,omitempty"`
	Pid           int       `json:"pid,omitempty"`
	Config        *WGConfig `json:"config,omitempty"`
	Apps          []string  `json:"apps,omitempty"`
	AppMark       int       `json:"appMark,omitempty"`
	Whitelist     bool      `json:"whitelist,omitempty"`
	SelfPid       int       `json:"selfPid,omitempty"`
	IP            string    `json:"ip,omitempty"`
}

type reply struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Rx    int64  `json:"rx,omitempty"`
	Tx    int64  `json:"tx,omitempty"`
}

// helper is a running privileged net-helper and the JSON command channel to it. The
// way it is launched and connected differs per platform (see spawn_*.go): Linux spawns
// it via pkexec and talks over stdio; Windows elevates it via UAC and talks over a
// named pipe. The command/reply wire format is identical.
type helper struct {
	enc  *json.Encoder
	dec  *json.Decoder
	stop func() // close the channel and wait for / terminate the process
}

// Controller owns the privileged helper process.
type Controller struct {
	exePath       string
	protectSocket string
	fwmark        int
	logw          io.Writer

	mu sync.Mutex
	h  *helper
}

// NewController prepares a controller for the given helper binary, protect socket
// name (abstract) and firewall mark.
func NewController(exePath, protectSocket string, fwmark int, logw ...io.Writer) *Controller {
	var w io.Writer
	if len(logw) > 0 {
		w = logw[0]
	}
	return &Controller{exePath: exePath, protectSocket: protectSocket, fwmark: fwmark, logw: w}
}

// Start launches the privileged helper (elevating via the platform's prompt: pkexec on
// Linux, UAC on Windows) and hands it the initial setup. It must run before vkturn
// starts, since vkturn dials the protect socket at launch on Linux.
func (c *Controller) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.h != nil {
		return errors.New("dataplane: already started")
	}
	h, err := startHelper(c.exePath, c.logw)
	if err != nil {
		return err
	}
	c.h = h
	logLine(c.logw, "dataplane: sending start command protect_socket=%s fwmark=%d", c.protectSocket, c.fwmark)
	if err := c.send(command{Cmd: "start", ProtectSocket: c.protectSocket, FwMark: c.fwmark}); err != nil {
		c.stopLocked()
		return err
	}
	logLine(c.logw, "dataplane: start command acknowledged")
	return nil
}

// CgroupAdd moves the vkturn process into the marking cgroup so all of its egress is
// fwmark-tagged. It must run after vkturn is spawned but before it opens any underlay
// socket (before Configure), so no socket escapes the mark.
func (c *Controller) CgroupAdd(pid int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.h == nil {
		return errors.New("dataplane: not started")
	}
	logLine(c.logw, "dataplane: sending cgadd pid=%d", pid)
	return c.send(command{Cmd: "cgadd", Pid: pid})
}

// WGUp brings the kernel WireGuard interface up with the given config.
func (c *Controller) WGUp(cfg WGConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.h == nil {
		return errors.New("dataplane: not started")
	}
	cfg.FwMark = c.fwmark
	logLine(c.logw, "dataplane: sending wgup interface=%s addresses=%d allowed_ips=%d whitelist=%v amnezia=%v", cfg.Interface, len(cfg.Addresses), len(cfg.AllowedIPs), cfg.Whitelist, cfg.Amnezia)
	return c.send(command{Cmd: "wgup", Config: &cfg})
}

// Bypass routes a single underlay destination IP (a VK TURN / peer server that vkturn
// connects to) around the tunnel via the physical gateway. Windows only; on Linux the
// underlay bypass is fwmark-based and this is a no-op the helper acknowledges. Safe to
// call repeatedly with the same IP (idempotent in the helper).
func (c *Controller) Bypass(ip string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.h == nil {
		return errors.New("dataplane: not started")
	}
	return c.send(command{Cmd: "bypass", IP: ip})
}

// Stats returns the tunnel's cumulative rx/tx bytes from the helper's wireguard-go device.
// Windows-only path (Linux reads sysfs directly); the caller polls it for the live rates.
func (c *Controller) Stats() (rx, tx int64, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.h == nil {
		return 0, 0, errors.New("dataplane: not started")
	}
	if err := c.h.enc.Encode(command{Cmd: "stats"}); err != nil {
		return 0, 0, fmt.Errorf("dataplane: write stats: %w", err)
	}
	var rep reply
	if err := c.h.dec.Decode(&rep); err != nil {
		return 0, 0, fmt.Errorf("dataplane: stats: %w", err)
	}
	if !rep.OK {
		return 0, 0, errors.New(rep.Error)
	}
	return rep.Rx, rep.Tx, nil
}

// Activate installs the deferred full-tunnel catch-all routes on Windows (two-phase: after
// the underlay bypass routes are in). No-op elsewhere - the helper acknowledges it.
func (c *Controller) Activate() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.h == nil {
		return errors.New("dataplane: not started")
	}
	return c.send(command{Cmd: "activate"})
}

// AppsUp sets up the per-app split-tunnel cgroup (marked with mark) and starts the
// process matcher for the given app exec ids. A nil/empty list clears it.
func (c *Controller) AppsUp(apps []string, mark int, whitelist bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.h == nil {
		return errors.New("dataplane: not started")
	}
	// In whitelist mode the app itself is not whitelisted, so its own traffic (the
	// exit-IP lookup) would go direct and show the physical IP. Send our PID so the
	// helper tunnels the app too and the Home card reflects the tunnel exit IP.
	return c.send(command{Cmd: "apps", Apps: apps, AppMark: mark, Whitelist: whitelist, SelfPid: os.Getpid()})
}

// AppsDown tears down the per-app split-tunnel cgroup and matcher.
func (c *Controller) AppsDown() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.h == nil {
		return nil
	}
	return c.send(command{Cmd: "appsdown"})
}

// Stop tears the interface down and terminates the helper.
func (c *Controller) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopLocked()
}

func (c *Controller) stopLocked() {
	if c.h == nil {
		return
	}
	_ = c.h.enc.Encode(command{Cmd: "stop"})
	c.h.stop()
	c.h = nil
}

// send writes one command and waits for the helper's reply.
func (c *Controller) send(cmd command) error {
	logLine(c.logw, "dataplane: command %s write", cmd.Cmd)
	if err := c.h.enc.Encode(cmd); err != nil {
		return fmt.Errorf("dataplane: write %s: %w", cmd.Cmd, err)
	}
	var rep reply
	logLine(c.logw, "dataplane: command %s wait reply", cmd.Cmd)
	if err := c.h.dec.Decode(&rep); err != nil {
		return fmt.Errorf("dataplane: %s: helper closed before reply: %w; if stderr says Not authorized, verify pkexec with '/run/wrappers/bin/pkexec env SHELL=/run/current-system/sw/bin/bash id'", cmd.Cmd, err)
	}
	logLine(c.logw, "dataplane: command %s reply ok=%v error=%q", cmd.Cmd, rep.OK, rep.Error)
	if !rep.OK {
		return fmt.Errorf("dataplane: %s: %s", cmd.Cmd, rep.Error)
	}
	return nil
}
