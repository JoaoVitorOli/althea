# Althea

One hotkey, one search bar, and the rest of your computer is a few keystrokes away: apps,
system commands, math and currencies, clipboard history, snippets, window layouts, AI, floating
notes, encrypted sync between your computers, and offline translation.

Launchers like this usually keep their best features behind a subscription and send your usage
somewhere. Althea ships every feature free, keeps everything on your machine, and treats every
feature as an extension you can read, change or add to in a fork.

> **Status:** working towards v1. Windows 11 first; Linux follows in v1.1.

## What works today

- **The launcher.** Alt+Space (or the Win key alone) opens a panel over every app, on the
  display with your pointer, on Acrylic. It hides when you click elsewhere and remembers where
  you were for a minute and a half.
- **Search.** Apps (Win32 and Store) and commands, matched fuzzily: "vsc" finds Visual Studio
  Code, "configuracoes" finds Configurações. What you pick often and recently ranks higher, and
  Althea learns per query: pick Slack after typing "s" and "s" means Slack.
- **Calculator.** `150 + 10%`, `20% of 150`, `255 in hex`, `10 km in mi`, `100 f to c`. Decimal
  commas work, and answers come back in your language's format. Enter copies.
- **System.** Lock, sleep, restart, shut down, sign out, empty the Recycle Bin, toggle dark mode.
  Anything you cannot undo asks for a second Enter.
- **Themes.** TOML files you can edit while Althea runs. They inherit from each other, and
  `system` follows Windows between light and dark.
- **English and Portuguese**, following Windows or your settings.

The [spec](docs/spec.md) has everything v1 will do and the milestones towards it.

## Run it

Requires Go 1.27.3 or later. Every other tool is pinned in a `go.mod`.

```bash
go tool mygo dev
```

The first run walks you through the hotkey, the theme and a couple of preferences. Settings
live in `%APPDATA%\Althea\settings.toml`, commented, and apply as you save.

## Make it yours

Every feature, the built-in ones included, is a Go package that implements the public
[`sdk`](sdk/sdk.go): commands, items to search, views (lists, grids, documents, forms), its own
storage and settings. Adding one to a fork is a new package in [`extensions/`](extensions/) and
one line in [`extensions/builtin.go`](extensions/builtin.go); the
[extension guide](docs/extensions.md) walks through a complete one.

Or skip Go: any script in `%APPDATA%\Althea\scripts` with an `@althea.title` comment becomes a
command (see [Script Commands](docs/extensions.md#script-commands)).

## Read more

| Document | Answers |
|---|---|
| [Spec](docs/spec.md) | What v1 does, its budgets and milestones |
| [Architecture](docs/architecture.md) | How the pieces fit and which way dependencies point |
| [Writing extensions](docs/extensions.md) | How to add a feature, in Go or as a script |
| [Decision records](docs/adr/README.md) | Why each big choice was made |
| [Contributing](CONTRIBUTING.md) | Setup, tooling and conventions |

## Privacy

No telemetry. Althea reaches the network only for what you ask: AI and online translation with a
provider you set up, downloads of the offline translator, and currency rates when a calculation has
a currency (once a day at most, and it can be turned off). Each request goes to the log.

## License

[GPL-3.0-or-later](LICENSE): fork it freely; forks you distribute stay open.
