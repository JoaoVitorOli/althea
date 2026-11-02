package windows_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JoaoVitorOli/althea/extensions/windows"
	"github.com/JoaoVitorOli/althea/sdk"
	"github.com/JoaoVitorOli/althea/sdk/sdktest"
)

// screen is a fake system: windows on displays, and a log of what was asked of it.
type screen struct {
	mu        sync.Mutex
	displays  []windows.Display
	target    uintptr
	open      map[uintptr]*windows.Window
	order     []uintptr // topmost first
	placedLog []uintptr
	focused   []uintptr
	launched  []string
	// opens says which window an app gets when launched.
	opens map[string]windows.Window
	bound map[string]func()
}

var work = windows.Rect{X: 0, Y: 0, W: 1920, H: 1040}

func newScreen() *screen {
	s := &screen{
		displays: []windows.Display{{ID: "one", Work: work}},
		open:     map[uintptr]*windows.Window{}, opens: map[string]windows.Window{}, bound: map[string]func(){},
	}
	s.add(windows.Window{Handle: 1, App: "editor.exe", Title: "Editor", Frame: windows.Rect{X: 100, Y: 100, W: 800, H: 600}})
	s.target = 1
	return s
}

func (s *screen) add(w windows.Window) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.open[w.Handle] = &w
	s.order = append(s.order, w.Handle)
}

func (s *screen) frame(h uintptr) windows.Rect {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.open[h].Frame
}

func (s *screen) platform() windows.Platform {
	return windows.Platform{
		Target:   func() uintptr { return s.target },
		Displays: func() ([]windows.Display, error) { return s.displays, nil },
		Frame: func(h uintptr) (windows.Rect, bool, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			w, ok := s.open[h]
			if !ok {
				return windows.Rect{}, false, errors.New("no such window")
			}
			return w.Frame, w.Maximized, nil
		},
		Place: func(h uintptr, r windows.Rect) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.open[h].Frame, s.open[h].Maximized = r, false
			s.placedLog = append(s.placedLog, h)
			return nil
		},
		Maximize: func(h uintptr) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.open[h].Maximized = true
			s.open[h].Frame = s.displays[0].Work
			return nil
		},
		Focus: func(h uintptr) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.focused = append(s.focused, h)
			return nil
		},
		OpenWindows: func() ([]windows.Window, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			out := make([]windows.Window, 0, len(s.order))
			for _, h := range s.order {
				out = append(out, *s.open[h])
			}
			return out, nil
		},
		Launch: func(app string) error {
			s.mu.Lock()
			s.launched = append(s.launched, app)
			w, ok := s.opens[app]
			s.mu.Unlock()
			if ok {
				s.add(w)
			}
			return nil
		},
		Bind: func(hotkey string, run func()) func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.bound[hotkey] = run
			return func() {
				s.mu.Lock()
				defer s.mu.Unlock()
				delete(s.bound, hotkey)
			}
		},
		ValidHotkey: func(h string) bool { return strings.Contains(h, "+") },
		Patience:    400 * time.Millisecond,
	}
}

func run(t *testing.T, e *windows.Extension, ctx sdk.Context, id string) error {
	t.Helper()
	cmds := e.Commands(ctx)
	i := slices.IndexFunc(cmds, func(c sdk.Command) bool { return c.ID == id })
	if i < 0 {
		t.Fatalf("no command %q", id)
	}
	return cmds[i].Run(ctx)
}

func TestSnapsCoverTheWorkArea(t *testing.T) {
	tests := []struct {
		id   string
		want windows.Rect
	}{
		{"left", windows.Rect{X: 0, Y: 0, W: 960, H: 1040}},
		{"right", windows.Rect{X: 960, Y: 0, W: 960, H: 1040}},
		{"top", windows.Rect{X: 0, Y: 0, W: 1920, H: 520}},
		{"bottom", windows.Rect{X: 0, Y: 520, W: 1920, H: 520}},
		{"third_first", windows.Rect{X: 0, Y: 0, W: 640, H: 1040}},
		{"third_center", windows.Rect{X: 640, Y: 0, W: 640, H: 1040}},
		{"third_last", windows.Rect{X: 1280, Y: 0, W: 640, H: 1040}},
		{"top_left", windows.Rect{X: 0, Y: 0, W: 960, H: 520}},
		{"top_right", windows.Rect{X: 960, Y: 0, W: 960, H: 520}},
		{"bottom_left", windows.Rect{X: 0, Y: 520, W: 960, H: 520}},
		{"bottom_right", windows.Rect{X: 960, Y: 520, W: 960, H: 520}},
		{"center", windows.Rect{X: 560, Y: 220, W: 800, H: 600}},
		{"almost_maximize", windows.Rect{X: 96, Y: 52, W: 1728, H: 936}},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			s := newScreen()
			e := windows.New(s.platform())
			ctx := sdktest.New(t, e, "en")
			if err := run(t, e, ctx, tt.id); err != nil {
				t.Fatal(err)
			}
			if got := s.frame(1); got != tt.want {
				t.Errorf("frame = %+v, want %+v", got, tt.want)
			}
			if ctx.Hidden == 0 || !slices.Equal(s.focused, []uintptr{1}) {
				t.Errorf("hidden %d, focused %v", ctx.Hidden, s.focused)
			}
		})
	}
}

func TestSnapsMeetWithoutAGapOnOddSizes(t *testing.T) {
	s := newScreen()
	s.displays = []windows.Display{{ID: "one", Work: windows.Rect{X: -1001, Y: 7, W: 1001, H: 701}}}
	e := windows.New(s.platform())
	ctx := sdktest.New(t, e, "en")
	var last int
	for i, id := range []string{"third_first", "third_center", "third_last"} {
		if err := run(t, e, ctx, id); err != nil {
			t.Fatal(err)
		}
		f := s.frame(1)
		if i > 0 && f.X != last {
			t.Errorf("%s starts at %d, want %d", id, f.X, last)
		}
		last = f.X + f.W
	}
	if last != 0 {
		t.Errorf("thirds end at %d, want the display's edge", last)
	}
}

func TestThirdsStackOnAPortraitDisplay(t *testing.T) {
	s := newScreen()
	s.displays = []windows.Display{{ID: "one", Work: windows.Rect{W: 1080, H: 1800}}}
	e := windows.New(s.platform())
	if err := run(t, e, sdktest.New(t, e, "en"), "third_center"); err != nil {
		t.Fatal(err)
	}
	if got, want := s.frame(1), (windows.Rect{Y: 600, W: 1080, H: 600}); got != want {
		t.Errorf("frame = %+v, want %+v", got, want)
	}
}

func TestSnapUsesTheDisplayOfTheWindow(t *testing.T) {
	s := newScreen()
	s.displays = append(s.displays, windows.Display{ID: "two", Work: windows.Rect{X: 1920, W: 1000, H: 800}})
	s.open[1].Frame = windows.Rect{X: 2000, Y: 100, W: 300, H: 300}
	e := windows.New(s.platform())
	if err := run(t, e, sdktest.New(t, e, "en"), "right"); err != nil {
		t.Fatal(err)
	}
	if got, want := s.frame(1), (windows.Rect{X: 2420, W: 500, H: 800}); got != want {
		t.Errorf("frame = %+v, want %+v", got, want)
	}
}

func TestNoWindowIsToldWithTheLauncherOpen(t *testing.T) {
	s := newScreen()
	s.target = 99
	e := windows.New(s.platform())
	ctx := sdktest.New(t, e, "pt-BR")
	err := run(t, e, ctx, "left")
	if err == nil || err.Error() != "Não há janela para mover" {
		t.Errorf("err = %v", err)
	}
	if ctx.Hidden != 0 {
		t.Error("the launcher hid, and the message with it")
	}
}

func TestRestoreGoesBackToBeforeTheFirstMove(t *testing.T) {
	s := newScreen()
	e := windows.New(s.platform())
	ctx := sdktest.New(t, e, "en")
	start := s.frame(1)
	if err := run(t, e, ctx, "restore"); err == nil {
		t.Error("restored a window Althea never moved")
	}
	for _, id := range []string{"left", "top", "center"} {
		if err := run(t, e, ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := run(t, e, ctx, "restore"); err != nil {
		t.Fatal(err)
	}
	if got := s.frame(1); got != start {
		t.Errorf("frame = %+v, want %+v", got, start)
	}

	// A window the user moved since starts anew from where they put it.
	if err := run(t, e, ctx, "left"); err != nil {
		t.Fatal(err)
	}
	moved := windows.Rect{X: 5, Y: 5, W: 500, H: 500}
	s.open[1].Frame = moved
	if err := run(t, e, ctx, "right"); err != nil {
		t.Fatal(err)
	}
	if err := run(t, e, ctx, "restore"); err != nil {
		t.Fatal(err)
	}
	if got := s.frame(1); got != moved {
		t.Errorf("frame = %+v, want %+v", got, moved)
	}
}

func TestRestoreMaximizesAgain(t *testing.T) {
	s := newScreen()
	s.open[1].Maximized = true
	e := windows.New(s.platform())
	ctx := sdktest.New(t, e, "en")
	if err := run(t, e, ctx, "left"); err != nil {
		t.Fatal(err)
	}
	if s.open[1].Maximized {
		t.Fatal("still maximized")
	}
	if err := run(t, e, ctx, "restore"); err != nil {
		t.Fatal(err)
	}
	if !s.open[1].Maximized {
		t.Error("not maximized again")
	}
}

func TestNextDisplayKeepsThePlaceAndWrapsAround(t *testing.T) {
	s := newScreen()
	e := windows.New(s.platform())
	ctx := sdktest.New(t, e, "en")
	if err := run(t, e, ctx, "next_display"); err == nil {
		t.Error("moved with one display")
	}
	// Listed primary first, though it is the rightmost.
	s.displays = []windows.Display{{ID: "one", Work: work}, {ID: "left", Work: windows.Rect{X: -960, W: 960, H: 520}}}
	s.open[1].Frame = windows.Rect{X: 960, Y: 520, W: 960, H: 520}
	if err := run(t, e, ctx, "next_display"); err != nil {
		t.Fatal(err)
	}
	if got, want := s.frame(1), (windows.Rect{X: -480, Y: 260, W: 480, H: 260}); got != want {
		t.Errorf("wrapped to %+v, want the same place on the leftmost display, %+v", got, want)
	}
	if err := run(t, e, ctx, "next_display"); err != nil {
		t.Fatal(err)
	}
	if got, want := s.frame(1), (windows.Rect{X: 960, Y: 520, W: 960, H: 520}); got != want {
		t.Errorf("frame = %+v, want it back, %+v", got, want)
	}
}

func saveLayout(t *testing.T, e *windows.Extension, ctx *sdktest.Context, hotkey string) windows.Layout {
	t.Helper()
	const name = "Work"
	if err := run(t, e, ctx, "layout_create"); err != nil {
		t.Fatal(err)
	}
	form := ctx.Pushed[len(ctx.Pushed)-1].(*sdk.Form)
	if err := form.Submit(ctx, sdk.Values{"name": name, "hotkey": hotkey}); err != nil {
		t.Fatal(err)
	}
	layouts, err := windows.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(layouts, func(l windows.Layout) bool { return l.Name == name })
	if i < 0 {
		t.Fatalf("no layout %q in %+v", name, layouts)
	}
	return layouts[i]
}

func TestLayoutPutsOpenWindowsBackInOrder(t *testing.T) {
	s := newScreen()
	s.add(windows.Window{Handle: 2, App: "chat.exe", Title: "Chat", Frame: windows.Rect{X: 960, Y: 0, W: 960, H: 1040}})
	e := windows.New(s.platform())
	ctx := sdktest.New(t, e, "en")
	want1, want2 := s.frame(1), s.frame(2)
	saveLayout(t, e, ctx, "")

	s.open[1].Frame = windows.Rect{X: 1, Y: 1, W: 300, H: 300}
	s.open[2].Frame = windows.Rect{X: 2, Y: 2, W: 300, H: 300}
	items, err := e.Items(ctx)
	if err != nil || len(items) != 1 || items[0].Title != "Work" {
		t.Fatalf("items = %+v, %v", items, err)
	}
	if err := items[0].Actions[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	if s.frame(1) != want1 || s.frame(2) != want2 {
		t.Errorf("frames = %+v, %+v, want %+v, %+v", s.frame(1), s.frame(2), want1, want2)
	}
	if !slices.Equal(s.placedLog, []uintptr{2, 1}) {
		t.Errorf("placed %v, want the top window last", s.placedLog)
	}
	if !slices.Equal(s.focused, []uintptr{1}) {
		t.Errorf("focused %v, want the top window", s.focused)
	}
	if len(s.launched) != 0 {
		t.Errorf("launched %v with everything open", s.launched)
	}
}

func TestLayoutOpensWhatIsClosedAndUnpluggedDisplaysFallBack(t *testing.T) {
	s := newScreen()
	s.displays = []windows.Display{{ID: "gone", Work: work}}
	e := windows.New(s.platform())
	ctx := sdktest.New(t, e, "en")
	l := saveLayout(t, e, ctx, "")

	s.displays = []windows.Display{{ID: "other", Work: windows.Rect{W: 960, H: 520}}}
	delete(s.open, 1)
	s.order = nil
	s.opens["editor.exe"] = windows.Window{Handle: 7, App: "editor.exe", Title: "Editor"}
	items, _ := e.Items(ctx)
	if items[0].ID != l.ID {
		t.Fatalf("item %+v", items[0])
	}
	if err := items[0].Actions[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.launched, []string{"editor.exe"}) {
		t.Errorf("launched %v", s.launched)
	}
	if got, want := s.frame(7), (windows.Rect{X: 50, Y: 50, W: 400, H: 300}); got != want {
		t.Errorf("frame = %+v, want %+v, the same share of the other display", got, want)
	}
}

func TestLayoutTellsOfAppsThatDoNotOpen(t *testing.T) {
	s := newScreen()
	e := windows.New(s.platform())
	ctx := sdktest.New(t, e, "en")
	saveLayout(t, e, ctx, "")
	delete(s.open, 1)
	s.order = nil
	items, _ := e.Items(ctx)
	err := items[0].Actions[0].Run(ctx)
	if err == nil || !strings.Contains(err.Error(), "1") {
		t.Errorf("err = %v, want the count of windows missing", err)
	}
}

func TestLayoutNeedsWindows(t *testing.T) {
	s := newScreen()
	delete(s.open, 1)
	s.order = nil
	e := windows.New(s.platform())
	if err := run(t, e, sdktest.New(t, e, "en"), "layout_create"); err == nil {
		t.Error("a layout of nothing was offered")
	}
}

func TestLayoutHotkeyIsChecked(t *testing.T) {
	s := newScreen()
	e := windows.New(s.platform())
	ctx := sdktest.New(t, e, "en")
	if err := run(t, e, ctx, "layout_create"); err != nil {
		t.Fatal(err)
	}
	form := ctx.Pushed[len(ctx.Pushed)-1].(*sdk.Form)
	i := slices.IndexFunc(form.Fields, func(f sdk.Field) bool { return f.ID == "hotkey" })
	if form.Fields[i].Validate("nope") == "" {
		t.Error("a bad hotkey passed")
	}
	if form.Fields[i].Validate("") != "" || form.Fields[i].Validate("Ctrl+Alt+1") != "" {
		t.Error("a good hotkey failed")
	}
}

func TestHotkeysFollowPreferencesAndLayouts(t *testing.T) {
	s := newScreen()
	e := windows.New(s.platform())

	serveCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sc := sdktest.New(t, e, "en")
	sc.Context = serveCtx
	sc.Prefs["left"] = "Ctrl+Alt+Left"
	done := make(chan error, 1)
	go func() { done <- e.Serve(sc) }()

	bound := func(h string) func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.bound[h]
	}
	waitFor := func(what string, ok func() bool) {
		t.Helper()
		for deadline := time.Now().Add(3 * time.Second); !ok(); {
			if time.Now().After(deadline) {
				t.Fatal(what)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	waitFor("the hotkey was not bound", func() bool { return bound("Ctrl+Alt+Left") != nil })
	bound("Ctrl+Alt+Left")()
	if got, want := s.frame(1), (windows.Rect{W: 960, H: 1040}); got != want {
		t.Errorf("frame = %+v, want %+v", got, want)
	}

	saveLayout(t, e, sc, "Ctrl+Alt+1")
	waitFor("the layout's hotkey was not bound", func() bool { return bound("Ctrl+Alt+1") != nil })

	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if h := bound("Ctrl+Alt+Left"); h != nil {
		t.Error("the hotkey stayed bound after Serve ended")
	}
	if h := bound("Ctrl+Alt+1"); h != nil {
		t.Error("the layout's hotkey stayed bound after Serve ended")
	}
}

func TestNoPlatformNoCommands(t *testing.T) {
	e := windows.New(windows.Platform{})
	ctx := sdktest.New(t, e, "en")
	if cmds := e.Commands(ctx); len(cmds) != 0 {
		t.Errorf("commands = %d", len(cmds))
	}
	if prefs := e.Preferences(ctx); len(prefs) != 0 {
		t.Errorf("preferences = %d", len(prefs))
	}
	if err := e.Serve(ctx); err != nil {
		t.Error(err)
	}
}

func TestEveryCommandIsTranslated(t *testing.T) {
	e := windows.New(newScreen().platform())
	en, pt := sdktest.New(t, e, "en"), sdktest.New(t, e, "pt-BR")
	want := []string{"left", "maximize", "restore", "next_display", "layout_create", "layout_manage"}
	cmds := e.Commands(en)
	for i, c := range e.Commands(pt) {
		if c.Title == cmds[i].Title {
			t.Errorf("%s: %q in both languages", c.ID, c.Title)
		}
	}
	for _, id := range want {
		if !slices.ContainsFunc(cmds, func(c sdk.Command) bool { return c.ID == id }) {
			t.Errorf("no command %q", id)
		}
	}
}
