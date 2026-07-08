package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/WINGS-N/wingsv-dex/internal/config"
	"github.com/WINGS-N/wingsv-dex/internal/vklogin"
	"github.com/WINGS-N/wingsv-dex/internal/vktp"
)

// fingerprintUA maps the selected browser fingerprint family to a representative
// desktop User-Agent (the canonical strings the vkturn relay impersonates), so the
// VK web session is created with the same identity the relay presents. "auto"/empty
// keeps the WebKitGTK default (a Safari UA).
func fingerprintUA(fp string) string {
	switch strings.ToLower(strings.TrimSpace(fp)) {
	case "chrome":
		return "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"
	case "edge":
		return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36 Edg/146.0.0.0"
	case "safari":
		return "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.6 Safari/605.1.15"
	case "firefox":
		return "Mozilla/5.0 (X11; Linux x86_64; rv:147.0) Gecko/20100101 Firefox/147.0"
	default:
		return ""
	}
}

// vkSession is the persisted VK web-session captured for account-mode TURN: the
// "k=v; k=v" cookie header (including HttpOnly remixsid) and the browser UA the
// relay uses to mint a privileged token.
type vkSession struct {
	Cookies   string `json:"cookies"`
	UserAgent string `json:"userAgent"`
}

// VKAuthService owns the VK account sign-in flow: it opens the native login window,
// delivers the captured session to the running relay over AppControl, and persists
// it so it survives restarts and relay-side rotation.
type VKAuthService struct {
	manager *vktp.Manager
	store   *config.Store
	dir     string

	mu      sync.Mutex
	session vkSession

	// authMu serializes sign-in attempts so a connect-time login and a concurrent
	// relay vk_cookies_required never open two windows: the second waiter re-checks
	// the cache (now populated) and just re-delivers.
	authMu sync.Mutex
}

// NewVKAuthService loads any persisted session from configDir.
func NewVKAuthService(manager *vktp.Manager, store *config.Store, configDir string) *VKAuthService {
	s := &VKAuthService{manager: manager, store: store, dir: configDir}
	s.session, _ = s.load()
	return s
}

func (s *VKAuthService) storageDir() string  { return filepath.Join(s.dir, "vk-webkit") }
func (s *VKAuthService) sessionFile() string { return filepath.Join(s.dir, "vk_session.json") }

// VKAuthStatus is the frontend-facing sign-in state.
type VKAuthStatus struct {
	LoggedIn bool `json:"loggedIn"`
}

// Status reports whether a usable VK session is stored.
func (s *VKAuthService) Status() VKAuthStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return VKAuthStatus{LoggedIn: hasRemixsid(s.session.Cookies)}
}

// Login opens the VK sign-in window and, on success, persists the session and
// delivers it to the relay if one is running.
func (s *VKAuthService) Login() (VKAuthStatus, error) {
	return s.loginWith("")
}

func (s *VKAuthService) loginWith(link string) (VKAuthStatus, error) {
	ua := fingerprintUA(s.store.Client().BrowserFingerprint)
	cookies, ua, err := vklogin.Capture(link, s.storageDir(), ua)
	if err != nil {
		return VKAuthStatus{}, err
	}
	s.mu.Lock()
	s.session = vkSession{Cookies: cookies, UserAgent: ua}
	_ = s.save(s.session)
	s.mu.Unlock()
	// Deliver to the relay if it is up; harmless no-op if it is not running yet.
	_ = s.manager.SetVKCookies(cookies, ua)
	return s.Status(), nil
}

// ClearCookies wipes the persisted session and the WebKit cookie store, so the next
// sign-in starts from a logged-out VK session (otherwise the store silently
// re-harvests the still-signed-in session).
func (s *VKAuthService) ClearCookies() (VKAuthStatus, error) {
	s.mu.Lock()
	s.session = vkSession{}
	_ = os.Remove(s.sessionFile())
	s.mu.Unlock()
	_ = vklogin.ClearStore(s.storageDir())
	return VKAuthStatus{LoggedIn: false}, nil
}

// ensureSession answers a relay vk_cookies_required / vk_account_auth(required)
// event: re-deliver the cached session silently, or open the login window at link
// when there is none. Blocks (opens a GTK window), so run it on its own goroutine.
func (s *VKAuthService) ensureSession(link string) {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	s.mu.Lock()
	sess := s.session
	s.mu.Unlock()
	if hasRemixsid(sess.Cookies) {
		_ = s.manager.SetVKCookies(sess.Cookies, sess.UserAgent)
		return
	}
	_, _ = s.loginWith(link)
}

// persistRotation stores a relay-rotated session pulled over GetVKCookies.
func (s *VKAuthService) persistRotation(cookies, ua string) {
	if !hasRemixsid(cookies) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if cookies == s.session.Cookies && ua == s.session.UserAgent {
		return
	}
	s.session = vkSession{Cookies: cookies, UserAgent: ua}
	_ = s.save(s.session)
}

func (s *VKAuthService) load() (vkSession, error) {
	raw, err := os.ReadFile(s.sessionFile())
	if err != nil {
		return vkSession{}, err
	}
	var sess vkSession
	if err := json.Unmarshal(raw, &sess); err != nil {
		return vkSession{}, err
	}
	return sess, nil
}

func (s *VKAuthService) save(sess vkSession) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.sessionFile() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.sessionFile())
}

func hasRemixsid(cookies string) bool { return strings.Contains(cookies, "remixsid") }
