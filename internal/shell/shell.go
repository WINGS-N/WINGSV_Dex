// Package shell owns the app window and the system tray. The window is a normal desktop
// window; closing it hides to the tray rather than quitting. The tray menu shows the
// window, connects/disconnects (a single item that reflects the current state) and quits.
package shell

import (
	_ "embed"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed appicon.png
var iconPNG []byte

// Deps are the callbacks the tray needs from the rest of the app.
type Deps struct {
	Status     func() string // current connection status ("connected", "connecting", ...)
	Connect    func()        // start connecting (must not block)
	Disconnect func()        // disconnect
	StateEvent string        // app event emitted when the connection status changes
}

// Controller wires the window and the tray together.
type Controller struct {
	app  *application.App
	win  *application.WebviewWindow
	tray *application.SystemTray
	menu *application.Menu
	deps Deps

	mu       sync.Mutex
	quitting bool
	connItem *application.MenuItem
}

// New creates the window and tray icon.
func New(app *application.App, deps Deps) *Controller {
	c := &Controller{app: app, deps: deps}

	c.win = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "WINGS V",
		Width:            420,
		Height:           820,
		MinWidth:         380,
		MinHeight:        640,
		BackgroundColour: application.NewRGB(0, 0, 0),
		URL:              "/",
		Linux:            application.LinuxWindow{Icon: iconPNG},
	})

	// Closing the window hides it to the tray instead of quitting; quit is explicit via
	// the tray menu. RegisterHook runs synchronously so the close can be cancelled.
	c.win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		c.mu.Lock()
		quitting := c.quitting
		c.mu.Unlock()
		if quitting {
			return
		}
		e.Cancel()
		c.win.Hide()
	})

	c.tray = app.SystemTray.New()
	c.tray.SetIcon(iconPNG)
	c.tray.SetTooltip("WINGS V")
	c.tray.SetMenu(c.buildMenu())

	if deps.StateEvent != "" {
		app.Event.On(deps.StateEvent, func(*application.CustomEvent) { c.refresh() })
	}
	c.refresh()
	return c
}

func (c *Controller) buildMenu() *application.Menu {
	m := application.NewMenu()
	m.Add("Открыть").OnClick(func(*application.Context) { c.show() })
	m.AddSeparator()
	c.connItem = m.Add("Подключить")
	c.connItem.OnClick(func(*application.Context) { c.toggleConnection() })
	m.AddSeparator()
	m.Add("Выход").OnClick(func(*application.Context) { c.quit() })
	c.menu = m
	return m
}

func (c *Controller) show() {
	c.win.Show()
	c.win.Focus()
}

func (c *Controller) toggleConnection() {
	if isActive(c.status()) {
		if c.deps.Disconnect != nil {
			c.deps.Disconnect()
		}
		return
	}
	if c.deps.Connect != nil {
		c.deps.Connect()
	}
}

func (c *Controller) quit() {
	c.mu.Lock()
	c.quitting = true
	c.mu.Unlock()
	c.app.Quit()
}

// refresh syncs the tray tooltip and the connect/disconnect item to the current status.
func (c *Controller) refresh() {
	st := c.status()
	if c.connItem != nil {
		if isActive(st) {
			c.connItem.SetLabel("Отключить")
		} else {
			c.connItem.SetLabel("Подключить")
		}
		c.tray.SetMenu(c.menu)
	}
	c.tray.SetTooltip(tooltip(st))
}

func (c *Controller) status() string {
	if c.deps.Status == nil {
		return ""
	}
	return c.deps.Status()
}

// isActive reports whether the connection is up or in transition, i.e. the tray action
// should offer to disconnect (which also cancels an in-progress connect).
func isActive(status string) bool {
	switch status {
	case "connected", "connecting", "stopping":
		return true
	}
	return false
}

func tooltip(status string) string {
	switch status {
	case "connected":
		return "WINGS V - подключено"
	case "connecting":
		return "WINGS V - подключение..."
	case "stopping":
		return "WINGS V - отключение..."
	case "error":
		return "WINGS V - ошибка"
	}
	return "WINGS V - отключено"
}
