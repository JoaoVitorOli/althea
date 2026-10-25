package ui

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/JoaoVitorOli/althea/internal/theme"
)

// What moves in Althea, and what does not.
//
// The launcher shows and hides, and the keyboard moves through results, hundreds of times a
// day: none of that animates. Motion is kept for feedback the user would
// otherwise miss (a press, a hover, a toast) and for what is rare (onboarding). MyGo goes to
// the target at once when the system asks for reduced motion; a theme's motion scale of 0
// does the same everywhere.

// strongEaseOut starts fast, for immediate feedback, and settles gently.
var strongEaseOut = ui.CubicBezier(0.23, 1, 0.32, 1)

// Motion gives the durations and curves of the theme's motion scale.
type Motion struct{ Scale float32 }

// MotionOf returns a theme's motion.
func MotionOf(t theme.Theme) Motion { return Motion{Scale: t.Motion.Scale} }

func (m Motion) scaled(d time.Duration) time.Duration {
	return time.Duration(float32(d) * max(m.Scale, 0))
}

// Off reports whether the theme turns motion off.
func (m Motion) Off() bool { return m.Scale <= 0 }

// Press is how a pressed element shrinks and comes back.
func (m Motion) Press() ui.Animation {
	return ui.Animation{Duration: m.scaled(120 * time.Millisecond), Ease: strongEaseOut}
}

// Hover is how hover colors fade.
func (m Motion) Hover() ui.Animation {
	return ui.Animation{Duration: m.scaled(120 * time.Millisecond), Ease: ui.EaseOut}
}

// Enter is how content arrives, as an onboarding step.
func (m Motion) Enter() ui.Animation {
	return ui.Animation{Duration: m.scaled(220 * time.Millisecond), Ease: strongEaseOut}
}

// Stagger is the delay of the i-th of several elements arriving together.
func (m Motion) Stagger(i int) time.Duration {
	return m.scaled(time.Duration(i) * 40 * time.Millisecond)
}

// The helpers below animate through MyGo unless motion is off, when they go to the target at
// once: a zero Animation is a spring in MyGo, not an instant change.

// to animates a value of el towards target.
func (m Motion) to(el ui.Element, key any, target float32, a ui.Animation) float32 {
	if m.Off() {
		return target
	}
	return el.AnimateTo(key, target, a)
}

// color animates a color of el towards target.
func (m Motion) color(el ui.Element, key any, target ui.Color, a ui.Animation) ui.Color {
	if m.Off() {
		return target
	}
	return el.AnimateColor(key, target, a)
}

// pose moves el into a pose.
func (m Motion) pose(el ui.Element, p ui.Pose, a ui.Animation) {
	if !m.Off() {
		el.AnimatePose(p, a)
	}
}

// pressed is the pose of a pressable element: a little smaller while pressed, never from or to
// nothing.
func pressed(down bool) ui.Pose {
	if down {
		return ui.Pose{Scale: 0.97}
	}
	return ui.Pose{}
}
