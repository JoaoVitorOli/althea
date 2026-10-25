// Package sdk is Althea's public extension API. Every feature of Althea, built in or added by
// a fork, is an Extension: it offers commands, items for the root search and direct answers,
// and acts through the Context the host lends it. The package depends on the standard library
// alone, so extensions need nothing of Althea's internals. See docs/adr/0003-extensions.md.
//
// This is the M2 subset; M3 adds views (List, Grid, Detail, Form), preferences and storage.
package sdk

import (
	"context"
	"log/slog"
)

// Extension is a unit of features.
type Extension interface {
	Manifest() Manifest
	// Commands are what the extension does, found from the root search.
	Commands() []Command
}

// Manifest describes an extension.
type Manifest struct {
	// ID is unique and stable, such as "althea.calculator": it namespaces the extension's
	// storage, settings and messages.
	ID    string
	Title string
	Icon  Icon
}

// Command is something the user runs from the root search.
type Command struct {
	ID       string // unique within the extension
	Title    string
	Subtitle string
	Icon     Icon
	Keywords []string
	// Run does the command. Commands that show a view arrive in M3.
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
	Run      func(ctx Context) error
}

// Icon is what a row shows beside its title.
type Icon struct {
	// File shows the icon the system has for a file, folder or application.
	File string
	// Glyph is one of the host's built-in icons, by name: "search", "command", "calculator",
	// "power", "lock", "moon", "restart", "trash", "settings", "app".
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
}
