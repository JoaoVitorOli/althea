package config_test

import (
	"maps"
	"os"
	"slices"
	"testing"

	"github.com/JoaoVitorOli/althea/internal/config"
)

func TestLoadReadsCommandHotkeysAndReportsWrongOnes(t *testing.T) {
	path := write(t, `[hotkey]
toggle = "Alt+Space"

[hotkey.commands]
"althea.windows/left" = "Ctrl+Alt+Left"
"althea.windows/right" = "Win"
"althea.system/lock" = "L"
"althea.system/sleep" = 3
`)
	s, problems, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"althea.windows/left": "Ctrl+Alt+Left"}
	if !maps.Equal(s.Hotkey.Commands, want) {
		t.Errorf("commands = %v, want %v", s.Hotkey.Commands, want)
	}
	const message = `must be a combination such as "Ctrl+Alt+Left"`
	wantProblems := []config.Problem{
		{Line: 7, Key: "hotkey.commands.althea.system/lock", Message: message},
		{Line: 8, Key: "hotkey.commands.althea.system/sleep", Message: message},
		{Line: 6, Key: "hotkey.commands.althea.windows/right", Message: message},
	}
	if !slices.Equal(problems, wantProblems) {
		t.Errorf("problems = %v, want %v", problems, wantProblems)
	}
}

func TestSetPathAddsCommandHotkeysThatLoadBack(t *testing.T) {
	path := write(t, "# mine\n[hotkey]\ntoggle = \"Alt+Space\"\n")
	for key, shortcut := range map[string]string{"althea.windows/left": "Ctrl+Alt+Left", "althea.windows/right": "Ctrl+Alt+Right"} {
		if err := config.SetPath(path, []string{"hotkey", "commands", key}, shortcut); err != nil {
			t.Fatal(err)
		}
	}
	if err := config.Unset(path, []string{"hotkey", "commands", "althea.windows/left"}); err != nil {
		t.Fatal(err)
	}
	s, problems, err := config.Load(path)
	if err != nil || len(problems) != 0 {
		data, _ := os.ReadFile(path)
		t.Fatalf("Load: %v, %v\n%s", problems, err, data)
	}
	want := map[string]string{"althea.windows/right": "Ctrl+Alt+Right"}
	if !maps.Equal(s.Hotkey.Commands, want) || s.Hotkey.Toggle != "Alt+Space" {
		t.Errorf("hotkeys = %+v, want commands %v", s.Hotkey, want)
	}
}
