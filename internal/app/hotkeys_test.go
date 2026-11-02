package app

import (
	"bytes"
	"io"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/JoaoVitorOli/althea/internal/config"
	"github.com/JoaoVitorOli/althea/internal/registry"
	"github.com/JoaoVitorOli/althea/sdk"
	"github.com/JoaoVitorOli/althea/sdk/sdktest"
)

func TestWantedHotkeysGiveEachCombinationToOne(t *testing.T) {
	var logged bytes.Buffer
	got := wantedHotkeys(config.Hotkey{
		Toggle: "Alt+Space",
		Commands: map[string]string{
			"b/two":   "Shift+Ctrl+K", // the same combination as a/one, spelled otherwise
			"a/one":   "ctrl+shift+k",
			"c/three": "alt+space", // the launcher's
			"d/four":  "Ctrl+Alt+D",
		},
	}, slog.New(slog.NewTextHandler(&logged, nil)))

	want := map[string]string{"a/one": "Ctrl+Shift+K", "d/four": "Ctrl+Alt+D"}
	if !maps.Equal(got, want) {
		t.Errorf("wanted %v, want %v", got, want)
	}
	for _, line := range []string{"command=b/two taken_by=a/one", "command=c/three taken_by=launcher"} {
		if !strings.Contains(logged.String(), line) {
			t.Errorf("the log lacks %q:\n%s", line, logged.String())
		}
	}
}

// fakeBinder binds as bindCommandHotkey does, and records what is bound.
type fakeBinder struct{ bound map[string]func() }

func (f *fakeBinder) bind(shortcut string, run func()) func() {
	f.bound[shortcut] = run
	return func() { delete(f.bound, shortcut) }
}

func TestCommandHotkeysFollowTheSettings(t *testing.T) {
	binder := &fakeBinder{bound: map[string]func(){}}
	var ran []string
	h := commandHotkeys{bind: binder.bind, run: func(command string) { ran = append(ran, command) }}

	h.sync(map[string]string{"a/one": "Ctrl+1", "a/two": "Ctrl+2"})
	binder.bound["Ctrl+2"]()
	if !slices.Equal(ran, []string{"a/two"}) {
		t.Errorf("ran %v", ran)
	}

	// A reload: one moved, one removed, one added.
	h.sync(map[string]string{"a/one": "Ctrl+3", "a/three": "Ctrl+4"})
	if got := slices.Sorted(maps.Keys(binder.bound)); !slices.Equal(got, []string{"Ctrl+3", "Ctrl+4"}) {
		t.Errorf("bound %v", got)
	}

	h.release()
	if len(binder.bound) != 0 {
		t.Errorf("still bound after release: %v", binder.bound)
	}
}

func hotkeyResult(confirm string, run func(sdk.Context) error) registry.Result {
	return registry.Result{
		Key: "althea.system/restart", Extension: "althea.system", Command: true,
		Item: sdk.Item{ID: "restart", Title: "Restart", Actions: []sdk.Action{{Title: "Restart", Confirm: confirm, Run: run}}},
	}
}

// fakeRunner is a commandRunner over one command, recording what it did.
type fakeRunner struct {
	commandRunner
	ctx       *sdktest.Context
	performed []string
	noted     int
	refreshes int
}

func newFakeRunner(t *testing.T, res *registry.Result, log *bytes.Buffer) *fakeRunner {
	t.Helper()
	f := &fakeRunner{ctx: sdktest.New(t, &settingsExtension{}, "en")}
	f.commandRunner = commandRunner{
		note: func() { f.noted++ },
		find: func(command string) (registry.Result, bool) {
			if res == nil || res.Key != command {
				return registry.Result{}, false
			}
			return *res, true
		},
		refresh: func() { f.refreshes++ },
		context: func(string) sdk.Context { return f.ctx },
		perform: func(res registry.Result, _ sdk.Action) { f.performed = append(f.performed, res.Key) },
		log:     slog.New(slog.NewTextHandler(log, nil)),
	}
	return f
}

func TestCommandHotkeyRunsTheCommandWithoutTheLauncher(t *testing.T) {
	res := hotkeyResult("", nil)
	f := newFakeRunner(t, &res, &bytes.Buffer{})
	f.run(res.Key)
	if !slices.Equal(f.performed, []string{res.Key}) || f.noted != 1 || f.refreshes != 0 {
		t.Errorf("performed %v, noted %d, refreshes %d", f.performed, f.noted, f.refreshes)
	}
	if len(f.ctx.Pushed) != 0 {
		t.Errorf("pushed %v, which would show the launcher", f.ctx.Pushed)
	}
}

func TestCommandHotkeyAsksTheConfirmationFirst(t *testing.T) {
	res := hotkeyResult("Press Enter again to restart", nil)
	f := newFakeRunner(t, &res, &bytes.Buffer{})
	f.run(res.Key)
	if len(f.performed) != 0 || len(f.ctx.Pushed) != 1 {
		t.Fatalf("performed %v, pushed %v", f.performed, f.ctx.Pushed)
	}
	list := f.ctx.Pushed[0].(*sdk.List)
	act := list.Items[0].Actions[0]
	if len(list.Items) != 1 || act.Confirm != "Press Enter again to restart" {
		t.Fatalf("asks with %+v", list.Items)
	}
	if err := act.Run(f.ctx); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.performed, []string{res.Key}) || f.ctx.Popped != 1 {
		t.Errorf("after confirming: performed %v, popped %d", f.performed, f.ctx.Popped)
	}
}

func TestCommandHotkeyLooksAgainThenLogsAnUnknownCommand(t *testing.T) {
	var logged bytes.Buffer
	f := newFakeRunner(t, nil, &logged)
	f.run("gone/command")
	if f.refreshes != 1 || len(f.performed) != 0 {
		t.Errorf("refreshes %d, performed %v", f.refreshes, f.performed)
	}
	if !strings.Contains(logged.String(), "command=gone/command") {
		t.Errorf("not logged:\n%s", logged.String())
	}
}

// hotkeySettings is the settings extension over a settings file holding text.
func hotkeySettings(t *testing.T, text string) *settingsExtension {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.toml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	command := func(ext, id, title string) registry.Result {
		return registry.Result{Key: ext + "/" + id, Extension: ext, Command: true, Item: sdk.Item{ID: id, Title: title}}
	}
	return &settingsExtension{
		prefs: newPreferences(path, &memVault{m: map[string]string{}}, slog.New(slog.NewTextHandler(io.Discard, nil))),
		commands: func() []registry.Result {
			return []registry.Result{
				command("althea.windows", "right", "Right Half"),
				command("althea.calculator", "calc", "Calculator"),
				command("althea.windows", "left", "Left Half"),
			}
		},
		titles: map[string]string{"althea.windows": "Window Management", "althea.calculator": "Calculator"},
	}
}

func TestHotkeyFormWritesTheSettingsAndShowsThemAtOnce(t *testing.T) {
	s := hotkeySettings(t, "# mine\n[hotkey]\ntoggle = \"Alt+Space\"\n")
	ctx := sdktest.New(t, s, "pt-BR")

	cmds := s.Commands(ctx)
	if len(cmds) != 1 || cmds[0].ID != "command-hotkeys" || cmds[0].Title != "Atalhos e aliases" {
		t.Fatalf("commands %+v", cmds)
	}
	if err := cmds[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	list := ctx.Pushed[0].(*sdk.List)
	order := make([]string, len(list.Items))
	for i, it := range list.Items {
		order[i] = it.ID
	}
	if want := []string{"althea.calculator/calc", "althea.windows/left", "althea.windows/right"}; !slices.Equal(order, want) {
		t.Fatalf("listed %v, want %v", order, want)
	}
	left := list.Items[1]
	if left.Section != "Window Management" || left.Accessory != "" || len(left.Actions) != 1 {
		t.Fatalf("item %+v", left)
	}

	if err := left.Actions[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	form := ctx.Pushed[1].(*sdk.Form)
	if err := form.Submit(ctx, sdk.Values{"hotkey": "alt+ctrl+left", "alias": " esq "}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(s.prefs.path)
	for _, want := range []string{"# mine", "[hotkey.commands]", `"althea.windows/left" = "Ctrl+Alt+Left"`, "[aliases]", `"althea.windows/left" = "esq"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("settings lack %q:\n%s", want, data)
		}
	}
	if saved, problems, err := config.Load(s.prefs.path); err != nil || len(problems) != 0 || saved.Hotkey.Commands["althea.windows/left"] != "Ctrl+Alt+Left" {
		t.Errorf("read back %+v, %v, %v", saved.Hotkey, problems, err)
	}

	// The list takes the place of the old one and of the form, with the hotkey, before the
	// watcher reports it.
	list = ctx.Pushed[2].(*sdk.List)
	left = list.Items[1]
	if ctx.Popped != 2 || left.Accessory != "esq · Ctrl+Alt+Left" || len(left.Actions) != 2 {
		t.Fatalf("popped %d, item %+v", ctx.Popped, left)
	}

	// Removing the hotkey leaves the file without it, and with the alias.
	if err := left.Actions[1].Run(ctx); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(s.prefs.path)
	if strings.Contains(string(data), "Ctrl+Alt+Left") || !strings.Contains(string(data), "esq") {
		t.Errorf("settings after removing the hotkey:\n%s", data)
	}
	left = ctx.Pushed[3].(*sdk.List).Items[1]
	if left.Accessory != "esq" || len(left.Actions) != 1 {
		t.Errorf("item after removing the hotkey: %+v", left)
	}

	// An emptied field removes the alias.
	if err := s.hotkeyForm(ctx, registry.Result{Key: "althea.windows/left"}).Submit(ctx, sdk.Values{"hotkey": "", "alias": ""}); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(s.prefs.path)
	if strings.Contains(string(data), "esq") {
		t.Errorf("alias still in the settings:\n%s", data)
	}
}

func TestWantedAliasesGiveEachOneToOneCommand(t *testing.T) {
	var logged bytes.Buffer
	got := wantedAliases(map[string]string{"b/two": "CAFÉ", "a/one": "cafe", "c/three": "cl"}, slog.New(slog.NewTextHandler(&logged, nil)))
	want := map[string]string{"a/one": "cafe", "c/three": "cl"}
	if !maps.Equal(got, want) {
		t.Errorf("wanted %v, want %v", got, want)
	}
	if !strings.Contains(logged.String(), "command=b/two taken_by=a/one") {
		t.Errorf("not logged:\n%s", logged.String())
	}
}

func TestAliasFieldTellsWhatIsWrong(t *testing.T) {
	s := hotkeySettings(t, "[aliases]\n\"althea.calculator/calc\" = \"ca\"\n\"althea.windows/left\" = \"esq\"\n")
	ctx := sdktest.New(t, s, "en")
	field := s.hotkeyForm(ctx, registry.Result{Key: "althea.windows/left", Item: sdk.Item{Title: "Left Half"}}).Fields[1]
	if field.Default != "esq" {
		t.Errorf("starts with %q", field.Default)
	}
	for _, tc := range []struct{ text, want string }{
		{"", ""},
		{"esq", ""}, // its own
		{"le", ""},
		{"two words", "Must be one word, as cl."},
		{"CÁ", "Already used by Calculator."},
	} {
		if got := field.Validate(tc.text); got != tc.want {
			t.Errorf("Validate(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

func TestHotkeyFormTellsWhatIsWrong(t *testing.T) {
	s := hotkeySettings(t, `[hotkey]
toggle = "Alt+Space"
[hotkey.commands]
"althea.calculator/calc" = "Ctrl+Alt+C"
"althea.windows/left" = "Ctrl+Alt+Left"
`)
	ctx := sdktest.New(t, s, "en")
	res := registry.Result{Key: "althea.windows/left", Extension: "althea.windows", Item: sdk.Item{Title: "Left Half"}}
	field := s.hotkeyForm(ctx, res).Fields[0]
	if field.Default != "Ctrl+Alt+Left" {
		t.Errorf("starts with %q", field.Default)
	}
	for _, tc := range []struct{ text, want string }{
		{"", ""},
		{"Ctrl+Alt+Left", ""}, // its own
		{"Ctrl+Alt+R", ""},
		{"K", "Must be modifiers and one key, as Ctrl+Shift+V."},
		{"Win", "Must be modifiers and one key, as Ctrl+Shift+V."},
		{"Ctrl+Q+P", "Must be modifiers and one key, as Ctrl+Shift+V."},
		{"alt+space", "Already opens Althea."},
		{"Alt+Ctrl+C", "Already used by Calculator."},
	} {
		if got := field.Validate(tc.text); got != tc.want {
			t.Errorf("Validate(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}
