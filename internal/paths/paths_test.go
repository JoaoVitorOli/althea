package paths_test

import (
	"path/filepath"
	"testing"

	"github.com/JoaoVitorOli/althea/internal/paths"
)

func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func TestWindowsFollowsAppDataFolders(t *testing.T) {
	p, err := paths.Resolve("windows", env(map[string]string{
		"APPDATA":      `C:\Users\João\AppData\Roaming`,
		"LOCALAPPDATA": `C:\Users\João\AppData\Local`,
	}), "")
	if err != nil {
		t.Fatal(err)
	}
	roaming := filepath.Join(`C:\Users\João\AppData\Roaming`, "Althea")
	local := filepath.Join(`C:\Users\João\AppData\Local`, "Althea")
	want := paths.Paths{
		Config:   roaming,
		Settings: filepath.Join(roaming, "settings.toml"),
		Themes:   filepath.Join(roaming, "themes"),
		Scripts:  filepath.Join(roaming, "scripts"),
		Commands: filepath.Join(roaming, "ai-commands"),
		Data:     local,
		Database: filepath.Join(local, "althea.db"),
		Cache:    filepath.Join(local, "cache"),
		Logs:     filepath.Join(local, "logs"),
	}
	if p != want {
		t.Errorf("Resolve =\n%+v\nwant\n%+v", p, want)
	}
}

func TestLinuxFollowsXDGWithFallbacks(t *testing.T) {
	p, err := paths.Resolve("linux", env(map[string]string{
		"HOME":            "/home/ana",
		"XDG_CONFIG_HOME": "/home/ana/.dotfiles",
	}), "")
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string][2]string{
		"Config": {p.Config, "/home/ana/.dotfiles/althea"},
		"Data":   {p.Data, "/home/ana/.local/share/althea"},
		"Cache":  {p.Cache, "/home/ana/.cache/althea"},
		"Logs":   {p.Logs, "/home/ana/.local/state/althea/logs"},
	}
	for name, c := range checks {
		if filepath.ToSlash(c[0]) != c[1] {
			t.Errorf("%s = %s, want %s", name, c[0], c[1])
		}
	}
}

func TestPortableKeepsEverythingNextToTheExecutable(t *testing.T) {
	p, err := paths.Resolve("windows", env(map[string]string{"APPDATA": `C:\x`}), `D:\Tools\Althea`)
	if err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(`D:\Tools\Althea`, "data")
	if p.Config != data || p.Data != data || p.Logs != filepath.Join(data, "logs") {
		t.Errorf("portable paths = %+v, want all under %s", p, data)
	}
}

func TestResolveFailsWithoutAHome(t *testing.T) {
	if _, err := paths.Resolve("windows", env(nil), ""); err == nil {
		t.Error("Resolve without APPDATA = nil error")
	}
	if _, err := paths.Resolve("linux", env(nil), ""); err == nil {
		t.Error("Resolve without HOME = nil error")
	}
}
