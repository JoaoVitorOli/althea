// Package netpolicy is the only way Althea reaches the network. Its client lets a request through
// only when the context it was made with says the user asked for it, and logs each one, so that the
// log holds every request Althea made. See docs/adr/0012-privacy.md.
package netpolicy

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// ErrNotAllowed is the error of a request nobody asked for.
var ErrNotAllowed = errors.New("netpolicy: this request does not come from something the user did")

type allowedKey struct{}

// Allow marks ctx as that of an action the user took or of a feature they turned on: requests made
// with it may leave.
func Allow(ctx context.Context) context.Context {
	return context.WithValue(ctx, allowedKey{}, true)
}

// Allowed reports whether requests made with ctx may leave.
func Allowed(ctx context.Context) bool {
	allowed, _ := ctx.Value(allowedKey{}).(bool)
	return allowed
}

// Policy makes the clients.
type Policy struct {
	log       *slog.Logger
	transport http.RoundTripper
}

// New returns a Policy logging to log. A nil transport keeps connections a little while, then lets
// them go, so that nothing lingers while the launcher is hidden.
func New(log *slog.Logger, transport http.RoundTripper) *Policy {
	if transport == nil {
		t, _ := http.DefaultTransport.(*http.Transport)
		t = t.Clone()
		t.IdleConnTimeout = 30 * time.Second
		transport = t
	}
	return &Policy{log: log, transport: transport}
}

// Client returns the client of a feature, as an extension's ID, which names it in the log. It has
// no time limit, as a streamed answer takes as long as it takes: requests end with their context.
func (p *Policy) Client(feature string) *http.Client {
	return &http.Client{Transport: guarded{p, feature}}
}

// guarded lets through what is allowed, and logs all of it.
type guarded struct {
	p       *Policy
	feature string
}

func (g guarded) RoundTrip(req *http.Request) (*http.Response, error) {
	// Never the query, which may hold what the user typed, nor the headers, which hold keys.
	log := g.p.log.With("feature", g.feature, "method", req.Method, "host", req.URL.Host, "path", req.URL.Path)
	if !Allowed(req.Context()) {
		log.Warn("network request refused")
		return nil, ErrNotAllowed
	}
	start := time.Now()
	res, err := g.p.transport.RoundTrip(req)
	if err != nil {
		log.Info("network request failed", "err", err, "took", time.Since(start))
		return nil, err
	}
	log.Info("network request", "status", res.StatusCode, "took", time.Since(start))
	return res, nil
}
