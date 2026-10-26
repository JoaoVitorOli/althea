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
	"github.com/JoaoVitorOli/althea/extensions/scripts"
	"github.com/JoaoVitorOli/althea/extensions/system"
	"github.com/JoaoVitorOli/althea/internal/frecency"
	"github.com/JoaoVitorOli/althea/internal/platform"
	"github.com/JoaoVitorOli/althea/internal/prefs"
	"github.com/JoaoVitorOli/althea/internal/registry"
	"github.com/JoaoVitorOli/althea/internal/store"
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
		Scripts: scripts.Platform{
			Dir: a.paths.Scripts, Command: a.services.Command, Edit: a.services.EditFile,
		},
	})
	for _, e := range exts {
		if err := a.addMessages(e.Manifest()); err != nil {
			return err
		}
	}
	exts = a.migrate(exts)
	settings := a.setUpPreferences(exts)
	if err := a.addMessages(settings.Manifest()); err != nil {
		return err
	}
	exts = append(exts, settings)

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
	a.root.Host = viewHost{a}
	a.root.Icons = &ui.Icons{Load: mygo.Shell.FileIcon, Loaded: func() { a.launcher.Window().Invalidate() }}
	go s.refresh()
	return nil
}

// migrate applies the extensions' migrations, and leaves out an extension whose tables cannot
// be brought up to date: it would fail on every call.
func (a *althea) migrate(exts []sdk.Extension) []sdk.Extension {
	kept := exts[:0]
	for _, e := range exts {
		m := e.Manifest()
		if m.Migrations != nil {
			if err := a.db.Migrate(context.Background(), m.ID, m.Migrations); err != nil {
				a.log.Error("extension left out: its migrations failed", "extension", m.ID, "err", err)
				continue
			}
		}
		kept = append(kept, e)
	}
	return kept
}

func (a *althea) addMessages(m sdk.Manifest) error {
	if m.Messages == nil {
		return nil
	}
	return a.catalog.Add(m.ID, m.Messages)
}

// setUpPreferences gathers the extensions' preferences and their values, keeps the values in
// step with settings.toml, and returns the settings commands that edit them.
func (a *althea) setUpPreferences(exts []sdk.Extension) *settingsExtension {
	a.prefs = newPreferences(a.paths.Settings, a.services.Vault, a.log)
	s := &settingsExtension{prefs: a.prefs}
	a.prefsSettings = s
	for _, e := range exts {
		c, ok := e.(sdk.Configurable)
		if !ok {
			continue
		}
		m := e.Manifest()
		var decl []sdk.Preference
		func() {
			defer func() {
				if p := recover(); p != nil {
					a.log.Error("extension panicked", "extension", m.ID, "call", "preferences", "panic", p)
				}
			}()
			decl = c.Preferences(a.extensionContext(context.Background(), m.ID))
		}()
		if len(decl) == 0 {
			continue
		}
		a.prefs.declare(m.ID, decl)
		s.exts = append(s.exts, m)
	}
	a.prefs.resolve(a.Settings())
	a.OnSettings(a.prefs.resolve)
	return s
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
		if s.a.prefs != nil && len(s.a.prefs.missing(res.Extension)) > 0 {
			s.askPreferences(res, act)
			return
		}
		if err := act.Run(ctx); err != nil {
			s.a.log.Warn("action failed", "extension", res.Extension, "action", act.Title, "err", err)
			ctx.Toast(err.Error())
		}
	}()
}

// viewHost runs the code of the views extensions push, as ui.Host.
type viewHost struct{ a *althea }

func (h viewHost) Go(ext string, fn func(ctx sdk.Context) error) (cancel func()) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer cancel()
		ectx := h.a.extensionContext(ctx, ext)
		defer func() {
			if p := recover(); p != nil {
				h.a.log.Error("extension panicked", "extension", ext, "panic", p)
			}
		}()
		if err := fn(ectx); err != nil && ctx.Err() == nil {
			h.a.log.Warn("view failed", "extension", ext, "err", err)
			ectx.Toast(err.Error())
		}
	}()
	return cancel
}

func (h viewHost) Update(fn func()) { h.a.launcher.Window().Update(fn) }

// askPreferences shows the preferences of an extension that misses required ones, and runs the
// action once they are saved.
func (s *searcher) askPreferences(res registry.Result, act sdk.Action) {
	settings := s.a.extensionContext(context.Background(), settingsID)
	m := sdk.Manifest{ID: res.Extension, Title: res.Extension}
	for _, e := range s.a.prefsSettings.exts {
		if e.ID == res.Extension {
			m = e
		}
	}
	settings.Push(s.a.prefsSettings.form(settings, m, func(sdk.Context) error {
		return act.Run(s.a.extensionContext(context.Background(), res.Extension))
	}))
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

func (h *hostContext) Push(v sdk.View) {
	h.a.launcher.Window().Update(func() { h.a.root.Push(h.ext, v) })
}

func (h *hostContext) Pop() { h.a.launcher.Window().Update(h.a.root.Pop) }

func (h *hostContext) Preferences() sdk.Preferences {
	if h.a.prefs == nil {
		return prefs.Preferences{}
	}
	return h.a.prefs.of(h.ext)
}

func (h *hostContext) Database() sdk.Database { return extensionDB{h.a.db} }

// extensionDB is the database as extensions see it.
type extensionDB struct{ *store.DB }

func (d extensionDB) Tx(ctx context.Context, fn func(tx sdk.Queryer) error) error {
	return d.DB.Tx(ctx, func(tx *store.Tx) error { return fn(tx) })
}

func (h *hostContext) Storage() sdk.Storage {
	return extensionStorage{h.a.db.KV(storageNamespace(h.ext))}
}

// storageNamespace is the namespace of an extension's Storage, apart from the host's own.
func storageNamespace(ext string) string { return "extension/" + ext }

// extensionStorage is an extension's Storage in the database.
type extensionStorage struct{ kv *store.KV }

func (s extensionStorage) Get(ctx context.Context, key string) ([]byte, bool, error) {
	return s.kv.Get(ctx, key)
}

func (s extensionStorage) Set(ctx context.Context, key string, value []byte) error {
	return s.kv.Set(ctx, key, value)
}

func (s extensionStorage) Delete(ctx context.Context, key string) error { return s.kv.Delete(ctx, key) }

func (s extensionStorage) List(ctx context.Context, prefix string) ([]sdk.Entry, error) {
	entries, err := s.kv.List(ctx, prefix)
	out := make([]sdk.Entry, len(entries))
	for i, e := range entries {
		out[i] = sdk.Entry(e)
	}
	return out, err
}

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
