package app

import (
	"context"
	"testing"

	"github.com/JoaoVitorOli/althea/internal/config"
	"github.com/JoaoVitorOli/althea/internal/ui"
)

func TestOnboardingIsAskedOnceAndSavesTheChoices(t *testing.T) {
	ctx := context.Background()
	p := portable(t)
	c, err := bootstrap(ctx, p, "en")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	if !c.needsOnboarding(ctx) {
		t.Fatal("a first run does not ask for the onboarding")
	}
	choices := ui.Choices{Hotkey: "Win", Theme: "althea-light", StartAtLogin: false, CheckUpdates: true}
	if err := c.finishOnboarding(ctx, choices); err != nil {
		t.Fatal(err)
	}
	if c.needsOnboarding(ctx) {
		t.Error("the onboarding is asked again after it finished")
	}

	s, problems, err := config.Load(p.Settings)
	if err != nil || len(problems) != 0 {
		t.Fatalf("settings: %v, %v", problems, err)
	}
	if s.Hotkey.Toggle != "Win" || s.Theme.Name != "althea-light" || s.General.StartAtLogin || !s.General.CheckUpdates {
		t.Errorf("settings after the onboarding = %+v", s)
	}
}

func TestChoicesStartFromTheSettings(t *testing.T) {
	s := config.Defaults()
	s.Hotkey.Toggle = "Ctrl+Space"
	if got := choicesOf(s); got != (ui.Choices{Hotkey: "Ctrl+Space", Theme: "system", StartAtLogin: true}) {
		t.Errorf("choicesOf = %+v", got)
	}
}
