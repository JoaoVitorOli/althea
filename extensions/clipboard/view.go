package clipboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/JoaoVitorOli/althea/sdk"
)

// entry is an item of the history, opened.
type entry struct {
	id     string
	kind   string
	body   content
	size   int
	copied time.Time
	pinned bool
	thumb  []byte
}

// maxShown bounds the items a search shows; the history keeps more.
const maxShown = 300

// entries returns the items matching query, pinned first, then newest first. Contents are
// sealed, so the search opens them here rather than in the database.
func (e *Extension) entries(ctx sdk.Context, query string) ([]entry, error) {
	s, err := e.seal()
	if err != nil {
		return nil, err
	}
	rows, err := ctx.Database().Query(ctx,
		`SELECT id, kind, content, size, copied, pinned FROM clipboard_items ORDER BY pinned DESC, copied DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	words := strings.Fields(strings.ToLower(query))
	var out []entry
	for rows.Next() && len(out) < maxShown {
		var (
			en     entry
			sealed []byte
			copied int64
		)
		if err := rows.Scan(&en.id, &en.kind, &sealed, &en.size, &copied, &en.pinned); err != nil {
			return nil, err
		}
		data, err := s.open(sealed)
		if err != nil {
			ctx.Log().Warn("clipboard item not opened", "id", en.id, "err", err)
			continue
		}
		if err := json.Unmarshal(data, &en.body); err != nil {
			continue
		}
		en.copied = time.UnixMilli(copied)
		if matches(ctx, en, words) {
			out = append(out, en)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, e.thumbs(ctx, s, out)
}

// matches reports whether every word is in an entry's text, files, app or kind.
func matches(ctx sdk.Context, en entry, words []string) bool {
	if len(words) == 0 {
		return true
	}
	hay := strings.ToLower(en.body.Text + "\n" + strings.Join(en.body.Files, "\n") + "\n" +
		en.body.Source + "\n" + ctx.T("kind."+en.kind))
	for _, w := range words {
		if !strings.Contains(hay, w) {
			return false
		}
	}
	return true
}

// thumbs reads the previews of the images among entries, in one query.
func (e *Extension) thumbs(ctx sdk.Context, s *sealer, entries []entry) error {
	byID := map[string]int{}
	var args []any
	for i, en := range entries {
		if en.kind == "image" {
			byID[en.id] = i
			args = append(args, en.id)
		}
	}
	if len(args) == 0 {
		return nil
	}
	rows, err := ctx.Database().Query(ctx,
		`SELECT id, thumb FROM clipboard_items WHERE id IN (?`+strings.Repeat(",?", len(args)-1)+`)`, args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			id     string
			sealed []byte
		)
		if err := rows.Scan(&id, &sealed); err != nil {
			return err
		}
		if thumb, err := s.open(sealed); err == nil {
			entries[byID[id]].thumb = thumb
		}
	}
	return rows.Err()
}

// list is the history: a search over it, the chosen item's preview beside.
func (e *Extension) list(ctx sdk.Context) *sdk.List {
	return &sdk.List{
		Title: ctx.T("title"), Placeholder: ctx.T("search"), ShowDetail: true,
		Empty: sdk.Empty{Title: ctx.T("empty"), Description: ctx.T("empty_hint"), Icon: sdk.Icon{Glyph: "clipboard"}},
		Search: func(ctx sdk.Context, query string) ([]sdk.Item, error) {
			entries, err := e.entries(ctx, query)
			if err != nil {
				return nil, err
			}
			items := make([]sdk.Item, len(entries))
			for i, en := range entries {
				items[i] = e.item(ctx, en)
			}
			return items, nil
		},
	}
}

func (e *Extension) item(ctx sdk.Context, en entry) sdk.Item {
	it := sdk.Item{
		ID: en.id, Title: title(ctx, en), Icon: icon(en), Section: e.section(ctx, en),
		Detail: e.detail(ctx, en), Actions: e.actions(ctx, en),
	}
	return it
}

// title is the first line of a text, the name of a file, or what an image is.
func title(ctx sdk.Context, en entry) string {
	switch en.kind {
	case "image":
		return ctx.T("kind.image")
	case "files":
		name := filepath.Base(en.body.Files[0])
		if n := len(en.body.Files); n > 1 {
			return ctx.T("files_more", "name", name, "count", n-1)
		}
		return name
	}
	for _, line := range strings.Split(en.body.Text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			if r := []rune(line); len(r) > 100 {
				return string(r[:100]) + "…"
			}
			return line
		}
	}
	return ctx.T("kind.text")
}

func icon(en entry) sdk.Icon {
	switch en.kind {
	case "image":
		return sdk.Icon{Data: en.thumb, Glyph: "image"}
	case "files":
		return sdk.Icon{File: en.body.Files[0], Glyph: "document"}
	case "link":
		return sdk.Icon{Glyph: "link"}
	case "color":
		return sdk.Icon{Data: swatch(strings.TrimSpace(en.body.Text)), Glyph: "color"}
	}
	return sdk.Icon{Glyph: "document"}
}

// swatch draws a hex color as a small PNG; nil for colors written otherwise.
func swatch(text string) []byte {
	hex := strings.TrimPrefix(text, "#")
	if len(hex) == 3 || len(hex) == 4 {
		var b strings.Builder
		for _, c := range hex {
			b.WriteRune(c)
			b.WriteRune(c)
		}
		hex = b.String()
	}
	if len(hex) == 6 {
		hex += "ff"
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil || len(hex) != 8 {
		return nil
	}
	c := color.NRGBA{R: uint8(v >> 24), G: uint8(v >> 16), B: uint8(v >> 8), A: uint8(v)} //nolint:gosec // bytes of a 32-bit color
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// section groups items by when they were copied, pinned ones first.
func (e *Extension) section(ctx sdk.Context, en entry) string {
	if en.pinned {
		return ctx.T("section.pinned")
	}
	now := e.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch {
	case !en.copied.Before(today):
		return ctx.T("section.today")
	case !en.copied.Before(today.AddDate(0, 0, -1)):
		return ctx.T("section.yesterday")
	case !en.copied.Before(today.AddDate(0, 0, -7)):
		return ctx.T("section.week")
	}
	return ctx.T("section.earlier")
}

// detail previews an item, with where and when it was copied.
func (e *Extension) detail(ctx sdk.Context, en entry) *sdk.Detail {
	d := &sdk.Detail{}
	switch en.kind {
	case "image":
		d.Image = en.thumb
	case "files":
		var b strings.Builder
		for _, f := range en.body.Files {
			fmt.Fprintf(&b, "- `%s`\n", f)
		}
		d.Markdown = b.String()
	case "color":
		d.Image = swatch(strings.TrimSpace(en.body.Text))
		d.Markdown = fence(en.body.Text)
	default:
		d.Markdown = fence(preview(en.body.Text))
	}
	if en.body.Source != "" {
		d.Metadata = append(d.Metadata, sdk.Metadata{Label: ctx.T("detail.app"), Text: en.body.Source})
	}
	d.Metadata = append(d.Metadata,
		sdk.Metadata{Label: ctx.T("detail.copied"), Text: en.copied.Format("2006-01-02 15:04")},
		sdk.Metadata{Label: ctx.T("detail.size"), Text: size(ctx, en)},
	)
	return d
}

// preview is the start of a long text: enough to recognize it.
func preview(text string) string {
	if r := []rune(text); len(r) > 4000 {
		return string(r[:4000]) + "\n…"
	}
	return text
}

func size(ctx sdk.Context, en entry) string {
	switch en.kind {
	case "image":
		return fmt.Sprintf("%.1f KB", float64(en.size)/1024)
	case "files":
		return ctx.T("detail.files", "count", en.size)
	}
	return ctx.T("detail.characters", "count", en.size)
}

func fence(text string) string {
	f := "```"
	for strings.Contains(text, f) {
		f += "`"
	}
	return f + "\n" + text + "\n" + f
}

// actions: paste on Enter, copy on Ctrl+Enter, then plain text, pin and delete.
func (e *Extension) actions(ctx sdk.Context, en entry) []sdk.Action {
	acts := []sdk.Action{
		{Title: ctx.T("action.paste"), Run: func(ctx sdk.Context) error {
			c, err := e.clip(ctx, en)
			if err != nil {
				return err
			}
			return ctx.Paste(c)
		}},
		{Title: ctx.T("action.copy"), Run: func(ctx sdk.Context) error {
			c, err := e.clip(ctx, en)
			if err != nil {
				return err
			}
			ctx.Toast(ctx.T("copied"))
			return ctx.Copy(c)
		}},
	}
	if en.body.HTML != "" {
		acts = append(acts, sdk.Action{Title: ctx.T("action.plain"), Shortcut: "Ctrl+Shift+V", Run: func(ctx sdk.Context) error {
			return ctx.Paste(sdk.Clip{Text: en.body.Text})
		}})
	}
	pin := ctx.T("action.pin")
	if en.pinned {
		pin = ctx.T("action.unpin")
	}
	return append(acts,
		sdk.Action{Title: pin, Shortcut: "Ctrl+P", Run: func(ctx sdk.Context) error {
			if _, err := ctx.Database().Exec(ctx, `UPDATE clipboard_items SET pinned = ? WHERE id = ?`, !en.pinned, en.id); err != nil {
				return err
			}
			return e.reopen(ctx)
		}},
		sdk.Action{
			Title: ctx.T("action.delete"), Shortcut: "Ctrl+D", Confirm: ctx.T("action.delete_confirm"),
			Run: func(ctx sdk.Context) error {
				if _, err := ctx.Database().Exec(ctx, `DELETE FROM clipboard_items WHERE id = ?`, en.id); err != nil {
					return err
				}
				return e.reopen(ctx)
			},
		},
		sdk.Action{
			Title: ctx.T("action.delete_all"), Shortcut: "Ctrl+Shift+D", Confirm: ctx.T("action.delete_all_confirm"),
			Run: func(ctx sdk.Context) error {
				if _, err := ctx.Database().Exec(ctx, `DELETE FROM clipboard_items WHERE pinned = 0`); err != nil {
					return err
				}
				return e.reopen(ctx)
			},
		},
	)
}

// clip is what pasting an item puts on the clipboard; an image is read whole only now.
func (e *Extension) clip(ctx sdk.Context, en entry) (sdk.Clip, error) {
	switch en.kind {
	case "files":
		return sdk.Clip{Files: en.body.Files}, nil
	case "image":
		s, err := e.seal()
		if err != nil {
			return sdk.Clip{}, err
		}
		var sealed []byte
		if err := ctx.Database().QueryRow(ctx, `SELECT image FROM clipboard_items WHERE id = ?`, en.id).Scan(&sealed); err != nil {
			return sdk.Clip{}, err
		}
		img, err := s.open(sealed)
		return sdk.Clip{Image: img}, err
	}
	return sdk.Clip{Text: en.body.Text, HTML: en.body.HTML}, nil
}

func (e *Extension) reopen(ctx sdk.Context) error {
	ctx.Pop()
	ctx.Push(e.list(ctx))
	return nil
}
