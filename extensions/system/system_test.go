package system_test

import (
	"slices"
	"testing"

	"github.com/JoaoVitorOli/althea/extensions/system"
	"github.com/JoaoVitorOli/althea/sdk"
	"github.com/JoaoVitorOli/althea/sdk/sdktest"
)

type calls []string

func platform(c *calls, dark *bool) system.Platform {
	rec := func(name string) func() error { return func() error { *c = append(*c, name); return nil } }
	return system.Platform{
		Lock: rec("lock"), Sleep: rec("sleep"), Restart: rec("restart"), ShutDown: rec("shutdown"),
		SignOut: rec("signout"), EmptyRecycleBin: rec("emptybin"),
		IsDarkMode:  func() bool { return *dark },
		SetDarkMode: func(on bool) error { *dark = on; *c = append(*c, "dark"); return nil },
	}
}

func command(t *testing.T, cmds []sdk.Command, id string) sdk.Command {
	t.Helper()
	i := slices.IndexFunc(cmds, func(c sdk.Command) bool { return c.ID == id })
	if i < 0 {
		t.Fatalf("no command %q", id)
	}
	return cmds[i]
}

func TestCommandsRunTheSystemsActions(t *testing.T) {
	var c calls
	dark := false
	ext := system.New(platform(&c, &dark))
	ctx := sdktest.New(t, ext, "pt-BR")
	cmds := ext.Commands(ctx)

	lock := command(t, cmds, "lock")
	if lock.Title != "Bloquear tela" {
		t.Errorf("lock title = %q, want it translated", lock.Title)
	}
	if !slices.Contains(lock.Keywords, "lock") || !slices.Contains(lock.Keywords, "bloquear") {
		t.Errorf("lock keywords = %v, want both languages", lock.Keywords)
	}
	for _, id := range []string{"lock", "sleep", "restart", "shutdown", "signout", "emptybin", "darkmode"} {
		if err := command(t, cmds, id).Run(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if want := []string{"lock", "sleep", "restart", "shutdown", "signout", "emptybin", "dark"}; !slices.Equal(c, want) {
		t.Errorf("calls = %v, want %v", c, want)
	}
	if !dark {
		t.Error("toggling dark mode from light did not turn it on")
	}
	if ctx.Hidden < 6 {
		t.Errorf("hid %d times, want the launcher out of the way of each action", ctx.Hidden)
	}
}

func TestWhatCannotBeUndoneAsksFirst(t *testing.T) {
	var c calls
	dark := false
	ext := system.New(platform(&c, &dark))
	ctx := sdktest.New(t, ext, "en")
	for _, id := range []string{"restart", "shutdown", "signout", "emptybin"} {
		if command(t, ext.Commands(ctx), id).Confirm == "" {
			t.Errorf("%s does not ask for confirmation", id)
		}
	}
	if command(t, ext.Commands(ctx), "lock").Confirm != "" {
		t.Error("lock asks for confirmation")
	}
}

func TestMissingPlatformFunctionsLeaveTheirCommandsOut(t *testing.T) {
	ext := system.New(system.Platform{Lock: func() error { return nil }})
	cmds := ext.Commands(sdktest.New(t, ext, "en"))
	if len(cmds) != 1 || cmds[0].ID != "lock" {
		t.Errorf("commands = %v, want lock alone", cmds)
	}
}
