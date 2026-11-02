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
	"time"

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

func TestSnapshotViews(t *testing.T) {
	th, _, _ := theme.Load("althea-dark", "")
	catalog := i18n.New("pt-BR")
	_ = catalog.Add("", Messages)
	r := NewRoot(catalog.T, th)
	tt := ui.NewTester(r.View, 720, 420)
	tt.SetScale(2)
	shot := func(name string, v sdk.View) {
		r.closeAll()
		r.Push("test", v)
		tt.Frame()
		tt.Advance(time.Second)
		save(t, tt, "view-"+name)
	}
	acts := []sdk.Action{{Title: "Colar"}, {Title: "Copiar"}}
	shot("list", &sdk.List{Title: "Snippets", Items: []sdk.Item{
		{ID: "1", Title: "Assinatura de e-mail", Subtitle: "Atenciosamente, João", Section: "Fixados", Icon: sdk.Icon{Glyph: "document"}, Actions: acts},
		{ID: "2", Title: "Endereço", Section: "Fixados", Icon: sdk.Icon{Glyph: "document"}, Accessory: "end", Actions: acts},
		{ID: "3", Title: "Data de hoje", Section: "Recentes", Icon: sdk.Icon{Glyph: "document"}, Actions: acts},
	}})
	shot("list-detail", &sdk.List{Title: "Clipboard", ShowDetail: true, Items: []sdk.Item{
		{
			ID: "1", Title: "func main() {}", Icon: sdk.Icon{Glyph: "document"}, Actions: acts,
			Detail: &sdk.Detail{Markdown: "```go\nfunc main() {}\n```", Metadata: []sdk.Metadata{{Label: "Aplicativo", Text: "VS Code"}, {Label: "Copiado", Text: "há 2 min"}}},
		},
		{ID: "2", Title: "https://go.dev", Icon: sdk.Icon{Glyph: "document"}, Actions: acts},
	}})
	var emoji []sdk.Item
	for _, e := range []string{"😀", "😁", "😂", "🤣", "😃", "😄", "😅", "😆", "😉", "😊", "😋", "😎"} {
		emoji = append(emoji, sdk.Item{ID: e, Title: "rosto " + e, Icon: sdk.Icon{Text: e}, Actions: acts})
	}
	shot("grid", &sdk.Grid{Title: "Emoji", Items: emoji, Columns: 8})
	shot("detail", &sdk.Detail{Title: "Sobre", Markdown: "# Althea\n\nUm launcher **rápido** e privado.\n\n- Busca\n- Extensões", Metadata: []sdk.Metadata{{Label: "Versão", Text: "1.0.0"}}, Actions: acts})
	shot("form", &sdk.Form{Title: "Novo snippet", Fields: []sdk.Field{
		{ID: "name", Title: "Nome", Placeholder: "Assinatura", Required: true},
		{ID: "text", Title: "Texto", Kind: sdk.TextArea, Info: "Use {clipboard} e {date}."},
		{ID: "kind", Title: "Tipo", Kind: sdk.Dropdown, Options: []sdk.Option{{Value: "t", Title: "Texto"}, {Value: "c", Title: "Código"}}},
		{ID: "pin", Title: "Fixar", Kind: sdk.Checkbox, Placeholder: "Mostrar no topo"},
	}})
	shot("empty", &sdk.List{Empty: sdk.Empty{Title: "Nenhum snippet", Description: "Crie um com Ctrl+N.", Icon: sdk.Icon{Glyph: "document"}}})
	r.Host = &syncHost{t: t, r: r}
	shot("chat", &sdk.Chat{
		Title: "Por que o céu é azul?", Placeholder: "Pergunte o que quiser…",
		Messages: []sdk.ChatMessage{
			{Mine: true, Text: "Por que o céu é azul?"},
			{Text: "Por causa do **espalhamento de Rayleigh**: a luz azul se espalha mais.\n\n- Comprimento de onda menor\n- Mais espalhamento"},
		},
		First: "E o pôr do sol?",
		Send: func(_ sdk.Context, _ string, show func(string)) error {
			show("A luz atravessa mais atmosfera, e **sobra o vermelho**.")
			return nil
		},
		Actions: acts,
	})
	shot("streamed", &sdk.Detail{
		Title:    "Inglês → Português",
		Metadata: []sdk.Metadata{{Label: "De", Text: "Inglês (detectado)"}, {Label: "Para", Text: "Português"}},
		Stream: func(_ sdk.Context, show func(string)) error {
			show("Bom dia, como vai você?")
			return nil
		},
		Actions: acts,
	})
	r.Reset()
	tt.Frame()
	_ = tt.Click("Buscar")
	tt.Type("por que o céu é azul")
	r.SetResults("por que o céu é azul", []registry.Result{{Key: "a", Fallback: true, Item: sdk.Item{
		Title: `Perguntar à IA: "por que o céu é azul"`, Icon: sdk.Icon{Glyph: "sparkles"}, Actions: acts,
	}}, {Key: "b", Fallback: true, Item: sdk.Item{
		Title: `Traduzir "por que o céu é azul"`, Icon: sdk.Icon{Glyph: "translate"}, Actions: acts,
	}}})
	r.SetTake("por que o céu é azul", "Perguntar à IA")
	tt.Frame()
	save(t, tt, "root-take")
}
