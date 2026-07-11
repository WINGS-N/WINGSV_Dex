package services

import (
	_ "embed"
	"os"
	"os/exec"
	"runtime"
	"sync"
)

//go:embed assets/eastermusic.mp3
var easterMusic []byte

// MusicService plays the onboarding easter-egg track. The WebKitGTK build on this Linux
// target cannot decode in-page media (both <audio> and <video> elements error out), so the
// track is played by a native system player instead of the webview. Best-effort: a no-op
// when no player is available.
type MusicService struct {
	mu   sync.Mutex
	cmd  *exec.Cmd
	path string
}

// NewMusicService constructs the easter-egg music service.
func NewMusicService() *MusicService { return &MusicService{} }

// Play starts the track from the beginning, stopping any current playback first.
func (m *MusicService) Play() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
	path, err := m.ensureFile()
	if err != nil {
		return err
	}
	cmd := playerCommand(path)
	if cmd == nil {
		return nil
	}
	hideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	m.cmd = cmd
	return nil
}

// Stop halts playback if running.
func (m *MusicService) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
}

func (m *MusicService) stopLocked() {
	if m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Kill()
		_ = m.cmd.Wait()
		m.cmd = nil
	}
}

// ensureFile lazily materialises the embedded mp3 to a temp file the player can open.
func (m *MusicService) ensureFile() (string, error) {
	if m.path != "" {
		return m.path, nil
	}
	f, err := os.CreateTemp("", "wingsv-dex-egg-*.mp3")
	if err != nil {
		return "", err
	}
	if _, err := f.Write(easterMusic); err != nil {
		_ = f.Close()
		return "", err
	}
	_ = f.Close()
	m.path = f.Name()
	return m.path, nil
}

// playerCommand returns a looping, headless player command for the first available player,
// or nil if none is found.
func playerCommand(path string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		ps := "$ErrorActionPreference='SilentlyContinue';Add-Type -AssemblyName presentationCore;" +
			"$p=New-Object System.Windows.Media.MediaPlayer;$p.Open([uri]'" + path + "');$p.Play();" +
			"while($true){Start-Sleep -Milliseconds 500}"
		return exec.Command("powershell", "-NoProfile", "-WindowStyle", "Hidden", "-Command", ps)
	}
	candidates := [][]string{
		{"mpv", "--no-video", "--really-quiet", "--loop=inf", "--", path},
		{"ffplay", "-nodisp", "-autoexit", "-loop", "0", "-loglevel", "quiet", path},
		{"cvlc", "--intf", "dummy", "--loop", "--play-and-exit", path},
		{"gst-play-1.0", path},
	}
	for _, c := range candidates {
		if p, err := exec.LookPath(c[0]); err == nil {
			return exec.Command(p, c[1:]...)
		}
	}
	return nil
}
