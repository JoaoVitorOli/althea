package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/JoaoVitorOli/althea/internal/config"
	"github.com/JoaoVitorOli/althea/internal/i18n"
	"github.com/JoaoVitorOli/althea/internal/logging"
	"github.com/JoaoVitorOli/althea/internal/paths"
	"github.com/JoaoVitorOli/althea/internal/store"
	"github.com/JoaoVitorOli/althea/internal/ui"
)

// core holds the services that run without a window: everything the UI is built on.
type core struct {
	paths    paths.Paths
	log      *slog.Logger
	db       *store.DB
	catalog  *i18n.Catalog
	settings atomic.Pointer[config.Settings]

	mu        sync.Mutex
	listeners []func(config.Settings)

	closers []func() error // in the order they were opened
}

// OnSettings calls fn, on the watcher's goroutine, with the settings each time they reload.
func (c *core) OnSettings(fn func(config.Settings)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.listeners = append(c.listeners, fn)
}

// bootstrap opens Althea's services in the directories of p. Locale is the system's, used when
// the settings ask for the system language.
func bootstrap(ctx context.Context, p paths.Paths, locale string) (_ *core, err error) {
	c := &core{paths: p}
	defer func() {
		if err != nil {
			_ = c.Close()
		}
	}()

	log, closeLog, err := logging.New(logging.Options{Dir: p.Logs})
	if err != nil {
		return nil, err
	}
	c.log = log
	c.closers = append(c.closers, closeLog)
	c.log.Info("started", "config", p.Config, "data", p.Data)

	if err := config.WriteDefault(p.Settings); err != nil {
		return nil, err
	}
	settings, problems, err := config.Load(p.Settings)
	if err != nil {
		return nil, err
	}
	c.logProblems(problems)
	c.settings.Store(&settings)

	watcher, err := config.Watch(p.Settings, c.reloaded)
	if err != nil {
		return nil, err
	}
	c.closers = append(c.closers, watcher.Close)

	c.db, err = store.Open(ctx, p.Database)
	if err != nil {
		return nil, err
	}
	c.closers = append(c.closers, c.db.Close)

	lang := settings.General.Language
	if lang == "system" {
		lang = i18n.Match(locale)
	}
	c.catalog = i18n.New(lang)
	if err := c.catalog.Add("", ui.Messages); err != nil {
		return nil, err
	}
	return c, nil
}

// Settings returns the current settings.
func (c *core) Settings() config.Settings { return *c.settings.Load() }

func (c *core) reloaded(s config.Settings, problems []config.Problem, err error) {
	if err != nil {
		c.log.Warn("settings not reloaded; keeping the last valid ones", "err", err)
		return
	}
	c.logProblems(problems)
	c.settings.Store(&s)
	c.log.Info("settings reloaded")
	c.mu.Lock()
	listeners := slices.Clone(c.listeners)
	c.mu.Unlock()
	for _, fn := range listeners {
		fn(s)
	}
}

func (c *core) logProblems(problems []config.Problem) {
	for _, p := range problems {
		c.log.Warn("settings problem", "line", p.Line, "key", p.Key, "problem", p.Message)
	}
}

// Close closes the services in the reverse order of opening.
func (c *core) Close() error {
	var errs []error
	for i := len(c.closers) - 1; i >= 0; i-- {
		if err := c.closers[i](); err != nil {
			errs = append(errs, err)
		}
	}
	c.closers = nil
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	return nil
}
