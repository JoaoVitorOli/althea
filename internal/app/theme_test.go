package app

import (
	"testing"

	"github.com/JoaoVitorOli/althea/internal/config"
)

func TestThemeName(t *testing.T) {
	s := config.Theme{Name: "system", Dark: "midnight", Light: "paper"}
	if got := themeName(s, true); got != "midnight" {
		t.Errorf("system in dark mode = %q", got)
	}
	if got := themeName(s, false); got != "paper" {
		t.Errorf("system in light mode = %q", got)
	}
	s.Name = "solarized"
	if got := themeName(s, true); got != "solarized" {
		t.Errorf("a named theme = %q", got)
	}
}
