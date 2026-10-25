package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/JoaoVitorOli/althea/internal/paths"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func portable(t *testing.T) paths.Paths {
	t.Helper()
	p, err := paths.Resolve("windows", func(string) string { return "" }, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestBootstrapPreparesAFirstRun(t *testing.T) {
	p := portable(t)
	c, err := bootstrap(context.Background(), p, "pt-BR")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.catalog.T("launcher.hint_close"); got != "Esc para fechar" {
		t.Errorf("system language pt-BR gives %q", got)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	for _, f := range []string{p.Settings, p.Database, filepath.Join(p.Logs, "althea.log")} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("first run did not create %s: %v", f, err)
		}
	}
	logged, _ := os.ReadFile(filepath.Join(p.Logs, "althea.log"))
	if !strings.Contains(string(logged), "started") {
		t.Errorf("log = %q, want a start record", logged)
	}
}

func TestBootstrapUsesTheConfiguredLanguage(t *testing.T) {
	p := portable(t)
	if err := os.MkdirAll(p.Config, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Settings, []byte("[general]\nlanguage = \"en\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := bootstrap(context.Background(), p, "pt-BR")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if got := c.catalog.T("launcher.hint_close"); got != "Esc to close" {
		t.Errorf("language = en in settings gives %q", got)
	}
}

func TestBootstrapLogsSettingsProblemsAndReloads(t *testing.T) {
	p := portable(t)
	if err := os.MkdirAll(p.Config, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Settings, []byte("[general]\nlanguage = \"fr\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := bootstrap(context.Background(), p, "en")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if s := c.Settings(); s.General.Language != "system" {
		t.Errorf("invalid language kept: %q", s.General.Language)
	}

	if err := os.WriteFile(p.Settings, []byte("[hotkey]\ntoggle = \"Win\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for c.Settings().Hotkey.Toggle != "Win" {
		if time.Now().After(deadline) {
			t.Fatal("settings not reloaded within 5s")
		}
		time.Sleep(20 * time.Millisecond)
	}

	logged, _ := os.ReadFile(filepath.Join(p.Logs, "althea.log"))
	if !strings.Contains(string(logged), "general.language") {
		t.Errorf("log = %q, want the settings problem", logged)
	}
}
