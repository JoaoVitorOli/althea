package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"github.com/JoaoVitorOli/althea/extensions/notes"
	"github.com/JoaoVitorOli/althea/internal/foldersync"
	"github.com/JoaoVitorOli/althea/internal/fswatch"
	"github.com/JoaoVitorOli/althea/internal/store"
	"github.com/JoaoVitorOli/althea/sdk"
)

const (
	// syncNamespace keeps the sync folder and this computer's ID; what was synced is kept apart,
	// to be forgotten when sync is turned off.
	syncNamespace      = "althea.sync"
	syncStateNamespace = "althea.sync.state"
	// syncKeyName names the folder's key in the vault: the passphrase is never kept.
	syncKeyName = "Althea/sync/key"
	// syncEvery is how often changes made here go to the folder; what other computers write
	// comes as soon as the sync tool brings it.
	syncEvery = time.Minute
)

// folderSync runs sync while it is on (docs/adr/0011-sync.md).
type folderSync struct {
	a *althea

	mu      sync.Mutex
	syncer  *foldersync.Syncer // nil while off
	watcher *fswatch.Watcher
	stop    chan struct{}
	last    time.Time // of the last sync, done or failed
	result  foldersync.Result
	err     error
}

func (a *althea) syncKV() *store.KV { return a.db.KV(syncNamespace) }

// setUpSync starts sync if it was turned on before.
func (a *althea) setUpSync() {
	a.syncing = &folderSync{a: a}
	if a.services.Vault == nil {
		return
	}
	ctx := context.Background()
	dir, ok, err := a.syncKV().Get(ctx, "folder")
	if err != nil || !ok {
		return
	}
	secret, ok, err := a.services.Vault.Get(syncKeyName)
	if err != nil || !ok {
		a.log.Warn("sync is on but its key is not in the vault; set it up again", "err", err)
		return
	}
	key, err := base64.StdEncoding.DecodeString(secret)
	if err != nil || len(key) != 32 {
		a.log.Warn("the sync key in the vault is damaged; set sync up again")
		return
	}
	if err := a.syncing.start(string(dir), key); err != nil {
		a.log.Error("sync not started", "err", err)
	}
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { a.syncing.halt() })
}

// on reports whether sync is on, and its folder.
func (f *folderSync) on() (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.syncer == nil {
		return "", false
	}
	return f.syncer.Dir, true
}

// turnOn joins the folder dir with passphrase, keeps its key and syncs a first time.
func (f *folderSync) turnOn(ctx context.Context, dir, passphrase string) (foldersync.Result, error) {
	a := f.a
	if a.services.Vault == nil {
		return foldersync.Result{}, errors.New("this system has no vault to keep the sync key in")
	}
	key, err := foldersync.Join(dir, passphrase)
	if err != nil {
		return foldersync.Result{}, err
	}
	if err := a.services.Vault.Set(syncKeyName, base64.StdEncoding.EncodeToString(key)); err != nil {
		return foldersync.Result{}, fmt.Errorf("keep the key: %w", err)
	}
	// A folder other than the last one starts anew.
	if last, _, _ := a.syncKV().Get(ctx, "folder"); string(last) != dir {
		if err := foldersync.Forget(ctx, a.db.KV(syncStateNamespace)); err != nil {
			return foldersync.Result{}, err
		}
	}
	if err := a.syncKV().Set(ctx, "folder", []byte(dir)); err != nil {
		return foldersync.Result{}, err
	}
	f.halt()
	if err := f.start(dir, key); err != nil {
		return foldersync.Result{}, err
	}
	return f.run(ctx)
}

// turnOff stops syncing and forgets the key; the folder keeps what is in it.
func (f *folderSync) turnOff(ctx context.Context) error {
	f.halt()
	a := f.a
	return errors.Join(
		a.services.Vault.Delete(syncKeyName),
		a.syncKV().Delete(ctx, "folder"),
		foldersync.Forget(ctx, a.db.KV(syncStateNamespace)),
	)
}

func (f *folderSync) start(dir string, key []byte) error {
	a := f.a
	ctx := context.Background()
	device, ok, err := a.syncKV().Get(ctx, "device")
	if err != nil {
		return err
	}
	if !ok {
		device = []byte(sdk.NewID())
		if err := a.syncKV().Set(ctx, "device", device); err != nil {
			return err
		}
	}
	name, _ := os.Hostname()
	syncer := &foldersync.Syncer{
		Dir: dir, Key: key, Device: string(device), Name: name,
		State: a.db.KV(syncStateNamespace), Sources: a.syncSources(),
		Now: time.Now, Log: a.log,
	}
	// What other computers write comes as the sync tool brings it.
	w, err := fswatch.WatchDir(filepath.Join(dir, "devices"),
		func(name string) bool { return strings.HasSuffix(name, ".sync") },
		func() { f.background() })
	if err != nil {
		return err
	}
	stop := make(chan struct{})
	f.mu.Lock()
	f.syncer, f.watcher, f.stop = syncer, w, stop
	f.mu.Unlock()
	go func() {
		t := time.NewTicker(syncEvery)
		defer t.Stop()
		f.background()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				f.background()
			}
		}
	}()
	return nil
}

func (f *folderSync) halt() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.syncer == nil {
		return
	}
	close(f.stop)
	_ = f.watcher.Close()
	f.syncer, f.watcher, f.stop = nil, nil, nil
}

// background syncs, logging what goes wrong.
func (f *folderSync) background() {
	if _, err := f.run(context.Background()); err != nil {
		f.a.log.Warn("sync failed", "err", err)
	}
}

// run syncs once, and shows what came from other computers.
func (f *folderSync) run(ctx context.Context) (foldersync.Result, error) {
	f.mu.Lock()
	syncer := f.syncer
	f.mu.Unlock()
	if syncer == nil {
		return foldersync.Result{}, errors.New("sync is off")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	res, err := syncer.Sync(ctx)
	f.mu.Lock()
	f.last, f.result, f.err = time.Now(), res, err
	f.mu.Unlock()
	if res.Pulled > 0 {
		f.a.log.Info("sync brought changes", "records", res.Pulled)
		f.pulled()
	}
	return res, err
}

// pulled shows what came: settings and themes reload as their files change; notes and the
// root search are told.
func (f *folderSync) pulled() {
	a := f.a
	mygo.RunOnMain(func() {
		if a.notes != nil && a.notes.view != nil {
			a.notes.view.Changed()
			a.notes.win.Window().Invalidate()
		}
	})
	if a.search != nil {
		go a.search.refreshNow()
	}
}

// status is what the last sync did, for the settings to show.
func (f *folderSync) status() (time.Time, foldersync.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.last, f.result, f.err
}

// syncSources are what syncs: never the clipboard history, nor the secrets in the vault.
func (a *althea) syncSources() []foldersync.Source {
	sources := []foldersync.Source{
		fileSource("settings/", filepath.Dir(a.paths.Settings), func(name string) bool {
			return name == filepath.Base(a.paths.Settings)
		}),
		fileSource("theme/", a.paths.Themes, tomlFile),
		fileSource("ai-command/", a.paths.Commands, tomlFile),
		kvSource("snippet/", a.db.KV(storageNamespace("althea.snippets")), "snippet/"),
		kvSource("quicklink/", a.db.KV(storageNamespace("althea.quicklinks")), "link/"),
	}
	if a.notes != nil && a.notes.ext != nil {
		ext := a.notes.ext
		ctx := func(ctx context.Context) sdk.Context { return a.extensionContext(ctx, notes.ID) }
		sources = append(sources, foldersync.Source{
			Prefix: "note/",
			Snapshot: func(c context.Context) (map[string][]byte, error) {
				a.notes.flush()
				return ext.Records(ctx(c))
			},
			Apply: func(c context.Context, id string, data []byte) error {
				return ext.PutRecord(ctx(c), id, data)
			},
		})
	}
	return sources
}

func tomlFile(name string) bool { return strings.HasSuffix(name, ".toml") }

// fileSource syncs the files of dir whose names match accepts.
func fileSource(prefix, dir string, match func(name string) bool) foldersync.Source {
	return foldersync.Source{
		Prefix: prefix,
		Snapshot: func(context.Context) (map[string][]byte, error) {
			entries, err := os.ReadDir(dir)
			if errors.Is(err, fs.ErrNotExist) {
				return nil, nil
			}
			if err != nil {
				return nil, err
			}
			out := map[string][]byte{}
			for _, e := range entries {
				if !e.Type().IsRegular() || !match(e.Name()) {
					continue
				}
				data, err := os.ReadFile(filepath.Join(dir, e.Name()))
				if err != nil {
					return nil, err
				}
				out[e.Name()] = data
			}
			return out, nil
		},
		Apply: func(_ context.Context, name string, data []byte) error {
			// Only a plain name this source would read: never a path out of dir.
			if name != filepath.Base(name) || name == "." || name == ".." || !match(name) {
				return fmt.Errorf("refused the file name %q", name)
			}
			path := filepath.Join(dir, name)
			if data == nil {
				if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
					return err
				}
				return nil
			}
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return err
			}
			return writeAtomic(path, data)
		},
	}
}

// kvSource syncs the keys of kv that start with keyPrefix.
func kvSource(prefix string, kv *store.KV, keyPrefix string) foldersync.Source {
	return foldersync.Source{
		Prefix: prefix,
		Snapshot: func(ctx context.Context) (map[string][]byte, error) {
			entries, err := kv.List(ctx, keyPrefix)
			if err != nil {
				return nil, err
			}
			out := make(map[string][]byte, len(entries))
			for _, e := range entries {
				out[strings.TrimPrefix(e.Key, keyPrefix)] = e.Value
			}
			return out, nil
		},
		Apply: func(ctx context.Context, key string, data []byte) error {
			if data == nil {
				return kv.Delete(ctx, keyPrefix+key)
			}
			return kv.Set(ctx, keyPrefix+key, data)
		},
	}
}

// writeAtomic writes a file whole or not at all, so that a watcher never reads half of it.
func writeAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".sync-*")
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		_ = os.Remove(f.Name())
	}
	return err
}

// command is the Sync command of the settings: setting it up, or what it did and what to
// do with it.
func (f *folderSync) command(ctx sdk.Context) sdk.Command {
	subtitle := ctx.T("sync.subtitle_off")
	if dir, on := f.on(); on {
		subtitle = ctx.T("sync.subtitle_on", "folder", dir)
	}
	return sdk.Command{
		ID: "sync", Title: ctx.T("sync.title"), Subtitle: subtitle,
		Icon: sdk.Icon{Glyph: "sync"}, Keywords: strings.Fields(ctx.T("sync.keywords")),
		Run: func(ctx sdk.Context) error {
			ctx.Push(&sdk.List{Title: ctx.T("sync.title"), Load: func(ctx sdk.Context) ([]sdk.Item, error) {
				return f.items(ctx), nil
			}})
			return nil
		},
	}
}

func (f *folderSync) items(ctx sdk.Context) []sdk.Item {
	what := sdk.Item{
		ID: "what", Title: ctx.T("sync.what"), Subtitle: ctx.T("sync.what_info"),
		Icon: sdk.Icon{Glyph: "check"},
	}
	dir, on := f.on()
	if !on {
		return []sdk.Item{{
			ID: "turn-on", Title: ctx.T("sync.turn_on"), Icon: sdk.Icon{Glyph: "sync"},
			Actions: []sdk.Action{{Title: ctx.T("sync.turn_on"), Run: func(ctx sdk.Context) error {
				ctx.Push(f.form(ctx))
				return nil
			}}},
		}, what}
	}
	last, res, err := f.status()
	status := ctx.T("sync.never")
	switch {
	case err != nil:
		status = ctx.T("sync.failed", "time", last.Format("15:04"), "error", err.Error())
	case !last.IsZero():
		status = ctx.T("sync.synced", "time", last.Format("15:04"), "devices", res.Devices)
	}
	return []sdk.Item{
		{
			ID: "now", Title: ctx.T("sync.now"), Subtitle: status, Icon: sdk.Icon{Glyph: "sync"},
			Actions: []sdk.Action{{Title: ctx.T("sync.now"), Run: func(ctx sdk.Context) error {
				res, err := f.run(ctx)
				ctx.Refresh()
				if err != nil {
					return err
				}
				ctx.Toast(ctx.T("sync.done", "pulled", res.Pulled, "pushed", res.Pushed))
				return nil
			}}},
		},
		{
			ID: "open", Title: ctx.T("sync.open"), Subtitle: dir, Icon: sdk.Icon{Glyph: "folder"},
			Actions: []sdk.Action{{Title: ctx.T("sync.open"), Run: func(ctx sdk.Context) error {
				return ctx.Open(dir)
			}}},
		},
		what,
		{
			ID: "off", Title: ctx.T("sync.off"), Icon: sdk.Icon{Glyph: "close"},
			Actions: []sdk.Action{{Title: ctx.T("sync.off"), Confirm: ctx.T("sync.off_confirm"), Run: func(ctx sdk.Context) error {
				if err := f.turnOff(ctx); err != nil {
					return err
				}
				ctx.Refresh()
				ctx.Toast(ctx.T("sync.off_done"))
				return nil
			}}},
		},
	}
}

// form sets sync up: the folder and the passphrase, typed twice.
func (f *folderSync) form(ctx sdk.Context) *sdk.Form {
	folder := ""
	if od := os.Getenv("OneDrive"); od != "" {
		folder = filepath.Join(od, "Althea")
	}
	return &sdk.Form{
		Title: ctx.T("sync.form_title"), SubmitTitle: ctx.T("sync.turn_on"),
		Fields: []sdk.Field{
			{
				ID: "folder", Title: ctx.T("sync.folder"), Info: ctx.T("sync.folder_info"), Default: folder, Required: true,
				Validate: func(v string) string {
					if !filepath.IsAbs(strings.TrimSpace(v)) {
						return ctx.T("sync.folder_relative")
					}
					return ""
				},
			},
			{
				ID: "passphrase", Title: ctx.T("sync.passphrase"), Info: ctx.T("sync.passphrase_info"),
				Kind: sdk.Password, Required: true,
				Validate: func(v string) string {
					if len([]rune(v)) < 8 {
						return ctx.T("sync.too_short")
					}
					return ""
				},
			},
			{ID: "confirm", Title: ctx.T("sync.confirm"), Kind: sdk.Password, Required: true},
		},
		Submit: func(ctx sdk.Context, v sdk.Values) error {
			if v["passphrase"] != v["confirm"] {
				return errors.New(ctx.T("sync.mismatch"))
			}
			res, err := f.turnOn(ctx, filepath.Clean(strings.TrimSpace(v["folder"])), v["passphrase"])
			if errors.Is(err, foldersync.ErrWrongPassphrase) {
				return errors.New(ctx.T("sync.wrong"))
			}
			if err != nil {
				return err
			}
			ctx.Toast(ctx.T("sync.on", "pulled", res.Pulled, "devices", res.Devices))
			return nil
		},
	}
}
