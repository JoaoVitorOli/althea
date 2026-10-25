package ui

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// Choices are what the onboarding asks for, written to the settings when it finishes.
type Choices struct {
	Hotkey       string // "Win" or an accelerator
	Theme        string // "system", "althea-dark" or "althea-light"
	StartAtLogin bool
	CheckUpdates bool
}

// Onboarding is the first run: a few steps that set the hotkey, the theme and the preferences
// worth asking about. It is seen once, so it is allowed more motion than anything else: each
// step's content arrives in a short stagger.
type Onboarding struct {
	T      func(key string, args ...any) string
	Motion Motion

	choices   Choices
	winKey    bool
	done      func(Choices)
	step      int
	recording bool
	recorder  ui.Handle
}

const (
	stepWelcome = iota
	stepHotkey
	stepTheme
	stepPreferences
	stepDone
	steps
)

// NewOnboarding starts the onboarding from the current choices. WinKey says whether the system
// can use the Win key alone; done receives the choices once the user finishes.
func NewOnboarding(t func(string, ...any) string, m Motion, initial Choices, winKey bool, done func(Choices)) *Onboarding {
	return &Onboarding{T: t, Motion: m, choices: initial, winKey: winKey, done: done}
}

// View builds the onboarding for one frame.
func (o *Onboarding) View(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(28, 36).Gap(20).Children(func() {
		o.dots(c)
		// A new element for each step, so its content arrives anew.
		ui.Column(c).Key(o.step).Grow(1).Gap(14).Children(func() {
			n := 0
			arrive := func(el ui.Element) {
				o.arrive(el, n)
				n++
			}
			switch o.step {
			case stepWelcome:
				arrive(ui.Text(c, o.T("onboarding.welcome.title")).FontSize(26).Bold().TextColor(t.Text))
				arrive(ui.Text(c, o.T("onboarding.welcome.body")).Wrap().MaxWidth(520).TextColor(t.TextMuted))
			case stepHotkey:
				arrive(ui.Text(c, o.T("onboarding.hotkey.title")).FontSize(22).Bold().TextColor(t.Text))
				arrive(ui.Text(c, o.T("onboarding.hotkey.body")).Wrap().MaxWidth(560).TextColor(t.TextMuted))
				arrive(o.hotkeyRecorder(c))
				arrive(ui.Row(c).Gap(8).Children(func() {
					o.option(c, "Alt+Space", o.choices.Hotkey == "Alt+Space", func() { o.choices.Hotkey = "Alt+Space" })
					if o.winKey {
						o.option(c, o.T("onboarding.hotkey.win"), o.choices.Hotkey == "Win", func() { o.choices.Hotkey = "Win" })
					}
				}))
			case stepTheme:
				arrive(ui.Text(c, o.T("onboarding.theme.title")).FontSize(22).Bold().TextColor(t.Text))
				arrive(ui.Row(c).Gap(12).Children(func() {
					o.themeCard(c, "system", o.T("onboarding.theme.system"))
					o.themeCard(c, "althea-dark", o.T("onboarding.theme.dark"))
					o.themeCard(c, "althea-light", o.T("onboarding.theme.light"))
				}))
			case stepPreferences:
				arrive(ui.Text(c, o.T("onboarding.preferences.title")).FontSize(22).Bold().TextColor(t.Text))
				arrive(o.toggleRow(c, &o.choices.StartAtLogin, o.T("onboarding.preferences.login"), o.T("onboarding.preferences.login_hint")))
				arrive(o.toggleRow(c, &o.choices.CheckUpdates, o.T("onboarding.preferences.updates"), o.T("onboarding.preferences.updates_hint")))
			case stepDone:
				arrive(ui.Text(c, o.T("onboarding.done.title")).FontSize(26).Bold().TextColor(t.Text))
				arrive(ui.Text(c, o.T("onboarding.done.body", "hotkey", o.choices.Hotkey)).TextColor(t.TextMuted))
			}
		})
		o.buttons(c)
	})
}

// arrive fades an element in and lifts it a little into place, after the elements before it.
func (o *Onboarding) arrive(el ui.Element, i int) {
	a := o.Motion.Enter()
	if o.Motion.Off() || a.Duration <= 0 {
		return // keyframes need time between their frames
	}
	opts := ui.KeyframeOptions{Delay: o.Motion.Stagger(i)}
	fade := el.Keyframes("fade", []ui.Keyframe{{Value: 0}, {At: a.Duration, Value: 1, Ease: strongEaseOut}}, opts)
	lift := el.Keyframes("lift", []ui.Keyframe{{Value: 8}, {At: a.Duration, Value: 0, Ease: strongEaseOut}}, opts)
	el.Opacity(fade).Translate(0, lift)
}

// dots show where the user is: the current step's dot is a short pill.
func (o *Onboarding) dots(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(6).Children(func() {
		for i := range steps {
			width, color := float32(6), t.TextMuted.Alpha(0.35)
			if i == o.step {
				width, color = 18, t.Accent
			}
			dot := ui.Box(c).Height(6).Radius(3)
			dot.Width(o.Motion.to(dot, "w", width, ui.Animation{Duration: o.Motion.Enter().Duration, Ease: strongEaseOut}))
			dot.Background(o.Motion.color(dot, "c", color, o.Motion.Hover()))
		}
	})
}

func (o *Onboarding) buttons(c *ui.Context) {
	ui.Row(c).Gap(10).Children(func() {
		ui.Box(c).Grow(1)
		if o.step > stepWelcome && o.step < stepDone {
			if o.button(c, o.T("onboarding.back"), false) {
				o.recording = false
				o.step--
			}
		}
		label := o.T("onboarding.continue")
		switch o.step {
		case stepWelcome:
			label = o.T("onboarding.start")
		case stepDone:
			label = o.T("onboarding.finish")
		}
		if o.button(c, label, true) {
			o.recording = false
			if o.step == stepDone {
				o.done(o.choices)
				return
			}
			o.step++
		}
	})
}

// button is a pressable button: its color follows the pointer and it gives a little as it is
// pressed. It reports a click.
func (o *Onboarding) button(c *ui.Context, label string, primary bool) bool {
	t := c.Theme()
	bg, fg := t.Surface, t.Text
	// Keyed by role: the primary button of one step must not lend its colors to the
	// secondary one of the next, which takes its place among the siblings.
	b := ui.Row(c).Key(primary).Padding(9, 18).Radius(t.Radius + 2).Label(label)
	switch {
	case primary && b.Hovered():
		bg, fg = t.AccentHover, t.AccentText
	case primary:
		bg, fg = t.Accent, t.AccentText
	case b.Hovered():
		bg = t.SurfaceHover
	}
	b.Background(o.Motion.color(b, "bg", bg, o.Motion.Hover()))
	o.Motion.pose(b, pressed(b.Pressed()), o.Motion.Press())
	b.Children(func() { ui.Text(c, label).FontWeight(600).TextColor(fg) })
	return b.Clicked()
}

// option is a pill choosing one value.
func (o *Onboarding) option(c *ui.Context, label string, chosen bool, choose func()) {
	t := c.Theme()
	bg, border := t.Surface, t.Border
	p := ui.Row(c).Padding(7, 14).Radius(999).BorderWidth(1).Label(label)
	if p.Hovered() {
		bg = t.SurfaceHover
	}
	if chosen {
		border = t.Accent
	}
	p.Background(o.Motion.color(p, "bg", bg, o.Motion.Hover())).BorderColor(o.Motion.color(p, "border", border, o.Motion.Hover()))
	o.Motion.pose(p, pressed(p.Pressed()), o.Motion.Press())
	p.Children(func() { ui.Text(c, label).TextColor(t.Text) })
	if p.Clicked() {
		o.recording = false
		choose()
	}
}

// hotkeyRecorder shows the hotkey as keycaps. Clicked, it listens for the next combination
// pressed; Escape stops it, and a key without a modifier is not taken.
func (o *Onboarding) hotkeyRecorder(c *ui.Context) ui.Element {
	t := c.Theme()
	border := t.Border
	if o.recording {
		border = t.Accent
	}
	rec := ui.Row(c).Height(64).Padding(0, 18).Gap(8).AlignItems(ui.Center).Radius(t.Radius + 4).
		BorderWidth(1).Background(t.Surface).Focusable().Bind(&o.recorder).Label(o.T("onboarding.hotkey.record"))
	rec.BorderColor(o.Motion.color(rec, "border", border, o.Motion.Hover()))
	rec.HandleInput(func(ev ui.InputEvent) bool {
		if !o.recording || ev.Kind != ui.InputKeyDown {
			return false
		}
		if ev.Key == ui.KeyEscape {
			o.recording = false
			return true
		}
		if acc, ok := Accelerator(ev.Mods, ev.Key); ok {
			o.choices.Hotkey, o.recording = acc, false
		}
		return true // while recording, every key is the recorder's
	})
	if rec.Clicked() {
		o.recording = true
		o.recorder.Focus()
	}
	rec.Children(func() {
		if o.recording {
			ui.Text(c, o.T("onboarding.hotkey.listening")).TextColor(t.Accent)
			return
		}
		for _, k := range strings.Split(o.choices.Hotkey, "+") {
			keycap(c, k)
		}
		ui.Box(c).Grow(1)
		ui.Text(c, o.T("onboarding.hotkey.record")).FontSize(12).TextColor(t.TextMuted)
	})
	return rec
}

// keycap draws one key of a shortcut, as on a keyboard.
func keycap(c *ui.Context, name string) {
	t := c.Theme()
	ui.Row(c).Padding(6, 12).Radius(6).Background(t.SurfaceHover).BorderWidth(1, 1, 2, 1).BorderColor(t.Border).
		Children(func() { ui.Text(c, name).FontWeight(600).TextColor(t.Text) })
}

// themeCard previews a theme's choice; the chosen one has an accent border.
func (o *Onboarding) themeCard(c *ui.Context, name, label string) {
	t := c.Theme()
	chosen := o.choices.Theme == name
	border := t.Border
	if chosen {
		border = t.Accent
	}
	card := ui.Column(c).Grow(1).Gap(10).Padding(10).Radius(t.Radius + 4).BorderWidth(2).Label(label)
	bg := t.Surface
	if card.Hovered() {
		bg = t.SurfaceHover
	}
	card.Background(o.Motion.color(card, "bg", bg, o.Motion.Hover())).BorderColor(o.Motion.color(card, "border", border, o.Motion.Hover()))
	o.Motion.pose(card, pressed(card.Pressed()), o.Motion.Press())
	card.Children(func() {
		preview := map[string][2]ui.Color{
			"althea-dark":  {ui.Hex("#15171f"), ui.Hex("#7b8cf0")},
			"althea-light": {ui.Hex("#f7f8fb"), ui.Hex("#4f5fd8")},
		}
		ui.Row(c).Height(64).Radius(t.Radius).Clip().Children(func() {
			if name == "system" {
				for _, n := range []string{"althea-light", "althea-dark"} {
					swatch(c, preview[n])
				}
				return
			}
			swatch(c, preview[name])
		})
		ui.Text(c, label).TextColor(t.Text)
	})
	if card.Clicked() {
		o.choices.Theme = name
	}
}

// swatch is a tiny launcher: a background with an accent search bar.
func swatch(c *ui.Context, colors [2]ui.Color) {
	ui.Column(c).Grow(1).Padding(10).Background(colors[0]).Children(func() {
		ui.Box(c).Height(8).Width(48).Radius(4).Background(colors[1])
	})
}

// toggleRow is a setting with a switch; the whole row toggles it, and the knob slides.
func (o *Onboarding) toggleRow(c *ui.Context, on *bool, label, hint string) ui.Element {
	t := c.Theme()
	row := ui.Row(c).Padding(12, 14).Gap(14).AlignItems(ui.Center).Radius(t.Radius + 2).Label(label)
	bg := t.Surface
	if row.Hovered() {
		bg = t.SurfaceHover
	}
	row.Background(o.Motion.color(row, "bg", bg, o.Motion.Hover()))
	if row.Clicked() {
		*on = !*on
	}
	row.Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, label).TextColor(t.Text)
			ui.Text(c, hint).FontSize(12).Wrap().TextColor(t.TextMuted)
		})
		track, x := t.TextMuted.Alpha(0.35), float32(2)
		if *on {
			track, x = t.Accent, 18
		}
		sw := ui.Box(c).Size(38, 22).Radius(11)
		sw.Background(o.Motion.color(sw, "track", track, o.Motion.Hover()))
		knobAnim := ui.Animation{Spring: ui.SpringOptions{Duration: time.Duration(float32(260*time.Millisecond) * o.Motion.Scale)}}
		sw.Children(func() {
			knob := ui.Box(c).Size(18, 18).Radius(9).Background(ui.Hex("#ffffff")).Margin(2, 0)
			knob.Translate(o.Motion.to(knob, "x", x, knobAnim), 0)
		})
	})
	return row
}
