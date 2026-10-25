package shell

import (
	"testing"
	"time"

	"github.com/egoist/mygo"

	"github.com/JoaoVitorOli/althea/internal/theme"
)

func TestPosition(t *testing.T) {
	tests := []struct {
		name         string
		area         mygo.Rectangle
		wantX, wantY int
	}{
		{"primary 1080p", mygo.Rectangle{Width: 1920, Height: 1040}, 600, 260},
		{"display to the right", mygo.Rectangle{X: 1920, Width: 2560, Height: 1400}, 1920 + 920, 350},
		{"taskbar on top", mygo.Rectangle{Y: 48, Width: 1920, Height: 1032}, 600, 48 + 258},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			x, y := Position(tc.area, Width)
			if x != tc.wantX || y != tc.wantY {
				t.Errorf("Position = (%d, %d), want (%d, %d)", x, y, tc.wantX, tc.wantY)
			}
		})
	}
}

func TestShouldReset(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		hiddenAt time.Time
		after    time.Duration
		want     bool
	}{
		{"first show", time.Time{}, 90 * time.Second, true},
		{"back within the delay", now.Add(-30 * time.Second), 90 * time.Second, false},
		{"back after the delay", now.Add(-90 * time.Second), 90 * time.Second, true},
		{"zero always resets", now.Add(-time.Second), 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldReset(tc.hiddenAt, now, tc.after); got != tc.want {
				t.Errorf("ShouldReset = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestVibrancyOfMaterials(t *testing.T) {
	for kind, want := range map[theme.MaterialKind]mygo.Vibrancy{
		theme.Acrylic: mygo.VibrancyAcrylic,
		theme.Mica:    mygo.VibrancyMica,
		theme.Glass:   mygo.VibrancyAcrylic, // the system blur under Althea's glass
		theme.Solid:   mygo.VibrancyNone,
	} {
		if got := VibrancyOf(kind); got != want {
			t.Errorf("VibrancyOf(%s) = %q, want %q", kind, got, want)
		}
	}
}
