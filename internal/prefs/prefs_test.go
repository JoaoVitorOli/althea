package prefs_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/JoaoVitorOli/althea/internal/prefs"
	"github.com/JoaoVitorOli/althea/sdk"
)

var decl = []sdk.Preference{
	{Name: "model", Kind: sdk.TextPreference, Default: "gpt-5"},
	{Name: "temperature", Kind: sdk.NumberPreference, Default: "0.7"},
	{Name: "stream", Kind: sdk.BoolPreference, Default: "true"},
	{
		Name: "provider", Kind: sdk.ChoicePreference, Default: "openai", Required: true,
		Options: []sdk.Option{{Value: "openai"}, {Value: "ollama"}},
	},
	{Name: "hotkey", Kind: sdk.HotkeyPreference},
	{Name: "api_key", Kind: sdk.SecretPreference, Required: true},
	{Name: "folder", Kind: sdk.PathPreference, Required: true},
}

func TestResolveTakesValidValuesAndDefaultsTheRest(t *testing.T) {
	values, problems := prefs.Resolve(decl, map[string]any{
		"model":       "llama3",
		"temperature": int64(1),
		"stream":      "yes",
		"provider":    "groq",
		"hotkey":      "Ctrl+Shift+A",
		"api_key":     "sk-oops",
		"colour":      "blue",
	})
	want := prefs.Values{"model": "llama3", "temperature": "1", "stream": "true", "provider": "openai", "hotkey": "Ctrl+Shift+A"}
	if !maps.Equal(values, want) {
		t.Errorf("values = %v, want %v", values, want)
	}
	var names []string
	for _, p := range problems {
		names = append(names, p.Name)
	}
	if want := []string{"stream", "provider", "api_key", "colour"}; !slices.Equal(names, want) {
		t.Errorf("problems = %v, want for %v", problems, want)
	}
}

func TestMissingRequiredPreferences(t *testing.T) {
	values, _ := prefs.Resolve(decl, nil)
	var names []string
	for _, p := range prefs.Missing(decl, values, func(string) bool { return false }) {
		names = append(names, p.Name)
	}
	if want := []string{"api_key", "folder"}; !slices.Equal(names, want) {
		t.Errorf("missing = %v, want %v", names, want)
	}
	values["folder"] = `C:\notes`
	if got := prefs.Missing(decl, values, func(string) bool { return true }); len(got) != 0 {
		t.Errorf("missing = %v, want none", got)
	}
}

func TestCheckAndEncodeWhatTheUserTypes(t *testing.T) {
	number, hot := decl[1], decl[4]
	if prefs.Check(number, "abc") == "" || prefs.Check(number, "0.2") != "" || prefs.Check(number, "") != "" {
		t.Error("numbers are not checked")
	}
	if prefs.Check(hot, "Ctrl+") == "" || prefs.Check(hot, "Alt+Q") != "" {
		t.Error("hotkeys are not checked")
	}
	if v, err := prefs.Encode(number, "2"); err != nil || v != int64(2) {
		t.Errorf("Encode = %v, %v", v, err)
	}
	if v, err := prefs.Encode(decl[2], "false"); err != nil || v != false {
		t.Errorf("Encode = %v, %v", v, err)
	}
	if _, err := prefs.Encode(decl[5], "sk"); err == nil {
		t.Error("a secret was encoded for the file")
	}
}

func TestPreferencesReadValues(t *testing.T) {
	p := prefs.Preferences{
		Values:     prefs.Values{"stream": "true", "temperature": "0.25", "model": "x"},
		ReadSecret: func(name string) (string, error) { return "secret-" + name, nil },
	}
	if !p.Bool("stream") || p.Number("temperature") != 0.25 || p.String("model") != "x" {
		t.Errorf("read %v %v %v", p.Bool("stream"), p.Number("temperature"), p.String("model"))
	}
	if s, _ := p.Secret("api_key"); s != "secret-api_key" {
		t.Errorf("secret %q", s)
	}
}
