package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/JoaoVitorOli/althea/internal/i18n"
)

// onboarding returns an onboarding in English and where the choices it finishes with land.
func onboarding(t *testing.T, winKey bool) (o *Onboarding, finished *[]Choices) {
	t.Helper()
	catalog := i18n.New("en")
	if err := catalog.Add("", Messages); err != nil {
		t.Fatal(err)
	}
	var done []Choices
	o = NewOnboarding(catalog.T, Motion{Scale: 1}, Choices{
		Hotkey: "Alt+Space", Theme: "system", StartAtLogin: true,
	}, winKey, func(c Choices) { done = append(done, c) })
	return o, &done
}

func click(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatalf("click %q: %v; texts %q", label, err, tt.Texts())
	}
}

func TestOnboardingWalksThroughEveryStep(t *testing.T) {
	o, done := onboarding(t, true)
	tt := ui.NewTester(o.View, 720, 420)

	click(t, tt, "Get started")
	click(t, tt, "Win key")
	click(t, tt, "Continue")
	click(t, tt, "Dark")
	click(t, tt, "Continue")
	click(t, tt, "Check for updates")
	click(t, tt, "Continue")
	if !tt.HasText("Press Win anywhere to open Althea.") {
		t.Errorf("last step does not show the chosen hotkey; texts %q", tt.Texts())
	}
	click(t, tt, "Start using Althea")

	want := Choices{Hotkey: "Win", Theme: "althea-dark", StartAtLogin: true, CheckUpdates: true}
	if len(*done) != 1 || (*done)[0] != want {
		t.Errorf("finished with %+v, want once %+v", *done, want)
	}
}

func TestOnboardingRecordsAHotkey(t *testing.T) {
	o, done := onboarding(t, true)
	tt := ui.NewTester(o.View, 720, 420)
	click(t, tt, "Get started")

	click(t, tt, "Record a hotkey")
	if !tt.HasText("Press the keys…") {
		t.Fatalf("recorder does not ask for keys; texts %q", tt.Texts())
	}
	tt.Key(ui.Ctrl, ui.KeySpace) // a modifier alone would not end it; this does
	if !tt.HasText("Ctrl") || !tt.HasText("Space") {
		t.Errorf("recorded hotkey not shown as keys; texts %q", tt.Texts())
	}

	click(t, tt, "Continue")
	click(t, tt, "Continue")
	click(t, tt, "Continue")
	click(t, tt, "Start using Althea")
	if len(*done) != 1 || (*done)[0].Hotkey != "Ctrl+Space" {
		t.Errorf("finished with %+v, want hotkey Ctrl+Space", *done)
	}
}

func TestOnboardingRecorderIgnoresAKeyAlone(t *testing.T) {
	o, _ := onboarding(t, true)
	tt := ui.NewTester(o.View, 720, 420)
	click(t, tt, "Get started")
	click(t, tt, "Record a hotkey")
	tt.Key(0, ui.KeyA)
	if !tt.HasText("Press the keys…") || o.choices.Hotkey != "Alt+Space" {
		t.Errorf("a plain key was taken as a hotkey: %q", o.choices.Hotkey)
	}
	tt.Key(0, ui.KeyEscape)
	if tt.HasText("Press the keys…") {
		t.Error("Escape does not stop recording")
	}
}

func TestOnboardingHidesTheWinKeyWhereItCannotWork(t *testing.T) {
	o, _ := onboarding(t, false)
	tt := ui.NewTester(o.View, 720, 420)
	click(t, tt, "Get started")
	if tt.HasText("Win key") {
		t.Error("offers the Win key on a system without the keyboard hook")
	}
}

func TestOnboardingGoesBack(t *testing.T) {
	o, _ := onboarding(t, true)
	tt := ui.NewTester(o.View, 720, 420)
	click(t, tt, "Get started")
	click(t, tt, "Continue")
	click(t, tt, "Back")
	if !tt.HasText("Choose your hotkey") {
		t.Errorf("Back does not return to the hotkey step; texts %q", tt.Texts())
	}
}

func TestOnboardingInPortuguese(t *testing.T) {
	catalog := i18n.New("pt-BR")
	if err := catalog.Add("", Messages); err != nil {
		t.Fatal(err)
	}
	o := NewOnboarding(catalog.T, Motion{Scale: 1}, Choices{Hotkey: "Alt+Space", Theme: "system"}, true, func(Choices) {})
	tt := ui.NewTester(o.View, 720, 420)
	click(t, tt, "Começar")
	if !tt.HasText("Escolha seu atalho") {
		t.Errorf("texts %q", tt.Texts())
	}
}

func TestOnboardingWithMotionOff(t *testing.T) {
	catalog := i18n.New("en")
	if err := catalog.Add("", Messages); err != nil {
		t.Fatal(err)
	}
	o := NewOnboarding(catalog.T, Motion{Scale: 0}, Choices{Hotkey: "Alt+Space", Theme: "system"}, true, func(Choices) {})
	tt := ui.NewTester(o.View, 720, 420)
	click(t, tt, "Get started")
	if !tt.HasText("Choose your hotkey") {
		t.Errorf("texts %q", tt.Texts())
	}
}
