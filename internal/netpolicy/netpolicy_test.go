package netpolicy_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/goleak"

	"github.com/JoaoVitorOli/althea/internal/netpolicy"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func serve(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = io.WriteString(w, "fine")
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func get(ctx context.Context, t *testing.T, c *http.Client, url string) (string, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	return string(body), err
}

func TestARequestNobodyAskedForNeverLeaves(t *testing.T) {
	srv, calls := serve(t)
	var logged bytes.Buffer
	c := netpolicy.New(slog.New(slog.NewTextHandler(&logged, nil)), srv.Client().Transport).Client("althea.ai")

	_, err := get(context.Background(), t, c, srv.URL)
	if !errors.Is(err, netpolicy.ErrNotAllowed) {
		t.Errorf("err = %v, want ErrNotAllowed", err)
	}
	if *calls != 0 {
		t.Errorf("the server heard %d requests", *calls)
	}
	if !strings.Contains(logged.String(), "network request refused") {
		t.Errorf("log %q", logged.String())
	}
}

func TestARequestTheUserAskedForLeavesAndIsLoggedWithoutItsQuery(t *testing.T) {
	srv, calls := serve(t)
	var logged bytes.Buffer
	c := netpolicy.New(slog.New(slog.NewTextHandler(&logged, nil)), srv.Client().Transport).Client("althea.ai")

	body, err := get(netpolicy.Allow(context.Background()), t, c, srv.URL+"/v1/x?q=my+secret+text")
	if err != nil || body != "fine" || *calls != 1 {
		t.Fatalf("body %q, err %v, calls %d", body, err, *calls)
	}
	log := logged.String()
	for _, want := range []string{"feature=althea.ai", "method=GET", "path=/v1/x", "status=200"} {
		if !strings.Contains(log, want) {
			t.Errorf("log lacks %q: %s", want, log)
		}
	}
	if strings.Contains(log, "secret") {
		t.Errorf("the log holds the query: %s", log)
	}
}

func TestAllowCarriesToDerivedContexts(t *testing.T) {
	ctx, cancel := context.WithCancel(netpolicy.Allow(context.Background()))
	defer cancel()
	if !netpolicy.Allowed(ctx) {
		t.Error("a context made from an allowed one is not allowed")
	}
	if netpolicy.Allowed(context.Background()) {
		t.Error("the background context is allowed")
	}
}
