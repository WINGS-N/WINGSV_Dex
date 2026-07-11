// Package vktp manages the vkturn child process (the vk-turn-proxy client) and
// drives it over the local AppControl gRPC IPC on a unix socket, // how libvkturn is launched and configured.
package vktp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/WINGS-N/wingsv-dex/internal/config"
	"github.com/WINGS-N/wingsv-dex/internal/gen/appcontrolpb"
)

type flushWriter interface {
	Flush() error
}

// Manager owns a single vkturn process and its AppControl client.
type Manager struct {
	binaryPath    string
	socketPath    string
	protectSocket string
	logw          io.Writer

	mu     sync.Mutex
	cmd    *exec.Cmd
	cancel context.CancelFunc
	conn   *grpc.ClientConn
	client appcontrolpb.AppControlClient
}

// NewManager builds a Manager. binaryPath points at the vkturn binary; socketPath is
// the AppControl unix socket (kept in the app's private config dir); protectSocket is
// the abstract name of the privileged protect socket vkturn dials to have its
// underlay sockets marked (empty disables it). logw receives vkturn's stdout/stderr;
// pass nil to discard.
func NewManager(binaryPath, socketPath, protectSocket string, logw io.Writer) *Manager {
	if logw == nil {
		logw = io.Discard
	}
	return &Manager{binaryPath: binaryPath, socketPath: socketPath, protectSocket: protectSocket, logw: logw}
}

// Start launches vkturn, connects the AppControl IPC, and applies the profile via
// Configure. Any previously running instance is stopped first. onSpawn, if non-nil,
// runs right after the process is spawned and before Configure - vkturn opens no
// underlay socket until Configure, so this is the moment to move it into the marking
// cgroup so every socket it later creates is fwmark-tagged.
func (m *Manager) Start(profile config.Profile, client config.ClientSettings, onSpawn func(pid int) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()

	token, err := randomToken()
	if err != nil {
		return err
	}
	if !isTCPSocket(m.socketPath) {
		_ = os.Remove(m.socketPath)
	}

	traceLog(m.logw, "vktp: starting vkturn binary=%s socket=%s", m.binaryPath, m.socketPath)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, m.binaryPath,
		"-app-grpc-socket", m.socketPath,
		"-app-grpc-token", token,
	)
	// stdout/stderr go to the log writer only; status/telemetry come over the
	// AppControl StreamEvents gRPC, not from scraping these lines.
	cmd.Stdout = m.logw
	cmd.Stderr = m.logw
	// Die with the parent so a crashed app never orphans vkturn holding 127.0.0.1:9000.
	setPdeathsig(cmd)
	if err := cmd.Start(); err != nil {
		cancel()
		traceLog(m.logw, "vktp: start vkturn failed: %v", err)
		return fmt.Errorf("vktp: start vkturn: %w", err)
	}
	traceLog(m.logw, "vktp: process started pid=%d", cmd.Process.Pid)

	if onSpawn != nil {
		traceLog(m.logw, "vktp: running onSpawn pid=%d", cmd.Process.Pid)
		if err := onSpawn(cmd.Process.Pid); err != nil {
			traceLog(m.logw, "vktp: onSpawn failed: %v", err)
			cancel()
			_ = cmd.Wait()
			m.flushLogs()
			return fmt.Errorf("vktp: on-spawn: %w", err)
		}
	}

	traceLog(m.logw, "vktp: waiting for AppControl listener")
	conn, err := m.dial(token)
	if err != nil {
		traceLog(m.logw, "vktp: AppControl dial failed: %v", err)
		cancel()
		_ = cmd.Wait()
		m.flushLogs()
		return err
	}
	traceLog(m.logw, "vktp: AppControl connected")
	appctl := appcontrolpb.NewAppControlClient(conn)

	traceLog(m.logw, "vktp: configuring relay managed=%v runtime=%s auth=%s threads=%d", profile.Managed, client.RuntimeMode, client.VKAuthMode, client.Threads)
	if err := configure(appctl, profile, client, m.protectSocket); err != nil {
		traceLog(m.logw, "vktp: configure failed: %v", err)
		_ = conn.Close()
		cancel()
		_ = cmd.Wait()
		m.flushLogs()
		return err
	}

	traceLog(m.logw, "vktp: configure succeeded")
	m.cmd, m.cancel, m.conn, m.client = cmd, cancel, conn, appctl
	return nil
}

// dial waits for the AppControl endpoint to come up, then opens a bearer-authenticated
// gRPC connection over it. The endpoint is a unix socket by path on Linux/macOS, or a
// "tcp:127.0.0.1:port" loopback listener on Windows (AF_UNIX + gRPC unix:// targets are
// unreliable there, and vkturn already supports the tcp: form with token auth).
func (m *Manager) dial(token string) (*grpc.ClientConn, error) {
	// Generous ceiling: on a slow machine (e.g. a low-spec VM) vkturn's cold start can take
	// well over ten seconds before it binds the AppControl listener. The loop polls every
	// 50ms and returns the instant the listener answers, so a fast host is not slowed.
	deadline := time.Now().Add(30 * time.Second)
	if addr, ok := tcpAddr(m.socketPath); ok {
		for {
			if c, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
				_ = c.Close()
				break
			}
			if time.Now().After(deadline) {
				return nil, errors.New("vktp: AppControl listener did not appear")
			}
			time.Sleep(50 * time.Millisecond)
		}
		return grpc.NewClient(
			addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithChainUnaryInterceptor(bearerUnary(token)),
			grpc.WithChainStreamInterceptor(bearerStream(token)),
		)
	}
	for {
		if _, err := os.Stat(m.socketPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return nil, errors.New("vktp: AppControl socket did not appear")
		}
		time.Sleep(50 * time.Millisecond)
	}
	return grpc.NewClient(
		"unix://"+m.socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(bearerUnary(token)),
		grpc.WithChainStreamInterceptor(bearerStream(token)),
	)
}

// isTCPSocket reports whether the AppControl address is a tcp: loopback endpoint.
func isTCPSocket(path string) bool { _, ok := tcpAddr(path); return ok }

// tcpAddr returns the host:port of a "tcp:host:port" AppControl address.
func tcpAddr(path string) (string, bool) {
	return strings.CutPrefix(path, "tcp:")
}

// Telemetry streams the connected-stream count to onCount until ctx is cancelled or
// the stream ends.
func (m *Manager) Telemetry(ctx context.Context, onCount func(int64)) error {
	m.mu.Lock()
	client := m.client
	m.mu.Unlock()
	if client == nil {
		return errors.New("vktp: not running")
	}
	stream, err := client.StreamTelemetry(ctx, &appcontrolpb.StreamTelemetryRequest{})
	if err != nil {
		return err
	}
	for {
		msg, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) || ctx.Err() != nil {
				return nil
			}
			return err
		}
		onCount(msg.GetConnectedStreams())
	}
}

// StreamUnderlayIPs streams the relay's underlay destination IPs (the VK TURN and peer
// servers it pins to the physical interface on Windows) to onIP until ctx is cancelled or
// the stream ends. The host installs a physical-gateway /32 bypass route for each so the
// underlay stays off the full tunnel. On Linux the relay never reports any (fwmark bypass),
// so onIP is simply never called.
func (m *Manager) StreamUnderlayIPs(ctx context.Context, onIP func(string)) error {
	m.mu.Lock()
	client := m.client
	m.mu.Unlock()
	if client == nil {
		return errors.New("vktp: not running")
	}
	stream, err := client.StreamUnderlayIPs(ctx, &appcontrolpb.StreamUnderlayIPsRequest{})
	if err != nil {
		return err
	}
	for {
		msg, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) || ctx.Err() != nil {
				return nil
			}
			return err
		}
		if ip := msg.GetIp(); ip != "" {
			onIP(ip)
		}
	}
}

// SetVKCookies pushes the captured VK web-session cookies and User-Agent to the
// running relay, which mints the privileged TURN token from them (account mode).
func (m *Manager) SetVKCookies(cookies, userAgent string) error {
	m.mu.Lock()
	client := m.client
	m.mu.Unlock()
	if client == nil {
		return errors.New("vktp: not running")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := client.SetVKCookies(ctx, &appcontrolpb.SetVKCookiesRequest{Cookies: cookies, UserAgent: userAgent})
	return err
}

// GetVKCookies pulls the relay's current (possibly rotated) VK session so the app
// can persist it and re-seed the login window.
func (m *Manager) GetVKCookies() (cookies, userAgent string, err error) {
	m.mu.Lock()
	client := m.client
	m.mu.Unlock()
	if client == nil {
		return "", "", errors.New("vktp: not running")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := client.GetVKCookies(ctx, &appcontrolpb.GetVKCookiesRequest{})
	if err != nil {
		return "", "", err
	}
	return resp.GetCookies(), resp.GetUserAgent(), nil
}

// PatchConfig live-applies a delta of runtime-mutable settings to the running relay
// without a restart. It is accepted synchronously; per-field progress arrives async
// as PatchStatusEvent over the event stream.
func (m *Manager) PatchConfig(req *appcontrolpb.PatchConfigRequest) error {
	m.mu.Lock()
	client := m.client
	m.mu.Unlock()
	if client == nil {
		return errors.New("vktp: not running")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := client.PatchConfig(ctx, req)
	if err != nil {
		return fmt.Errorf("vktp: patch: %w", err)
	}
	if e := strings.TrimSpace(resp.GetError()); e != "" {
		return fmt.Errorf("vktp: patch rejected: %s", e)
	}
	if !resp.GetAccepted() {
		return errors.New("vktp: patch not accepted")
	}
	return nil
}

// Provision performs the managed-profile WireGuard provisioning over AppControl:
// the relay mints the client's WG config through its DTLS PROVISION exchange with
// the node. Retries briefly (the root path is slow to be ready).
func (m *Manager) Provision(clientID string, token []byte, hwid string, localPort uint32) (*appcontrolpb.WireguardConfig, error) {
	m.mu.Lock()
	client := m.client
	m.mu.Unlock()
	if client == nil {
		return nil, errors.New("vktp: not running")
	}
	deadline := time.Now().Add(20 * time.Second)
	var lastErr error
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		resp, err := client.Provision(ctx, &appcontrolpb.ProvisionRequest{
			ClientId:  clientID,
			Token:     token,
			Hwid:      hwid,
			LocalPort: localPort,
		})
		cancel()
		if err == nil {
			if e := strings.TrimSpace(resp.GetError()); e != "" {
				lastErr = fmt.Errorf("vktp: provision rejected: %s", e)
			} else if resp.GetWg() == nil {
				lastErr = errors.New("vktp: provision returned no config")
			} else {
				return resp.GetWg(), nil
			}
		} else {
			lastErr = fmt.Errorf("vktp: provision: %w", err)
		}
		if time.Now().After(deadline) {
			return nil, lastErr
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// Running reports whether a vkturn instance is currently managed.
func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cmd != nil
}

// Stop terminates the vkturn process and closes the IPC.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
}

func traceLog(w io.Writer, format string, args ...any) {
	if w != nil {
		_, _ = fmt.Fprintf(w, format+"\n", args...)
	}
}

func (m *Manager) flushLogs() {
	if f, ok := m.logw.(flushWriter); ok {
		_ = f.Flush()
	}
}

func (m *Manager) stopLocked() {
	if m.conn != nil {
		_ = m.conn.Close()
		m.conn = nil
	}
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	if m.cmd != nil {
		_ = m.cmd.Wait()
		m.cmd = nil
		m.flushLogs()
	}
	m.client = nil
	// Belt-and-suspenders: kill any vkturn orphaned by a disconnect race or crash
	// (not the tracked m.cmd) so it never lingers holding 127.0.0.1:9000. Covers both
	// Stop (power/disconnect) and the stopLocked at the head of Start.
	reapStaleVkturn(m.binaryPath)
	_ = os.Remove(m.socketPath)
}

// Events streams the relay's structured control events (status phases, caps,
// captcha, vk-account prompts) to onEvent until ctx is cancelled or the stream
// ends. This is the typed AppControl channel that replaces scraping PROXY_EVENT
// lines from stdout. The connected-stream counter comes from Telemetry, not here.
func (m *Manager) Events(ctx context.Context, onEvent func(*appcontrolpb.ProxyEvent)) error {
	m.mu.Lock()
	client := m.client
	m.mu.Unlock()
	if client == nil {
		return errors.New("vktp: not running")
	}
	stream, err := client.StreamEvents(ctx, &appcontrolpb.StreamEventsRequest{})
	if err != nil {
		return err
	}
	for {
		ev, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) || ctx.Err() != nil {
				return nil
			}
			return err
		}
		onEvent(ev)
	}
}

// configure maps a profile onto a ConfigureRequest and applies it, mirroring the
// Android ProxyTunnelService.buildConfigure mapping for the VK TURN + WireGuard path.
func configure(client appcontrolpb.AppControlClient, p config.Profile, cs config.ClientSettings, protectSocket string) error {
	req := buildConfigure(p, cs, protectSocket)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	resp, err := client.Configure(ctx, req)
	if err != nil {
		return fmt.Errorf("vktp: configure: %w", err)
	}
	if e := strings.TrimSpace(resp.GetError()); e != "" {
		return fmt.Errorf("vktp: configure rejected: %s", e)
	}
	return nil
}

func buildConfigure(p config.Profile, cs config.ClientSettings, protectSocket string) *appcontrolpb.ConfigureRequest {
	s := p.Settings
	// Threads, creds group size, the VK auth mode, the session mode, the browser
	// fingerprint and the VK-links pool are device-global client parameters shared
	// across every profile, not part of the per-profile snapshot.
	sessionMode := cs.TurnSessionMode
	if p.Managed {
		// Managed profiles provision over the mu/v1 SessionHello channel, so the
		// relay must run in mu session mode.
		sessionMode = "mu"
	}
	req := &appcontrolpb.ConfigureRequest{
		Peer:            p.VKTurnEndpoint,
		VkLink:          strings.Join(cs.VKLinks, ","),
		VkLinkSecondary: cs.VKLinkSecondary,
		Listen:          cs.LocalEndpoint,
		Threads:         int32(cs.Threads),
		CredsGroupSize:  int32(cs.CredsGroupSize),
		Udp:             s.UseUDP,
		NoDtls:          s.NoObfuscation,
		ManualCaptcha:   s.ManualCaptcha,
		CaptchaSolver:   s.CaptchaAutoSolver,
		VkAuth:          cs.VKAuthMode,
		SessionMode:     sessionMode,
		BrowserFp:       cs.BrowserFingerprint,
		TurnHost:        s.TurnHost,
		TurnPort:        s.TurnPort,
		DnsMode:         s.DNSMode,
		UserDns:         s.UserDNS,
		ProtectSock:     protectSocket,
	}
	// WRAP fields only ride along when obfuscation is on.
	if s.WrapMode != "" && s.WrapMode != "off" {
		req.WrapMode = s.WrapMode
		req.WrapCipher = s.WrapCipher
		req.WrapKeyHex = s.WrapKeyHex
		req.WrapSendKey = s.WrapSendKey
	}
	return req
}

func bearerUnary(token string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		return invoker(withBearer(ctx, token), method, req, reply, cc, opts...)
	}
}

func bearerStream(token string) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		return streamer(withBearer(ctx, token), desc, cc, method, opts...)
	}
}

func withBearer(ctx context.Context, token string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
}

func randomToken() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
