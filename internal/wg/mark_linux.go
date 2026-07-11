//go:build linux

package wg

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

var nftCandidates = []string{
	"nft",
	"/run/current-system/sw/bin/nft",
	"/usr/sbin/nft",
	"/usr/bin/nft",
	"/sbin/nft",
	"/bin/nft",
}

// VkturnCgroup is the dedicated cgroup v2 the vkturn process is moved into so all of
// its egress can be fwmark-tagged wholesale. Per-socket SO_MARK (the protect bridge)
// only covers sockets vkturn opens through its protect dialer and misses the ones
// pion/directNet create directly, which then leak into the full tunnel once WG is up.
// Marking the whole process by cgroup - the Linux analogue of the Android kernel-WG
// path's UID routing, but scoped to one process so unrelated same-user traffic is not
// diverted - closes that gap.
const VkturnCgroup = "wingsv-dex-vkturn"

// AppsCgroup holds the per-app split-tunnel processes; its egress is fwmark-tagged
// with the bypass mark (bypass mode) or the tunnel mark (whitelist mode).
const AppsCgroup = "wingsv-dex-apps"

// cgroupRoot is the unified cgroup v2 mount point.
const cgroupRoot = "/sys/fs/cgroup"

// VkturnNftTable / AppsNftTable are the dedicated inet tables we own; deleting the
// whole table is a clean, idempotent teardown. Separate tables so the vkturn and
// apps marks can be installed and torn down independently.
const (
	VkturnNftTable     = "wingsv_dex"
	AppsNftTable       = "wingsv_dex_apps"
	MasqNftTable       = "wingsv_dex_masq"
	BypassMasqNftTable = "wingsv_dex_bypass_masq"
)

// SetTunnelMasquerade installs (on) or removes (off) a source NAT that rewrites the
// source of whitelisted-app packets leaving the tunnel to the tunnel interface's own
// address. It is needed only in app-routing whitelist mode: there an app connects
// while still unmarked (the cgroup nft rule marks packets in the output hook, after
// the socket already picked a route), so the socket binds the physical-link source.
// The nft mark then reroutes the packet into the tunnel, but its source is still the
// physical IP, which the peer's WireGuard drops as outside allowed_ips - the tunnel
// sends and never gets a reply. Masquerading to the interface address fixes the source
// and conntrack rewrites the return path back to the app's socket.
func SetTunnelMasquerade(on bool, ifname string, mark int) error {
	_ = runNft("delete", "table", "inet", MasqNftTable)
	if !on {
		return nil
	}
	ensureModule("nft_masq")
	ruleset := fmt.Sprintf(`table inet %s {
	chain postrouting {
		type nat hook postrouting priority srcnat; policy accept;
		oifname "%s" meta mark 0x%x counter masquerade
	}
}`, MasqNftTable, ifname, mark)
	if err := runNftStdin(ruleset); err != nil {
		return fmt.Errorf("wg: install tunnel masquerade: %w", err)
	}
	return nil
}

// SetBypassMasquerade SNATs mark-tagged traffic to the outbound interface's address. In
// bypass mode the default route is the tunnel, so a bypass app's socket picks the tunnel IP
// (10.x) as its source; the mark then routes it out the physical link, where that source is
// a martian the upstream router drops. Masquerading rewrites it to the physical IP. No
// oifname scope is needed: only mark-tagged traffic leaves via the physical link (the
// "not fwmark -> tunnel" rule plus the physical main-table default), and vkturn's own
// underlay is already physical-sourced so masquerading it is a no-op. Not scoping to an
// interface avoids depending on physical-egress detection, which was unreliable on live
// mode switches. Whitelist mode uses SetTunnelMasquerade instead (masquerade into the tunnel).
func SetBypassMasquerade(on bool, mark int) error {
	_ = runNft("delete", "table", "inet", BypassMasqNftTable)
	if !on {
		return nil
	}
	ensureModule("nft_masq")
	ruleset := fmt.Sprintf(`table inet %s {
	chain postrouting {
		type nat hook postrouting priority srcnat; policy accept;
		meta mark 0x%x counter masquerade
	}
}`, BypassMasqNftTable, mark)
	if err := runNftStdin(ruleset); err != nil {
		return fmt.Errorf("wg: install bypass masquerade: %w", err)
	}
	return nil
}

// CgroupMark owns a cgroup and the nftables rule that fwmark-tags its egress.
// Everything here needs root, so it lives in the net-helper.
type CgroupMark struct {
	name  string
	path  string
	table string
}

// SetupCgroupMark creates the cgroup and installs the nftables rule that marks every
// packet from it with fwmark, in the given (own) inet table. The rule resolves the
// cgroup path to an id at insert time, so the directory is created first. Idempotent:
// a stale table from a previous run is replaced.
func SetupCgroupMark(name string, fwmark int, table string) (*CgroupMark, error) {
	if _, err := os.Stat(filepath.Join(cgroupRoot, "cgroup.controllers")); err != nil {
		return nil, fmt.Errorf("wg: cgroup v2 not mounted at %s: %w", cgroupRoot, err)
	}
	ensureModule("nft_socket")
	m := &CgroupMark{name: name, path: filepath.Join(cgroupRoot, name), table: table}
	if err := os.Mkdir(m.path, 0o755); err != nil && !os.IsExist(err) {
		return nil, fmt.Errorf("wg: create cgroup %s: %w", m.path, err)
	}
	// Replace any stale table from a prior run, then install fresh.
	_ = runNft("delete", "table", "inet", table)
	// The counter makes the match verifiable live (nft list table inet <table>).
	ruleset := fmt.Sprintf(`table inet %s {
	chain markout {
		type route hook output priority mangle; policy accept;
		socket cgroupv2 level 1 "%s" counter meta mark set 0x%x
	}
}`, table, name, fwmark)
	if err := runNftStdin(ruleset); err != nil {
		_ = os.Remove(m.path)
		return nil, fmt.Errorf("wg: install nft mark rule: %w", err)
	}
	return m, nil
}

// Add moves a process (vkturn) into the cgroup so its future sockets carry the cgroup
// tag and get marked. Call it after spawning vkturn but before it opens any underlay
// socket (i.e. before Configure), so no socket escapes the mark.
func (m *CgroupMark) Add(pid int) error {
	if m == nil {
		return nil
	}
	if err := os.WriteFile(filepath.Join(m.path, "cgroup.procs"), []byte(strconv.Itoa(pid)), 0o644); err != nil {
		return fmt.Errorf("wg: add pid %d to cgroup: %w", pid, err)
	}
	return nil
}

// Close removes the nft rule and the cgroup, moving any lingering pids back to the
// root cgroup first (rmdir fails on a non-empty cgroup).
func (m *CgroupMark) Close() error {
	if m == nil {
		return nil
	}
	_ = runNft("delete", "table", "inet", m.table)
	m.drainToRoot()
	_ = os.Remove(m.path)
	return nil
}

func (m *CgroupMark) drainToRoot() {
	b, err := os.ReadFile(filepath.Join(m.path, "cgroup.procs"))
	if err != nil {
		return
	}
	rootProcs := filepath.Join(cgroupRoot, "cgroup.procs")
	for _, pid := range strings.Fields(string(b)) {
		_ = os.WriteFile(rootProcs, []byte(pid), 0o644)
	}
}

// ensureModule loads a kernel module by name when it is not already present. Some kernels
// (seen on custom builds) do not autoload nft_socket (for the cgroup "socket cgroupv2"
// match) or wireguard (for the tunnel device), so the operation fails until it is
// modprobed. The net-helper runs as root, so no extra elevation is needed.
func ensureModule(name string) {
	if b, err := os.ReadFile("/proc/modules"); err == nil && strings.Contains(string(b), name+" ") {
		return
	}
	_ = exec.Command("modprobe", name).Run()
}

func runNft(args ...string) error {
	nft, err := commandPath(nftCandidates)
	if err != nil {
		return err
	}
	return exec.Command(nft, args...).Run()
}

func runNftStdin(ruleset string) error {
	nft, err := commandPath(nftCandidates)
	if err != nil {
		return err
	}
	cmd := exec.Command(nft, "-f", "-")
	cmd.Stdin = strings.NewReader(ruleset)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func commandPath(candidates []string) (string, error) {
	for _, candidate := range candidates {
		if strings.Contains(candidate, "/") {
			if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
				return candidate, nil
			}
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("wg: nft not found; install nftables system-wide or expose nft at /run/current-system/sw/bin/nft")
}
