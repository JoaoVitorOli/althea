# Althea

A keyboard-first launcher for the desktop, in the spirit of other launchers — with every Pro feature free,
private by default, and easy to extend.

> **Status:** in development towards v1. Windows 11 first, Linux in v1.1.

## Highlights (v1)

- Launch apps, files and commands from one search bar, ranked by how you use them.
- Calculator, clipboard history, snippets, quicklinks, emoji, window management and layouts.
- AI with your own keys or local models, offline translation, floating notes, sync through your
  own folder — no subscription.
- Themes you can write or edit visually, with glass materials and considered motion.
- Every feature is an extension: fork it and add yours with one package and one line.
- No telemetry. Nothing leaves your machine unless you ask.

## Documentation

- [Product spec](docs/spec.md) — what v1 does
- [Architecture](docs/architecture.md) — how it is built
- [Decision records](docs/adr/README.md) — why
- [Contributing](CONTRIBUTING.md) — setup, tooling and conventions

## Building

Requires Go 1.27+.

```bash
go tool mygo dev      # run with live reload
go test ./...         # run the tests
go tool mygo build    # build installers into build/
```

## License

[GPL-3.0-or-later](LICENSE).
