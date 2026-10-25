# Architecture

How Althea is put together. Read [spec.md](spec.md) first for what it does.

## The big picture

```
                    ┌──────────────────────────────────────────────┐
  hotkey / tray ──▶ │ shell     window lifecycle, hotkeys, tray    │  MyGo
                    ├──────────────────────────────────────────────┤
  keys / pointer ─▶ │ ui        root view, sdk view renderers,     │  MyGo
                    │           components, motion, theme → ui     │
                    ├──────────────────────────────────────────────┤
                    │ registry  extensions, commands, aliases,     │
                    │           root query fan-out and ranking     │  pure Go
                    │ search    fuzzy matcher, frecency            │
                    ├──────────────────────────────────────────────┤
                    │ sdk       the public extension API           │  pure Go
                    │ extensions/*  every feature, built-in or not │
                    ├──────────────────────────────────────────────┤
                    │ config · theme · store · i18n · secrets ·    │  pure Go
                    │ netpolicy · scripts                          │
                    ├──────────────────────────────────────────────┤
                    │ platform  OS services behind interfaces      │  windows / linux / fake
                    └──────────────────────────────────────────────┘
```

Only the top two layers and the platform adapters know MyGo or the OS. Everything in between is
plain Go that runs and tests anywhere, so Linux support is a matter of writing adapters.

## Repository layout

```
cmd/althea/              main: flags, then app.Run
internal/app/            composition root: builds every service and wires them together
sdk/                     public extension API (stdlib only)
sdk/custom/              escape hatch: a view drawn with MyGo's ui package
extensions/              built-in extensions, one package each
  builtin.go             the list of built-in extensions — a fork adds one line here
  apps/ system/ calculator/ files/ quicklinks/ snippets/ clipboard/
  windows/ emoji/ settings/ ai/ translator/ notes/ sync/ themes/
internal/registry/       loads extensions, indexes commands, aliases, hotkeys; runs root queries
internal/search/         fuzzy matching, scoring, frecency
internal/config/         settings.toml: schema, defaults, validation, hot-reload
internal/theme/          theme tokens, TOML loading, `extends`, hot-reload, mapping to ui.Theme
internal/store/          SQLite: open, migrations, namespaced key-value, query counter
internal/i18n/           message catalogs and lookup
internal/secrets/        OS vault access and AES-GCM encryption of data at rest
internal/netpolicy/      the only HTTP client: enforces and logs the network policy
internal/scripts/        Script Command metadata parser and runner
internal/platform/       interfaces for OS services
  windows/ linux/ fake/  adapters; fake is used by tests
internal/shell/          launcher window: show, hide, position, focus, hotkey, tray, onboarding
internal/ui/             MyGo views
  components/            search bar, list row, action panel, footer, toast, kbd hint
  motion/                durations, springs, reduced-motion handling
  render/                renders sdk.View values (List, Grid, Detail, Form)
docs/                    spec, architecture, ADRs, guides
```

## Dependency rules

Enforced by `depguard` in golangci-lint, so they cannot drift:

| Package | May import |
|---|---|
| `sdk` | stdlib |
| `sdk/custom` | `sdk`, `mygo/ui` |
| `extensions/*` | `sdk`, `sdk/custom`, stdlib, third-party libraries — **not** `internal/*` |
| `internal/search`, `config`, `theme`, `store`, `i18n`, `secrets`, `netpolicy`, `scripts`, `registry` | stdlib, third-party, each other where needed — **not** MyGo |
| `internal/platform/*` | OS APIs (`purego`, `golang.org/x/sys`), MyGo where it wraps the OS for us |
| `internal/ui`, `internal/shell` | everything above |
| `internal/app` | everything |

Extensions never reach into `internal/`. What they need from the host comes through
`sdk.Context`, so the built-ins prove that the public API is enough.

## The extension API

A sketch of `sdk`; the final shape is designed and documented in M3.

```go
// Extension is a unit of features: built-ins and third-party ones implement it alike.
type Extension interface {
	Manifest() Manifest
	Commands() []Command
}

type Manifest struct {
	ID          string // reverse-DNS-ish, e.g. "althea.calculator"; namespaces storage and settings
	Title       string
	Icon        Icon
	Preferences []Preference // typed; shown in settings and stored under [extensions.<id>]
	Messages    fs.FS        // i18n catalogs (en.toml, pt-BR.toml)
}

type Command struct {
	ID       string
	Title    string
	Subtitle string
	Icon     Icon
	Keywords []string
	Run      func(ctx Context) (View, error) // nil View for commands without a view
}

// RootProvider is implemented by extensions that answer the root search directly,
// as the calculator and file search do.
type RootProvider interface {
	Search(ctx Context, query string) ([]Item, error)
}

// Context is what the host lends an extension for one call: cancellation, its own storage,
// preferences, and the services the host exposes.
type Context interface {
	context.Context
	Store() Store           // namespaced key-value and SQL
	Prefs() Prefs
	Secrets() Secrets
	Clipboard() Clipboard
	Open(target string) error
	HTTP() *http.Client     // goes through netpolicy
	Push(View)              // navigation
	Toast(Toast)
	Log() *slog.Logger
	T(key string, args ...any) string
}
```

Views are values the host renders, so every extension gets the theme, the glass and the motion
for free:

```go
type List struct {
	Items        []Item
	Sections     []Section
	SearchBar    SearchBar                                   // placeholder, filtering
	OnSearch     func(ctx Context, q string) ([]Item, error) // dynamic lists; runs off the UI thread
	Detail       bool                                        // split list with a detail pane
	EmptyView    *Empty
}
type Item struct {
	ID, Title, Subtitle string
	Icon        Icon
	Accessories []Accessory
	Keywords    []string
	Detail      *Detail
	Actions     []Action // first is primary (Enter), second secondary (Ctrl+Enter)
}
type Grid struct{ /* ... */ }
type Detail struct{ Markdown string; Metadata []Metadata; Actions []Action }
type Form struct{ Fields []Field; OnSubmit func(ctx Context, values Values) error }
```

Registering a feature in a fork:

```go
// extensions/builtin.go
func Builtin() []sdk.Extension {
	return []sdk.Extension{
		apps.New(),
		calculator.New(),
		// ...
		weather.New(), // ← the fork's new extension
	}
}
```

## Threads and data flow

MyGo builds the view on the main thread every frame. Althea keeps one rule: **UI state changes only
on the main thread; everything else runs on goroutines and hands results back with
`Window.Update`.**

A keystroke in the root search:

1. The search field reports `Changed`; the view asks the registry for results.
2. The registry cancels the previous query's context, then
   - matches the static command index with the fuzzy matcher (microseconds, in memory);
   - fans out to every `RootProvider` on its own goroutine with a deadline.
3. The ranker merges matches with frecency boosts and sends a result set back through
   `Window.Update`. Late providers append their results with a transition instead of blocking.
4. The virtualized list renders only the rows in view.

Every call into an extension is wrapped: a panic becomes an error toast and a log entry, and a
call that overruns its deadline is cancelled, so one bad extension cannot freeze or crash Althea.

## Configuration, themes and hot-reload

`config` and `theme` load TOML into typed structs, validate them, and watch their files. A change
produces a new immutable value that is swapped in on the main thread; an invalid file keeps the
last valid value and shows a toast naming the file, line and problem.

## Storage

One SQLite database in WAL mode through `modernc.org/sqlite` (pure Go). Migrations are numbered SQL
files embedded per package. Extensions get a key-value namespace and may own tables prefixed with
their ID. In tests the store wraps the driver with a query counter so a test can assert how many
queries an operation makes — the N+1 guard.

## Platform services

`internal/platform` declares narrow interfaces, one per OS service, for example:

```go
type Hotkeys interface {
	Register(h Hotkey, fn func()) (unregister func(), err error)
}
type KeyHook interface { // WH_KEYBOARD_LL on Windows
	Start(handler func(KeyEvent) Verdict) (stop func(), err error)
}
type Vault interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte) error
	Delete(key string) error
}
```

`windows/` implements them with Win32 through `purego` and `x/sys/windows`, `linux/` will in
v1.1, and `fake/` gives tests deterministic versions.

## Testing

- **Unit**: pure packages (`search`, `config`, `theme`, calculator parser, script metadata).
- **Integration**: the store against a real SQLite file in `t.TempDir()`; registry with fake
  extensions; extensions against `fake` platform services.
- **View tests**: `ui.NewTester` runs views headless — typing, keys, clicks and what is shown.
- **End-to-end**: a few smoke tests on Windows CI driving the real window.
- **Guards** in every test binary: `goleak.VerifyTestMain`, `-race` in CI, query budgets.
