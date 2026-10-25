package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/JoaoVitorOli/althea/internal/theme"
)

func TestToUIMapsTokens(t *testing.T) {
	th, _, err := theme.Load("althea-light", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	u := ToUI(th)
	if u.Dark {
		t.Error("althea-light maps to a dark ui.Theme")
	}
	checks := []struct {
		name string
		got  ui.Color
		want theme.Color
	}{
		{"Background", u.Background, th.Colors.Background},
		{"Accent", u.Accent, th.Colors.Accent},
		{"TextMuted", u.TextMuted, th.Colors.TextMuted},
		{"Focus", u.Focus, th.Colors.Focus},
	}
	for _, c := range checks {
		if c.got != (ui.Color{R: c.want.R, G: c.want.G, B: c.want.B, A: c.want.A}) {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if u.Radius != th.Shape.ControlRadius || u.FontSize != th.Font.Size || u.Spacing != th.Space.Gap {
		t.Errorf("metrics: radius %v, font size %v, spacing %v", u.Radius, u.FontSize, u.Spacing)
	}
}
