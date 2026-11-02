package ui

import (
	"errors"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/JoaoVitorOli/althea/sdk"
	"github.com/JoaoVitorOli/althea/sdk/sdktest"
)

var languages = []sdk.Option{
	{Value: "de", Title: "Alemão", Badge: "DE"},
	{Value: "en", Title: "Inglês", Badge: "EN"},
	{Value: "pt", Title: "Português", Badge: "PT"},
	{Value: "tr", Title: "Turco", Badge: "TR"},
}

func converter(convert func(sdk.Conversion, func(sdk.Converted)) error) *sdk.Converter {
	return &sdk.Converter{
		Title: "Traduzir", Placeholder: "Digite o texto", ResultPlaceholder: "A tradução aparece aqui", ConvertTitle: "Traduzir",
		From: sdk.Picker{
			Title: "De", Placeholder: "Buscar idiomas", Value: "auto",
			Options: append([]sdk.Option{{Value: "auto", Title: "Detectar"}}, languages...),
		},
		To: sdk.Picker{Title: "Para", Placeholder: "Buscar idiomas", Value: "en", Options: languages},
		Convert: func(_ sdk.Context, c sdk.Conversion, show func(sdk.Converted)) error {
			return convert(c, show)
		},
	}
}

// marking translates by marking the text with its languages.
func marking(asked *[]sdk.Conversion) func(sdk.Conversion, func(sdk.Converted)) error {
	return func(c sdk.Conversion, show func(sdk.Converted)) error {
		*asked = append(*asked, c)
		out := sdk.Converted{Text: c.From + ">" + c.To + ":" + c.Text}
		if c.From == "auto" {
			out.From = "tr"
		}
		show(out)
		return nil
	}
}

func TestConverterRunsOnEnterNotAsTheUserTypes(t *testing.T) {
	r, tt, _ := pushing(t)
	var asked []sdk.Conversion
	r.Push("test", converter(marking(&asked)))
	tt.Frame()
	if !tt.HasText("A tradução aparece aqui") || !tt.HasText("Detectar") || !tt.HasText("Inglês") {
		t.Errorf("texts %q", tt.Texts())
	}

	tt.Type("Naber?")
	tt.Frame()
	if len(asked) != 0 {
		t.Fatalf("ran as it was typed: %v", asked)
	}
	tt.Key(ui.Shift, ui.KeyEnter)
	tt.Type("Oi")
	p := r.stack[0].(*converterPage)
	if p.text != "Naber?\nOi" || len(asked) != 0 {
		t.Fatalf("Shift+Enter should break the line: %q, %v", p.text, asked)
	}

	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	want := []sdk.Conversion{{Text: "Naber?\nOi", From: "auto", To: "en"}}
	if !slices.Equal(asked, want) {
		t.Fatalf("asked %v", asked)
	}
	if !tt.HasText("auto>en:Naber?\nOi") {
		t.Errorf("texts %q", tt.Texts())
	}
	if !tt.HasText("Turco (detected)") {
		t.Errorf("does not say what it detected; texts %q", tt.Texts())
	}
}

func TestConverterDoesNotRunWithNothingTyped(t *testing.T) {
	r, tt, _ := pushing(t)
	var asked []sdk.Conversion
	r.Push("test", converter(marking(&asked)))
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	if len(asked) != 0 {
		t.Errorf("asked %v", asked)
	}
}

func TestConverterRunsAtOnceWhenAsked(t *testing.T) {
	r, tt, _ := pushing(t)
	var asked []sdk.Conversion
	v := converter(marking(&asked))
	v.Text, v.RunAtOnce = "Selected", true
	r.Push("test", v)
	tt.Frame()
	if len(asked) != 1 || asked[0].Text != "Selected" {
		t.Errorf("asked %v", asked)
	}
}

func TestLanguagesAreFoundBySearchingAndChoosingOneRunsAgain(t *testing.T) {
	r, tt, _ := pushing(t)
	var asked []sdk.Conversion
	r.Push("test", converter(marking(&asked)))
	tt.Frame()
	tt.Type("Naber?")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()

	tt.Key(ui.Ctrl, ui.KeyL)
	tt.Frame()
	if len(r.stack) != 2 {
		t.Fatalf("stack %d; texts %q", len(r.stack), tt.Texts())
	}
	list := r.stack[1].(*listPage)
	if got := list.shown[list.selected].item.ID; got != "en" {
		t.Errorf("starts on %q, want the current language", got)
	}

	tt.Type("portugues") // without its accent, as people type
	tt.Frame()
	if tt.HasText("Alemão") || !tt.HasText("Português") {
		t.Errorf("texts %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()

	if len(r.stack) != 1 {
		t.Fatalf("the options did not close: stack %d", len(r.stack))
	}
	if got := asked[len(asked)-1]; got.To != "pt" || got.Text != "Naber?" {
		t.Errorf("asked %+v", got)
	}
	tt.Type("!")
	if p := r.stack[0].(*converterPage); p.text != "Naber?!" {
		t.Errorf("the keyboard did not come back to the text: %q", p.text)
	}
}

func TestLanguagesOpenWithAClick(t *testing.T) {
	r, tt, _ := pushing(t)
	var asked []sdk.Conversion
	r.Push("test", converter(marking(&asked)))
	tt.Frame()
	if err := tt.Click("De"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Buscar idiomas") && len(r.stack) != 2 {
		t.Errorf("stack %d; texts %q", len(r.stack), tt.Texts())
	}
}

func TestSwapTurnsTheResultIntoTheTextUsingWhatWasDetected(t *testing.T) {
	r, tt, _ := pushing(t)
	var asked []sdk.Conversion
	r.Push("test", converter(marking(&asked)))
	tt.Frame()

	tt.Key(ui.Ctrl, ui.KeyS) // nothing known of From yet: nothing to swap
	tt.Frame()
	p := r.stack[0].(*converterPage)
	if p.from != "auto" || p.to != "en" {
		t.Fatalf("from %q, to %q", p.from, p.to)
	}

	tt.Type("Naber?")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Key(ui.Ctrl, ui.KeyS)
	tt.Frame()
	if p.from != "en" || p.to != "tr" || p.detected != "" {
		t.Errorf("from %q, to %q, detected %q", p.from, p.to, p.detected)
	}
	if got := asked[len(asked)-1]; got.Text != "auto>en:Naber?" || got.From != "en" || got.To != "tr" {
		t.Errorf("asked %+v", got)
	}
}

func TestConverterCopiesTheResultAndNothingBeforeThereIsOne(t *testing.T) {
	r, tt, _ := pushing(t)
	var asked []sdk.Conversion
	r.Push("test", converter(marking(&asked)))
	tt.Frame()
	p := r.stack[0].(*converterPage)
	if err := p.actions()[1].Run(sdktest.New(t, testExt{}, "en")); err != nil {
		t.Fatal(err)
	}

	tt.Type("Oi")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	ctx := sdktest.New(t, testExt{}, "en")
	if err := p.actions()[1].Run(ctx); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ctx.Copied, []string{"auto>en:Oi"}) {
		t.Errorf("copied %q", ctx.Copied)
	}
}

func TestConverterShowsWhatWentWrongInPlaceOfTheResult(t *testing.T) {
	r, tt, h := pushing(t)
	fail := true
	r.Push("test", converter(func(_ sdk.Conversion, show func(sdk.Converted)) error {
		if fail {
			return errors.New("O motor ainda não está disponível")
		}
		show(sdk.Converted{Text: "ok"})
		return nil
	}))
	tt.Frame()
	tt.Type("Oi")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if !tt.HasText("O motor ainda não está disponível") {
		t.Errorf("texts %q", tt.Texts())
	}
	if len(h.errs) != 0 {
		t.Errorf("shown twice, as a toast too: %v", h.errs)
	}
	fail = false
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if tt.HasText("O motor ainda não está disponível") || !tt.HasText("ok") {
		t.Errorf("texts %q", tt.Texts())
	}
}

func TestConverterNotesGiveWayToTheResult(t *testing.T) {
	r, tt, _ := pushing(t)
	var shows []func(sdk.Converted)
	r.Push("test", converter(func(_ sdk.Conversion, show func(sdk.Converted)) error {
		shows = append(shows, show)
		return nil
	}))
	tt.Frame()
	tt.Type("Oi")
	tt.Key(0, ui.KeyEnter)
	shows[0](sdk.Converted{Note: "Baixando… 10%", To: "pt"})
	tt.Frame()
	p := r.stack[0].(*converterPage)
	if !tt.HasText("Baixando… 10%") || p.to != "pt" {
		t.Errorf("texts %q, to %q", tt.Texts(), p.to)
	}
	shows[0](sdk.Converted{Text: "Olá"})
	tt.Frame()
	if tt.HasText("Baixando… 10%") || !tt.HasText("Olá") {
		t.Errorf("texts %q", tt.Texts())
	}
}

func TestAFormsLongDropdownCanBeSearched(t *testing.T) {
	r, tt, _ := pushing(t)
	var options []sdk.Option
	for _, name := range []string{"Alemão", "Búlgaro", "Catalão", "Dinamarquês", "Espanhol", "Francês", "Grego", "Hindi", "Inglês", "Português"} {
		options = append(options, sdk.Option{Value: name, Title: name})
	}
	var got sdk.Values
	r.Push("test", &sdk.Form{
		Title:  "Tradutor",
		Fields: []sdk.Field{{ID: "to", Title: "Para", Kind: sdk.Dropdown, Default: "Alemão", Options: options}},
		Submit: func(_ sdk.Context, v sdk.Values) error { got = v; return nil },
	})
	tt.Frame()
	if err := tt.Click("Para"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type("Portu")
	tt.Frame()
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Key(ui.Ctrl, ui.KeyEnter)
	if got["to"] != "Português" {
		t.Errorf("values %v; texts %q", got, tt.Texts())
	}
}

func TestTheActionPanelTakesTheArrowsFromTheText(t *testing.T) {
	r, tt, _ := pushing(t)
	var asked []sdk.Conversion
	r.Push("test", converter(marking(&asked)))
	tt.Frame()
	tt.Type("Oi")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	asked = nil
	p := r.stack[0].(*converterPage)

	tt.Key(ui.Ctrl, ui.KeyK)
	tt.Frame()
	if !r.actions.open {
		t.Fatal("the panel did not open")
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyDown) // on "Para"
	tt.Key(0, ui.KeyUp)
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if r.actions.selected != 4 {
		t.Fatalf("selected %d, want 4", r.actions.selected)
	}
	if p.text != "Oi" {
		t.Errorf("the text changed: %q", p.text)
	}

	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if r.actions.open || len(r.stack) != 2 {
		t.Fatalf("open %v, stack %d; texts %q", r.actions.open, len(r.stack), tt.Texts())
	}
	if len(asked) != 0 {
		t.Errorf("Enter ran the translation rather than the action: %v", asked)
	}
	if !tt.HasText("Português") || !tt.HasText("Turco") {
		t.Errorf("texts %q", tt.Texts())
	}
}

func TestTheActionPanelRunsWhatIsClicked(t *testing.T) {
	r, tt, _ := pushing(t)
	var asked []sdk.Conversion
	r.Push("test", converter(marking(&asked)))
	tt.Frame()
	tt.Type("Oi")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()

	tt.Key(ui.Ctrl, ui.KeyK)
	tt.Frame()
	if err := tt.Click("Copy Result"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if r.actions.open {
		t.Error("the click did not run the action")
	}
}

func TestEscapeClosesTheActionPanelAlone(t *testing.T) {
	r, tt, _ := pushing(t)
	var asked []sdk.Conversion
	r.Push("test", converter(marking(&asked)))
	tt.Frame()
	tt.Key(ui.Ctrl, ui.KeyK)
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if r.actions.open || len(r.stack) != 1 {
		t.Errorf("open %v, stack %d", r.actions.open, len(r.stack))
	}
}
