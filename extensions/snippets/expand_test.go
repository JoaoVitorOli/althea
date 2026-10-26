package snippets_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/JoaoVitorOli/althea/extensions/snippets"
	"github.com/JoaoVitorOli/althea/sdk/sdktest"
)

// typed is a fake keyboard: what the extension listens with, and what it typed.
type typed struct {
	mu       sync.Mutex
	on       func(snippets.Key)
	stopped  bool
	typed    []string
	erased   []int
	backs    []int
	password bool
}

func (f *typed) platform() snippets.Platform {
	return snippets.Platform{
		Keys: func(on func(snippets.Key)) (func(), error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.on = on
			return func() { f.mu.Lock(); f.stopped = true; f.mu.Unlock() }, nil
		},
		Type: func(erase int, text string, back int) error {
			f.erased, f.typed, f.backs = append(f.erased, erase), append(f.typed, text), append(f.backs, back)
			return nil
		},
		InPassword: func() bool { return f.password },
	}
}

func (f *typed) listening() func(snippets.Key) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.on
}

func (f *typed) write(s string) {
	on := f.listening()
	for _, r := range s {
		on(snippets.Key{Text: string(r)})
	}
}

// serving runs the extension's service until the test ends.
func serving(t *testing.T, e *snippets.Extension, ctx *sdktest.Context, f *typed) {
	t.Helper()
	c, cancel := context.WithCancel(context.Background())
	ctx.Context = c
	done := make(chan error)
	go func() { done <- e.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	for deadline := time.Now().Add(time.Second); f.listening() == nil; {
		if time.Now().After(deadline) {
			t.Fatal("expansion did not start")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestKeywordsExpandAsTheyAreTyped(t *testing.T) {
	f := &typed{}
	e := snippets.New(f.platform())
	e.Now = func() time.Time { return time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC) }
	ctx := sdktest.New(t, e, "en")
	if err := create(t, e, ctx, "Address", "Rua das Flores, 10{cursor}!", ";addr"); err != nil {
		t.Fatal(err)
	}
	ctx.Prefs["expand"] = "true"
	serving(t, e, ctx, f)

	f.write("my ;adx")
	f.listening()(snippets.Key{Backspace: true})
	f.write("dr") // ";addr" once the Backspace takes the x
	if len(f.typed) != 1 || f.typed[0] != "Rua das Flores, 10!" || f.erased[0] != 5 || f.backs[0] != 1 {
		t.Fatalf("typed %q, erased %v, back %v", f.typed, f.erased, f.backs)
	}

	// A key that ends the word forgets what came before it.
	f.write(";ad")
	f.listening()(snippets.Key{})
	f.write("dr")
	if len(f.typed) != 1 {
		t.Errorf("expanded across a word break: %q", f.typed)
	}
}

func TestNothingExpandsInPasswordFields(t *testing.T) {
	f := &typed{password: true}
	e := snippets.New(f.platform())
	ctx := sdktest.New(t, e, "en")
	_ = create(t, e, ctx, "Address", "Rua", ";addr")
	ctx.Prefs["expand"] = "true"
	serving(t, e, ctx, f)
	f.write(";addr")
	if len(f.typed) != 0 {
		t.Errorf("typed %q in a password field", f.typed)
	}
}

func TestExpansionIsOffUntilTurnedOn(t *testing.T) {
	f := &typed{}
	e := snippets.New(f.platform())
	ctx := sdktest.New(t, e, "en")
	if ctx.Prefs["expand"] != "false" {
		t.Errorf("expand defaults to %q", ctx.Prefs["expand"])
	}
	if err := e.Serve(ctx); err != nil || f.listening() != nil {
		t.Errorf("Serve = %v; listened %v", err, f.listening() != nil)
	}
	if prefs := snippets.New(snippets.Platform{}).Preferences(ctx); len(prefs) != 0 {
		t.Error("expansion is offered where the host cannot watch the keyboard")
	}
}
