package main

import (
	"embed"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/WINGS-N/wingsv-dex/internal/applog"
	"github.com/WINGS-N/wingsv-dex/internal/config"
	"github.com/WINGS-N/wingsv-dex/internal/nethelper"
	"github.com/WINGS-N/wingsv-dex/internal/services"
	"github.com/WINGS-N/wingsv-dex/internal/shell"
	"github.com/WINGS-N/wingsv-dex/internal/vklogin"
	"github.com/WINGS-N/wingsv-dex/internal/vktp"
)

// Wails embeds the built frontend into the binary. Everything under frontend/dist
// is served to the webview.

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Privileged data-plane mode: re-launched via pkexec to run the kernel
	// WireGuard + protect socket under CAP_NET_ADMIN. Handled before any GUI setup.
	if len(os.Args) > 1 && os.Args[1] == "--net-helper" {
		if err := nethelper.Run(); err != nil {
			log.Fatal(err)
		}
		return
	}

	// Out-of-process VK sign-in (Windows): a WebView2 process may host only one
	// environment, and the app owns one, so the sign-in window runs here in a fresh
	// process and returns the captured session on stdout. Handled before any GUI setup.
	if len(os.Args) > 1 && os.Args[1] == "--vk-login" {
		if err := vklogin.RunChild(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}

	configDir := appConfigDir()

	store, err := config.NewStore(filepath.Join(configDir, "profiles.json"))
	if err != nil {
		log.Fatal(err)
	}
	logStore, err := applog.NewStore(configDir)
	if err != nil {
		log.Fatal(err)
	}

	exePath, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}

	proxyLogWriter := applog.NewLineWriter(logStore, applog.ChannelProxy, os.Stderr)
	manager := vktp.NewManager(vkturnBinaryPath(), appControlAddr(configDir), services.ProtectSocket, proxyLogWriter)
	vkAuthSvc := services.NewVKAuthService(manager, store, configDir)
	connectionSvc := services.NewConnectionService(store, logStore, manager, vkAuthSvc, exePath)
	aboutSvc := services.NewAboutService(func() { manager.Stop() })

	app := application.New(application.Options{
		Name:        "WINGS V DeX",
		Description: "WINGS V DeX desktop client (VK TURN)",
		Services: []application.Service{
			application.NewService(services.NewProfilesService(store, func() { connectionSvc.OnSettingsChanged() })),
			application.NewService(connectionSvc),
			application.NewService(vkAuthSvc),
			application.NewService(services.NewAppsService(store, func() { _ = connectionSvc.ApplyAppRouting() })),
			application.NewService(services.NewLogsService(logStore)),
			application.NewService(aboutSvc),
			application.NewService(services.NewOnboardingService(configDir)),
			application.NewService(services.NewMusicService()),
			application.NewService(services.NewAvatarService(configDir)),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		Linux: application.LinuxOptions{
			// The window hides to the tray on close instead of quitting; the app lives
			// on in the tray until the user picks Quit.
			DisableQuitOnLastWindowClosed: true,
		},
		Windows: application.WindowsOptions{
			// Same tray behaviour as Linux: closing the window keeps the app alive in the
			// notification area (the tray icon reopens it as a popover).
			DisableQuitOnLastWindowClosed: true,
		},
	})
	connectionSvc.SetApp(app)
	aboutSvc.SetApp(app)

	// Owns the window and the system tray (show window, connect/disconnect, quit).
	shell.New(app, shell.Deps{
		Status:     func() string { return connectionSvc.State().Status },
		Connect:    func() { go func() { _, _ = connectionSvc.Connect() }() },
		Disconnect: func() { connectionSvc.Disconnect() },
		StateEvent: services.ConnectionStateEvent,
	})

	if err := app.Run(); err != nil {
		manager.Stop()
		log.Fatal(err)
	}
	manager.Stop()
}

// appConfigDir is the per-user directory holding the profile store and the vkturn
// AppControl socket.
func appConfigDir() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = "."
	}
	return filepath.Join(dir, "wingsv-dex")
}

// appControlAddr picks the AppControl IPC endpoint vkturn listens on. Unix domain
// sockets are used on Linux/macOS; on Windows, where AF_UNIX + gRPC unix:// targets are
// unreliable, a loopback TCP listener on a free port is used (vkturn selects it via the
// "tcp:" prefix; access is gated by the bearer token). Falls back to a fixed port if a
// free one cannot be probed.
func appControlAddr(configDir string) string {
	if runtime.GOOS != "windows" {
		return filepath.Join(configDir, "appcontrol.sock")
	}
	port := 47654
	if l, err := net.Listen("tcp", "127.0.0.1:0"); err == nil {
		port = l.Addr().(*net.TCPAddr).Port
		_ = l.Close()
	}
	return fmt.Sprintf("tcp:127.0.0.1:%d", port)
}

// vkturnBinaryPath locates the vkturn child binary: next to the app executable
// first (production layout), then bin/vkturn in the working tree (dev), then PATH.
func vkturnBinaryPath() string {
	name := "vkturn"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if _, err := os.Stat(filepath.Join("bin", name)); err == nil {
		if abs, err := filepath.Abs(filepath.Join("bin", name)); err == nil {
			return abs
		}
	}
	return name
}
