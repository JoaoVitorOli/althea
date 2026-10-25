package shell

import "testing"

func TestPosition(t *testing.T) {
	tests := []struct {
		name               string
		areaX, areaY, w, h int
		wantX, wantY       int
	}{
		{"primary 1080p", 0, 0, 1920, 1040, 600, 260},
		{"display to the right", 1920, 0, 2560, 1400, 1920 + 920, 350},
		{"taskbar on top", 0, 48, 1920, 1032, 600, 48 + 258},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			x, y := Position(tc.areaX, tc.areaY, tc.w, tc.h)
			if x != tc.wantX || y != tc.wantY {
				t.Errorf("Position = (%d, %d), want (%d, %d)", x, y, tc.wantX, tc.wantY)
			}
		})
	}
}
