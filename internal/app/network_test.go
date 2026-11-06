package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JoaoVitorOli/althea/internal/netpolicy"
	"github.com/JoaoVitorOli/althea/internal/registry"
	"github.com/JoaoVitorOli/althea/sdk"
)

// What runs by itself is refused the network, and what the user set off reaches it.
func TestExtensionsReachTheNetworkOnlyForWhatTheUserDid(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer srv.Close()
	a := &althea{net: netpolicy.New(slog.New(slog.DiscardHandler), srv.Client().Transport)}
	a.core = &core{log: slog.New(slog.DiscardHandler)}
	do := func(ctx context.Context, c *http.Client) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		res, err := c.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		_, _ = io.Copy(io.Discard, res.Body)
		return nil
	}

	alone := a.extensionContext(context.Background(), "althea.ai")
	if err := do(alone, alone.HTTP()); !errors.Is(err, netpolicy.ErrNotAllowed) {
		t.Errorf("err %v", err)
	}
	if calls != 0 {
		t.Fatalf("the server heard %d requests", calls)
	}

	asked := a.actionContext(context.Background(), "althea.ai")
	if err := do(asked, asked.HTTP()); err != nil || calls != 1 {
		t.Errorf("err %v, calls %d", err, calls)
	}

	// Lending it to another extension, as the Translator does the AI provider, keeps the permission.
	lent := a.extensionContext(asked, "althea.translator")
	if err := do(lent, lent.HTTP()); err != nil || calls != 2 {
		t.Errorf("err %v, calls %d", err, calls)
	}
}

// An Answer may fetch what the query needs, in the background, but not what runs by itself.
func TestAnswersFetchInTheBackground(t *testing.T) {
	calls := make(chan struct{}, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls <- struct{}{} }))
	defer srv.Close()
	a := &althea{net: netpolicy.New(slog.New(slog.DiscardHandler), srv.Client().Transport)}
	a.core = &core{log: slog.New(slog.DiscardHandler)}
	fetch := func(parent context.Context) error {
		ctx, cancel := context.WithTimeout(parent, 50*time.Millisecond)
		h := a.extensionContext(ctx, "althea.calculator")
		cancel() // as an Answer's ends before what it started
		done := make(chan error, 1)
		h.Go(func(ctx sdk.Context) {
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, http.NoBody)
			res, err := ctx.HTTP().Do(req)
			if err == nil {
				res.Body.Close()
			}
			done <- err
		})
		return <-done
	}

	if err := fetch(context.Background()); !errors.Is(err, netpolicy.ErrNotAllowed) {
		t.Errorf("Go of an Items call: err %v, want it refused", err)
	}
	if err := fetch(registry.Answer(context.Background())); err != nil {
		t.Errorf("Go of an Answer: err %v", err)
	}
	if len(calls) != 1 {
		t.Errorf("the server heard %d requests, want 1", len(calls))
	}
}
