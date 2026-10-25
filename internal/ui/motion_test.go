package ui

import (
	"testing"
	"time"

	"github.com/JoaoVitorOli/althea/internal/theme"
)

func TestMotionFollowsTheThemeScale(t *testing.T) {
	normal := Motion{Scale: 1}
	if d := normal.Press().Duration; d != 120*time.Millisecond {
		t.Errorf("press = %v, want 120ms", d)
	}
	if d := normal.Enter().Duration; d != 220*time.Millisecond {
		t.Errorf("enter = %v, want 220ms", d)
	}
	if d := (Motion{Scale: 0.5}).Enter().Duration; d != 110*time.Millisecond {
		t.Errorf("enter at half scale = %v, want 110ms", d)
	}
	if d := normal.Stagger(3); d != 120*time.Millisecond {
		t.Errorf("stagger of the 4th item = %v, want 120ms", d)
	}

	off := Motion{Scale: 0}
	if off.Enter().Duration != 0 || off.Press().Duration != 0 || off.Stagger(5) != 0 || !off.Off() {
		t.Error("motion scale 0 still animates")
	}
}

func TestMotionOfATheme(t *testing.T) {
	th, _, _ := theme.Load("althea-dark", t.TempDir())
	th.Motion.Scale = 2
	if got := MotionOf(th).Hover().Duration; got != 240*time.Millisecond {
		t.Errorf("hover at scale 2 = %v", got)
	}
}
