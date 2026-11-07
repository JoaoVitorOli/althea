package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"github.com/JoaoVitorOli/althea/internal/config"
	"github.com/JoaoVitorOli/althea/sdk"
	"github.com/JoaoVitorOli/althea/sdk/sdktest"
)

// fakeUpdates is an updater whose feed answers what the test says, counting the checks.
func fakeUpdates(t *testing.T, optedIn bool, answer func() (*mygo.Update, error)) (*updater, *int) {
	t.Helper()
	c, err := bootstrap(context.Background(), portable(t), "en")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	s := config.Defaults()
	s.General.CheckUpdates = optedIn
	c.settings.Store(&s)
	checks := 0
	u := newUpdater(&althea{core: c})
	u.enabled = func() bool { return true }
	u.check = func(context.Context) (*mygo.Update, error) {
		checks++
		return answer()
	}
	return u, &checks
}

// waitIdle waits for a background check to finish.
func waitIdle(t *testing.T, u *updater) {
	t.Helper()
	for range 200 {
		u.mu.Lock()
		running := u.running
		u.mu.Unlock()
		if !running {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the check never finished")
}

func TestNoUpdateCheckWithoutOptingIn(t *testing.T) {
	u, checks := fakeUpdates(t, false, func() (*mygo.Update, error) { return nil, nil })
	u.soon()
	waitIdle(t, u)
	if *checks != 0 {
		t.Errorf("checked %d times without the user opting in", *checks)
	}
}

func TestUpdatesAreCheckedOnceADay(t *testing.T) {
	u, checks := fakeUpdates(t, true, func() (*mygo.Update, error) { return nil, nil })
	u.soon()
	waitIdle(t, u)
	u.soon()
	waitIdle(t, u)
	if *checks != 1 {
		t.Errorf("checked %d times in a day", *checks)
	}
	u.mu.Lock()
	u.checked = time.Now().Add(-25 * time.Hour)
	u.mu.Unlock()
	u.soon()
	waitIdle(t, u)
	if *checks != 2 {
		t.Errorf("checked %d times after a day", *checks)
	}
}

func TestAFailedCheckIsTriedAgain(t *testing.T) {
	fail := true
	u, checks := fakeUpdates(t, true, func() (*mygo.Update, error) {
		if fail {
			return nil, errors.New("offline")
		}
		return nil, nil
	})
	u.soon()
	waitIdle(t, u)
	fail = false
	u.soon()
	waitIdle(t, u)
	if *checks != 2 {
		t.Errorf("checked %d times", *checks)
	}
}

func TestAFoundUpdateIsOffered(t *testing.T) {
	u, _ := fakeUpdates(t, true, func() (*mygo.Update, error) {
		return &mygo.Update{Version: "1.1.0", Notes: "- Faster search"}, nil
	})
	s := &settingsExtension{updates: u}
	if err := u.a.addMessages(s.Manifest()); err != nil {
		t.Fatal(err)
	}
	ctx := sdktest.New(t, s, "en")
	if cmd := u.command(ctx); cmd.Title != "Check for Updates" {
		t.Errorf("before: %q", cmd.Title)
	}
	if err := u.command(ctx).Run(ctx); err != nil {
		t.Fatal(err)
	}
	d, ok := ctx.Pushed[0].(*sdk.Detail)
	if !ok || d.Actions[0].Title != "Install and Restart" {
		t.Fatalf("pushed %#v", ctx.Pushed)
	}
	if cmd := u.command(ctx); cmd.Title != "Update to Althea 1.1.0" {
		t.Errorf("after: %q", cmd.Title)
	}
}
