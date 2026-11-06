// Package sdk is Althea's public extension API. Every feature of Althea, built in or added by
// a fork, is an Extension: it offers commands, items for the root search and direct answers,
// and acts through the Context the host lends it. The package depends on the standard library
// alone, so extensions need nothing of Althea's internals. See docs/adr/0003-extensions.md.
package sdk

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
)

// Extension is a unit of features.
type Extension interface {
	Manifest() Manifest
	// Commands are what the extension does, found from the root search. Ctx translates
	// their titles.
	Commands(ctx Context) []Command
}

// Manifest describes an extension.
type Manifest struct {
	// ID is unique and stable, such as "althea.calculator": it namespaces the extension's
	// storage, settings and messages.
	ID    string
	Title string
	Icon  Icon
	// Messages are the extension's catalogs, en.toml and pt-BR.toml, for Context.T.
	Messages fs.FS
	// Migrations create and change the extension's tables in Database: SQL files named as
	// 0001_create_items.sql, each applied once, in order, before the extension is first called.
	Migrations fs.FS
}

// Command is something the user runs from the root search.
type Command struct {
	ID       string // unique within the extension
	Title    string
	Subtitle string
	Icon     Icon
	Keywords []string
	// Confirm, when set, is asked before the command runs, as Action.Confirm.
	Confirm string
	// Run does the command; it may show a view with ctx.Push.
	Run func(ctx Context) error
}

// ItemSource is an extension that contributes items to the root search, as the installed
// applications. The host asks again when the items may have changed.
type ItemSource interface {
	Items(ctx Context) ([]Item, error)
}

// Answerer is an extension that answers a query directly, above the other results, as the
// calculator answers "2+2". It must return quickly: the host cancels ctx after a few
// milliseconds and shows the results without the answer.
type Answerer interface {
	// Answer returns nil when it has no answer for the query.
	Answer(ctx Context, query string) (*Item, error)
}

// FallbackSource is an extension with something to offer when the root search finds nothing, as
// searching the web for what was typed. Its items show in the order of the extensions. It must
// return at once, doing nothing until the user picks an item.
type FallbackSource interface {
	Fallbacks(ctx Context, query string) []Item
}

// QueryTaker is an extension the user can hand what they typed in the root search to, with Tab,
// as to ask AI about it. The footer shows the action's title while there is a query. The first
// extension that takes a query gets it.
type QueryTaker interface {
	// TakeQuery returns what Tab does with query, nil to leave it. It must return at once, doing
	// nothing until the user presses Tab.
	TakeQuery(ctx Context, query string) *Action
}

// Item is a result in the root search.
type Item struct {
	ID       string // unique within the extension
	Title    string
	Subtitle string
	Icon     Icon
	Keywords []string
	// Accessory is a short text at the row's end, as "Application" or "Command".
	Accessory string
	// Section groups the items of a List or Grid under a heading, in the order sections first
	// appear. The root search has no sections.
	Section string
	// Detail is shown beside the item in a List with ShowDetail.
	Detail *Detail
	// Actions are what the user can do with the item: the first runs on Enter, the second on
	// Ctrl+Enter, all of them from the action panel (Ctrl+K).
	Actions []Action
}

// Action is something done with an item.
type Action struct {
	Title string
	// Shortcut runs the action from the item, as "Ctrl+Shift+C"; the first two actions have
	// Enter and Ctrl+Enter.
	Shortcut string
	Icon     Icon
	// Confirm, when set, is asked before the action runs, for what cannot be undone, as
	// "Press Enter again to restart": the action runs on the second press.
	Confirm string
	Run     func(ctx Context) error
}

// Icon is what a row shows beside its title.
type Icon struct {
	// File shows the icon the system has for a file, folder or application.
	File string
	// Image shows an image file (PNG, JPEG, GIF, WebP or BMP) as it is.
	Image string
	// Data shows an image held in memory, as a PNG, for one that must not be written to a file.
	Data []byte
	// Text shows a short text as the icon, as an emoji.
	Text string
	// Glyph is one of the host's built-in icons, by name: "search", "command", "calculator",
	// "power", "lock", "moon", "restart", "trash", "settings", "app", "logout", "sun",
	// "script", "document", "link", "clipboard", "image", "color", "folder", "sparkles", "translate", "swap", "copy", "check", "note", "plus", "eye", "pencil", and "window.left",
	// "window.right", "window.top", "window.bottom", "window.third_first", "window.third_center",
	// "window.third_last", "window.top_left", "window.top_right", "window.bottom_left",
	// "window.bottom_right", "window.center", "window.maximize", "window.almost", "window.display",
	// "window.layout".
	Glyph string
}

// Clip is what Context.Paste pastes: text, an image or files.
type Clip struct {
	Text string
	// HTML goes with Text, for apps that paste formatting.
	HTML string
	// Image is a PNG.
	Image []byte
	// Files are paths, pasted as copied files are, as into a folder.
	Files []string
	// CaretBack moves the caret this many characters left after the text is pasted, to where a
	// snippet's {cursor} was.
	CaretBack int
}

// Context is what the host lends an extension for one call: cancellation, a log, and the
// host's services.
type Context interface {
	context.Context
	Log() *slog.Logger
	// Open opens a file, folder, application or URL with the system.
	Open(target string) error
	// CopyText puts text on the clipboard.
	CopyText(text string) error
	// Copy puts a clip on the clipboard: text, an image or files.
	Copy(c Clip) error
	// ClipboardText is the text on the clipboard, "" when it holds none.
	ClipboardText() string
	// SelectedText is the text selected in the app the user was in before the launcher showed, ""
	// when there is none or the system cannot tell. It presses Ctrl+C in that app, leaving the
	// clipboard as it was, so call it only when the user asked for something to be done with the
	// selection, off the main thread, as in Run. The launcher hides meanwhile and shows again.
	SelectedText() (string, error)
	// Paste puts a clip on the clipboard and pastes it into the app the user was in before the
	// launcher showed, hiding the launcher.
	Paste(c Clip) error
	// Hide hides the launcher, as after an action that hands over to another app.
	Hide()
	// Refresh asks the host to collect the extensions' items again, as after the extension
	// added or removed some.
	Refresh()
	// Toast shows a short message in the launcher.
	Toast(message string)
	// T translates a key of the extension's Messages into the user's language, filling
	// {name} placeholders from args given as name, value pairs.
	T(key string, args ...any) string
	// Language is the user's language, as "en" or "pt-BR", for formatting.
	Language() string
	// Region is the user's country, as "BR", or "" when the system does not say, for defaults
	// such as a currency.
	Region() string
	// Go runs fn on a goroutine of its own with a Context that outlives this call, for what takes
	// longer than the call may, as fetching data an answer needs. Requests made with fn's Context
	// leave when this call's would, and also when this call is an Answer: the user typed something
	// that needs the data. Never send what the user typed; the host logs every request. A panic
	// in fn is logged.
	Go(fn func(ctx Context))
	// Push shows a view over the current one; Escape comes back.
	Push(v View)
	// Pop closes the view on top, back to the one before it or to the root search.
	Pop()
	// Storage is the extension's own key-value store, kept across runs.
	Storage() Storage
	// Database is the database of the extension's tables, for more than Storage holds.
	Database() Database
	// Preferences are the values the user set for the extension's Preferences.
	Preferences() Preferences
	// HTTP is the client for every request the extension makes (docs/adr/0012-privacy.md). The host
	// logs each request and lets through only those made with a Context of something the user did:
	// running a command or an action, or opening a view. Pass that Context to the request, as in
	// http.NewRequestWithContext(ctx, ...). The Context of Commands, Items, Answer, Fallbacks,
	// Preferences and Serve is refused; an Answer that needs data fetches it with Go. The client
	// has no time limit: a request ends with its Context.
	HTTP() *http.Client
}

// Storage keeps an extension's values by key in Althea's database, apart from every other
// extension's. Values are bytes: encode them as suits, as JSON.
type Storage interface {
	// Get returns the value of key, and whether it is set.
	Get(ctx context.Context, key string) (value []byte, ok bool, err error)
	// Set sets the value of key.
	Set(ctx context.Context, key string, value []byte) error
	// Delete removes key; removing a key that is not set is not an error.
	Delete(ctx context.Context, key string) error
	// List returns the entries whose keys start with prefix, in key order.
	List(ctx context.Context, prefix string) ([]Entry, error)
}

// Entry is a key of Storage and its value.
type Entry struct {
	Key   string
	Value []byte
}

// Service is an extension with work of its own in the background while Althea runs, as
// watching the clipboard or the keyboard.
type Service interface {
	// Serve runs until ctx is done. The host starts it once the extensions are set up, and
	// again, with a new ctx, each time the extension's preferences change; a Serve that
	// returns before is not run again until then. Its error is logged.
	Serve(ctx Context) error
}
