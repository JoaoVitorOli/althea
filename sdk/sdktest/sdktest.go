// Package sdktest helps test extensions: a Context that records what an extension asks of the
// host and translates with the extension's own catalogs.
package sdktest

import (
	"context"
	"io"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/JoaoVitorOli/althea/internal/i18n"
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
	Pushed []sdk.View
	Popped int
	// Store is what the extension kept in Storage, by key.
	Store   map[string][]byte
	catalog *i18n.Catalog
	prefix  string
	lang    string
}

// New returns a Context translating into lang with the extension's messages.
func New(t testing.TB, ext sdk.Extension, lang string) *Context {
	t.Helper()
	m := ext.Manifest()
	c := &Context{Context: context.Background(), catalog: i18n.New(lang), prefix: m.ID + ".", lang: lang}
	if m.Messages != nil {
		if err := c.catalog.Add(m.ID, m.Messages); err != nil {
			t.Fatal(err)
		}
	}
	return c
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
