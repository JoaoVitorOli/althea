// Package files is the File Search extension: an index of the files and folders in the folders
// the user chooses (Desktop, Documents, Downloads, Pictures, Videos and Music by default),
// searched by any part of their names and kept up to date as they change.
package files

import (
	"embed"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/JoaoVitorOli/althea/sdk"
)

//go:embed locales/*.toml migrations/*.sql
var embedded embed.FS

// Platform is what the extension needs from the host.
type Platform struct {
	// Folders returns the folders searched by default.
	Folders func() []string
	// Watch calls changed with each path that changes under dir, "" when changes were lost,
	// until stop; nil reads the folders only as Althea starts.
	Watch func(dir string, changed func(path string)) (stop func(), err error)
	// Reveal shows a file in its folder, in the file manager.
	Reveal func(path string) error
}

// Extension is the File Search extension.
type Extension struct {
	p Platform
	// Scanned, when set, hears that the first scan ended, for testing.
	Scanned func()
	// Now is the clock.
	Now func() time.Time
}

// New returns the File Search extension.
func New(p Platform) *Extension { return &Extension{p: p, Now: time.Now} }

// Manifest implements sdk.Extension.
func (*Extension) Manifest() sdk.Manifest {
	messages, _ := fs.Sub(embedded, "locales")
	migrations, _ := fs.Sub(embedded, "migrations")
	return sdk.Manifest{
		ID: "althea.files", Title: "File Search", Icon: sdk.Icon{Glyph: "folder"},
		Messages: messages, Migrations: migrations,
	}
}

// defaultExclude are folder names left out of the index: they hold many files nobody looks for.
const defaultExclude = "node_modules; __pycache__; venv; $RECYCLE.BIN; AppData"

// Preferences implements sdk.Configurable.
func (*Extension) Preferences(ctx sdk.Context) []sdk.Preference {
	return []sdk.Preference{
		{Name: "folders", Title: ctx.T("prefs.folders"), Description: ctx.T("prefs.folders_info"), Kind: sdk.TextPreference},
		{Name: "exclude", Title: ctx.T("prefs.exclude"), Description: ctx.T("prefs.exclude_info"), Kind: sdk.TextPreference, Default: defaultExclude},
	}
}

// roots are the folders the index covers: the user's, or the default ones.
func (e *Extension) roots(ctx sdk.Context) []string {
	var roots []string
	for _, f := range strings.Split(ctx.Preferences().String("folders"), ";") {
		if f = strings.TrimSpace(f); f != "" {
			roots = append(roots, filepath.Clean(f))
		}
	}
	if len(roots) == 0 && e.p.Folders != nil {
		roots = e.p.Folders()
	}
	// A folder inside another is covered by it.
	slices.Sort(roots)
	return slices.CompactFunc(roots, func(a, b string) bool {
		return strings.HasPrefix(strings.ToLower(b), strings.ToLower(a)+string(filepath.Separator)) || strings.EqualFold(a, b)
	})
}

func (e *Extension) indexer(ctx sdk.Context) *indexer {
	exclude := map[string]bool{}
	for _, n := range strings.Split(ctx.Preferences().String("exclude"), ";") {
		if n = strings.TrimSpace(n); n != "" {
			exclude[strings.ToLower(n)] = true
		}
	}
	return &indexer{e: e, ctx: ctx, roots: e.roots(ctx), exclude: exclude}
}

// Serve implements sdk.Service: it reads the folders as Althea starts, and again when the
// preferences change, then follows their changes.
func (e *Extension) Serve(ctx sdk.Context) error {
	x := e.indexer(ctx)
	start := time.Now()
	// Watching first: what changes while the folders are read is heard, and comes after the scan.
	stop := x.watch()
	defer stop()
	if err := x.scan(); err != nil {
		return err
	}
	ctx.Log().Info("files indexed", "folders", len(x.roots), "took", time.Since(start))
	if e.Scanned != nil {
		e.Scanned()
	}
	return x.follow()
}

// Commands implements sdk.Extension.
func (e *Extension) Commands(ctx sdk.Context) []sdk.Command {
	return []sdk.Command{{
		ID: "search", Title: ctx.T("title"), Icon: sdk.Icon{Glyph: "folder"},
		Keywords: []string{"files", "find", "folder", "arquivos", "procurar", "pasta", "documentos"},
		Run: func(ctx sdk.Context) error {
			ctx.Push(&sdk.List{
				Title: ctx.T("title"), Placeholder: ctx.T("search"), Empty: e.empty(ctx), Search: e.search,
			})
			return nil
		},
	}}
}

// Fallbacks implements sdk.FallbackSource.
func (e *Extension) Fallbacks(ctx sdk.Context, query string) []sdk.Item {
	return []sdk.Item{{
		ID: "search", Title: ctx.T("fallback", "query", query), Icon: sdk.Icon{Glyph: "folder"},
		Actions: []sdk.Action{{Title: ctx.T("title"), Run: func(ctx sdk.Context) error {
			// The search field stays empty, with the query in its place, until the user types
			// another: what is typed replaces the query rather than adding to it.
			ctx.Push(&sdk.List{
				Title: ctx.T("title"), Placeholder: query, Empty: e.empty(ctx),
				Search: func(ctx sdk.Context, typed string) ([]sdk.Item, error) {
					if typed == "" {
						typed = query
					}
					return e.search(ctx, typed)
				},
			})
			return nil
		}}},
	}}
}

// empty is what a search that finds no file shows.
func (*Extension) empty(ctx sdk.Context) sdk.Empty {
	return sdk.Empty{Title: ctx.T("empty"), Description: ctx.T("empty_hint"), Icon: sdk.Icon{Glyph: "folder"}}
}

// shown is how many files a search shows.
const shown = 60

// search finds the files whose names hold every word of query, the best matches first; with an
// empty query, the files changed most recently.
func (e *Extension) search(ctx sdk.Context, query string) ([]sdk.Item, error) {
	found, err := Find(ctx, query)
	if err != nil {
		return nil, err
	}
	items := make([]sdk.Item, len(found))
	for i, f := range found {
		items[i] = e.item(ctx, f)
	}
	return items, nil
}

// File is a file or folder found.
type File struct {
	Path     string
	Name     string
	Folder   bool
	Modified time.Time
}

// Find returns the files matching query, the best first.
func Find(ctx sdk.Context, query string) ([]File, error) {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return find(ctx, `SELECT path, name, folder, modified FROM files_entries WHERE folder = 0 ORDER BY modified DESC LIMIT ?`, shown)
	}
	// Trigrams find words of three letters and more anywhere in a name; shorter ones, at its start.
	var long []string
	for _, w := range words {
		if utf8.RuneCountInString(w) >= 3 {
			long = append(long, `"`+strings.ReplaceAll(w, `"`, `""`)+`"`)
		}
	}
	var candidates []File
	var err error
	if len(long) > 0 {
		candidates, err = find(ctx, `SELECT e.path, e.name, e.folder, e.modified FROM files_names f
			JOIN files_entries e ON e.id = f.rowid WHERE files_names MATCH ? LIMIT 2000`, strings.Join(long, " AND "))
	} else {
		candidates, err = find(ctx, `SELECT path, name, folder, modified FROM files_entries
			WHERE name LIKE ? ESCAPE '\' ORDER BY modified DESC LIMIT 2000`, escapeLike(words[0])+"%")
	}
	if err != nil {
		return nil, err
	}
	type scored struct {
		f     File
		score int
	}
	var out []scored
	for _, f := range candidates {
		if s, ok := score(f, words); ok {
			out = append(out, scored{f, s})
		}
	}
	slices.SortStableFunc(out, func(a, b scored) int {
		if a.score != b.score {
			return b.score - a.score
		}
		return b.f.Modified.Compare(a.f.Modified)
	})
	files := make([]File, 0, min(len(out), shown))
	for _, s := range out[:min(len(out), shown)] {
		files = append(files, s.f)
	}
	return files, nil
}

func find(ctx sdk.Context, query string, args ...any) ([]File, error) {
	rows, err := ctx.Database().Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []File
	for rows.Next() {
		var (
			f        File
			modified int64
		)
		if err := rows.Scan(&f.Path, &f.Name, &f.Folder, &modified); err != nil {
			return nil, err
		}
		f.Modified = time.UnixMilli(modified)
		out = append(out, f)
	}
	return out, rows.Err()
}

// score ranks a name for the words typed, accents aside: the whole name, then its start, then the
// start of one of its words, then anywhere; shallow paths a little above deep ones.
func score(f File, words []string) (int, bool) {
	name := fold(f.Name)
	stem := strings.TrimSuffix(name, fold(filepath.Ext(f.Name)))
	total := 0
	for _, w := range words {
		w = fold(w)
		i := strings.Index(name, w)
		switch {
		case i < 0:
			return 0, false
		case name == w || stem == w:
			total += 1000
		case i == 0:
			total += 500
		case strings.ContainsAny(name[i-1:i], " -_.()[]"):
			total += 300
		default:
			total += 100
		}
	}
	return total - strings.Count(f.Path, string(filepath.Separator)), true
}

// fold lowers a name and takes the accents off its letters, as the trigram index does.
func fold(s string) string {
	return strings.Map(func(r rune) rune {
		if b, ok := plain[r]; ok {
			return b
		}
		return r
	}, strings.ToLower(s))
}

var plain = map[rune]rune{
	'á': 'a', 'à': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a', 'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e',
	'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i', 'ó': 'o', 'ò': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u', 'ç': 'c', 'ñ': 'n',
}

func (e *Extension) item(ctx sdk.Context, f File) sdk.Item {
	acts := []sdk.Action{
		{Title: ctx.T("action.open"), Run: func(ctx sdk.Context) error {
			ctx.Hide()
			return ctx.Open(f.Path)
		}},
	}
	if e.p.Reveal != nil {
		acts = append(acts, sdk.Action{Title: ctx.T("action.reveal"), Run: func(ctx sdk.Context) error {
			ctx.Hide()
			return e.p.Reveal(f.Path)
		}})
	}
	acts = append(acts,
		sdk.Action{Title: ctx.T("action.copy_path"), Shortcut: "Ctrl+Shift+C", Run: func(ctx sdk.Context) error {
			ctx.Toast(ctx.T("copied"))
			return ctx.CopyText(f.Path)
		}},
		sdk.Action{Title: ctx.T("action.copy_file"), Shortcut: "Ctrl+Shift+F", Run: func(ctx sdk.Context) error {
			ctx.Toast(ctx.T("copied"))
			return ctx.Copy(sdk.Clip{Files: []string{f.Path}})
		}},
	)
	return sdk.Item{
		ID: f.Path, Title: f.Name, Subtitle: filepath.Dir(f.Path), Icon: sdk.Icon{File: f.Path, Glyph: "document"},
		Actions: acts,
	}
}
