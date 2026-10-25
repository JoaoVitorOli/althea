package app

import (
	"context"
	"fmt"

	"github.com/JoaoVitorOli/althea/internal/config"
	"github.com/JoaoVitorOli/althea/internal/ui"
)

// onboarding is the store namespace that remembers the onboarding finished.
const onboarding = "althea.onboarding"

// needsOnboarding reports whether the user has yet to go through the onboarding.
func (c *core) needsOnboarding(ctx context.Context) bool {
	_, done, err := c.db.KV(onboarding).Get(ctx, "done")
	if err != nil {
		c.log.Warn("onboarding state", "err", err)
		return false // better no onboarding than one on every start
	}
	return !done
}

// finishOnboarding writes the choices into the settings file, which the watcher then applies,
// and remembers that the onboarding is done.
func (c *core) finishOnboarding(ctx context.Context, ch ui.Choices) error {
	for key, v := range map[string]any{
		"hotkey.toggle":          ch.Hotkey,
		"theme.name":             ch.Theme,
		"general.start_at_login": ch.StartAtLogin,
		"general.check_updates":  ch.CheckUpdates,
	} {
		if err := config.Set(c.paths.Settings, key, v); err != nil {
			return fmt.Errorf("onboarding: %w", err)
		}
	}
	return c.db.KV(onboarding).Set(ctx, "done", []byte("1"))
}

// choicesOf returns what the onboarding starts from: the current settings.
func choicesOf(s config.Settings) ui.Choices {
	return ui.Choices{
		Hotkey:       s.Hotkey.Toggle,
		Theme:        s.Theme.Name,
		StartAtLogin: s.General.StartAtLogin,
		CheckUpdates: s.General.CheckUpdates,
	}
}
