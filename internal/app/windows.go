package app

import (
	"sync"

	"github.com/egoist/mygo"

	"github.com/JoaoVitorOli/althea/extensions/windows"
	"github.com/JoaoVitorOli/althea/internal/hotkey"
	"github.com/JoaoVitorOli/althea/internal/platform"
)

// windowsPlatform hands the Window Management extension what the platform can do; nothing where
// it cannot move windows, so the commands stay out.
func (a *althea) windowsPlatform() windows.Platform {
	m := a.services.Windows
	if m == nil {
		return windows.Platform{}
	}
	return windows.Platform{
		// The launcher holds the keyboard while it shows, so the window to act on is the one it
		// took it from.
		Target: func() uintptr {
			w, own := m.Foreground()
			if own {
				return a.pasteTarget.Load()
			}
			return w
		},
		Displays: func() ([]windows.Display, error) {
			displays, err := m.Displays()
			out := make([]windows.Display, len(displays))
			for i, d := range displays {
				out[i] = windows.Display{ID: d.ID, Work: windows.Rect(d.Work)}
			}
			return out, err
		},
		Frame: func(w uintptr) (windows.Rect, bool, error) {
			r, maximized, err := m.Frame(w)
			return windows.Rect(r), maximized, err
		},
		Place:    func(w uintptr, r windows.Rect) error { return m.Place(w, platform.Rect(r)) },
		Maximize: m.Maximize,
		Focus:    m.Focus,
		OpenWindows: func() ([]windows.Window, error) {
			open, err := m.OpenWindows()
			out := make([]windows.Window, len(open))
			for i, w := range open {
				out[i] = windows.Window{
					Handle: w.Handle, App: w.App, Title: w.Title, Frame: windows.Rect(w.Frame), Maximized: w.Maximized,
				}
			}
			return out, err
		},
		Launch: func(app string) error { return mygo.Shell.LaunchApp(mygo.InstalledApp{Path: app}) },
		Bind:   a.bindCommandHotkey,
		ValidHotkey: func(s string) bool {
			b, err := hotkey.Parse(s)
			return err == nil && !b.WinKey
		},
	}
}

// bindCommandHotkey runs run when shortcut is pressed, on a goroutine, until unbind. The system
// binds on the main thread, and a service unbinds as it stops, while the main thread may be
// waiting for it to stop: bindings are queued, never waited for.
func (a *althea) bindCommandHotkey(shortcut string, run func()) (unbind func()) {
	b, err := hotkey.Parse(shortcut)
	if err != nil || b.WinKey {
		a.log.Warn("hotkey not understood", "hotkey", shortcut, "err", err)
		return func() {}
	}
	bound := false // used on the queue alone
	a.shortcuts.do(func() {
		err := mygo.GlobalShortcut.Register(b.Accelerator, func() {
			go func() {
				defer func() {
					if p := recover(); p != nil {
						a.log.Error("hotkey panicked", "hotkey", shortcut, "panic", p)
					}
				}()
				run()
			}()
		})
		if err != nil {
			a.log.Warn("hotkey not registered", "hotkey", shortcut, "err", err)
			return
		}
		bound = true
	})
	return func() {
		a.shortcuts.do(func() {
			if bound {
				mygo.GlobalShortcut.Unregister(b.Accelerator)
			}
		})
	}
}

// serial runs functions in the order given, one at a time, on a goroutine that lasts while there
// are some. The zero serial is ready.
type serial struct {
	mu      sync.Mutex
	queue   []func()
	running bool
}

// do queues fn and returns at once.
func (s *serial) do(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue = append(s.queue, fn)
	if !s.running {
		s.running = true
		go s.drain()
	}
}

func (s *serial) drain() {
	for {
		s.mu.Lock()
		if len(s.queue) == 0 {
			s.running = false
			s.mu.Unlock()
			return
		}
		fn := s.queue[0]
		s.queue = s.queue[1:]
		s.mu.Unlock()
		fn()
	}
}
