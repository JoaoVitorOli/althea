package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"github.com/JoaoVitorOli/althea/sdk"
)

// checkEvery is how long after one check for updates the next may run, when the user opted in.
// Checks run as Althea starts and as the launcher shows, never on a timer (docs/spec.md).
const checkEvery = 24 * time.Hour

// updater checks for and installs new versions of Althea, signed with the key in mygo.json and
// published on GitHub Releases. Checks without the user asking happen only when they opted in
// (general.check_updates, docs/adr/0012-privacy.md).
type updater struct {
	a *althea
	// check and enabled are mygo.Updater's, replaced in tests.
	check   func(ctx context.Context) (*mygo.Update, error)
	enabled func() bool

	mu      sync.Mutex
	checked time.Time
	running bool
	found   *mygo.Update
}

func newUpdater(a *althea) *updater {
	return &updater{a: a, check: mygo.Updater.Check, enabled: mygo.Updater.Enabled}
}

// soon checks in the background when the user opted in and the last check is a day old.
func (u *updater) soon() {
	if u == nil || !u.a.Settings().General.CheckUpdates || !u.enabled() {
		return
	}
	u.mu.Lock()
	due := !u.running && time.Since(u.checked) >= checkEvery
	if due {
		u.running = true
	}
	u.mu.Unlock()
	if !due {
		return
	}
	go func() {
		if _, err := u.run(context.Background()); err != nil {
			u.a.log.Info("update check failed", "err", err)
		}
	}()
}

// run checks now, and remembers what it found.
func (u *updater) run(ctx context.Context) (*mygo.Update, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	up, err := u.check(ctx)
	u.mu.Lock()
	u.running = false
	if err == nil {
		u.checked, u.found = time.Now(), up
	}
	u.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if up != nil {
		u.a.log.Info("update available", "version", up.Version)
		if u.a.search != nil {
			go u.a.search.refreshNow() // the command now offers it
		}
	}
	return up, nil
}

func (u *updater) available() *mygo.Update {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.found
}

// command checks for updates, or, with one found, shows it.
func (u *updater) command(ctx sdk.Context) sdk.Command {
	cmd := sdk.Command{
		ID: "check-updates", Title: ctx.T("updates.check"), Subtitle: ctx.T("updates.current", "version", mygo.App.Version()),
		Icon: sdk.Icon{Glyph: "restart"}, Keywords: strings.Fields(ctx.T("updates.keywords")),
		Run: func(ctx sdk.Context) error {
			up, err := u.run(ctx)
			if err != nil {
				return fmt.Errorf("%s: %w", ctx.T("updates.failed"), err)
			}
			if up == nil {
				ctx.Toast(ctx.T("updates.up_to_date", "version", mygo.App.Version()))
				return nil
			}
			ctx.Push(u.detail(ctx, up))
			return nil
		},
	}
	if up := u.available(); up != nil {
		cmd.Title = ctx.T("updates.install_title", "version", up.Version)
		cmd.Run = func(ctx sdk.Context) error {
			ctx.Push(u.detail(ctx, up))
			return nil
		}
	}
	return cmd
}

// detail shows a new version's notes, with the action that installs it and restarts Althea.
func (u *updater) detail(ctx sdk.Context, up *mygo.Update) *sdk.Detail {
	notes := up.Notes
	if strings.TrimSpace(notes) == "" {
		notes = ctx.T("updates.no_notes")
	}
	return &sdk.Detail{
		Title:    ctx.T("updates.install_title", "version", up.Version),
		Markdown: "# Althea " + up.Version + "\n\n" + notes,
		Actions: []sdk.Action{{
			Title: ctx.T("updates.install"), Icon: sdk.Icon{Glyph: "restart"},
			Run: func(ctx sdk.Context) error {
				ctx.Toast(ctx.T("updates.downloading", "version", up.Version))
				if err := up.Install(ctx, nil); err != nil {
					if errors.Is(err, context.Canceled) {
						return nil
					}
					return fmt.Errorf("%s: %w", ctx.T("updates.failed"), err)
				}
				u.a.log.Info("update installed, restarting", "version", up.Version)
				mygo.RunOnMain(func() {
					if u.a.notes != nil {
						u.a.notes.flush()
					}
					mygo.App.Relaunch()
				})
				return nil
			},
		}},
	}
}
