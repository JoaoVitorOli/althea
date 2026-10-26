// Package clipboard is the Clipboard History extension: what the user copies, kept to search
// and paste again. Text, links, colors, images and files are kept encrypted in the database,
// with the key in the system's vault. Content that its app marks as private, as passwords from a
// password manager, is never kept, nor what apps on the user's ignore list copy.
package clipboard

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // copied images arrive as PNG; JPEG for safety
	"image/png"
	"io/fs"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/image/draw"

	"github.com/JoaoVitorOli/althea/sdk"
)

//go:embed locales/*.toml migrations/*.sql
var files embed.FS

// Platform is what the extension needs from the host. With a nil function, the history is not
// kept, as on a system where Althea cannot watch the clipboard.
type Platform struct {
	// Watch calls copied, one at a time, with each new content of the clipboard that its app
	// did not mark as private, until stop.
	Watch func(copied func(Copied)) (stop func(), err error)
	// Key returns the 32-byte key the history is encrypted with, kept in the system's vault.
	Key func() ([]byte, error)
}

// Copied is a content of the clipboard: files, an image or text, the first set being what it is.
type Copied struct {
	Files []string
	Image []byte // PNG
	Text  string
	HTML  string
	// Source is the executable of the app that copied, as "chrome.exe", or "".
	Source string
}

// Extension is the Clipboard History extension.
type Extension struct {
	p Platform
	// Now is the clock.
	Now func() time.Time

	mu     sync.Mutex
	sealer *sealer
}

// New returns the Clipboard History extension.
func New(p Platform) *Extension { return &Extension{p: p, Now: time.Now} }

// Manifest implements sdk.Extension.
func (*Extension) Manifest() sdk.Manifest {
	messages, _ := fs.Sub(files, "locales")
	migrations, _ := fs.Sub(files, "migrations")
	return sdk.Manifest{
		ID: "althea.clipboard", Title: "Clipboard History", Icon: sdk.Icon{Glyph: "clipboard"},
		Messages: messages, Migrations: migrations,
	}
}

// The values of the retention preference.
var retentions = map[string]time.Duration{
	"day": 24 * time.Hour, "week": 7 * 24 * time.Hour, "month": 30 * 24 * time.Hour,
	"quarter": 90 * 24 * time.Hour, "year": 365 * 24 * time.Hour, "forever": 0,
}

// Preferences implements sdk.Configurable.
func (*Extension) Preferences(ctx sdk.Context) []sdk.Preference {
	var keep []sdk.Option
	for _, v := range []string{"day", "week", "month", "quarter", "year", "forever"} {
		keep = append(keep, sdk.Option{Value: v, Title: ctx.T("prefs.keep_" + v)})
	}
	return []sdk.Preference{
		{Name: "keep", Title: ctx.T("prefs.keep"), Kind: sdk.ChoicePreference, Default: "forever", Options: keep},
		{Name: "ignore", Title: ctx.T("prefs.ignore"), Description: ctx.T("prefs.ignore_info"), Kind: sdk.TextPreference},
		{Name: "images", Title: ctx.T("prefs.images"), Kind: sdk.BoolPreference, Default: "true"},
	}
}

// Commands implements sdk.Extension.
func (e *Extension) Commands(ctx sdk.Context) []sdk.Command {
	if e.p.Key == nil {
		return nil
	}
	return []sdk.Command{{
		ID: "history", Title: ctx.T("title"), Icon: sdk.Icon{Glyph: "clipboard"},
		Keywords: []string{"clipboard", "history", "paste", "área de transferência", "histórico", "colar", "cl"},
		Run: func(ctx sdk.Context) error {
			ctx.Push(e.list(ctx))
			return nil
		},
	}}
}

// seal returns the sealer, made from the vault's key the first time.
func (e *Extension) seal() (*sealer, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sealer != nil {
		return e.sealer, nil
	}
	if e.p.Key == nil {
		return nil, errors.New("this system cannot keep a clipboard history yet")
	}
	key, err := e.p.Key()
	if err != nil {
		return nil, fmt.Errorf("clipboard key: %w", err)
	}
	s, err := newSealer(key)
	if err != nil {
		return nil, err
	}
	e.sealer = s
	return s, nil
}

// Serve implements sdk.Service: it keeps what is copied while Althea runs.
func (e *Extension) Serve(ctx sdk.Context) error {
	if e.p.Watch == nil || e.p.Key == nil {
		return nil
	}
	if _, err := e.seal(); err != nil {
		return err
	}
	prefs := ctx.Preferences()
	ignore := map[string]bool{}
	for _, app := range strings.Split(prefs.String("ignore"), ",") {
		if app = strings.ToLower(strings.TrimSpace(app)); app != "" {
			ignore[app] = true
			ignore[app+".exe"] = true
		}
	}
	keepImages := prefs.Bool("images")
	keep := retentions[prefs.String("keep")]
	stop, err := e.p.Watch(func(c Copied) {
		if ignore[strings.ToLower(c.Source)] || (c.Image != nil && c.Files == nil && !keepImages) {
			return
		}
		if err := e.Record(ctx, c); err != nil {
			ctx.Log().Warn("clipboard not kept", "err", err)
		}
		if keep > 0 {
			if err := e.prune(ctx, keep); err != nil {
				ctx.Log().Warn("clipboard not pruned", "err", err)
			}
		}
	})
	if err != nil {
		return err
	}
	defer stop()
	<-ctx.Done()
	return nil
}

// Limits on what is kept: larger content is left out.
const (
	maxText  = 1 << 20  // 1 MB of text
	maxImage = 32 << 20 // 32 MB of PNG
	thumbPx  = 480
)

// content is what an item keeps sealed, apart from its image.
type content struct {
	Text   string   `json:"t,omitempty"`
	HTML   string   `json:"h,omitempty"`
	Files  []string `json:"f,omitempty"`
	Source string   `json:"s,omitempty"`
}

var (
	colorText = regexp.MustCompile(`^(?i)(#([0-9a-f]{3}|[0-9a-f]{4}|[0-9a-f]{6}|[0-9a-f]{8})|(rgb|hsl)a?\([\d\s.,%/]+\))$`)
	linkText  = regexp.MustCompile(`^(?i)(https?|ftp|mailto|file):\S+$`)
)

// kindOf tells what a text is: a link, a color or text.
func kindOf(text string) string {
	t := strings.TrimSpace(text)
	switch {
	case linkText.MatchString(t):
		return "link"
	case colorText.MatchString(t):
		return "color"
	}
	return "text"
}

// Record keeps a copied content, newest first. Copying again what the history has moves it
// to the top.
func (e *Extension) Record(ctx sdk.Context, c Copied) error {
	s, err := e.seal()
	if err != nil {
		return err
	}
	var (
		kind        string
		size        int
		digest      []byte
		full, thumb []byte
	)
	body := content{Source: c.Source}
	switch {
	case len(c.Files) > 0:
		kind, size, body.Files = "files", len(c.Files), c.Files
		digest = s.digest(kind, []byte(strings.Join(c.Files, "\x00")))
	case c.Image != nil:
		if len(c.Image) > maxImage {
			return nil
		}
		kind, size = "image", len(c.Image)
		digest = s.digest(kind, c.Image)
		if thumb, err = thumbnail(c.Image); err != nil {
			return fmt.Errorf("image: %w", err)
		}
		full, thumb = s.seal(c.Image), s.seal(thumb)
	case strings.TrimSpace(c.Text) != "":
		if len(c.Text) > maxText {
			return nil
		}
		kind, size, body.Text, body.HTML = kindOf(c.Text), utf8.RuneCountInString(c.Text), c.Text, c.HTML
		digest = s.digest("text", []byte(c.Text))
	default:
		return nil
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	now := e.Now().UnixMilli()
	_, err = ctx.Database().Exec(ctx, `
		INSERT INTO clipboard_items (id, kind, digest, content, thumb, image, size, copied)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (digest) DO UPDATE SET copied = excluded.copied, content = excluded.content`,
		sdk.NewID(), kind, digest, s.seal(data), thumb, full, size, now)
	return err
}

// thumbnail scales an image down to a preview.
func thumbnail(data []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	if b.Dx() > thumbPx || b.Dy() > thumbPx {
		scale := float64(thumbPx) / float64(max(b.Dx(), b.Dy()))
		dst := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(b.Dx())*scale)), max(1, int(float64(b.Dy())*scale))))
		draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
		img = dst
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// prune forgets what was copied longer ago than keep, but for pinned items.
func (e *Extension) prune(ctx sdk.Context, keep time.Duration) error {
	cutoff := e.Now().Add(-keep).UnixMilli()
	_, err := ctx.Database().Exec(ctx, `DELETE FROM clipboard_items WHERE pinned = 0 AND copied < ?`, cutoff)
	return err
}
