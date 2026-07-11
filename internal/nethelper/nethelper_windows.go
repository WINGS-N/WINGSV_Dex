//go:build windows

// Windows side of the privileged data plane. Unlike Linux (pkexec + stdio), the elevated
// helper connects back to a named pipe hosted by the GUI (stdio cannot cross the UAC
// boundary), then drives the wintun + wireguard-go tunnel over the same JSON protocol.
package nethelper

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/Microsoft/go-winio"

	"github.com/WINGS-N/wingsv-dex/internal/wgwin"
)

// wgConfig mirrors the on-wire data-plane config (dataplane.WGConfig json tags).
type wgConfig struct {
	Interface     string   `json:"interface"`
	PrivateKey    string   `json:"privateKey"`
	Addresses     []string `json:"addresses"`
	MTU           int      `json:"mtu"`
	PeerPublicKey string   `json:"peerPublicKey"`
	PresharedKey  string   `json:"presharedKey"`
	PeerEndpoint  string   `json:"peerEndpoint"`
	AllowedIPs    []string `json:"allowedIps"`
	DNS           []string `json:"dns"`
	Amnezia       bool     `json:"amnezia"`
}

type command struct {
	Cmd           string    `json:"cmd"`
	ProtectSocket string    `json:"protectSocket"`
	FwMark        int       `json:"fwmark"`
	Pid           int       `json:"pid"`
	Config        *wgConfig `json:"config"`
	Apps          []string  `json:"apps"`
	AppMark       int       `json:"appMark"`
	Whitelist     bool      `json:"whitelist"`
	SelfPid       int       `json:"selfPid"`
	IP            string    `json:"ip"`

	XrayBin    string `json:"xrayBin"`
	XrayConfig string `json:"xrayConfig"`
	TunName    string `json:"tunName"`
	DatDir     string `json:"datDir"`
	EnableIPv6 bool   `json:"enableIpv6"`
}

type reply struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Rx    int64  `json:"rx,omitempty"`
	Tx    int64  `json:"tx,omitempty"`
}

// Run connects to the GUI's control pipe (name from --pipe) and serves the data-plane
// control loop, tearing everything down on stop or when the pipe closes.
// openHelperLog opens the net-helper log file under LocalAppData (falling back to the temp
// dir), truncating any previous run.
func openHelperLog() *os.File {
	dir := os.Getenv("LOCALAPPDATA")
	if dir == "" {
		dir = os.TempDir()
	} else {
		dir = filepath.Join(dir, "WINGS V")
		_ = os.MkdirAll(dir, 0o755)
	}
	f, err := os.OpenFile(filepath.Join(dir, "nethelper.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil
	}
	return f
}

func Run() error {
	log.SetPrefix("net-helper: ")
	log.SetFlags(log.Ltime)
	// This helper runs elevated and window-hidden, so its stdio is not visible to the GUI's
	// console. Send all logs - including wireguard-go's, which writes to os.Stdout - to a
	// file so the tunnel/data-plane diagnostics can be inspected.
	if f := openHelperLog(); f != nil {
		os.Stdout = f
		os.Stderr = f
		log.SetOutput(f)
	}

	pipeName := pipeArg()
	if pipeName == "" {
		return io.EOF
	}
	conn, err := winio.DialPipe(pipeName, nil)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	var tun *wgwin.Tunnel
	var vkturnPid int
	var xray *xrayChild
	teardown := func() {
		if xray != nil {
			xray.stop()
			xray = nil
		}
		if tun != nil {
			tun.Down()
			tun = nil
		}
	}
	defer teardown()

	for {
		var cmd command
		if err := dec.Decode(&cmd); err != nil {
			return nil
		}
		switch cmd.Cmd {
		case "start":
			// No protect socket on Windows; the vkturn underlay bypass is a WFP concern
			// handled by wgup/cgadd. Acknowledge so the GUI proceeds.
			log.Printf("started (elevated)")
			_ = enc.Encode(reply{OK: true})
		case "cgadd":
			// Record vkturn's pid for the WFP tunnel-bypass exclusion (see wgup TODO).
			vkturnPid = cmd.Pid
			log.Printf("cgadd: vkturn pid %d", vkturnPid)
			_ = enc.Encode(reply{OK: true})
		case "wgup":
			if cmd.Config == nil {
				_ = enc.Encode(reply{Error: "wgup: missing config"})
				continue
			}
			teardown()
			t, err := wgwin.Up(toWgwin(cmd.Config))
			if err != nil {
				log.Printf("wgup failed: %v", err)
				_ = enc.Encode(reply{Error: err.Error()})
				continue
			}
			tun = t
			// vkturn's own underlay bypasses the full-tunnel default route by pinning its
			// sockets to the physical interface (IP_UNICAST_IF), so no WFP exclusion is
			// needed here. vkturnPid is kept for a future per-app split-tunnel via WFP.
			log.Printf("wgup ok: %s peer=%s endpoint=%s (vkturn pid %d)", cmd.Config.Interface, shortKey(cmd.Config.PeerPublicKey), cmd.Config.PeerEndpoint, vkturnPid)
			_ = enc.Encode(reply{OK: true})
		case "bypass":
			// vkturn reported an underlay destination (VK TURN / peer server IP); pin a /32
			// route to it out the physical gateway so it skips the full-tunnel routes.
			if tun != nil {
				tun.AddBypassRoute(cmd.IP)
			}
			_ = enc.Encode(reply{OK: true})
		case "activate":
			// Install the deferred full-tunnel catch-all now that the /32 bypass routes are in.
			if tun != nil {
				tun.Activate()
			}
			_ = enc.Encode(reply{OK: true})
		case "stats":
			// Live traffic counters from the wireguard-go device (no sysfs on Windows).
			var rx, tx int64
			if tun != nil {
				rx, tx = tun.Stats()
			}
			_ = enc.Encode(reply{OK: true, Rx: rx, Tx: tx})
		case "wgdown":
			teardown()
			_ = enc.Encode(reply{OK: true})
		case "apps":
			// TODO(windows-split-tunnel): per-app bypass/whitelist via WFP.
			_ = enc.Encode(reply{OK: true})
		case "appsdown":
			_ = enc.Encode(reply{OK: true})
		case "xrayup":
			if xray != nil {
				xray.stop()
				xray = nil
			}
			x, err := startXray(cmd.XrayBin, cmd.XrayConfig, cmd.TunName, cmd.DatDir, cmd.EnableIPv6)
			if err != nil {
				log.Printf("xrayup failed: %v", err)
				_ = enc.Encode(reply{Error: err.Error()})
				continue
			}
			xray = x
			log.Printf("xrayup ok: tun=%s pid=%d", cmd.TunName, x.cmd.Process.Pid)
			_ = enc.Encode(reply{OK: true})
		case "xraydown":
			if xray != nil {
				xray.stop()
				xray = nil
			}
			_ = enc.Encode(reply{OK: true})
		case "stop":
			return nil
		default:
			_ = enc.Encode(reply{Error: "unknown command"})
		}
	}
}

func toWgwin(c *wgConfig) wgwin.Config {
	return wgwin.Config{
		Interface:     c.Interface,
		PrivateKey:    c.PrivateKey,
		Addresses:     c.Addresses,
		MTU:           c.MTU,
		PeerPublicKey: c.PeerPublicKey,
		PresharedKey:  c.PresharedKey,
		PeerEndpoint:  c.PeerEndpoint,
		AllowedIPs:    c.AllowedIPs,
		DNS:           c.DNS,
		Amnezia:       c.Amnezia,
	}
}

// pipeArg returns the value of the --pipe flag the GUI passes when elevating the helper.
func pipeArg() string {
	for i, a := range os.Args {
		if a == "--pipe" && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	return ""
}

func shortKey(k string) string {
	if len(k) > 8 {
		return k[:8] + "..."
	}
	return k
}
