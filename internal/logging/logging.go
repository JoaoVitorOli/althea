// Package logging writes Althea's log to althea.log in the logs directory, rotating it by size
// so it never grows without bound. Logs stay on the machine (docs/adr/0012-privacy.md).
package logging

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// Options configure the log.
type Options struct {
	Dir     string     // the logs directory, created if needed
	Level   slog.Level // the lowest level written; Info by default
	MaxSize int64      // bytes before the file rotates; 5 MiB by default
	Keep    int        // rotated files kept, althea.1.log being the newest; 3 by default
}

// New returns a logger writing to the log file, and a function closing the file.
func New(o Options) (*slog.Logger, func() error, error) {
	if o.MaxSize <= 0 {
		o.MaxSize = 5 << 20
	}
	if o.Keep <= 0 {
		o.Keep = 3
	}
	if err := os.MkdirAll(o.Dir, 0o700); err != nil {
		return nil, nil, fmt.Errorf("logs: %w", err)
	}
	w := &rotating{dir: o.Dir, max: o.MaxSize, keep: o.Keep}
	if err := w.open(); err != nil {
		return nil, nil, err
	}
	log := slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: o.Level}))
	return log, w.Close, nil
}

// rotating is a file that moves aside to althea.1.log when the next write would grow it past
// max, shifting older files up and dropping the oldest.
type rotating struct {
	dir  string
	max  int64
	keep int

	mu   sync.Mutex
	f    *os.File
	size int64
}

func (w *rotating) name(i int) string {
	if i == 0 {
		return filepath.Join(w.dir, "althea.log")
	}
	return filepath.Join(w.dir, fmt.Sprintf("althea.%d.log", i))
}

func (w *rotating) open() error {
	f, err := os.OpenFile(w.name(0), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("logs: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("logs: %w", err)
	}
	w.f, w.size = f, info.Size()
	return nil
}

func (w *rotating) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size > 0 && w.size+int64(len(p)) > w.max {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *rotating) rotate() error {
	if err := w.f.Close(); err != nil {
		return err
	}
	_ = os.Remove(w.name(w.keep))
	for i := w.keep - 1; i >= 0; i-- {
		if err := os.Rename(w.name(i), w.name(i+1)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return w.open()
}

func (w *rotating) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.f.Close()
}
