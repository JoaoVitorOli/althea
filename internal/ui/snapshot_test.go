//go:build snapshots

package ui

// Snapshots render views to PNG files for looking at them without a window:
//
//	ALTHEA_SNAPSHOTS=dir go test -tags snapshots -run Snapshot ./internal/ui

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/JoaoVitorOli/althea/internal/i18n"
	"github.com/JoaoVitorOli/althea/internal/registry"
	"github.com/JoaoVitorOli/althea/internal/theme"
	"github.com/JoaoVitorOli/althea/sdk"
)

func save(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("ALTHEA_SNAPSHOTS")
	if dir == "" {
		t.Skip("set ALTHEA_SNAPSHOTS to a directory")
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotOnboarding(t *testing.T) {
	for _, name := range []string{"althea-dark", "althea-light"} {
		th, _, _ := theme.Load(name, "")
		catalog := i18n.New("pt-BR")
		_ = catalog.Add("", Messages)
		r := NewRoot(catalog.T, th)
		r.Hotkey = "Alt+Space"
		tt := ui.NewTester(r.View, 720, 420)
		tt.SetScale(2)
		tt.Frame()
		save(t, tt, name+"-launcher")

		o := NewOnboarding(catalog.T, Motion{Scale: 0}, Choices{Hotkey: "Alt+Space", Theme: "system", StartAtLogin: true}, true, func(Choices) {})
		view := func(c *ui.Context) {
			c.SetTheme(ToUI(th))
			c.Root().Background(ToUI(th).Background)
			o.View(c)
		}
		tt = ui.NewTester(view, 720, 420)
		tt.SetScale(2)
		for i, next := range []string{"Começar", "Continuar", "Continuar", "Continuar", ""} {
			tt.Frame()
			save(t, tt, fmt.Sprintf("%s-onboarding-%d", name, i))
			if next != "" {
				if err := tt.Click(next); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func TestSnapshotResults(t *testing.T) {
	th, _, _ := theme.Load("althea-dark", "")
	catalog := i18n.New("pt-BR")
	_ = catalog.Add("", Messages)
	r := NewRoot(catalog.T, th)
	tt := ui.NewTester(r.View, 720, 420)
	tt.SetScale(2)
	_ = tt.Click("Buscar")
	tt.Type("vsc")
	acts := []sdk.Action{{Title: "Abrir"}, {Title: "Mostrar na pasta"}, {Title: "Copiar caminho", Shortcut: "Ctrl+Shift+C"}}
	r.SetResults("vsc", []registry.Result{
		{Key: "a", Item: sdk.Item{Title: "Visual Studio Code", Subtitle: "Microsoft", Icon: sdk.Icon{Glyph: "app"}, Accessory: "Aplicativo", Actions: acts}, Positions: []int{0, 7, 14}},
		{Key: "b", Item: sdk.Item{Title: "Visual Studio Code - Insiders", Icon: sdk.Icon{Glyph: "app"}, Accessory: "Aplicativo", Actions: acts}, Positions: []int{0, 7, 14}},
		{Key: "c", Command: true, Item: sdk.Item{Title: "Visual Basic Scripts", Icon: sdk.Icon{Glyph: "command"}, Actions: acts}, Positions: []int{0, 13, 14}},
	})
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	save(t, tt, "results")
	tt.Key(ui.Ctrl, ui.KeyK)
	tt.Frame()
	save(t, tt, "results-actions")
}
