package config

import "github.com/JoaoVitorOli/althea/internal/fswatch"

// Watcher reloads the settings file while it is open.
type Watcher struct{ fs *fswatch.Watcher }

// Watch calls loaded with the result of Load each time the settings file at path is saved, on
// a goroutine of its own. Callers keep their last valid settings when err is set.
func Watch(path string, loaded func(s Settings, problems []Problem, err error)) (*Watcher, error) {
	w, err := fswatch.Watch(path, func() { loaded(Load(path)) })
	if err != nil {
		return nil, err
	}
	return &Watcher{fs: w}, nil
}

// Close stops watching.
func (w *Watcher) Close() error { return w.fs.Close() }
