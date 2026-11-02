package files

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JoaoVitorOli/althea/sdk"
)

// batch is how many entries a transaction of the scan writes.
const batch = 2000

// indexer keeps the index in step with the folders: a full scan as it starts, then the changes
// the watcher hears.
type indexer struct {
	// changes are what the watchers heard, for follow; lost says some did not fit.
	changes chan string
	lost    chan struct{}

	e       *Extension
	ctx     sdk.Context
	roots   []string
	exclude map[string]bool // folder names left out, lowercase
}

// skip reports whether a file or folder is left out of the index: names starting with a dot,
// and folders named in the exclusions, wherever they are.
func (x *indexer) skip(name string, dir bool) bool {
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "~$") {
		return true
	}
	return dir && x.exclude[strings.ToLower(name)]
}

// excluded reports whether a path lies in a left-out folder.
func (x *indexer) excluded(path string) bool {
	for _, root := range x.roots {
		rel, err := filepath.Rel(root, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		parts := strings.Split(rel, string(filepath.Separator))
		for i, p := range parts {
			if x.skip(p, i < len(parts)-1) {
				return true
			}
		}
		return false
	}
	return true // outside every folder
}

type entry struct {
	path, name string
	folder     bool
	modified   int64
}

// scan reads the folders whole, then forgets what they no longer hold.
func (x *indexer) scan() error {
	gen := x.e.Now().UnixMilli()
	var pending []entry
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		err := x.write(pending, gen)
		pending = pending[:0]
		return err
	}
	for _, root := range x.roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if x.ctx.Err() != nil {
				return x.ctx.Err()
			}
			if err != nil || path == root {
				return nil // unreadable folders are skipped, not fatal
			}
			if x.skip(d.Name(), d.IsDir()) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			en, ok := entryOf(path, d)
			if !ok {
				return nil
			}
			pending = append(pending, en)
			if len(pending) >= batch {
				return flush()
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	if err := flush(); err != nil {
		return err
	}
	_, err := x.ctx.Database().Exec(x.ctx, `DELETE FROM files_entries WHERE seen < ?`, gen)
	return err
}

func entryOf(path string, d fs.DirEntry) (entry, bool) {
	info, err := d.Info()
	if err != nil {
		return entry{}, false
	}
	return entry{path: path, name: d.Name(), folder: d.IsDir(), modified: info.ModTime().UnixMilli()}, true
}

// write keeps entries, as found by scan gen.
func (x *indexer) write(entries []entry, gen int64) error {
	return x.ctx.Database().Tx(x.ctx, func(tx sdk.Queryer) error {
		for _, en := range entries {
			if _, err := tx.Exec(x.ctx, `
				INSERT INTO files_entries (path, name, folder, modified, seen) VALUES (?, ?, ?, ?, ?)
				ON CONFLICT (path) DO UPDATE SET
					name = excluded.name, folder = excluded.folder, modified = excluded.modified, seen = excluded.seen`,
				en.path, en.name, en.folder, en.modified, gen); err != nil {
				return err
			}
		}
		return nil
	})
}

// changed brings the index in step with paths that changed: what exists is kept, and a new
// folder read whole; what is gone is forgotten, with all it held.
func (x *indexer) changed(paths map[string]bool) error {
	gen := x.e.Now().UnixMilli()
	var keep []entry
	for path := range paths {
		if x.excluded(path) {
			continue
		}
		info, err := os.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			if _, err := x.ctx.Database().Exec(x.ctx,
				`DELETE FROM files_entries WHERE path = ? OR path LIKE ? ESCAPE '\'`,
				path, escapeLike(path+string(filepath.Separator))+"%"); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			continue
		}
		keep = append(keep, entry{path: path, name: info.Name(), folder: info.IsDir(), modified: info.ModTime().UnixMilli()})
		if info.IsDir() && !x.known(path) {
			// A folder new to the index, as one moved in: read what it holds.
			_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
				if err != nil || p == path {
					return nil
				}
				if x.skip(d.Name(), d.IsDir()) {
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
				if en, ok := entryOf(p, d); ok {
					keep = append(keep, en)
				}
				return nil
			})
		}
	}
	for start := 0; start < len(keep); start += batch {
		if err := x.write(keep[start:min(start+batch, len(keep))], gen); err != nil {
			return err
		}
	}
	return nil
}

func (x *indexer) known(path string) bool {
	var n int
	_ = x.ctx.Database().QueryRow(x.ctx, `SELECT count(*) FROM files_entries WHERE path = ?`, path).Scan(&n)
	return n > 0
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// settle is how long changes gather before the index takes them, as a copy of many files
// makes many changes.
const settle = 300 * time.Millisecond

// watch starts hearing the changes of each folder, for follow, and returns what stops it.
func (x *indexer) watch() (stop func()) {
	x.changes = make(chan string, 1024)
	x.lost = make(chan struct{}, 1)
	if x.e.p.Watch == nil {
		return func() {}
	}
	var stops []func()
	for _, root := range x.roots {
		stop, err := x.e.p.Watch(root, func(path string) {
			if path == "" {
				path = root // changes were lost: read the folder again
			}
			select {
			case x.changes <- path:
			default:
				// More than fit at once, as while the scan holds them: read everything again.
				select {
				case x.lost <- struct{}{}:
				default:
				}
			}
		})
		if err != nil {
			x.ctx.Log().Warn("folder not watched", "folder", root, "err", err)
			continue
		}
		stops = append(stops, stop)
	}
	return func() {
		for _, stop := range stops {
			stop()
		}
	}
}

// follow keeps the index in step with what watch hears until ctx is done.
func (x *indexer) follow() error {
	pending := map[string]bool{}
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	for {
		select {
		case <-x.ctx.Done():
			return nil
		case p := <-x.changes:
			pending[p] = true
			timer.Reset(settle)
		case <-x.lost:
			for _, root := range x.roots {
				pending[root] = true
			}
			timer.Reset(settle)
		case <-timer.C:
			rescan := false
			for _, root := range x.roots {
				rescan = rescan || pending[root]
			}
			var err error
			if rescan {
				err = x.scan()
			} else {
				err = x.changed(pending)
			}
			if err != nil && x.ctx.Err() == nil {
				x.ctx.Log().Warn("index not updated", "err", err)
			}
			pending = map[string]bool{}
		}
	}
}
