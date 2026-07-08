//go:build linux

// Package nethelper is the privileged (CAP_NET_ADMIN) side of the VK TURN data
// plane. It runs as a separate process launched via pkexec and driven over stdin by
// the main app: it hosts the vkturn protect socket and brings the kernel WireGuard
// interface up and down. Keeping it separate confines root to a small, auditable
// binary path instead of the whole GUI.
package nethelper

import (
	"encoding/json"
	"io"
	"log"
	"os"

	"github.com/WINGS-N/wingsv-dex/internal/wg"
)

// command is one control line the main app writes to the helper's stdin.
type command struct {
	Cmd           string     `json:"cmd"` // start | cgadd | wgup | wgdown | apps | appsdown | stop
	ProtectSocket string     `json:"protectSocket"`
	FwMark        int        `json:"fwmark"`
	Pid           int        `json:"pid"`
	Config        *wg.Config `json:"config"`
	Apps          []string   `json:"apps"`      // split-tunnel app exec ids
	AppMark       int        `json:"appMark"`   // fwmark for the apps cgroup (bypass or tunnel)
	Whitelist     bool       `json:"whitelist"` // whitelist mode: swap the tunnel routing rule live
	SelfPid       int        `json:"selfPid"`   // the app's own pid: tunneled too in whitelist mode
}

type reply struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// Run reads control commands from stdin and applies them, tearing everything down
// on stop or when stdin closes (i.e. the main app exited). This is the entry point
// for the `--net-helper` mode of the binary.
func Run() error {
	log.SetPrefix("net-helper: ")
	log.SetFlags(log.Ltime)
	log.Printf("started (uid=%d)", os.Getuid())
	dec := json.NewDecoder(os.Stdin)
	enc := json.NewEncoder(os.Stdout)

	var protect *wg.ProtectServer
	var cmark *wg.CgroupMark
	var active *wg.Config
	var appsCg *wg.CgroupMark
	var matcher *wg.AppMatcher
	appsDown := func() {
		if matcher != nil {
			matcher.Stop()
			matcher = nil
		}
		if appsCg != nil {
			_ = appsCg.Close()
			appsCg = nil
		}
	}
	teardown := func() {
		appsDown()
		if active != nil {
			_ = wg.Down(*active)
			active = nil
		}
		if cmark != nil {
			_ = cmark.Close()
			cmark = nil
		}
		if protect != nil {
			_ = protect.Close()
			protect = nil
		}
	}
	defer teardown()

	for {
		var cmd command
		if err := dec.Decode(&cmd); err != nil {
			if err == io.EOF {
				return nil
			}
			return nil
		}
		switch cmd.Cmd {
		case "start":
			p, err := wg.ListenProtect(cmd.ProtectSocket, cmd.FwMark)
			if err != nil {
				log.Printf("protect listen @%s failed: %v", cmd.ProtectSocket, err)
				_ = enc.Encode(reply{Error: err.Error()})
				continue
			}
			cm, err := wg.SetupCgroupMark(wg.VkturnCgroup, cmd.FwMark, wg.VkturnNftTable)
			if err != nil {
				log.Printf("cgroup mark setup failed: %v", err)
				_ = p.Close()
				_ = enc.Encode(reply{Error: err.Error()})
				continue
			}
			protect, cmark = p, cm
			log.Printf("protect @%s + cgroup %s nft-mark up (fwmark 0x%x)", cmd.ProtectSocket, wg.VkturnCgroup, cmd.FwMark)
			_ = enc.Encode(reply{OK: true})
		case "cgadd":
			if cmark == nil {
				_ = enc.Encode(reply{Error: "cgadd: cgroup not set up"})
				continue
			}
			if err := cmark.Add(cmd.Pid); err != nil {
				log.Printf("cgadd pid %d failed: %v", cmd.Pid, err)
				_ = enc.Encode(reply{Error: err.Error()})
				continue
			}
			log.Printf("cgadd: vkturn pid %d marked via cgroup", cmd.Pid)
			_ = enc.Encode(reply{OK: true})
		case "wgup":
			if cmd.Config == nil {
				_ = enc.Encode(reply{Error: "wgup: missing config"})
				continue
			}
			if err := wg.Up(*cmd.Config); err != nil {
				log.Printf("wgup %s failed: %v", cmd.Config.Interface, err)
				_ = enc.Encode(reply{Error: err.Error()})
				continue
			}
			active = cmd.Config
			log.Printf("wgup ok: %s peer=%s endpoint=%s addrs=%v allowed=%v mtu=%d fwmark=0x%x table=%d",
				cmd.Config.Interface, shortKey(cmd.Config.PeerPublicKey), cmd.Config.PeerEndpoint,
				cmd.Config.Addresses, cmd.Config.AllowedIPs, cmd.Config.MTU, cmd.Config.FwMark, cmd.Config.Table)
			_ = enc.Encode(reply{OK: true})
		case "wgdown":
			if active != nil {
				_ = wg.Down(*active)
				active = nil
			}
			_ = enc.Encode(reply{OK: true})
		case "apps":
			// Live-apply: if the whitelist mode changed, swap the tunnel routing rule
			// on the running interface (no wg/vkturn restart), then rebuild the apps
			// cgroup + matcher from scratch.
			if active != nil && active.Whitelist != cmd.Whitelist {
				if err := wg.SwapTunnelRule(*active, active.Whitelist, cmd.Whitelist); err != nil {
					log.Printf("apps reroute (whitelist=%v) failed: %v", cmd.Whitelist, err)
					_ = enc.Encode(reply{Error: err.Error()})
					continue
				}
				active.Whitelist = cmd.Whitelist
				log.Printf("apps reroute: whitelist=%v", cmd.Whitelist)
			}
			appsDown()
			if len(cmd.Apps) == 0 {
				_ = enc.Encode(reply{OK: true})
				continue
			}
			ac, err := wg.SetupCgroupMark(wg.AppsCgroup, cmd.AppMark, wg.AppsNftTable)
			if err != nil {
				log.Printf("apps cgroup setup failed: %v", err)
				_ = enc.Encode(reply{Error: err.Error()})
				continue
			}
			appsCg = ac
			matcher = wg.StartAppMatcher(ac, cmd.Apps)
			// Whitelist: also tunnel the app itself so its exit-IP lookup reflects the
			// tunnel (in whitelist the unmarked default is the physical link).
			if cmd.Whitelist && cmd.SelfPid > 0 {
				if err := ac.Add(cmd.SelfPid); err != nil {
					log.Printf("apps: add self pid %d to tunnel cgroup failed: %v", cmd.SelfPid, err)
				}
			}
			log.Printf("apps split-tunnel up: %d app(s) marked 0x%x via cgroup %s (whitelist=%v)", len(cmd.Apps), cmd.AppMark, wg.AppsCgroup, cmd.Whitelist)
			_ = enc.Encode(reply{OK: true})
		case "appsdown":
			appsDown()
			_ = enc.Encode(reply{OK: true})
		case "stop":
			return nil
		default:
			_ = enc.Encode(reply{Error: "unknown command"})
		}
	}
}

func shortKey(k string) string {
	if len(k) > 8 {
		return k[:8] + "..."
	}
	return k
}
