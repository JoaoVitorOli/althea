package shell

import (
	_ "embed"
	"fmt"

	"github.com/egoist/mygo"
)

//go:embed tray.png
var trayIcon []byte

// TrayActions are what the tray's menu does.
type TrayActions struct {
	T            func(key string, args ...any) string
	Hotkey       string
	Open         func()
	Settings     func()
	Themes       func()
	Logs         func()
	StartAtLogin func() bool
	SetLogin     func(bool)
}

// Tray is Althea's icon in the notification area.
type Tray struct{ tray *mygo.Tray }

// NewTray shows the tray icon. A click opens the launcher; the menu has the rest.
func NewTray(a TrayActions) (*Tray, error) {
	t, err := mygo.NewTray(mygo.TrayOptions{Icon: trayIcon, ToolTip: "Althea", ID: "althea"})
	if err != nil {
		return nil, fmt.Errorf("tray: %w", err)
	}
	t.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Label: a.T("tray.open"), Accelerator: a.Hotkey, Click: func(*mygo.MenuItem, *mygo.Window) { a.Open() }},
		mygo.Separator(),
		{Label: a.T("tray.settings"), Click: func(*mygo.MenuItem, *mygo.Window) { a.Settings() }},
		{Label: a.T("tray.themes"), Click: func(*mygo.MenuItem, *mygo.Window) { a.Themes() }},
		{Label: a.T("tray.logs"), Click: func(*mygo.MenuItem, *mygo.Window) { a.Logs() }},
		{
			Label: a.T("tray.start_at_login"), Type: mygo.MenuItemCheckbox, Checked: a.StartAtLogin(),
			Click: func(it *mygo.MenuItem, _ *mygo.Window) { a.SetLogin(it.Checked) },
		},
		mygo.Separator(),
		{Label: a.T("tray.quit"), Role: mygo.RoleQuit},
	}))
	return &Tray{tray: t}, nil
}

// Destroy removes the icon.
func (t *Tray) Destroy() { t.tray.Destroy() }
