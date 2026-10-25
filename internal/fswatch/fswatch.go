// Package fswatch tells when a file changes, however it is saved: written in place, or written
// elsewhere and renamed over it, as most editors do. Bursts of events become one call.
package fswatch

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Debounce is how long a file must be quiet after an event before the change is reported.
const Debounce = 100 * time.Millisecond

// Watcher watches one file until it is closed.
type Watcher struct {
	fs   *fsnotify.Watcher
	done chan struct{}
	wg   sync.WaitGroup
}

// Watch calls changed, on a goroutine of its own, after the file at path changes. It watches
// the file's directory, so the file may be created, replaced or removed while watched.
func Watch(path string, changed func()) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("watch %s: %w", path, err)
	}
	if err := fsw.Add(filepath.Dir(path)); err != nil {
		_ = fsw.Close()
		return nil, fmt.Errorf("watch %s: %w", path, err)
	}
	w := &Watcher{fs: fsw, done: make(chan struct{})}
	w.wg.Add(1)
	go w.loop(filepath.Clean(path), changed)
	return w, nil
}

func (w *Watcher) loop(path string, changed func()) {
	defer w.wg.Done()
	timer := time.NewTimer(Debounce)
	timer.Stop()
	defer timer.Stop()
	for {
		select {
		case <-w.done:
			return
		case e, ok := <-w.fs.Events:
			if !ok {
				return
			}
			if filepath.Clean(e.Name) == path {
				timer.Reset(Debounce)
			}
		case _, ok := <-w.fs.Errors:
			if !ok {
				return
			}
			// An overflowed event queue may hide a change: look again.
			timer.Reset(Debounce)
		case <-timer.C:
			changed()
		}
	}
}

// Close stops watching and waits until no call to changed is running.
func (w *Watcher) Close() error {
	close(w.done)
	err := w.fs.Close()
	w.wg.Wait()
	return err
}
