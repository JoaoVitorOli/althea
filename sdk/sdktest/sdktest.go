// Package sdktest helps test extensions: a Context that records what an extension asks of the
// host and translates with the extension's own catalogs.
package sdktest

import (
	"context"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/JoaoVitorOli/althea/internal/i18n"
	"github.com/JoaoVitorOli/althea/internal/prefs"
	"github.com/JoaoVitorOli/althea/internal/store"
	"github.com/JoaoVitorOli/althea/sdk"
)

// Context is an sdk.Context for tests. Its fields record the calls; it is safe for concurrent
// use.
type Context struct {
	context.Context

	mu     sync.Mutex
	Opened []string
	Copied []string
	Toasts []string
	Hidden int
	// Refreshed counts the refreshes asked.
	Refreshed int
	// Clips are what Copy copied.
	Clips []sdk.Clip
	// Pasted are the clips pasted; Clipboard is what ClipboardText returns.
	Pasted    []sdk.Clip
	Clipboard string
	// Selection is what SelectedText returns, and SelectionErr its error. Selections counts the
	// calls, which press keys in the user's app.
	Selection    string
	SelectionErr error
	Selections   int
	// Requests are the requests made with HTTP, as "POST https://host/path".
	Requests []string
	Pushed   []sdk.View
	Popped   int
	// Store is what the extension kept in Storage, by key.
	Store map[string][]byte
	// Prefs are the values of the extension's preferences: their defaults at first, for the
	// test to change. Secrets are the secret ones.
	Prefs   map[string]string
	Secrets map[string]string

	catalog *i18n.Catalog
	prefix  string
	lang    string

	t          testing.TB
	migrations fs.FS
	dbOnce     sync.Once
	db         sdk.Database
}

// New returns a Context translating into lang with the extension's messages.
func New(t testing.TB, ext sdk.Extension, lang string) *Context {
	t.Helper()
	m := ext.Manifest()
	c := &Context{
		Context: context.Background(), catalog: i18n.New(lang), prefix: m.ID + ".", lang: lang,
		t: t, migrations: m.Migrations,
	}
	if m.Messages != nil {
		if err := c.catalog.Add(m.ID, m.Messages); err != nil {
			t.Fatal(err)
		}
	}
	c.Prefs, c.Secrets = map[string]string{}, map[string]string{}
	if cf, ok := ext.(sdk.Configurable); ok {
		values, _ := prefs.Resolve(cf.Preferences(c), nil)
		maps.Copy(c.Prefs, values)
	}
	return c
}

// Database is a real database in a temporary folder, opened on first use with the extension's
// migrations applied, and closed when the test ends.
func (c *Context) Database() sdk.Database {
	c.dbOnce.Do(func() {
		db, err := store.Open(context.Background(), filepath.Join(c.t.TempDir(), "althea.db"))
		if err != nil {
			c.t.Fatal(err)
		}
		c.t.Cleanup(func() { _ = db.Close() })
		if c.migrations != nil {
			if err := db.Migrate(context.Background(), strings.TrimSuffix(c.prefix, "."), c.migrations); err != nil {
				c.t.Fatal(err)
			}
		}
		c.db = database{db}
	})
	return c.db
}

type database struct{ *store.DB }

func (d database) Tx(ctx context.Context, fn func(tx sdk.Queryer) error) error {
	return d.DB.Tx(ctx, func(tx *store.Tx) error { return fn(tx) })
}

// Preferences reads Prefs and Secrets.
func (c *Context) Preferences() sdk.Preferences {
	c.mu.Lock()
	defer c.mu.Unlock()
	secrets := maps.Clone(c.Secrets)
	return prefs.Preferences{Values: maps.Clone(c.Prefs), ReadSecret: func(name string) (string, error) {
		return secrets[name], nil
	}}
}

// Log discards.
func (*Context) Log() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// Open records target.
func (c *Context) Open(target string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Opened = append(c.Opened, target)
	return nil
}

// CopyText records text.
func (c *Context) CopyText(text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Copied = append(c.Copied, text)
	return nil
}

// Copy records the clip, its text among Copied.
func (c *Context) Copy(clip sdk.Clip) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Clips = append(c.Clips, clip)
	c.Copied = append(c.Copied, clip.Text)
	return nil
}

// ClipboardText returns Clipboard.
func (c *Context) ClipboardText() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Clipboard
}

// SelectedText returns Selection, counting the call.
func (c *Context) SelectedText() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Selections++
	return c.Selection, c.SelectionErr
}

// HTTP records each request in Requests and sends it for real: point the extension at an
// httptest server. Unlike the host's, it refuses nothing.
func (c *Context) HTTP() *http.Client { return &http.Client{Transport: recorder{c}} }

type recorder struct{ c *Context }

func (r recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	r.c.mu.Lock()
	r.c.Requests = append(r.c.Requests, req.Method+" "+req.URL.Scheme+"://"+req.URL.Host+req.URL.Path)
	r.c.mu.Unlock()
	return http.DefaultTransport.RoundTrip(req)
}

// Paste records the clip, puts its text in Clipboard and counts a hide, as the host hides to
// paste.
func (c *Context) Paste(clip sdk.Clip) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Pasted = append(c.Pasted, clip)
	c.Clipboard = clip.Text
	c.Hidden++
	return nil
}

// Refresh counts.
func (c *Context) Refresh() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Refreshed++
}

// Hide counts.
func (c *Context) Hide() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Hidden++
}

// Toast records message.
func (c *Context) Toast(message string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Toasts = append(c.Toasts, message)
}

// T translates with the extension's catalogs.
func (c *Context) T(key string, args ...any) string { return c.catalog.T(c.prefix+key, args...) }

// Language is the language the Context was made with.
func (c *Context) Language() string { return c.lang }

// Push records v.
func (c *Context) Push(v sdk.View) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Pushed = append(c.Pushed, v)
}

// Pop counts.
func (c *Context) Pop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Popped++
}

// Storage keeps values in Store.
func (c *Context) Storage() sdk.Storage { return storage{c} }

type storage struct{ c *Context }

func (s storage) Get(_ context.Context, key string) ([]byte, bool, error) {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	v, ok := s.c.Store[key]
	return slices.Clone(v), ok, nil
}

func (s storage) Set(_ context.Context, key string, value []byte) error {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	if s.c.Store == nil {
		s.c.Store = map[string][]byte{}
	}
	s.c.Store[key] = append([]byte{}, value...)
	return nil
}

func (s storage) Delete(_ context.Context, key string) error {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	delete(s.c.Store, key)
	return nil
}

func (s storage) List(_ context.Context, prefix string) ([]sdk.Entry, error) {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	var out []sdk.Entry
	for _, k := range slices.Sorted(maps.Keys(s.c.Store)) {
		if strings.HasPrefix(k, prefix) {
			out = append(out, sdk.Entry{Key: k, Value: slices.Clone(s.c.Store[k])})
		}
	}
	return out, nil
}
