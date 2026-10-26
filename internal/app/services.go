package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/JoaoVitorOli/althea/sdk"
)

// services runs the extensions that work in the background (sdk.Service), each again when its
// preferences change.
type services struct {
	a *althea

	mu      sync.Mutex
	running map[string]*serving // by extension
	exts    map[string]sdk.Service
	closed  bool
}

// serving is one run of a service.
type serving struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func newServices(a *althea, exts []sdk.Extension) *services {
	s := &services{a: a, running: map[string]*serving{}, exts: map[string]sdk.Service{}}
	for _, e := range exts {
		if svc, ok := e.(sdk.Service); ok {
			s.exts[e.Manifest().ID] = svc
		}
	}
	return s
}

// startAll starts every service.
func (s *services) startAll() {
	for id := range s.exts {
		s.restart(id)
	}
}

// restart stops the service of ext, if it runs, and starts it again with a new context.
func (s *services) restart(ext string) {
	svc, ok := s.exts[ext]
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if old := s.running[ext]; old != nil {
		old.cancel()
		<-old.done
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &serving{cancel: cancel, done: make(chan struct{})}
	s.running[ext] = run
	go func() {
		defer close(run.done)
		defer cancel()
		if err := s.serve(ctx, svc, ext); err != nil && ctx.Err() == nil {
			s.a.log.Warn("extension service stopped", "extension", ext, "err", err)
		}
	}()
}

// serve runs a service, its panic an error.
func (s *services) serve(ctx context.Context, svc sdk.Service, ext string) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic: %v", p)
			s.a.log.Error("extension panicked", "extension", ext, "call", "serve", "panic", p)
		}
	}()
	return svc.Serve(s.a.extensionContext(ctx, ext))
}

// stopAll stops every service and waits for them, for quitting.
func (s *services) stopAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for _, run := range s.running {
		run.cancel()
		<-run.done
	}
}
