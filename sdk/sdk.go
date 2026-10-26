// Package sdk is Althea's public extension API. Every feature of Althea, built in or added by
// a fork, is an Extension: it offers commands, items for the root search and direct answers,
// and acts through the Context the host lends it. The package depends on the standard library
// alone, so extensions need nothing of Althea's internals. See docs/adr/0003-extensions.md.
package sdk

import (
	"context"
	"io/fs"
	"log/slog"
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
	// Text shows a short text as the icon, as an emoji.
	Text string
	// Glyph is one of the host's built-in icons, by name: "search", "command", "calculator",
	// "power", "lock", "moon", "restart", "trash", "settings", "app", "logout", "sun",
	// "script", "document".
	Glyph string
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
	// Hide hides the launcher, as after an action that hands over to another app.
	Hide()
	// Toast shows a short message in the launcher.
	Toast(message string)
	// T translates a key of the extension's Messages into the user's language, filling
	// {name} placeholders from args given as name, value pairs.
	T(key string, args ...any) string
	// Language is the user's language, as "en" or "pt-BR", for formatting.
	Language() string
	// Push shows a view over the current one; Escape comes back.
	Push(v View)
	// Pop closes the view on top, back to the one before it or to the root search.
	Pop()
	// Storage is the extension's own key-value store, kept across runs.
	Storage() Storage
	// Preferences are the values the user set for the extension's Preferences.
	Preferences() Preferences
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
