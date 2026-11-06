// Package foldersync keeps the user's settings, themes and other records the same on each of
// their computers through a folder they sync with any tool, as OneDrive, Dropbox or Syncthing
// (docs/adr/0011-sync.md).
//
// Each computer writes only its own file in the folder's devices directory: every record it
// knows, with a clock and the computer that wrote it, compressed and encrypted with a key derived
// from the sync passphrase. A sync reads every computer's file and keeps, for each record, the
// latest write: the highest clock, then the highest device ID. As no two computers write the same
// file, the sync tool never has a conflict to resolve.
package foldersync

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/JoaoVitorOli/althea/internal/store"
)

// devicesDir holds a file per computer.
const devicesDir = "devices"

// fileExt ends a computer's file.
const fileExt = ".sync"

// Source is a kind of record: what a computer has of it, and how to write what came from
// another.
type Source struct {
	// Prefix starts the key of each record of the source, as "theme/".
	Prefix string
	// Snapshot returns every record the computer has, by key without the prefix.
	Snapshot func(ctx context.Context) (map[string][]byte, error)
	// Apply writes a record that came from another computer, or deletes it for nil data.
	Apply func(ctx context.Context, key string, data []byte) error
}

// Syncer syncs the Sources with the folder. Its fields are set before the first Sync.
type Syncer struct {
	// Dir is the sync folder, and Key the key Join returned for it.
	Dir string
	Key []byte
	// Device is this computer's ID, and Name what it is called, as its host name.
	Device, Name string
	// State is where the syncer remembers what it synced, kept apart from every other namespace.
	State   *store.KV
	Sources []Source
	Now     func() time.Time
	Log     *slog.Logger

	mu sync.Mutex
}

// Result is what a sync did.
type Result struct {
	// Pulled is how many records came from other computers, and Pushed how many of this one's
	// changes went to the folder.
	Pulled, Pushed int
	// Devices is how many computers share the folder, this one included.
	Devices int
}

// entry is what the syncer remembers of a record: its hash, to tell a change, and its clock.
type entry struct {
	Hash    string `json:"h,omitempty"`
	Clock   int64  `json:"c"`
	Device  string `json:"d"`
	Deleted bool   `json:"x,omitempty"`
}

// record is a record in a computer's file.
type record struct {
	Key     string `json:"k"`
	Clock   int64  `json:"c"`
	Device  string `json:"d"`
	Deleted bool   `json:"x,omitempty"`
	Data    []byte `json:"v,omitempty"`
}

// deviceFile is a computer's file, before it is sealed.
type deviceFile struct {
	Version int      `json:"version"`
	Device  string   `json:"device"`
	Name    string   `json:"name"`
	Records []record `json:"records"`
}

// newer reports whether a write of clock and device comes after one of clock2 and device2.
func newer(clock int64, device string, clock2 int64, device2 string) bool {
	return clock > clock2 || (clock == clock2 && device > device2)
}

func hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Sync sends this computer's changes to the folder and writes those of the others here. Records
// that cannot be written are logged and come again on the next sync; any other error stops it
// before anything is written.
func (s *Syncer) Sync(ctx context.Context) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var res Result

	local := map[string][]byte{}
	for _, src := range s.Sources {
		snap, err := src.Snapshot(ctx)
		if err != nil {
			return res, fmt.Errorf("read %s: %w", strings.TrimSuffix(src.Prefix, "/"), err)
		}
		for k, v := range snap {
			local[src.Prefix+k] = v
		}
	}
	state, joined, err := s.load(ctx)
	if err != nil {
		return res, err
	}
	changed := map[string]bool{}
	files, err := s.readAll()
	if err != nil {
		return res, err
	}

	// What changed here since the last sync gets a clock past every one seen: the time, unless a
	// clock already went beyond it.
	var top int64
	for _, e := range state {
		top = max(top, e.Clock)
	}
	for _, f := range files {
		for _, r := range f.Records {
			top = max(top, r.Clock)
		}
	}
	tick := func() int64 {
		top = max(s.Now().UnixMilli(), top+1)
		return top
	}
	inFolder := map[string]bool{}
	if !joined {
		for _, f := range files {
			for _, r := range f.Records {
				inFolder[r.Key] = true
			}
		}
	}
	for _, k := range sortedKeys(local) {
		h := hash(local[k])
		e, ok := state[k]
		switch {
		case !ok && inFolder[k]:
			// What a computer had before it joined is older than what the folder has of it.
			state[k] = entry{Hash: h, Clock: -1, Device: s.Device}
		case !ok, e.Deleted, e.Hash != h:
			state[k] = entry{Hash: h, Clock: tick(), Device: s.Device}
		default:
			continue
		}
		changed[k] = true
		res.Pushed++
	}
	for _, k := range sortedKeys(state) {
		if _, ok := local[k]; !ok && !state[k].Deleted && s.source(k) != nil {
			state[k] = entry{Clock: tick(), Device: s.Device, Deleted: true}
			changed[k] = true
			res.Pushed++
		}
	}

	res.Devices = len(files)
	if !slices.ContainsFunc(files, func(f deviceFile) bool { return f.Device == s.Device }) {
		res.Devices++
	}
	incoming := map[string]record{}
	for _, f := range files {
		for _, r := range f.Records {
			if s.source(r.Key) == nil {
				continue // a kind of record of a newer Althea
			}
			best, ok := incoming[r.Key]
			if !ok {
				cur := state[r.Key]
				best = record{Clock: cur.Clock, Device: cur.Device}
				if _, known := state[r.Key]; !known {
					best.Clock = -1
				}
			}
			if newer(r.Clock, r.Device, best.Clock, best.Device) {
				incoming[r.Key] = r
			}
		}
	}

	var failed []error
	for _, k := range sortedKeys(incoming) {
		r := incoming[k]
		have, present := local[k]
		if r.Deleted && present || !r.Deleted && (!present || !bytes.Equal(have, r.Data)) {
			src := s.source(k)
			data := r.Data
			if r.Deleted {
				data = nil
			} else if data == nil {
				data = []byte{}
			}
			if err := src.Apply(ctx, strings.TrimPrefix(k, src.Prefix), data); err != nil {
				s.Log.Warn("sync: record not written", "key", k, "err", err)
				failed = append(failed, err)
				continue
			}
			res.Pulled++
		}
		if changed[k] && state[k].Device == s.Device {
			res.Pushed-- // a change here that a later one elsewhere replaced
		}
		e := entry{Clock: r.Clock, Device: r.Device, Deleted: r.Deleted}
		if r.Deleted {
			delete(local, k)
		} else {
			e.Hash, local[k] = hash(r.Data), r.Data
		}
		state[k] = e
		changed[k] = true
	}

	if err := s.write(ctx, state, local); err != nil {
		return res, err
	}
	for k := range changed {
		data, err := json.Marshal(state[k])
		if err != nil {
			return res, err
		}
		if err := s.State.Set(ctx, statePrefix+k, data); err != nil {
			return res, err
		}
	}
	if !joined {
		if err := s.State.Set(ctx, joinedKey, []byte("1")); err != nil {
			return res, err
		}
	}
	if len(failed) > 0 {
		return res, fmt.Errorf("%d records not written: %w", len(failed), errors.Join(failed...))
	}
	return res, nil
}

const (
	statePrefix = "state/"
	joinedKey   = "joined"
	writtenKey  = "written"
)

// Forget forgets what was synced, as sync is turned off: turned on again, the computer joins
// anew.
func Forget(ctx context.Context, state *store.KV) error {
	entries, err := state.List(ctx, "")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := state.Delete(ctx, e.Key); err != nil {
			return err
		}
	}
	return nil
}

// load returns what the syncer remembers, and whether this computer synced before.
func (s *Syncer) load(ctx context.Context) (map[string]entry, bool, error) {
	entries, err := s.State.List(ctx, statePrefix)
	if err != nil {
		return nil, false, err
	}
	state := make(map[string]entry, len(entries))
	for _, e := range entries {
		var v entry
		if err := json.Unmarshal(e.Value, &v); err != nil {
			continue // forgotten: the record counts as new
		}
		state[strings.TrimPrefix(e.Key, statePrefix)] = v
	}
	_, joined, err := s.State.Get(ctx, joinedKey)
	return state, joined, err
}

// source returns the source of the record of key, nil for none.
func (s *Syncer) source(key string) *Source {
	for i := range s.Sources {
		if strings.HasPrefix(key, s.Sources[i].Prefix) {
			return &s.Sources[i]
		}
	}
	return nil
}

// readAll reads every computer's file. One that cannot be read, as one the sync tool is still
// writing, is logged and left for the next sync.
func (s *Syncer) readAll() ([]deviceFile, error) {
	dir := filepath.Join(s.Dir, devicesDir)
	names, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []deviceFile
	for _, n := range names {
		device, ok := strings.CutSuffix(n.Name(), fileExt)
		if !ok || n.IsDir() {
			continue
		}
		f, err := s.read(filepath.Join(dir, n.Name()), device)
		if err != nil {
			s.Log.Warn("sync: a computer's file not read", "file", n.Name(), "err", err)
			continue
		}
		files = append(files, f)
	}
	return files, nil
}

func (s *Syncer) read(path, device string) (deviceFile, error) {
	var f deviceFile
	sealed, err := os.ReadFile(path)
	if err != nil {
		return f, err
	}
	packed, err := open(s.Key, sealed, "device/"+device)
	if err != nil {
		return f, errors.New("not encrypted with this folder's key")
	}
	zr, err := gzip.NewReader(bytes.NewReader(packed))
	if err != nil {
		return f, err
	}
	data, err := io.ReadAll(io.LimitReader(zr, 256<<20))
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return f, err
	}
	if f.Version != 1 || f.Device != device {
		return f, errors.New("of another version or computer")
	}
	return f, nil
}

// write writes this computer's file with every record it has, unless it would write what is
// there already.
func (s *Syncer) write(ctx context.Context, state map[string]entry, local map[string][]byte) error {
	f := deviceFile{Version: 1, Device: s.Device, Name: s.Name, Records: []record{}}
	for _, k := range sortedKeys(state) {
		e := state[k]
		switch data, ok := local[k]; {
		case e.Deleted:
			f.Records = append(f.Records, record{Key: k, Clock: e.Clock, Device: e.Device, Deleted: true})
		case ok:
			f.Records = append(f.Records, record{Key: k, Clock: e.Clock, Device: e.Device, Data: data})
		}
	}
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	path := filepath.Join(s.Dir, devicesDir, s.Device+fileExt)
	sum := hash(data)
	if last, ok, _ := s.State.Get(ctx, writtenKey); ok && string(last) == sum {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
	}
	var packed bytes.Buffer
	zw := gzip.NewWriter(&packed)
	_, _ = zw.Write(data)
	if err := zw.Close(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := writeFile(path, seal(s.Key, packed.Bytes(), "device/"+s.Device)); err != nil {
		return err
	}
	return s.State.Set(ctx, writtenKey, []byte(sum))
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
