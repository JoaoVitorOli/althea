package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/JoaoVitorOli/althea/internal/i18n"
)

func TestRootShowsTitleAndHintInTheLanguage(t *testing.T) {
	for lang, hint := range map[string]string{"en": "Esc to close", "pt-BR": "Esc para fechar"} {
		catalog := i18n.New(lang)
		if err := catalog.Add("", Messages); err != nil {
			t.Fatal(err)
		}
		r := &Root{T: catalog.T}
		tt := ui.NewTester(r.View, 720, 420)
		for _, want := range []string{"Althea", hint} {
			if !tt.HasText(want) {
				t.Errorf("%s: root view does not show %q; texts %q", lang, want, tt.Texts())
			}
		}
	}
}
