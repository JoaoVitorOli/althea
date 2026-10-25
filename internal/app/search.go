package app

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"github.com/JoaoVitorOli/althea/extensions"
	"github.com/JoaoVitorOli/althea/extensions/apps"
	"github.com/JoaoVitorOli/althea/extensions/system"
	"github.com/JoaoVitorOli/althea/internal/frecency"
	"github.com/JoaoVitorOli/althea/internal/platform"
	"github.com/JoaoVitorOli/althea/internal/registry"
	"github.com/JoaoVitorOli/althea/internal/ui"
	"github.com/JoaoVitorOli/althea/sdk"
)

// refreshEvery is the least time between two refreshes of the extensions' items, which run as
// the launcher shows: apps installed meanwhile appear the next time.
const refreshEvery = 30 * time.Second

// searcher runs the root search off the main thread: each keystroke cancels the query before
// it, and results reach the view through Window.Update.
type searcher struct {
	a        *althea
	registry *registry.Registry
	frecency *frecency.Frecency

	mu        sync.Mutex
	cancel    context.CancelFunc
	refreshed time.Time
}

func (a *althea) setUpSearch() error {
	exts := extensions.Builtin(extensions.Platforms{
		Apps:   appsPlatform(),
		System: systemPlatform(a.services.System),
	})
	for _, e := range exts {
		m := e.Manifest()
		if m.Messages != nil {
			if err := a.catalog.Add(m.ID, m.Messages); err != nil {
				return err
			}
		}
	}
	f, err := frecency.Load(context.Background(), a.db.KV("althea.frecency"), time.Now)
	if err != nil {
		return err
	}
	s := &searcher{a: a, frecency: f}
	s.registry = registry.New(exts, registry.Deps{
		Use:     f.Score,
		Context: a.extensionContext,
		Log:     a.log,
	})
	a.search = s
	a.root.OnQuery = s.query
	a.root.OnRun = s.run
	a.root.Icons = &ui.Icons{Load: mygo.Shell.FileIcon, Loaded: func() { a.launcher.Window().Invalidate() }}
	go s.refresh()
	return nil
}

// refresh collects the extensions' items again, unless it did a moment ago, then shows the
// results of the current query anew.
func (s *searcher) refresh() {
	s.mu.Lock()
	if time.Since(s.refreshed) < refreshEvery {
		s.mu.Unlock()
		return
	}
	s.refreshed = time.Now()
	s.mu.Unlock()

	start := time.Now()
	if err := s.registry.Refresh(context.Background()); err != nil {
		s.a.log.Warn("refresh", "err", err)
	}
	s.a.log.Info("refreshed items", "items", s.registry.Len(), "took", time.Since(start))
	s.a.launcher.Window().Update(func() { s.query(s.a.root.Query()) })
}

// query searches for q, in place of the search before it.
func (s *searcher) query(q string) {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.cancel = cancel
	s.mu.Unlock()
	go func() {
		results := s.registry.Query(ctx, q)
		if ctx.Err() != nil {
			return
		}
		s.a.launcher.Window().Update(func() { s.a.root.SetResults(q, results) })
	}()
}

// run runs an action of a result, counting the pick for ranking. It runs off the main thread,
// so that a slow extension never freezes the launcher.
func (s *searcher) run(res registry.Result, action int) {
	query := s.a.root.Query()
	act := res.Item.Actions[action]
	go func() {
		if !res.Answer {
			if err := s.frecency.Record(context.Background(), res.Key, query); err != nil {
				s.a.log.Warn("frecency", "err", err)
			}
		}
		if act.Run == nil {
			return
		}
		ctx := s.a.extensionContext(context.Background(), res.Extension)
		if err := act.Run(ctx); err != nil {
			s.a.log.Warn("action failed", "extension", res.Extension, "action", act.Title, "err", err)
			ctx.Toast(err.Error())
		}
	}()
}

// hostContext is the sdk.Context extensions get.
type hostContext struct {
	context.Context
	a   *althea
	ext string
	log *slog.Logger
}

func (a *althea) extensionContext(ctx context.Context, ext string) sdk.Context {
	return &hostContext{Context: ctx, a: a, ext: ext, log: a.log.With("extension", ext)}
}

func (h *hostContext) Log() *slog.Logger { return h.log }

func (h *hostContext) Open(target string) error {
	if strings.Contains(target, "://") {
		return mygo.Shell.OpenExternal(target)
	}
	return mygo.Shell.OpenPath(target)
}

func (*hostContext) CopyText(text string) error {
	mygo.Clipboard.WriteText(text)
	return nil
}

func (h *hostContext) Hide() { h.a.launcher.Window().Update(h.a.launcher.Hide) }

func (h *hostContext) Toast(message string) {
	h.a.launcher.Window().Update(func() { h.a.root.Toast(message) })
}

func (h *hostContext) T(key string, args ...any) string {
	return h.a.catalog.T(h.ext+"."+key, args...)
}

func (h *hostContext) Language() string { return h.a.catalog.Language() }

// appsPlatform reaches installed apps through MyGo: Win32 and Store apps on Windows.
func appsPlatform() apps.Platform {
	return apps.Platform{
		List: func() ([]apps.App, error) {
			installed, err := mygo.Shell.InstalledApps()
			out := make([]apps.App, len(installed))
			for i, a := range installed {
				out[i] = apps.App{Name: a.Name, ID: a.ID, Path: a.Path}
			}
			return out, err
		},
		Launch: func(a apps.App) error {
			return mygo.Shell.LaunchApp(mygo.InstalledApp{Name: a.Name, ID: a.ID, Path: a.Path})
		},
	}
}

// systemPlatform hands the System extension what the platform can do; nothing where it can do
// nothing, so the commands stay out.
func systemPlatform(s platform.System) system.Platform {
	if s == nil {
		return system.Platform{}
	}
	return system.Platform{
		Lock: s.Lock, Sleep: s.Sleep, Restart: s.Restart, ShutDown: s.ShutDown, SignOut: s.SignOut,
		EmptyRecycleBin: s.EmptyRecycleBin, IsDarkMode: s.IsDarkMode, SetDarkMode: s.SetDarkMode,
	}
}
