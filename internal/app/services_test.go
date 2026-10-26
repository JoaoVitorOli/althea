package app

import (
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JoaoVitorOli/althea/sdk"
)

type counting struct {
	starts, stops atomic.Int32
	panics        bool
}

func (*counting) Manifest() sdk.Manifest             { return sdk.Manifest{ID: "althea.counting"} }
func (*counting) Commands(sdk.Context) []sdk.Command { return nil }

func (c *counting) Serve(ctx sdk.Context) error {
	c.starts.Add(1)
	if c.panics {
		panic("boom")
	}
	<-ctx.Done()
	c.stops.Add(1)
	return nil
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); !ok(); {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestServicesRunAndRestart(t *testing.T) {
	a := &althea{core: &core{log: slog.New(slog.NewTextHandler(io.Discard, nil))}}
	c := &counting{}
	s := newServices(a, []sdk.Extension{c})
	s.startAll()
	waitFor(t, "the start", func() bool { return c.starts.Load() == 1 })
	s.restart("althea.counting")
	waitFor(t, "the restart", func() bool { return c.starts.Load() == 2 })
	if c.stops.Load() != 1 {
		t.Errorf("stops = %d: the first run was not stopped before the second", c.stops.Load())
	}
	s.stopAll()
	if c.stops.Load() != 2 {
		t.Errorf("stops = %d after stopAll", c.stops.Load())
	}
	s.restart("althea.counting")
	time.Sleep(10 * time.Millisecond)
	if c.starts.Load() != 2 {
		t.Error("a service started after stopAll")
	}
}

func TestAPanickingServiceIsContained(t *testing.T) {
	a := &althea{core: &core{log: slog.New(slog.NewTextHandler(io.Discard, nil))}}
	c := &counting{panics: true}
	s := newServices(a, []sdk.Extension{c})
	s.startAll()
	waitFor(t, "the start", func() bool { return c.starts.Load() == 1 })
	s.stopAll()
}
