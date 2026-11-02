package translator

import (
	"context"
	"crypto/sha256"
	_ "embed" // the pinned models and engine
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/JoaoVitorOli/althea/sdk"
)

// File is something to download: where from, how large, and what it must hash to.
type File struct {
	Role   string `json:"role"` // a model's: "model", "lex" or "vocab"
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Pair is the model that translates one way between two languages.
type Pair struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Files []File `json:"files"`
}

// Key names the pair, as "en-pt".
func (p Pair) Key() string { return p.From + "-" + p.To }

func (p Pair) size() int64 {
	var total int64
	for _, f := range p.Files {
		total += f.Size
	}
	return total
}

//go:embed models.json
var modelsJSON []byte

//go:embed engine.json
var engineJSON []byte

// pinnedModels are the models tools/bergamotmodels pinned.
func pinnedModels() []Pair {
	var pairs []Pair
	_ = json.Unmarshal(modelsJSON, &pairs) // checked by a test
	return pairs
}

// pinnedEngine is the engine CI built for this system, by .github/workflows/bergamot.yml; no URL
// where there is none yet.
func pinnedEngine() File {
	var byPlatform map[string]File
	_ = json.Unmarshal(engineJSON, &byPlatform) // checked by a test
	return byPlatform[runtime.GOOS+"/"+runtime.GOARCH]
}

// fileNames are the names a model's files take, by role.
var fileNames = map[string]string{"model": "model.bin", "lex": "lex.bin", "vocab": "vocab.spm"}

// modelConfig tells the engine how to run a model. Its paths are relative to the models folder, the
// engine's working directory: it cannot open paths with accents (tools/bergamot/main.cpp).
const modelConfig = `models:
  - %[1]s/model.bin
vocabs:
  - %[1]s/vocab.spm
  - %[1]s/vocab.spm
shortlist:
  - %[1]s/lex.bin
  - false
beam-size: 1
normalize: 1.0
word-penalty: 0
max-length-break: 128
mini-batch-words: 1024
workspace: 128
max-length-factor: 2.0
skip-cost: true
cpu-threads: 0
quiet: true
quiet-translation: true
gemm-precision: int8shiftAlphaAll
alignment: soft
`

func (e *Extension) modelsDir() string { return filepath.Join(e.p.Dir, "models") }

// enginePath is where the engine goes, by its hash, so that a new one never mixes with the old.
func (e *Extension) enginePath() string {
	return filepath.Join(e.p.Dir, "engine", e.engineFile.SHA256, "bergamot"+exeSuffix())
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// complete reports whether a file is in place, by its size: its hash was checked as it came.
func complete(path string, size int64) bool {
	info, err := os.Stat(path)
	return err == nil && info.Size() == size
}

func (e *Extension) engineInstalled() bool {
	return complete(e.enginePath(), e.engineFile.Size)
}

func (e *Extension) pairInstalled(p Pair) bool {
	dir := filepath.Join(e.modelsDir(), p.Key())
	for _, f := range p.Files {
		if !complete(filepath.Join(dir, fileNames[f.Role]), f.Size) {
			return false
		}
	}
	_, err := os.Stat(filepath.Join(dir, "config.yml"))
	return err == nil
}

// route returns the models that translate from one language to another: the pair itself, or two
// through English.
func (e *Extension) route(from, to string) ([]Pair, bool) {
	find := func(from, to string) (Pair, bool) {
		i := slices.IndexFunc(e.models, func(p Pair) bool { return p.From == from && p.To == to })
		if i < 0 {
			return Pair{}, false
		}
		return e.models[i], true
	}
	if p, ok := find(from, to); ok {
		return []Pair{p}, true
	}
	first, ok1 := find(from, pivot)
	second, ok2 := find(pivot, to)
	if ok1 && ok2 {
		return []Pair{first, second}, true
	}
	return nil, false
}

// download is what offline translation still lacks: nothing when ready.
type download struct {
	engine bool
	pairs  []Pair
}

func (d download) empty() bool { return !d.engine && len(d.pairs) == 0 }

func (e *Extension) lacking(pairs []Pair) download {
	d := download{engine: !e.engineInstalled()}
	for _, p := range pairs {
		if !e.pairInstalled(p) {
			d.pairs = append(d.pairs, p)
		}
	}
	return d
}

func (e *Extension) downloadSize(d download) int64 {
	var total int64
	if d.engine {
		total = e.engineFile.Size
	}
	for _, p := range d.pairs {
		total += p.size()
	}
	return total
}

// megabytes spells a size as the user reads it.
func megabytes(size int64) string { return fmt.Sprintf("%.0f MB", float64(size)/1e6) }

// install downloads what d says, telling how far it is. A file that does not hash to what is pinned
// is thrown away, so that only what was verified is ever there to run.
func (e *Extension) install(ctx sdk.Context, d download, progress func(percent int)) error {
	total := e.downloadSize(d)
	var done int64
	last := -1
	moved := func(n int64) {
		done += n
		if p := int(done * 100 / max(total, 1)); p != last {
			last = p
			progress(p)
		}
	}
	if d.engine {
		path := e.enginePath()
		if err := fetch(ctx, e.engineFile, path, moved); err != nil {
			return err
		}
	}
	for _, p := range d.pairs {
		dir := filepath.Join(e.modelsDir(), p.Key())
		for _, f := range p.Files {
			path := filepath.Join(dir, fileNames[f.Role])
			if complete(path, f.Size) {
				moved(f.Size)
				continue
			}
			if err := fetch(ctx, f, path, moved); err != nil {
				return err
			}
		}
		config := fmt.Sprintf(modelConfig, p.Key())
		if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(config), 0o600); err != nil {
			return fmt.Errorf("%s: %w", ctx.T("download.failed"), err)
		}
	}
	return nil
}

// fetch downloads f to path through a temporary file, checking its size and hash.
func fetch(ctx sdk.Context, f File, path string, moved func(int64)) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, http.NoBody)
	if err != nil {
		return err
	}
	res, err := ctx.HTTP().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		ctx.Log().Warn("download failed", "url", f.URL, "err", err)
		return errors.New(ctx.T("download.unreachable", "host", req.URL.Host))
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return errors.New(ctx.T("download.status", "host", req.URL.Host, "status", res.StatusCode))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("%s: %w", ctx.T("download.failed"), err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "download-*")
	if err != nil {
		return fmt.Errorf("%s: %w", ctx.T("download.failed"), err)
	}
	defer func() {
		_ = tmp.Close()
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()
	hash := sha256.New()
	body := io.TeeReader(io.LimitReader(res.Body, f.Size+1), hash)
	written, err := io.Copy(tmp, progressReader{body, moved})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		ctx.Log().Warn("download failed", "url", f.URL, "err", err)
		return errors.New(ctx.T("download.unreachable", "host", req.URL.Host))
	}
	if sum := hex.EncodeToString(hash.Sum(nil)); written != f.Size || !strings.EqualFold(sum, f.SHA256) {
		ctx.Log().Warn("download did not verify", "url", f.URL, "size", written, "sha256", sum)
		return errors.New(ctx.T("download.corrupt"))
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("%s: %w", ctx.T("download.failed"), err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("%s: %w", ctx.T("download.failed"), err)
	}
	return nil
}

// progressReader tells of what passes through it.
type progressReader struct {
	r     io.Reader
	moved func(int64)
}

func (p progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.moved(int64(n))
	}
	return n, err
}

// removePair deletes a model.
func (e *Extension) removePair(p Pair) error {
	return os.RemoveAll(filepath.Join(e.modelsDir(), p.Key()))
}

// engineCommand checks the engine again, as what is about to run, and makes its command.
func (e *Extension) engineCommand() (*exec.Cmd, error) {
	path := e.enginePath()
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open the translation engine: %w", err)
	}
	defer func() { _ = f.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return nil, fmt.Errorf("read the translation engine: %w", err)
	}
	if sum := hex.EncodeToString(hash.Sum(nil)); !strings.EqualFold(sum, e.engineFile.SHA256) {
		return nil, errors.New("the translation engine was changed since it was downloaded: remove it and download it again")
	}
	cmd := e.p.Command(context.Background(), path)
	cmd.Dir = e.modelsDir()
	return cmd, nil
}
