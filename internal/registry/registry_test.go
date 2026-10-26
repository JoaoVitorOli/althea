package registry_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/JoaoVitorOli/althea/internal/registry"
	"github.com/JoaoVitorOli/althea/sdk"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

// fake is an extension whose behavior each test chooses.
type fake struct {
	id       string
	commands []sdk.Command
	items    func() ([]sdk.Item, error)
	answer   func(ctx context.Context, q string) (*sdk.Item, error)
}

func (f *fake) Manifest() sdk.Manifest             { return sdk.Manifest{ID: f.id, Title: f.id} }
func (f *fake) Commands(sdk.Context) []sdk.Command { return f.commands }

type source struct{ *fake }

func (s source) Items(sdk.Context) ([]sdk.Item, error) { return s.items() }

type answerer struct{ *fake }

func (a answerer) Answer(ctx sdk.Context, q string) (*sdk.Item, error) { return a.answer(ctx, q) }

type hostContext struct {
	context.Context
	log *slog.Logger
}

func (h hostContext) Log() *slog.Logger           { return h.log }
func (hostContext) Open(string) error             { return nil }
func (hostContext) CopyText(string) error         { return nil }
func (hostContext) Hide()                         {}
func (hostContext) Toast(string)                  {}
func (hostContext) T(key string, _ ...any) string { return key }
func (hostContext) Language() string              { return "en" }
func (hostContext) Push(sdk.View)                 {}
func (hostContext) Pop()                          {}
func (hostContext) Storage() sdk.Storage          { return nil }
func (hostContext) Preferences() sdk.Preferences  { return nil }
func (hostContext) Database() sdk.Database        { return nil }
func (hostContext) ClipboardText() string         { return "" }
func (hostContext) Paste(string) error            { return nil }
func (hostContext) Refresh()                      {}

func newRegistry(t *testing.T, exts ...sdk.Extension) *registry.Registry {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := registry.New(exts, registry.Deps{
		Use:           func(string, string) float64 { return 0 },
		Context:       func(ctx context.Context, _ string) sdk.Context { return hostContext{ctx, log} },
		Log:           log,
		AnswerTimeout: 50 * time.Millisecond,
	})
	if err := r.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	return r
}

func titles(rs []registry.Result) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Item.Title)
	}
	return out
}

var apps = source{&fake{id: "althea.apps", items: func() ([]sdk.Item, error) {
	return []sdk.Item{{ID: "code", Title: "Visual Studio Code"}, {ID: "chrome", Title: "Google Chrome"}}, nil
}}}

var system = &fake{id: "althea.system", commands: []sdk.Command{
	{ID: "lock", Title: "Lock Screen", Keywords: []string{"bloquear"}, Run: func(sdk.Context) error { return nil }},
}}

func TestQueryFindsCommandsAndItems(t *testing.T) {
	r := newRegistry(t, apps, system)
	if got := titles(r.Query(context.Background(), "chr")); !slices.Equal(got, []string{"Google Chrome"}) {
		t.Errorf("Query(chr) = %v", got)
	}
	got := r.Query(context.Background(), "lock")
	if len(got) != 1 || got[0].Item.Title != "Lock Screen" || got[0].Key != "althea.system/lock" {
		t.Fatalf("Query(lock) = %+v", got)
	}
	if !got[0].Command || len(got[0].Item.Actions) == 0 || got[0].Item.Actions[0].Run == nil {
		t.Error("a command has no action to run it")
	}
	if got := titles(r.Query(context.Background(), "bloquear")); !slices.Equal(got, []string{"Lock Screen"}) {
		t.Errorf("Query by keyword = %v", got)
	}
}

func TestAnswersComeFirstAndSlowOnesAreDropped(t *testing.T) {
	calc := answerer{&fake{id: "althea.calculator", answer: func(_ context.Context, q string) (*sdk.Item, error) {
		if q == "2+2" {
			return &sdk.Item{ID: "result", Title: "4"}, nil
		}
		return nil, nil
	}}}
	slow := answerer{&fake{id: "slow", answer: func(ctx context.Context, _ string) (*sdk.Item, error) {
		<-ctx.Done()
		return &sdk.Item{ID: "late", Title: "too late"}, nil
	}}}
	everything := source{&fake{id: "all", items: func() ([]sdk.Item, error) {
		return []sdk.Item{{ID: "x", Title: "2+2 notes"}}, nil
	}}}
	r := newRegistry(t, calc, slow, everything)

	start := time.Now()
	got := r.Query(context.Background(), "2+2")
	if took := time.Since(start); took > time.Second {
		t.Errorf("Query waited %v for a slow extension", took)
	}
	if !slices.Equal(titles(got), []string{"4", "2+2 notes"}) || !got[0].Answer {
		t.Errorf("Query(2+2) = %v, want the answer first and the late one dropped", titles(got))
	}
}

func TestAMisbehavingExtensionCannotBreakTheSearch(t *testing.T) {
	panics := source{&fake{id: "panics", items: func() ([]sdk.Item, error) { panic("boom") }}}
	fails := source{&fake{id: "fails", items: func() ([]sdk.Item, error) { return nil, errors.New("offline") }}}
	panicAnswer := answerer{&fake{id: "panic-answer", answer: func(context.Context, string) (*sdk.Item, error) { panic("bang") }}}
	r := newRegistry(t, panics, fails, panicAnswer, apps)
	if got := titles(r.Query(context.Background(), "code")); !slices.Equal(got, []string{"Visual Studio Code"}) {
		t.Errorf("Query(code) = %v", got)
	}
}

func TestEmptyQueryListsWhatIsUsedMost(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := registry.New([]sdk.Extension{apps, system}, registry.Deps{
		Use: func(key, _ string) float64 {
			if key == "althea.apps/chrome" {
				return 5
			}
			return 0
		},
		Context: func(ctx context.Context, _ string) sdk.Context { return hostContext{ctx, log} },
		Log:     log,
	})
	if err := r.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := titles(r.Query(context.Background(), "")); got[0] != "Google Chrome" || len(got) != 3 {
		t.Errorf("Query(\"\") = %v", got)
	}
}

func TestRefreshWhileQuerying(t *testing.T) {
	r := newRegistry(t, apps, system)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() { _ = r.Refresh(context.Background()) })
		wg.Go(func() { r.Query(context.Background(), "c") })
	}
	wg.Wait()
}
