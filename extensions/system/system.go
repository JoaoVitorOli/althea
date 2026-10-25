// Package system is the System extension: lock, sleep, restart, shut down, sign out, empty the
// recycle bin and toggle dark mode, from the root search. What cannot be undone asks first.
package system

import (
	"embed"
	"io/fs"

	"github.com/JoaoVitorOli/althea/sdk"
)

//go:embed locales/*.toml
var locales embed.FS

// Platform is how the extension acts on the system; the host provides it. A nil function
// leaves its command out, as on a system that cannot do it.
type Platform struct {
	Lock, Sleep, Restart, ShutDown, SignOut, EmptyRecycleBin func() error
	IsDarkMode                                               func() bool
	SetDarkMode                                              func(on bool) error
}

// Extension is the System extension.
type Extension struct{ p Platform }

// New returns the System extension.
func New(p Platform) *Extension { return &Extension{p: p} }

// Manifest implements sdk.Extension.
func (*Extension) Manifest() sdk.Manifest {
	messages, _ := fs.Sub(locales, "locales")
	return sdk.Manifest{ID: "althea.system", Title: "System", Icon: sdk.Icon{Glyph: "power"}, Messages: messages}
}

// command describes one command: its platform function, glyph, keywords in every language
// Althea speaks, and whether it asks first.
type command struct {
	id       string
	run      func() error
	glyph    string
	keywords []string
	confirm  bool
}

// Commands implements sdk.Extension.
func (e *Extension) Commands(ctx sdk.Context) []sdk.Command {
	defs := []command{
		{"lock", e.p.Lock, "lock", []string{"lock", "bloquear", "travar"}, false},
		{"sleep", e.p.Sleep, "moon", []string{"sleep", "suspend", "suspender", "dormir"}, false},
		{"restart", e.p.Restart, "restart", []string{"restart", "reboot", "reiniciar"}, true},
		{"shutdown", e.p.ShutDown, "power", []string{"shut down", "power off", "desligar"}, true},
		{"signout", e.p.SignOut, "logout", []string{"sign out", "log out", "logoff", "sair", "encerrar sessão"}, true},
		{"emptybin", e.p.EmptyRecycleBin, "trash", []string{"empty recycle bin", "trash", "esvaziar lixeira"}, true},
	}
	var cmds []sdk.Command
	for _, d := range defs {
		if d.run == nil {
			continue
		}
		c := sdk.Command{
			ID: d.id, Title: ctx.T(d.id), Icon: sdk.Icon{Glyph: d.glyph}, Keywords: d.keywords,
			Run: hideThen(d.run),
		}
		if d.confirm {
			c.Confirm = ctx.T(d.id + "_confirm")
		}
		cmds = append(cmds, c)
	}
	if e.p.SetDarkMode != nil && e.p.IsDarkMode != nil {
		cmds = append(cmds, sdk.Command{
			ID: "darkmode", Title: ctx.T("darkmode"), Icon: sdk.Icon{Glyph: "sun"},
			Keywords: []string{"dark mode", "light mode", "theme", "modo escuro", "modo claro", "tema"},
			Run: func(sdk.Context) error {
				return e.p.SetDarkMode(!e.p.IsDarkMode())
			},
		})
	}
	return cmds
}

// hideThen hides the launcher before acting, so that it is out of the way of a locked screen
// or a shutdown.
func hideThen(run func() error) func(sdk.Context) error {
	return func(ctx sdk.Context) error {
		ctx.Hide()
		return run()
	}
}
