package ui

import (
	"sync"

	"github.com/egoist/mygo/ui"

	"github.com/JoaoVitorOli/althea/sdk"
)

// svg wraps the paths of a 24×24 stroked icon.
func svg(paths string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" ` +
		`stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">` + paths + `</svg>`))
}

// glyphs are the host's built-in icons, by the names sdk.Icon.Glyph uses.
var glyphs = map[string]*ui.SVG{
	"search":     searchIcon,
	"command":    svg(`<path d="M9 6a3 3 0 1 0-3 3h12a3 3 0 1 0-3-3v12a3 3 0 1 0 3-3H6a3 3 0 1 0 3 3z"/>`),
	"calculator": svg(`<rect x="5" y="3" width="14" height="18" rx="2.5"/><path d="M8.5 7h7M8.5 12h.01M12 12h.01M15.5 12h.01M8.5 16h.01M12 16h.01M15.5 16h.01"/>`),
	"power":      svg(`<path d="M12 3v9"/><path d="M6.3 7.3a8 8 0 1 0 11.4 0"/>`),
	"lock":       svg(`<rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>`),
	"moon":       svg(`<path d="M20 14.5A8 8 0 1 1 9.5 4 6.5 6.5 0 0 0 20 14.5z"/>`),
	"restart":    svg(`<path d="M4 12a8 8 0 1 0 2.3-5.6L4 8.7"/><path d="M4 4v4.7h4.7"/>`),
	"trash":      svg(`<path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3"/>`),
	"settings":   svg(`<circle cx="12" cy="12" r="3"/><path d="M12 2.5v3M12 18.5v3M4.2 6l2.6 1.5M17.2 16.5l2.6 1.5M4.2 18l2.6-1.5M17.2 7.5 19.8 6"/>`),
	"app":        svg(`<rect x="4" y="4" width="16" height="16" rx="4"/><path d="M9 12h6"/>`),
	"logout":     svg(`<path d="M15 4h3a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-3"/><path d="M10 16l-4-4 4-4M6 12h10"/>`),
	"sun":        svg(`<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>`),
}

// Icons loads the icons the system has for files and applications, off the main thread, and
// keeps them. The view asks for an icon every frame; until it is loaded, the row shows the
// glyph, and the window draws again once it is.
type Icons struct {
	// Load returns a PNG of a file's icon, px pixels wide, as mygo.Shell.FileIcon does.
	Load func(path string, px int) ([]byte, error)
	// Loaded asks the window for a frame, as mygo.Window.Invalidate does.
	Loaded func()

	mu      sync.Mutex
	bitmaps map[string]*ui.Bitmap // nil where the file has none
	pending map[string]bool
}

// File returns the icon of path, or nil while it loads or where there is none.
func (ic *Icons) File(path string, px int) *ui.Bitmap {
	if ic == nil || ic.Load == nil || path == "" {
		return nil
	}
	ic.mu.Lock()
	defer ic.mu.Unlock()
	if b, ok := ic.bitmaps[path]; ok {
		return b
	}
	if ic.pending[path] {
		return nil
	}
	if ic.pending == nil {
		ic.pending, ic.bitmaps = map[string]bool{}, map[string]*ui.Bitmap{}
	}
	ic.pending[path] = true
	go func() {
		var bitmap *ui.Bitmap
		if data, err := ic.Load(path, px); err == nil {
			bitmap, _ = ui.DecodeBitmap(data)
		}
		ic.mu.Lock()
		ic.bitmaps[path] = bitmap
		delete(ic.pending, path)
		ic.mu.Unlock()
		if ic.Loaded != nil {
			ic.Loaded()
		}
	}()
	return nil
}

// icon draws an sdk.Icon at size DIPs: the file's own icon once loaded, else its glyph.
func (r *Root) icon(c *ui.Context, icon sdk.Icon, size float32, color ui.Color) {
	if b := r.Icons.File(icon.File, int(size*2)); b != nil {
		ui.Image(c, b).Size(size, size)
		return
	}
	g, ok := glyphs[icon.Glyph]
	if !ok {
		g = glyphs["app"]
	}
	ui.Row(c).Size(size, size).Center().Children(func() {
		ui.Icon(c, g).FontSize(size * 0.8).TextColor(color)
	})
}
