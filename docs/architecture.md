# Architecture

Althea is a thin native shell around plain Go. The shell knows the window and the operating
system; everything that decides what to show (search, ranking, settings, themes, storage) is
ordinary Go that runs and tests anywhere. That split is the main thing to preserve: it is what
makes Linux a matter of writing adapters, and what lets most of the code be tested without a
window. [spec.md](spec.md) says what Althea does; this says how.

```
  hotkey, tray ──▶ shell ──────────────▶ ui ◀── keys, pointer
                     │                   │
                     │      app (composition root: wires the rest)
                     │                   │
                     ▼                   ▼
                 platform ◀─────── registry ──▶ search, frecency
                (windows,                │
                 linux)                  ▼
                                    extensions/* ── via sdk only
                                         │
                 config · theme · store · i18n · paths · logging
```

Only `shell`, `ui`, `app` and the `platform` adapters import MyGo or the OS. `depguard` enforces
it (see [Dependency rules](#dependency-rules)), so the boundary cannot drift quietly.

## Where things live

| Package | Owns |
|---|---|
| `cmd/althea` | `main`: nothing but `app.Run` |
| `internal/app` | The composition root, the only package that knows all the others. Builds the services, wires the search to the window, applies reloaded settings |
| `internal/shell` | The launcher panel: where it opens, when it hides, when it starts over; the tray |
| `internal/ui` | Every view: the search, results, action panel, onboarding, and the pages that draw extensions' views; theme tokens mapped to MyGo; motion tokens |
| `internal/registry` | What the extensions offer, gathered and queried, with each extension's failures contained |
| `internal/search` | The fuzzy match and the ranking |
| `internal/frecency` | How often and how recently each item is picked |
| `internal/config`, `internal/theme` | The TOML files the user edits: validation with line numbers, hot-reload, in-place edits that keep comments |
| `internal/prefs` | Extensions' preferences: the values in `settings.toml` checked against what each declares |
| `internal/store` | One SQLite database: per-extension key-value, migrations, a query counter for tests |
| `internal/i18n`, `paths`, `logging`, `fswatch`, `tomlpos`, `hotkey` | Small single-purpose pieces the above share |
| `internal/soak` | Measures a release build against the budgets, and runs it for hours to find leaks (Windows) |
| `internal/foldersync` | Sync through a folder: the passphrase key, each computer's sealed file, the merge |
| `internal/netpolicy` | The only way to the network: each request allowed only for something the user did, and logged |
| `internal/platform` | Interfaces for OS services and their [adapters](../internal/platform/README.md) |
| `sdk`, `sdk/sdktest` | The public extension API, and a recording `Context` for testing extensions |
| `extensions/*` | Every feature, Script Commands included. [How to add one](extensions.md) |

Floating Notes is `extensions/notes` (the notes, sealed with a key in the vault) and its window,
which the host owns: `internal/shell` makes it, `internal/ui` draws it and `internal/app` keeps
what is typed, written once typing pauses.

The theme editor is a window of the host as well (`internal/ui/themeeditor.go`): it edits a theme's
tokens beside a launcher drawn with them, and `theme.Write` saves the file with `extends` and only
what differs, never letting a theme extend itself; the themes watcher then applies it.

Sync is `internal/foldersync`, run by `internal/app/sync.go`. Each computer writes only its own
file in the sync folder's `devices` directory: every record it has (settings.toml, each theme and
AI Command file, each snippet and quicklink, each note), with a clock and the computer that wrote
it, gzipped and sealed with AES-256-GCM under a key derived from the passphrase with Argon2id.
The folder's `althea-sync.json` holds the salt, the parameters and a sealed check that tells a
wrong passphrase. A sync reads every computer's file and keeps, per record, the highest clock
(the time of the write, or one past the highest clock seen), then the highest device ID; deletes
travel as tombstones. As no two computers write the same file, the sync tool never has a conflict.
A computer that joins yields what it had to what the folder holds. The key is kept in the vault,
never the passphrase. Sync runs a few seconds after something may have changed (the launcher
hides, a note is written, the settings or a theme reload, another computer's file arrives) and
never on a timer while nothing happens.

## Dependency rules

| Package | May import |
|---|---|
| `sdk` | the standard library alone |
| `extensions/*` | `sdk`, the standard library, third-party libraries. **Not** `internal/` |
| the core (`search`, `registry`, `config`, `theme`, `store`, …) | each other and third-party libraries. **Not** MyGo |
| `platform/windows`, `platform/linux` | OS APIs, and MyGo where it already wraps the OS |
| `shell`, `ui`, `app` | anything |

Extensions reach the host only through `sdk.Context`, and the built-ins follow the same rule.
That is deliberate: if Calculator or Applications needed something the SDK lacks, a fork's
extension would too, so the built-ins keep the public API honest.

## The extension API

An extension implements `sdk.Extension` and, optionally, more:

```go
type Extension interface {
	Manifest() Manifest           // ID, title, icon, message catalogs
	Commands(ctx Context) []Command
}

type ItemSource interface { // contributes items to the root search, as Applications does
	Items(ctx Context) ([]Item, error)
}

type Answerer interface { // answers a query directly, above the results, as Calculator does
	Answer(ctx Context, query string) (*Item, error)
}

type Configurable interface { // declares preferences, edited in Extension Preferences
	Preferences(ctx Context) []Preference
}

type QueryTaker interface { // takes what was typed in the root search with Tab, as Quick AI does
	TakeQuery(ctx Context, query string) *Action
}
```

`Context` lends what the host offers for one call: cancellation, a logger, opening files and
URLs, the clipboard and the selection, the network, hiding the launcher, toasts, translation and the user's language, the
extension's storage and preferences, and `Push`/`Pop` for views. Each `Item` carries its
`Actions`; the first runs on Enter, the second on Ctrl+Enter, all of them from Ctrl+K, and any
of them may ask for confirmation first. [extensions.md](extensions.md) is the guide.

### Views

Extensions push view values (`List`, `Grid`, `Detail`, `Form`, `Chat`, `Converter`); `ui.Root` keeps them in a stack
of pages over the root search and draws the one on top. Extension code that a page calls
(loading items, searching, submitting a form, running an action) goes through `ui.Host`, which
`app` implements: it runs on a goroutine with a cancellable context, its panics are recovered,
its errors become toasts, and its results come back with `Window.Update`. A page that closes
cancels its work in flight, and a search answered after a newer one began is dropped. A `Detail`
with `Stream` and a `Chat` show text as it comes, and Escape stops it first, keeping what came.

### The network

Everything goes through `ctx.HTTP()`, a client from `internal/netpolicy`. A request leaves only if
its context descends from `netpolicy.Allow`, which `app` adds to what the user sets off: running a
command or an action, Tab, opening a view. What runs by itself (`Commands`, `Items`, `Answer`,
`Fallbacks`, `Preferences`, `Serve`) has no such context, so a request from there is refused and
logged, whatever the extension meant. Each request is logged with its method, host and path, never
its query, headers or body. Lint keeps `http.Get` and the default client out of the code.

### Preferences and secrets

`settings.toml` keeps each extension's values in an `[extensions."<id>"]` table, which
`config` loads unchecked; `prefs.Resolve` checks it against the extension's declaration, and a
wrong value falls back to the default with a logged problem, as the host's settings do.
Secrets never reach the file: `platform.Vault` keeps them in Windows Credential Manager under
`Althea/<extension>/<name>`. The Extension Preferences screen is an extension too, living in
`app` because it writes the file and the vault.

## A keystroke, end to end

```
  ui: field Changed ──▶ app: cancel the previous query, start this one on a goroutine
                              │
                              ├─▶ every Answerer at once, each with a 50 ms deadline
                              └─▶ the index: fuzzy match + frecency, ~1 ms for 10k items
                              │
  ui: SetResults ◀── Window.Update ◀── answers first, then ranked items
```

The rule that keeps this safe: **UI state changes only on the main thread.** Work happens on
goroutines and comes back through `Window.Update`. Results for a query the user has since
changed are dropped by `Root.SetResults`. An answer that misses its deadline is dropped too,
rather than shuffling the list after the user has started reading it.

Every call into an extension goes through `registry.guard`: a panic becomes a log entry, an
error costs that extension its results, and the search goes on without it.

## Files the user edits

`settings.toml` and themes are read into typed values and validated. A mistake becomes a
problem with its line number and leaves that one setting or token at its default or inherited
value; a file that does not parse at all keeps the last good version. Problems go to the log
today; surfacing them in the launcher is planned.

Saving either file applies it within a tenth of a second. `fswatch` watches the directory rather
than the file, because most editors save by writing a new file and renaming it over the old one.

When Althea writes settings itself, as the onboarding does, `config.Set` changes the value in
place, so the user's comments and ordering survive.

Any command can have a hotkey, kept in `[hotkey.commands]` by the command's key,
`"<extension>/<command>"`, and written by the **Hotkeys and Aliases** command in Althea, which keeps
aliases the same way in `[aliases]`. Pressing a hotkey
runs the command as it is at that moment with the launcher left hidden; the launcher shows only
for a view the command pushes, a confirmation it asks or a preference it misses. A combination
goes to one taker, the launcher's own hotkey first, and the rest are logged; so does an alias.

## Storage

One SQLite database in WAL mode, through `modernc.org/sqlite`, which needs no C compiler and so
keeps cross-compilation working. Each package or extension owns a key-value namespace and may
create tables through numbered migrations, each applied once, in a transaction.

`store.DB.Queries()` counts statements. Tests read it around an operation to assert, say, that
loading frecency costs one query whatever the number of items. That is the N+1 guard.

## Testing

- Pure packages are tested directly, with real files in `t.TempDir()` and a real SQLite file.
- Views run headless with `ui.NewTester`: tests type, press keys and click, then read what shows.
  `go test -tags snapshots` renders views to PNG for looking at them without a window.
- Extensions are tested with fake platforms and `sdktest.Context`.
- Every test binary checks for leaked goroutines. CI runs everything with the race detector on
  Windows and Linux.
- Timing tests hold the keystroke budget for 10 000 items.
