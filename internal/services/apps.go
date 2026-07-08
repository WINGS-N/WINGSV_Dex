package services

import (
	"bufio"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/WINGS-N/wingsv-dex/internal/config"
)

// AppsService backs the per-app split-tunnel screen: it enumerates installed desktop
// applications and stores the routing mode + per-app lists.
type AppsService struct {
	store    *config.Store
	onChange func() // re-applies routing to the live data plane after a settings edit
}

// NewAppsService wires the service to the store; onChange (may be nil) is invoked
// after every routing edit so a running tunnel can apply it live.
func NewAppsService(store *config.Store, onChange func()) *AppsService {
	return &AppsService{store: store, onChange: onChange}
}

// InstalledApp is a desktop application the user can route. Exec is the executable
// basename (the id matched against a process's comm/exe); Icon is the freedesktop
// icon name (resolved to a real image on the frontend later).
type InstalledApp struct {
	Name   string `json:"name"`
	Exec   string `json:"exec"`
	Icon   string `json:"icon"`
	System bool   `json:"system"`
}

// AppRouting is the frontend-facing split-tunnel state.
type AppRouting struct {
	Mode      string   `json:"mode"`
	Bypass    []string `json:"bypass"`
	Whitelist []string `json:"whitelist"`
}

func toAppRouting(a config.AppRoutingSettings) AppRouting {
	return AppRouting{Mode: a.Mode, Bypass: a.Bypass, Whitelist: a.Whitelist}
}

// Routing returns the current split-tunnel mode and lists.
func (s *AppsService) Routing() AppRouting {
	return toAppRouting(s.store.AppRouting())
}

// SetRouting persists the split-tunnel mode and lists.
func (s *AppsService) SetRouting(r AppRouting) (AppRouting, error) {
	if err := s.store.SetAppRouting(config.AppRoutingSettings{Mode: r.Mode, Bypass: r.Bypass, Whitelist: r.Whitelist}); err != nil {
		return AppRouting{}, err
	}
	if s.onChange != nil {
		s.onChange()
	}
	return s.Routing(), nil
}

// List enumerates installed desktop applications from the XDG applications
// directories, deduped by executable, sorted by name.
func (s *AppsService) List() []InstalledApp {
	seen := map[string]int{} // exec -> index into out
	var out []InstalledApp
	for _, dir := range applicationDirs() {
		fromUserDir := strings.HasPrefix(dir, userDataHome())
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".desktop") {
				continue
			}
			app, ok := parseDesktopEntry(filepath.Join(dir, e.Name()))
			if !ok {
				continue
			}
			if idx, dup := seen[app.Exec]; dup {
				// A per-user .desktop overrides a system-wide one for the same exec.
				if fromUserDir {
					out[idx] = app
				}
				continue
			}
			seen[app.Exec] = len(out)
			out = append(out, app)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	// Resolve each icon name to a data: URI so the frontend can show the real app
	// icon (the webview cannot load file:// paths). Cached by name.
	theme := activeIconTheme()
	cache := map[string]string{}
	for i := range out {
		out[i].Icon = resolveIconDataURI(out[i].Icon, theme, cache)
	}
	return out
}

// activeIconTheme returns the desktop's current icon theme via gsettings, or "" if
// unavailable (the hicolor/Adwaita fallbacks still cover most apps).
func activeIconTheme() string {
	out, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "icon-theme").Output()
	if err != nil {
		return ""
	}
	return strings.Trim(strings.TrimSpace(string(out)), "'\"")
}

// resolveIconDataURI turns a freedesktop icon name (or absolute path) into a data:
// URI, searching pixmaps and the icon-theme directories. Returns "" when not found
// (the frontend then shows a letter avatar).
func resolveIconDataURI(name, theme string, cache map[string]string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if uri, ok := cache[name]; ok {
		return uri
	}
	uri := ""
	if filepath.IsAbs(name) {
		uri = fileToDataURI(name)
	} else {
		for _, p := range iconCandidatePaths(name, theme) {
			if uri = fileToDataURI(p); uri != "" {
				break
			}
		}
	}
	cache[name] = uri
	return uri
}

// iconCandidatePaths lists likely icon files for a name, best (visible, small) first:
// pixmaps, then the active theme, Adwaita and hicolor at a few app sizes, then svg.
func iconCandidatePaths(name, theme string) []string {
	bases := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		bases = append(bases, filepath.Join(home, ".local", "share", "icons"), filepath.Join(home, ".icons"))
	}
	bases = append(bases, "/usr/local/share/icons", "/usr/share/icons")

	themes := []string{}
	if theme != "" {
		themes = append(themes, theme)
	}
	themes = append(themes, "Adwaita", "hicolor")
	sizes := []string{"48x48", "64x64", "32x32", "128x128", "96x96", "256x256"}

	var paths []string
	// Loose pixmaps first.
	for _, ext := range []string{"png", "svg"} {
		paths = append(paths, "/usr/share/pixmaps/"+name+"."+ext)
	}
	for _, base := range bases {
		for _, th := range themes {
			for _, sz := range sizes {
				paths = append(paths, filepath.Join(base, th, sz, "apps", name+".png"))
			}
			paths = append(paths, filepath.Join(base, th, "scalable", "apps", name+".svg"))
		}
	}
	return paths
}

// fileToDataURI reads a png/svg icon file and returns it as a data: URI, or "" on
// error or an unsupported/oversized file.
func fileToDataURI(path string) string {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() || fi.Size() > 512*1024 {
		return ""
	}
	var mime string
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		mime = "image/png"
	case ".svg":
		mime = "image/svg+xml"
	default:
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		return ""
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b)
}

// applicationDirs returns the XDG applications directories, user dir first so its
// entries take precedence over system ones.
func applicationDirs() []string {
	var dirs []string
	dirs = append(dirs, filepath.Join(userDataHome(), "applications"))
	dataDirs := os.Getenv("XDG_DATA_DIRS")
	if dataDirs == "" {
		dataDirs = "/usr/local/share:/usr/share"
	}
	for _, d := range strings.Split(dataDirs, ":") {
		if d = strings.TrimSpace(d); d != "" {
			dirs = append(dirs, filepath.Join(d, "applications"))
		}
	}
	return dirs
}

func userDataHome() string {
	if v := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share")
}

// parseDesktopEntry reads the [Desktop Entry] group of a .desktop file, returning a
// displayable application. Non-application, hidden and NoDisplay entries are skipped.
func parseDesktopEntry(path string) (InstalledApp, bool) {
	f, err := os.Open(path)
	if err != nil {
		return InstalledApp{}, false
	}
	defer f.Close()

	var name, exec, icon, categories, onlyShowIn string
	inEntry := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			inEntry = line == "[Desktop Entry]"
			continue
		}
		if !inEntry || line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "Type":
			if strings.TrimSpace(v) != "Application" {
				return InstalledApp{}, false
			}
		case "NoDisplay", "Hidden":
			if strings.EqualFold(strings.TrimSpace(v), "true") {
				return InstalledApp{}, false
			}
		case "Name":
			if name == "" {
				name = strings.TrimSpace(v)
			}
		case "Exec":
			exec = execBasename(v)
		case "Icon":
			icon = strings.TrimSpace(v)
		case "Categories":
			categories = v
		case "OnlyShowIn":
			onlyShowIn = v
		}
	}
	if name == "" || exec == "" {
		return InstalledApp{}, false
	}
	return InstalledApp{Name: name, Exec: exec, Icon: icon, System: isSystemApp(categories, onlyShowIn)}, true
}

// isSystemApp classifies a desktop entry as a system/DE component (rather than a
// user-facing app) by its .desktop metadata, since Linux has no per-app "installed
// by the user" flag and install location only means system-wide vs per-user. An
// OnlyShowIn binding marks a DE-specific entry (settings panels, DE tools); the
// System/Settings/Screensaver categories mark system tooling.
func isSystemApp(categories, onlyShowIn string) bool {
	if strings.TrimSpace(onlyShowIn) != "" {
		return true
	}
	for _, c := range strings.Split(categories, ";") {
		switch strings.TrimSpace(c) {
		case "System", "Settings", "Screensaver":
			return true
		}
	}
	return false
}

// execBasename extracts the executable basename from a desktop Exec line, dropping
// field codes (%u, %F, ...) and arguments.
func execBasename(exec string) string {
	fields := strings.Fields(exec)
	for _, f := range fields {
		if strings.HasPrefix(f, "%") {
			continue
		}
		// Skip common env/launcher prefixes so the real binary is matched.
		if f == "env" || f == "sh" || f == "bash" || strings.Contains(f, "=") {
			continue
		}
		return filepath.Base(strings.Trim(f, `"`))
	}
	return ""
}
