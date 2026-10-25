// Package fswatch tells when files change, however they are saved: written in place, or
// written elsewhere and renamed over them, as most editors do. Bursts of events become one
// call.
package fswatch

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Debounce is how long files must be quiet after an event before the change is reported.
const Debounce = 100 * time.Millisecond

// Watcher watches until it is closed.
type Watcher struct {
	fs   *fsnotify.Watcher
	done chan struct{}
	wg   sync.WaitGroup
}

// Watch calls changed, on a goroutine of its own, after the file at path changes. It watches
// the file's directory, so the file may be created, replaced or removed while watched.
func Watch(path string, changed func()) (*Watcher, error) {
	name := filepath.Base(path)
	return watch(filepath.Dir(path), func(n string) bool { return n == name }, changed)
}

// WatchDir calls changed, on a goroutine of its own, after a file in dir whose name match
// accepts changes, is created or removed. It creates dir if it is missing.
func WatchDir(dir string, match func(name string) bool, changed func()) (*Watcher, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("watch %s: %w", dir, err)
	}
	return watch(dir, match, changed)
}

func watch(dir string, match func(string) bool, changed func()) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("watch %s: %w", dir, err)
	}
	if err := fsw.Add(dir); err != nil {
		_ = fsw.Close()
		return nil, fmt.Errorf("watch %s: %w", dir, err)
	}
	w := &Watcher{fs: fsw, done: make(chan struct{})}
	w.wg.Add(1)
	go w.loop(match, changed)
	return w, nil
}

func (w *Watcher) loop(match func(string) bool, changed func()) {
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
			if match(filepath.Base(e.Name)) {
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
