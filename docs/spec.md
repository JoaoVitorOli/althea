# Althea v1 — Product Spec

Althea is a keyboard-first launcher for the desktop, in the spirit of other launchers: one hotkey opens a
search bar from which you launch apps, run commands and use built-in tools. Everything other launchers
sells in its Pro plan ships in Althea for free, and anyone can extend it.

This document is the source of truth for **what** v1 is. [architecture.md](architecture.md)
describes **how** it is built, and [adr/](adr/) records **why** each decision was made.

## Principles

1. **Instant.** The window shows within 50 ms of the hotkey and results update within a frame.
2. **Private.** No telemetry. No network request leaves the app unless the user asked for it or
   turned on a feature that needs it.
3. **Modular.** Every feature, built-in or not, is an extension using the same public API. A fork
   adds a feature by adding a package and one line.
4. **Configurable.** Every behavior worth changing is a setting, in a readable TOML file and in the
   settings screen.
5. **Delightful.** Every interaction has considered motion: short, interruptible, never in the way,
   and off when the system asks for reduced motion.

## Platforms

| Platform | v1 | Notes |
|---|---|---|
| Windows 11 x64 (22000+) | ✅ First class | Acrylic/Mica backdrops via DWM |
| Windows 10 x64 | ⚠️ Runs | No system backdrop; translucent solid fallback |
| Linux x64 (X11, Wayland) | v1.1 | Compiles and tests in CI from day one; hotkeys via XDG portal on Wayland |
| Windows ARM64, macOS | ❌ | Out of scope |

## Shell and window

- Frameless, always-on-top launcher window, centered horizontally on the display under the cursor,
  at a quarter of its height.
- Hides when it loses focus. `Esc` goes back one level and hides at the root.
- Re-opens at the root after a configurable delay (default 90 s); otherwise keeps the last state.
- Lives in the tray (open, settings, quit). Single instance: a second launch focuses the first.
- Starts with Windows (asked in onboarding).
- **Hotkey**: default `Alt+Space`. Any combination can be recorded, or the `Win` key alone (via a
  low-level keyboard hook that suppresses the Start menu only when `Win` is pressed and released
  with nothing in between). If the chosen hotkey is taken, the user is told and offered another.
- **Onboarding** on first run: hotkey recorder, theme picker, start at login, update checks opt-in,
  language.

## Root search

- One search field; results from every extension, ranked together.
- Fuzzy matching with scoring for prefixes, word starts and acronyms ("vsc" → Visual Studio Code).
- **Frecency**: what you pick often and recently ranks higher, per query prefix.
- Inline answers at the top: calculator results, conversions, quick AI prompt hint.
- **Aliases** (type `cl` for Clipboard History) and **per-command hotkeys**.
- **Action panel** (`Ctrl+K`): every item has a primary action (`Enter`), a secondary
  (`Ctrl+Enter`) and more, each with a shortcut shown in the footer.
- Fallback commands when nothing matches: search the web, ask AI, search files.

## Built-in extensions

Each one is an extension in `extensions/` (see [architecture.md](architecture.md)).

### Core

| Extension | v1 scope |
|---|---|
| **Applications** | Win32 apps (Start menu `.lnk`, registry App Paths) and UWP/Store apps (`shell:AppsFolder`), icons cached; actions: open, run as admin, show in folder, copy path, uninstall settings |
| **System** | Lock, sleep, restart, shut down, log out, empty recycle bin, toggle dark mode, open Windows settings pages |
| **Calculator** | Expressions, percentages, units, bases (hex/bin), dates and durations, time zones, currency (rates fetched only when used, cached daily) |
| **File Search** | Own index of configured folders (default: Desktop, Documents, Downloads, Pictures, Videos, Music), incremental watcher; Everything used as an optional backend for whole disks |
| **Quicklinks** | URLs or paths with `{query}` and `{clipboard}` placeholders, own icon and alias |
| **Snippets** | Searchable text snippets with placeholders (`{date}`, `{clipboard}`, `{cursor}`); automatic expansion by keyword is **opt-in** (keyboard hook), paused in password fields |
| **Clipboard History** | Text, images, files, links, colors; unlimited, encrypted at rest, retention configurable; ignores password managers (`ExcludeClipboardContentFromMonitorProcessing`) and listed apps; pin, paste as plain text, OCR text in images |
| **Window Management** | Halves, thirds, quarters, center, maximize, restore, next display, almost-maximize; hotkeys for each; **custom layouts** (a set of apps placed together, one command) |
| **Emoji & Symbols** | Search by name and keyword in English and Portuguese, skin tones, recent |
| **Settings** | The settings screen, theme picker, extension list with their preferences and hotkeys |

### Free versions of paid launchers

| Extension | v1 scope |
|---|---|
| **AI** | Providers behind one interface: OpenAI-compatible (OpenAI, Ollama, LM Studio, llama.cpp, OpenRouter, Groq), Anthropic, Gemini. Quick AI from the root search (`Tab` to ask), AI Chat with saved conversations, **AI Commands** defined in TOML that run on the selected text or clipboard (fix grammar, summarize, explain code, …). Streaming everywhere. Keys stored in the OS vault. |
| **Translator** | Offline with Bergamot: engine and `tiny` models downloaded on demand and verified with sha256, run as a persistent subprocess; online providers optional (the configured AI provider, DeepL, LibreTranslate). Translates typed text or the selection; caches results. |
| **Themes** | Custom themes as TOML files with full tokens, inheriting from a base theme, hot-reloaded; a visual **theme editor** that writes the same files. |
| **Sync** | Settings, themes, snippets, quicklinks, AI commands and notes exported to a folder the user picks (OneDrive, Dropbox, Syncthing), end-to-end encrypted with a sync passphrase; merged per record, last write wins per item. Clipboard history stays local. |
| **Floating Notes** | Unlimited Markdown notes in a small always-on-top window, searchable from the root. |

### Extensibility

- **Go extensions**: compiled in, same API as the built-ins. Writing one is documented in
  `docs/extensions.md` with a template.
- **Script Commands**: any executable script in a scripts folder becomes a command. Metadata lives
  in comments (`# @althea.title`, `# @althea.mode`, `# @althea.argument1`), output shown inline,
  as a toast, or as a list.

## Themes

A theme is a TOML file in `%APPDATA%\Althea\themes\`. It can set:

- **Appearance** (`light` or `dark`) and **colors**: background, surfaces, text, accent, borders,
  selection, status. Settings choose a theme, or `system` to follow Windows between a light and a
  dark one.
- **Material**: `acrylic`, `mica`, `glass` (shader), or `solid`; blur, opacity and tint.
- **Shape and space**: radii, paddings, row height, density.
- **Type**: font family and sizes.
- **Motion**: a scale for durations (0 turns motion off) and the spring settings.

A theme may `extends = "althea-dark"` and override only what it changes. Invalid values are
reported in a toast with the file and line, and the last valid theme stays applied.

## Configuration

- `%APPDATA%\Althea\settings.toml`, commented, validated against a schema, hot-reloaded on save.
- Each extension declares typed preferences (string, number, bool, enum, path, secret, hotkey);
  they appear in the settings screen and under `[extensions.<id>]` in the file.
- Secrets never go in the file: they are stored in Windows Credential Manager.

## Data

- One SQLite database (`althea.db`) with versioned migrations, WAL and FTS5.
- Each extension gets its own namespace (key-value store and tables prefixed by its ID).
- Clipboard history and notes are encrypted with AES-GCM; the key lives in the OS vault.

## Languages

English and Portuguese (Brazil), following the system language, configurable. Every string goes
through a message catalog; extensions bring their own catalogs.

## Privacy and network

The only network traffic Althea makes:

| Traffic | When |
|---|---|
| Update check | If the user opted in during onboarding or in settings |
| Model downloads (translator, local AI helpers) | When the user turns the feature on |
| AI and translation providers | When the user runs a command with a provider configured |
| Currency rates | When a calculation uses a currency, at most once a day |
| Quicklinks, web search | When the user runs them (opens the browser) |

Crash reports are written locally; the user can copy one into an issue.

## Performance budgets

Checked by benchmarks and regression tests; CI fails if a budget is missed or a benchmark gets
more than 10 % slower than `main`.

| Metric | Budget |
|---|---|
| Hotkey to visible window | < 50 ms p95 |
| Keystroke to updated results (10 000 items) | < 16 ms p95 |
| Animations and scrolling | 60 fps, no dropped frames |
| Idle memory (after warm-up) | < 60 MB private working set, as Task Manager shows it (measured with MyGo's `internal/idlemem`) |
| Cold start to ready | < 300 ms |
| Idle CPU | ~0 % (no timers while hidden) |

## Quality

- **TDD** for every behavior, vertical slices; core coverage ≥ 80 %.
- **Lint and format**: gofumpt, gci, golangci-lint v2 (strict), commit message linting.
- **Continuous guards**: tests under `-race`, goroutine leak checks, a query counter that fails tests
  on N+1 patterns, govulncheck and gosec.
- **Final audit** (M8): 24 h soak test watching heap, handles and GDI objects; fuzzing of parsers;
  threat review of hooks, clipboard, downloads, scripts and AI; report in `docs/audit/`.

## Distribution

- Windows installer and portable zip on GitHub Releases, built by CI on tags.
- Signed (ed25519) auto-updates with deltas; update checks are opt-in.
- Authenticode signing added through SignPath (free for open source) after the first release;
  winget and Scoop manifests after v1.

## License

GPL-3.0-or-later.

## Milestones

Each milestone ends with a pause for the maintainer to test.

| # | Milestone | Done when |
|---|---|---|
| M0 | Foundation | Layout, lint/format, hooks, CI (Windows + Linux), config, SQLite + migrations, i18n, logging |
| M1 | Shell | Window, hotkey (incl. `Win`), tray, single instance, start at login, onboarding, theme engine with materials and motion |
| M2 | Search | Registry, fuzzy + frecency, root list, action panel, Applications, System, Calculator |
| M3 | Extension API | Public SDK, declarative views (List, Grid, Detail, Form), preferences, Script Commands, extension guide |
| M4 | Productivity | Clipboard History, Snippets, Quicklinks, Emoji, File Search |
| M5 | Windows | Window Management and custom layouts |
| M6 | Intelligence | AI (providers, Quick AI, chat, AI Commands) and Translator |
| M7 | Personal | Floating Notes, Sync, theme editor |
| M8 | Release | Security and performance audit, installer, updater, v1.0.0 |
