package i18n_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/JoaoVitorOli/althea/internal/i18n"
)

var host = fstest.MapFS{
	"en.toml": {Data: []byte(`
quit = "Quit"
greeting = "Hello, {name}!"

[search]
placeholder = "Search for apps and commands…"
results = "{count} results for {query}"
`)},
	"pt-BR.toml": {Data: []byte(`
quit = "Sair"
greeting = "Olá, {name}!"

[search]
placeholder = "Busque apps e comandos…"
`)},
}

func catalog(t *testing.T, lang string) *i18n.Catalog {
	t.Helper()
	c := i18n.New(lang)
	if err := c.Add("", host); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestTranslatesIntoTheLanguage(t *testing.T) {
	c := catalog(t, "pt-BR")
	if got := c.T("search.placeholder"); got != "Busque apps e comandos…" {
		t.Errorf("T = %q", got)
	}
}

func TestFallsBackToEnglishThenToTheKey(t *testing.T) {
	c := catalog(t, "pt-BR")
	if got := c.T("search.results", "count", 3, "query", "code"); got != "3 results for code" {
		t.Errorf("missing pt-BR message = %q, want the English one", got)
	}
	if got := c.T("nowhere.to.be.found"); got != "nowhere.to.be.found" {
		t.Errorf("missing message = %q, want the key", got)
	}
}

func TestFillsNamedPlaceholders(t *testing.T) {
	c := catalog(t, "en")
	if got := c.T("greeting", "name", "Ada"); got != "Hello, Ada!" {
		t.Errorf("T = %q", got)
	}
	if got := c.T("greeting"); got != "Hello, {name}!" {
		t.Errorf("T without args = %q, want the placeholder kept", got)
	}
}

func TestExtensionsBringNamespacedCatalogs(t *testing.T) {
	c := catalog(t, "pt-BR")
	ext := fstest.MapFS{
		"en.toml":    {Data: []byte(`title = "Calculator"`)},
		"pt-BR.toml": {Data: []byte(`title = "Calculadora"`)},
	}
	if err := c.Add("althea.calculator", ext); err != nil {
		t.Fatal(err)
	}
	if got := c.T("althea.calculator.title"); got != "Calculadora" {
		t.Errorf("T = %q", got)
	}
	if got := c.T("quit"); got != "Sair" {
		t.Errorf("host message after Add = %q", got)
	}
}

func TestAddReportsTheBrokenFile(t *testing.T) {
	c := i18n.New("en")
	err := c.Add("x", fstest.MapFS{"en.toml": {Data: []byte("title = ")}})
	if err == nil || !strings.Contains(err.Error(), "en.toml") {
		t.Errorf("Add = %v, want an error naming en.toml", err)
	}
}

func TestMatchPicksASupportedLanguage(t *testing.T) {
	for tag, want := range map[string]string{
		"pt-BR": "pt-BR", "pt_BR.UTF-8": "pt-BR", "pt-PT": "pt-BR", "pt": "pt-BR",
		"en-US": "en", "en-GB": "en", "de-DE": "en", "": "en",
	} {
		if got := i18n.Match(tag); got != want {
			t.Errorf("Match(%q) = %q, want %q", tag, got, want)
		}
	}
}
