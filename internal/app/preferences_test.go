package app

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/JoaoVitorOli/althea/internal/config"
	"github.com/JoaoVitorOli/althea/sdk"
	"github.com/JoaoVitorOli/althea/sdk/sdktest"
)

type memVault struct {
	mu sync.Mutex
	m  map[string]string
}

func (v *memVault) Get(name string) (string, bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	s, ok := v.m[name]
	return s, ok, nil
}

func (v *memVault) Set(name, secret string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.m[name] = secret
	return nil
}

func (v *memVault) Delete(name string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.m, name)
	return nil
}

var aiPrefs = []sdk.Preference{
	{Name: "api_key", Title: "API Key", Kind: sdk.SecretPreference, Required: true},
	{Name: "model", Title: "Model", Default: "gpt-5"},
	{Name: "temperature", Title: "Temperature", Kind: sdk.NumberPreference, Default: "0.7"},
	{Name: "stream", Title: "Stream", Kind: sdk.BoolPreference, Default: "true"},
}

func TestPreferencesFormSavesToTheFileAndTheVault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.toml")
	vault := &memVault{m: map[string]string{}}
	p := newPreferences(path, vault, slog.New(slog.NewTextHandler(io.Discard, nil)))
	p.declare("althea.ai", aiPrefs)
	p.resolve(config.Defaults())
	if got := p.missing("althea.ai"); len(got) != 1 || got[0].Name != "api_key" {
		t.Fatalf("missing = %v, want the API key", got)
	}

	s := &settingsExtension{prefs: p, exts: []sdk.Manifest{{ID: "althea.ai", Title: "AI"}}}
	ctx := sdktest.New(t, s, "en")
	ran := 0
	form := s.form(ctx, s.exts[0], func(sdk.Context) error { ran++; return nil })
	if form.Fields[0].Kind != sdk.Password || form.Fields[3].Kind != sdk.Checkbox {
		t.Errorf("fields %+v", form.Fields)
	}
	if msg := form.Fields[2].Validate("warm"); msg == "" {
		t.Error("a number field takes text")
	}
	err := form.Submit(ctx, sdk.Values{"api_key": "sk-1", "model": "llama3", "temperature": "0.2", "stream": "true"})
	if err != nil || ran != 1 {
		t.Fatalf("Submit = %v, ran %d", err, ran)
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{`[extensions."althea.ai"]`, `model = "llama3"`, "temperature = 0.2"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("settings lack %q:\n%s", want, data)
		}
	}
	if strings.Contains(string(data), "sk-1") || strings.Contains(string(data), "stream") {
		t.Errorf("settings hold a secret or an unchanged value:\n%s", data)
	}
	if vault.m["Althea/althea.ai/api_key"] != "sk-1" {
		t.Errorf("vault = %v", vault.m)
	}
	got := p.of("althea.ai")
	if key, _ := got.Secret("api_key"); key != "sk-1" || got.String("model") != "llama3" || got.Number("temperature") != 0.2 {
		t.Errorf("read back %q %q %v", key, got.String("model"), got.Number("temperature"))
	}
	if len(p.missing("althea.ai")) != 0 {
		t.Error("still missing after saving")
	}

	// A saved secret is not asked again; an emptied value goes back to its default.
	form = s.form(ctx, s.exts[0], nil)
	if form.Fields[0].Required || form.Fields[0].Placeholder == "" {
		t.Errorf("secret field %+v", form.Fields[0])
	}
	if err := form.Submit(ctx, sdk.Values{"api_key": "", "model": "", "temperature": "0.2", "stream": "true"}); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if strings.Contains(string(data), "model") || vault.m["Althea/althea.ai/api_key"] != "sk-1" {
		t.Errorf("settings:\n%s\nvault %v", data, vault.m)
	}
	if p.of("althea.ai").String("model") != "gpt-5" {
		t.Errorf("model = %q, want the default", p.of("althea.ai").String("model"))
	}
}

func TestSettingsListsConfigurableExtensions(t *testing.T) {
	p := newPreferences(filepath.Join(t.TempDir(), "settings.toml"), &memVault{m: map[string]string{}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	p.declare("althea.ai", aiPrefs)
	p.resolve(config.Defaults())
	s := &settingsExtension{prefs: p, exts: []sdk.Manifest{{ID: "althea.ai", Title: "AI"}}}
	ctx := sdktest.New(t, s, "pt-BR")
	cmds := s.Commands(ctx)
	if len(cmds) != 1 || cmds[0].Title != "Preferências das extensões" {
		t.Fatalf("commands %+v", cmds)
	}
	if err := cmds[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	list := ctx.Pushed[0].(*sdk.List)
	if len(list.Items) != 1 || list.Items[0].Accessory != "Precisa configurar" || list.Items[0].Subtitle != "4 configurações" {
		t.Errorf("items %+v", list.Items)
	}
	if (&settingsExtension{prefs: p}).Commands(ctx) != nil {
		t.Error("offers the command without configurable extensions")
	}
}
