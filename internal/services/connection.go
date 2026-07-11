package services

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/WINGS-N/wingsv-dex/internal/applog"
	"github.com/WINGS-N/wingsv-dex/internal/config"
	"github.com/WINGS-N/wingsv-dex/internal/dataplane"
	"github.com/WINGS-N/wingsv-dex/internal/gen/appcontrolpb"
	"github.com/WINGS-N/wingsv-dex/internal/vktp"
)

// ProtectSocket is the abstract unix socket name the privileged helper hosts and
// vkturn dials (via its Configure protect_sock) to get its underlay sockets marked.
// main passes it to the vktp Manager so both sides agree.
const ProtectSocket = "wingsv-dex-protect"

const (
	wgInterface = "wingsv0"
	fwMark      = 0x8888
	routeTable  = 0x8888
	// appsTunnelMark tags whitelisted apps so the data plane routes only them into
	// the tunnel (must match wg.AppsMark used by the whitelist routing rule).
	appsTunnelMark = 0x8889
)

// Connection status values, mirroring how the Android home screen reads.
const (
	StatusDisconnected = "disconnected"
	StatusConnecting   = "connecting"
	StatusConnected    = "connected"
	StatusStopping     = "stopping"
)

// ConnectionStateEvent is the event name the frontend subscribes to.
const ConnectionStateEvent = "connection:state"

// PatchStatusEventName carries live-patch per-field progress to the frontend.
const PatchStatusEventName = "connection:patch"

// PatchStatus is the frontend-facing live-patch progress for one settings field.
// State is one of applying | applied | failed | reverted_needs_restart.
type PatchStatus struct {
	RequestID string `json:"requestId"`
	Field     string `json:"field"`
	State     string `json:"state"`
	Message   string `json:"message"`
}

// NoticeEventName carries a transient toast-style message to the frontend.
const NoticeEventName = "connection:notice"

// Notice is a transient message shown as a top pill (e.g. a settings change that
// triggered a reconnect). Kind is info | warn | error.
type Notice struct {
	Message string `json:"message"`
	Kind    string `json:"kind"`
}

// ConnectionState is the frontend-facing connection snapshot. Streams and Threads
// drive the "Подключение (N/M)" / "Подключено (N/M)" pill text; Stage carries the
// connecting sub-stage ("captcha"/"auth"/"turn", empty otherwise) so the pill can
// show "Подключение (TURN auth)…" before streams start filling.
type ConnectionState struct {
	Status   string `json:"status"`
	Streams  int    `json:"streams"`
	Threads  int    `json:"threads"`
	Stage    string `json:"stage"`
	Endpoint string `json:"endpoint"`
	Title    string `json:"title"`
	Error    string `json:"error"`
}

// Connecting sub-stages, CONNECT_STAGE_* tokens. computeStage
// picks one by priority (captcha > auth > turn > none).
const (
	StageCaptcha = "captcha"
	StageAuth    = "auth"
	StageTurn    = "turn"
)

// ConnectionService connects/disconnects the active VK TURN profile via the vkturn
// child process and streams live telemetry to the frontend.
type ConnectionService struct {
	store    *config.Store
	logStore *applog.Store
	manager  *vktp.Manager
	exePath  string

	vkauth *VKAuthService

	mu             sync.Mutex
	app            *application.App
	state          ConnectionState
	telCancel      context.CancelFunc
	dp             *dataplane.Controller
	ipInfo         IPInfo
	vkAuthInFlight bool

	// The profile + client settings the running relay was configured with, so a
	// settings edit can be diffed into a live PatchConfig delta or a reconnect.
	appliedProfile config.Profile
	appliedClient  config.ClientSettings

	// Connecting sub-stage inputs, fed by the ProxyEvent stream (see startStreams).
	authReady      bool
	captchaActive  bool
	captchaLockout bool

	// warmup is closed once the first stream comes up, so Connect can hold WGUp
	// until the session is established (Android parity: provision + warmup, then WG).
	warmup     chan struct{}
	warmupDone bool
}

// NewConnectionService wires the service to the store, the vkturn manager, the VK
// account sign-in service, and the path of this executable (re-launched as the
// privileged net-helper). vkauth is taken as a constructor argument rather than an
// exported setter so Wails does not treat VKAuthService as a boundary model.
func NewConnectionService(store *config.Store, logStore *applog.Store, manager *vktp.Manager, vkauth *VKAuthService, exePath string) *ConnectionService {
	return &ConnectionService{
		store:    store,
		logStore: logStore,
		manager:  manager,
		vkauth:   vkauth,
		exePath:  exePath,
		state:    ConnectionState{Status: StatusDisconnected},
	}
}

// SetApp gives the service the running application so it can emit state events.
// Called from main once the app exists.
func (s *ConnectionService) SetApp(app *application.App) {
	s.mu.Lock()
	s.app = app
	s.mu.Unlock()
}

// State returns the current connection snapshot.
func (s *ConnectionService) State() ConnectionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Connect starts vkturn for the active profile and begins streaming telemetry.
func (s *ConnectionService) Connect() (ConnectionState, error) {
	profile, ok := s.activeProfile()
	if !ok {
		return s.State(), errors.New("no active profile")
	}
	// Threads (and the rest of the connecting/telemetry counters) come from the
	// device-global client settings, not the per-profile snapshot.
	client := s.store.Client()
	s.runtimeLogf("connect requested title=%q runtime=%s auth=%s managed=%v", profile.Title, client.RuntimeMode, client.VKAuthMode, profile.Managed)

	s.setState(ConnectionState{
		Status:   StatusConnecting,
		Threads:  client.Threads,
		Stage:    StageAuth,
		Endpoint: profile.VKTurnEndpoint,
		Title:    profile.Title,
	})

	// 1. Privileged helper first (pkexec prompt): it hosts the protect socket that
	// vkturn dials at launch to have its underlay sockets marked.
	dp := dataplane.NewController(s.exePath, ProtectSocket, fwMark, s.runtimeLogWriter())
	if err := dp.Start(); err != nil {
		s.runtimeLogf("dp.Start failure: %v", err)
		return s.fail(err)
	}
	s.runtimeLogf("dp.Start succeeded")

	// 2. vkturn: move it into the marking cgroup the instant it is spawned (before
	// Configure opens any underlay socket), so every socket it creates is
	// fwmark-tagged and bypasses the tunnel. Its Configure also carries the protect
	// socket name as a belt-and-suspenders per-socket mark.
	s.runtimeLogf("manager.Start begin")
	if err := s.manager.Start(profile, client, dp.CgroupAdd); err != nil {
		s.runtimeLogf("manager.Start failure: %v", err)
		dp.Stop()
		return s.fail(err)
	}
	s.runtimeLogf("manager.Start succeeded")

	// Subscribe to the event stream now, right after Configure, so the captcha/auth
	// phases reach the pill before Provision and the tunnel come up.
	ctx := s.beginStreams(client.Threads)

	// Account mode: the relay mints its TURN token (and, for managed profiles, runs
	// Provision) from the VK web session, so it must have one BEFORE we call
	// resolveWG/Provision - otherwise Provision's RPC deadline expires while the user
	// is still signing in. Deliver a cached session or open the sign-in window and
	// block here until it is captured, then require that a session exists.
	if client.VKAuthMode == "account" && s.vkauth != nil {
		s.vkauth.ensureSession("")
		if !s.vkauth.Status().LoggedIn {
			s.runtimeLogf("missing VK login for account auth mode")
			s.stopStreams()
			s.manager.Stop()
			dp.Stop()
			return s.fail(errors.New("для account-режима нужен вход в VK"))
		}
	}

	// 3. Resolve the WireGuard config (provisioned for managed profiles) and bring
	// the tunnel up through the helper.
	wgCfg, err := s.resolveWG(profile, client)
	if err != nil {
		s.runtimeLogf("resolveWG failure: %v", err)
		s.stopStreams()
		s.manager.Stop()
		dp.Stop()
		return s.fail(err)
	}
	// Per-app split tunnel: whitelist mode inverts the tunnel routing (only the
	// selected apps' marked traffic enters the tunnel), so the flag must be set
	// before WGUp installs the routing rules.
	appRouting := s.store.AppRouting()
	wgCfg.Whitelist = appRouting.Mode == "whitelist"
	// VPN mode brings the kernel WireGuard interface + full-tunnel routing up. In
	// proxy-only mode vkturn still runs and serves its local endpoint, but no tunnel
	// is created and system traffic is left untouched (bring-your-own WireGuard, and
	// it doubles as a way to see the session without WG in the path).
	if client.RuntimeMode != "proxy" {
		if wgCfg.Amnezia {
			if avail := s.AWGAvailability(); !avail.Available {
				s.stopStreams()
				s.manager.Stop()
				dp.Stop()
				return s.fail(fmt.Errorf("AmneziaWG недоступен на этой машине, установите: %s", strings.Join(avail.Packages, ", ")))
			}
		}
		// Wait for the first stream to come up before the tunnel, so
		// WG data is not forwarded into a session that is still negotiating.
		s.runtimeLogf("waitWarmup begin")
		s.waitWarmup(15 * time.Second)
		s.runtimeLogf("waitWarmup done")
		if err := dp.WGUp(wgCfg); err != nil {
			s.runtimeLogf("WGUp failure: %v", err)
			s.stopStreams()
			s.manager.Stop()
			dp.Stop()
			return s.fail(err)
		}
		// Windows: the relay streams the underlay IPs it pins to the physical NIC; install a
		// /32 physical-gateway bypass route for each so vkturn's own traffic keeps a valid
		// source and stays off the full tunnel. No-op on Linux (fwmark bypass reports none).
		s.startUnderlayBypass(ctx, dp)
		// Move the selected apps into the marking cgroup: bypass apps carry the same
		// bypass mark as vkturn (direct); whitelist apps carry the tunnel mark that
		// tunnelRule routes into the tunnel. Best-effort: a failure here does not fail
		// the connect.
		if list := appRouting.ActiveList(); len(list) > 0 {
			mark := fwMark
			if appRouting.Mode == "whitelist" {
				mark = appsTunnelMark
			}
			_ = dp.AppsUp(list, mark, wgCfg.Whitelist) // best-effort: must not fail the connect
		}
	}

	s.mu.Lock()
	s.dp = dp
	s.appliedProfile = profile
	s.appliedClient = client
	s.mu.Unlock()

	s.startTelemetryAndStats(ctx, client.Threads)
	if client.VKAuthMode == "account" && s.vkauth != nil {
		s.startCookieRotationPoll(ctx)
	}
	s.runtimeLogf("connected state ready title=%q", profile.Title)
	return s.State(), nil
}

// startUnderlayBypass runs the Windows two-phase tunnel activation: WGUp brought the wintun
// adapter up WITHOUT the full-tunnel catch-all, so here we first install a /32 physical
// bypass route for every underlay IP the relay pins (VK TURN / peer servers), then, once
// that initial batch settles, activate the catch-all. Installing the bypass first keeps
// vkturn's already-established underlay sockets from being re-routed mid-flight (which
// changes their NAT mapping and drops the TURN allocations). Later IPs add their /32 live.
// On Linux WGUp already installed the tunnel routes, so this is a no-op.
func (s *ConnectionService) startUnderlayBypass(ctx context.Context, dp *dataplane.Controller) {
	if runtime.GOOS != "windows" {
		return
	}
	ips := make(chan string, 64)
	go func() {
		_ = s.manager.StreamUnderlayIPs(ctx, func(ip string) {
			select {
			case ips <- ip:
			case <-ctx.Done():
			}
		})
		close(ips)
	}()
	go func() {
		activated := false
		activate := func() {
			if activated {
				return
			}
			activated = true
			if err := dp.Activate(); err != nil {
				s.runtimeLogf("[bypass] activate full tunnel: %v", err)
			} else {
				s.runtimeLogf("[bypass] full tunnel activated")
			}
		}
		// Activate once the underlay-IP burst goes quiet (or after a cap if none arrive), so
		// the catch-all lands after the bypass routes.
		settle := time.NewTimer(3 * time.Second)
		defer settle.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case ip, ok := <-ips:
				if !ok {
					activate()
					return
				}
				if err := dp.Bypass(ip); err != nil {
					s.runtimeLogf("[bypass] route installed failed len=%d: %v", len(ip), err)
				} else {
					s.runtimeLogf("[bypass] route installed len=%d", len(ip))
				}
				if !activated {
					settle.Reset(1500 * time.Millisecond)
				}
			case <-settle.C:
				activate()
			}
		}
	}()
}

func (s *ConnectionService) fail(err error) (ConnectionState, error) {
	s.setState(ConnectionState{Status: StatusDisconnected, Error: err.Error()})
	return s.State(), err
}

func (s *ConnectionService) runtimeLogWriter() *applog.LineWriter {
	if s.logStore == nil {
		return nil
	}
	return applog.NewLineWriter(s.logStore, applog.ChannelRuntime, nil)
}

func (s *ConnectionService) runtimeLogf(format string, args ...any) {
	s.appendLogf(applog.ChannelRuntime, format, args...)
}

func (s *ConnectionService) proxyLogf(format string, args ...any) {
	s.appendLogf(applog.ChannelProxy, format, args...)
}

func (s *ConnectionService) appendLogf(channel string, format string, args ...any) {
	msg := applog.Redact(fmt.Sprintf(format, args...))
	log.Printf("%s", msg)
	if s.logStore != nil {
		_ = s.logStore.Append(channel, msg)
		s.emit2(LogsUpdatedEvent, LogUpdate{Channel: channel})
	}
}

// resolveWG builds the WireGuard data-plane config: provisioned on connect for
// managed profiles (the node mints it), otherwise straight from the profile. The
// peer endpoint is always vkturn's local listen, so WG traffic flows into vkturn.
func (s *ConnectionService) resolveWG(p config.Profile, cs config.ClientSettings) (dataplane.WGConfig, error) {
	s.runtimeLogf("[resolveWG] managed=%v privkey_len=%d peerpub_len=%d addr_len=%d", p.Managed, len(p.WG.PrivateKey), len(p.WG.PublicKey), len(p.WG.Addresses))
	if p.Managed {
		token, err := base64.StdEncoding.DecodeString(p.ProvisionToken)
		if err != nil {
			return dataplane.WGConfig{}, fmt.Errorf("provision token: %w", err)
		}
		wg, err := s.manager.Provision(p.ProvisionClientID, token, machineHWID(), uint32(endpointPort(cs.LocalEndpoint)))
		if err != nil {
			return dataplane.WGConfig{}, err
		}
		s.runtimeLogf("[provision] privkey_len=%d peerpub_len=%d addr_len=%d allowed_len=%d", len(wg.GetPrivateKey()), len(wg.GetServerPublicKey()), len(wg.GetAddress()), len(wg.GetAllowedIps()))
		cfg := dataplane.WGConfig{
			Interface:     wgInterface,
			PrivateKey:    wg.GetPrivateKey(),
			PeerPublicKey: wg.GetServerPublicKey(),
			Addresses:     splitCSV(wg.GetAddress()),
			AllowedIPs:    splitCSV(wg.GetAllowedIps()),
			MTU:           int(wg.GetMtu()),
			PeerEndpoint:  cs.LocalEndpoint,
			FwMark:        fwMark,
			Table:         routeTable,
		}
		s.applyAWG(&cfg, p)
		return cfg, nil
	}
	cfg := dataplane.WGConfig{
		Interface:     wgInterface,
		PrivateKey:    p.WG.PrivateKey,
		PeerPublicKey: p.WG.PublicKey,
		PresharedKey:  p.WG.PresharedKey,
		Addresses:     splitCSV(p.WG.Addresses),
		AllowedIPs:    splitCSV(p.WG.AllowedIPs),
		MTU:           p.WG.MTU,
		PeerEndpoint:  cs.LocalEndpoint,
		FwMark:        fwMark,
		Table:         routeTable,
	}
	s.applyAWG(&cfg, p)
	return cfg, nil
}

// AWGAvailability reports whether the AmneziaWG data plane can run on this machine,
// and if not, which packages the user needs to install. The bring-up needs the `awg`
// tool (amneziawg-tools) and the amneziawg kernel module (amneziawg-dkms).
type AWGAvailability struct {
	Available bool     `json:"available"`
	Packages  []string `json:"packages"`
}

// AWGAvailability checks for the amneziawg tool and kernel module.
func (s *ConnectionService) AWGAvailability() AWGAvailability {
	var missing []string
	if _, err := exec.LookPath("awg"); err != nil {
		missing = append(missing, "amneziawg-tools")
	}
	if !amneziaModuleAvailable() {
		missing = append(missing, "amneziawg-dkms")
	}
	return AWGAvailability{Available: len(missing) == 0, Packages: missing}
}

func amneziaModuleAvailable() bool {
	if _, err := os.Stat("/sys/module/amneziawg"); err == nil {
		return true // already loaded
	}
	return exec.Command("modinfo", "amneziawg").Run() == nil // installed (dkms built)
}

// applyAWG switches the data-plane config to AmneziaWG when the profile's sub-backend
// is AWG, carrying the junk params parsed from the profile (empty for managed configs
// whose node did not mint them).
func (s *ConnectionService) applyAWG(cfg *dataplane.WGConfig, p config.Profile) {
	if p.TransportKind != "awg" {
		return
	}
	cfg.Amnezia = true
	cfg.Jc, cfg.Jmin, cfg.Jmax = p.WG.Jc, p.WG.Jmin, p.WG.Jmax
	cfg.S1, cfg.S2, cfg.S3, cfg.S4 = p.WG.S1, p.WG.S2, p.WG.S3, p.WG.S4
	cfg.H1, cfg.H2, cfg.H3, cfg.H4 = p.WG.H1, p.WG.H2, p.WG.H3, p.WG.H4
}

func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func endpointPort(endpoint string) int {
	_, port, found := strings.Cut(endpoint, ":")
	if !found {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(port))
	return n
}

// machineHWID returns a stable per-machine id for provisioning (a stable per-machine id like
// ANDROID_ID), from /etc/machine-id with the hostname as a fallback.
func machineHWID() string {
	if b, err := os.ReadFile("/etc/machine-id"); err == nil {
		if id := strings.TrimSpace(string(b)); id != "" {
			return id
		}
	}
	host, _ := os.Hostname()
	return strings.TrimSpace(host)
}

// ApplyAppRouting re-applies the current per-app split-tunnel settings to the live
// data plane (routing-mode swap + cgroup/matcher rebuild) without restarting vkturn
// or the tunnel. No-op when not connected; the settings then apply on next connect.
func (s *ConnectionService) ApplyAppRouting() error {
	s.mu.Lock()
	dp := s.dp
	s.mu.Unlock()
	if dp == nil {
		return nil
	}
	a := s.store.AppRouting()
	whitelist := a.Mode == "whitelist"
	mark := fwMark
	if whitelist {
		mark = appsTunnelMark
	}
	return dp.AppsUp(a.ActiveList(), mark, whitelist)
}

// OnSettingsChanged is called after a VK TURN settings edit is saved. When connected
// it diffs the change against the applied config and either live-patches the running
// relay (PatchConfig, no restart) or, for WG-transport / non-live-patchable changes,
// triggers a full reconnect. A no-op when not connected (the change applies on the
// next connect).
func (s *ConnectionService) OnSettingsChanged() {
	s.mu.Lock()
	connected := s.dp != nil && (s.state.Status == StatusConnected || s.state.Status == StatusConnecting)
	oldP, oldC := s.appliedProfile, s.appliedClient
	s.mu.Unlock()
	if !connected {
		return
	}
	newP, ok := s.activeProfile()
	if !ok {
		return
	}
	newC := s.store.Client()

	if needsReconnect(oldP, oldC, newP, newC) {
		s.emit2(NoticeEventName, Notice{Message: "Переподключение для применения настроек", Kind: "info"})
		s.runtimeLogf("settings changed: reconnect required")
		go s.reconnect()
		return
	}
	delta := buildPatchDelta(oldP, oldC, newP, newC)
	s.mu.Lock()
	s.appliedProfile = newP
	s.appliedClient = newC
	s.mu.Unlock()
	if delta == nil {
		return // nothing the relay cares about changed
	}
	if err := s.manager.PatchConfig(delta); err != nil {
		s.runtimeLogf("settings live patch failure: %v", err)
		go s.reconnect()
		return
	}
}

// reconnect tears the tunnel down and brings it back up, for settings changes the
// relay cannot live-patch (WG transport, session mode, provisioned endpoint, ...).
func (s *ConnectionService) reconnect() {
	s.runtimeLogf("reconnect start")
	s.Disconnect()
	time.Sleep(400 * time.Millisecond)
	_, _ = s.Connect()
}

// Disconnect stops the vkturn process, tears the tunnel down, and clears the streams.
func (s *ConnectionService) Disconnect() ConnectionState {
	s.runtimeLogf("disconnect start")
	s.mu.Lock()
	if s.telCancel != nil {
		s.telCancel()
		s.telCancel = nil
	}
	dp := s.dp
	s.dp = nil
	s.mu.Unlock()

	s.setState(ConnectionState{Status: StatusStopping})
	s.manager.Stop()
	if dp != nil {
		dp.Stop()
	}
	s.mu.Lock()
	s.ipInfo = IPInfo{}
	s.mu.Unlock()
	s.emit2(TrafficStatsEvent, TrafficStats{})
	s.emit2(IPInfoEvent, IPInfo{})
	s.setState(ConnectionState{Status: StatusDisconnected})
	s.runtimeLogf("disconnect complete")
	// The tunnel is down and routing is back on the physical link (dp.Stop tore it
	// down synchronously): re-look up the now-physical exit IP so the card shows the
	// real address instead of staying blank on the old tunnel IP.
	go func() { _, _ = s.RefreshIPInfo() }()
	return s.State()
}

func (s *ConnectionService) activeProfile() (config.Profile, bool) {
	activeID := s.store.ActiveID()
	if activeID == "" {
		return config.Profile{}, false
	}
	for _, p := range s.store.List() {
		if p.ID == activeID {
			return p, true
		}
	}
	return config.Profile{}, false
}

// beginStreams subscribes to StreamEvents immediately after Configure - before
// Provision and WGUp - so the early connecting phases (captcha, auth) reach the pill
// instead of being missed while the tunnel is still coming up. It resets the sub-stage
// inputs and returns the cancel context the later telemetry/stats streams share.
func (s *ConnectionService) beginStreams(threads int) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.telCancel = cancel
	s.authReady = false
	s.captchaActive = false
	s.captchaLockout = false
	s.warmup = make(chan struct{})
	s.warmupDone = false
	s.mu.Unlock()

	go func() {
		if err := s.manager.Events(ctx, func(ev *appcontrolpb.ProxyEvent) {
			s.logProxyEvent(ev)
			// VK account-mode session requests are handled off the pill path: the
			// relay asks for the web session (empty required event) or drives the
			// sign-in WebView with a join link.
			if ev.GetVkCookiesRequired() != nil {
				s.requestVKAuth("")
			} else if a := ev.GetVkAccountAuth(); a != nil && a.GetPhase() == "required" {
				s.requestVKAuth(a.GetLink())
			}
			// Live-patch per-field progress rides the same event stream.
			if ps := ev.GetPatchStatus(); ps != nil {
				s.emit2(PatchStatusEventName, PatchStatus{
					RequestID: ps.GetRequestId(),
					Field:     ps.GetField(),
					State:     ps.GetState(),
					Message:   ps.GetMessage(),
				})
			}
			s.mu.Lock()
			if s.state.Status != StatusConnecting && s.state.Status != StatusConnected {
				s.mu.Unlock()
				return
			}
			prev := s.state.Status
			if !s.applyEventLocked(ev) {
				s.mu.Unlock()
				return
			}
			justConnected := prev != StatusConnected && s.state.Status == StatusConnected
			s.state.Threads = threads
			s.state.Stage = s.computeStageLocked()
			snapshot := s.state
			s.mu.Unlock()
			if justConnected {
				// Refresh the exit IP the instant we flip to connected; the timed
				// retries in refreshIPInfoAsync then settle it on the tunnel IP.
				go func() { _, _ = s.RefreshIPInfo() }()
			}
			s.emit(snapshot)
		}); err != nil {
			s.proxyLogf("appcontrol events stream error: %v", err)
		}
	}()
	return ctx
}

// startTelemetryAndStats starts the N/M counter, traffic stats and exit-IP lookup once
// the WireGuard interface exists (after WGUp), sharing beginStreams' cancel context.
func (s *ConnectionService) startTelemetryAndStats(ctx context.Context, threads int) {
	s.startStatsPoller(ctx)
	s.refreshIPInfoAsync(ctx)

	go func() {
		if err := s.manager.Telemetry(ctx, func(count int64) {
			s.mu.Lock()
			if s.state.Status == StatusStopping || s.state.Status == StatusDisconnected {
				s.mu.Unlock()
				return
			}
			s.state.Streams = int(count)
			s.state.Threads = threads
			s.state.Stage = s.computeStageLocked()
			snapshot := s.state
			s.mu.Unlock()
			s.emit(snapshot)
		}); err != nil {
			s.proxyLogf("appcontrol telemetry stream error: %v", err)
		}
	}()
}

func (s *ConnectionService) logProxyEvent(ev *appcontrolpb.ProxyEvent) {
	switch {
	case ev.GetStatus() != nil:
		st := ev.GetStatus()
		s.proxyLogf("appcontrol status phase=%s connected_streams=%d", st.GetPhase(), st.GetConnectedStreams())
	case ev.GetCaptcha() != nil:
		s.proxyLogf("appcontrol captcha state=%s", ev.GetCaptcha().GetState())
	case ev.GetLockout() != nil:
		s.proxyLogf("appcontrol lockout seconds=%d", ev.GetLockout().GetSeconds())
	case ev.GetVkCookiesRequired() != nil:
		s.proxyLogf("appcontrol vk_cookies_required")
	case ev.GetVkAccountAuth() != nil:
		a := ev.GetVkAccountAuth()
		s.proxyLogf("appcontrol vk_account_auth phase=%s has_link=%v", a.GetPhase(), strings.TrimSpace(a.GetLink()) != "")
	case ev.GetPatchStatus() != nil:
		ps := ev.GetPatchStatus()
		s.proxyLogf("appcontrol patch field=%s state=%s message=%q", ps.GetField(), ps.GetState(), ps.GetMessage())
	default:
		s.proxyLogf("appcontrol event unhandled")
	}
}

// requestVKAuth answers a relay session request: re-deliver the cached VK session
// or open the sign-in window at link. Guarded so overlapping events (the relay may
// repeat the request) never stack a second login window.
func (s *ConnectionService) requestVKAuth(link string) {
	if s.vkauth == nil {
		return
	}
	// The relay re-emits vk_cookies_required after it REJECTS a delivered session
	// (VK's web-token mint said no). Re-delivering the same cached cookies would just
	// livelock, so only open a fresh sign-in when there is no session at all; a
	// rejected cached session is left to fail the connect instead of spamming.
	if s.vkauth.Status().LoggedIn {
		return
	}
	s.mu.Lock()
	if s.vkAuthInFlight {
		s.mu.Unlock()
		return
	}
	s.vkAuthInFlight = true
	s.mu.Unlock()
	go func() {
		s.vkauth.ensureSession(link)
		s.mu.Lock()
		s.vkAuthInFlight = false
		s.mu.Unlock()
	}()
}

// startCookieRotationPoll pulls the relay's current VK session periodically so a
// relay-side cookie rotation is persisted (and re-seeds the login window next time).
func (s *ConnectionService) startCookieRotationPoll(ctx context.Context) {
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				cookies, ua, err := s.manager.GetVKCookies()
				if err == nil {
					s.vkauth.persistRotation(cookies, ua)
				}
			}
		}
	}()
}

// stopStreams cancels the event/telemetry/stats streams (used on a failed connect).
func (s *ConnectionService) stopStreams() {
	s.mu.Lock()
	if s.telCancel != nil {
		s.telCancel()
		s.telCancel = nil
	}
	s.mu.Unlock()
}

// signalWarmupLocked closes the warmup channel the first time a stream comes up.
// Callers hold s.mu.
func (s *ConnectionService) signalWarmupLocked() {
	if !s.warmupDone && s.warmup != nil {
		s.warmupDone = true
		close(s.warmup)
	}
}

// waitWarmup blocks until the first stream comes up or the deadline passes, so WGUp
// runs after the session is established (Android's provision -> warmup -> tunnel
// order) instead of forwarding WG data into a still-negotiating session.
func (s *ConnectionService) waitWarmup(timeout time.Duration) {
	s.mu.Lock()
	warmup := s.warmup
	s.mu.Unlock()
	if warmup == nil {
		return
	}
	select {
	case <-warmup:
	case <-time.After(timeout):
	}
}

// applyEventLocked folds one ProxyEvent into the connecting sub-stage inputs and the
// connected transition, dispatchProxyEvent handlers. It returns
// false when the event carries nothing the pill cares about (skip the emit). Callers
// hold s.mu.
func (s *ConnectionService) applyEventLocked(ev *appcontrolpb.ProxyEvent) bool {
	switch {
	case ev.GetStatus() != nil:
		st := ev.GetStatus()
		switch phase := st.GetPhase(); {
		case phase == "auth_ready":
			// Auth done clears any captcha stage
			// (the auto-solver emits no completion event, it just proceeds to auth).
			s.authReady = true
			s.captchaActive = false
			s.captchaLockout = false
		case phase == "dtls_ready" || phase == "ok":
			// dtls_ready/ok is fully up: auth done, captcha cleared.
			s.authReady = true
			s.captchaLockout = false
			if s.state.Status == StatusConnecting {
				s.state.Status = StatusConnected
			}
			s.signalWarmupLocked()
		case phase == "dtls_alive":
			if s.state.Status == StatusConnecting {
				s.state.Status = StatusConnected
			}
			s.signalWarmupLocked()
		case strings.HasPrefix(phase, "captcha_lockout"):
			s.captchaLockout = true
		case phase == "turn_ready":
			// no dedicated stage; falls through to the counter once streams fill
		default:
			return false
		}
		if cs := int(st.GetConnectedStreams()); cs > 0 {
			s.state.Streams = cs
		}
		return true
	case ev.GetCaptcha() != nil:
		switch strings.ToLower(strings.TrimSpace(ev.GetCaptcha().GetState())) {
		case "solving", "required", "pending":
			s.captchaActive = true
		case "solved":
			s.captchaActive = false
			s.captchaLockout = false
		case "cancelled", "expired":
			s.captchaActive = false
		default:
			return false
		}
		return true
	case ev.GetLockout() != nil:
		s.captchaLockout = ev.GetLockout().GetSeconds() > 0
		return true
	default:
		return false
	}
}

// computeStageLocked derives the connecting sub-stage the same way the
// computeConnectingStage does: only while connecting, priority captcha > auth > turn,
// then empty once streams are filling (the counter takes over). Callers hold s.mu.
func (s *ConnectionService) computeStageLocked() string {
	if s.state.Status != StatusConnecting {
		return ""
	}
	if s.captchaActive || s.captchaLockout {
		return StageCaptcha
	}
	if !s.authReady {
		return StageAuth
	}
	if s.state.Streams <= 0 {
		return StageTurn
	}
	return ""
}

func (s *ConnectionService) setState(state ConnectionState) {
	s.mu.Lock()
	s.state = state
	snapshot := s.state
	s.mu.Unlock()
	s.emit(snapshot)
}

func (s *ConnectionService) emit(state ConnectionState) {
	s.emit2(ConnectionStateEvent, state)
}

func (s *ConnectionService) emit2(name string, data any) {
	s.mu.Lock()
	app := s.app
	s.mu.Unlock()
	if app != nil {
		app.Event.Emit(name, data)
	}
}
