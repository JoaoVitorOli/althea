# Extensions

Everything Althea does is an extension here, and a fork adds features the same way the built-ins
were written. Each package implements the public [`sdk`](../sdk/sdk.go) and nothing else of
Althea's: `depguard` rejects an import of `internal/`. **[docs/extensions.md](../docs/extensions.md)
is the full guide.**

```
            ┌──────────────┐  Commands(ctx)   ┌────────────┐
            │              │─────────────────▶│            │
            │  extension   │  Items(ctx)      │  registry  │──▶ root search
            │  (your pkg)  │─────────────────▶│            │
            │              │  Answer(ctx, q)  │            │
            └──────┬───────┘─────────────────▶└────────────┘
                   │ ctx: Push views, Storage, Preferences, Open, CopyText, Hide, Toast, T
                   ▼
                 host
```

Pick the shape by what the feature is:

| The feature… | Implement | Example |
|---|---|---|
| does something by name | `Commands` | [system](system/system.go): Lock Screen, Restart |
| contributes things to search through | `ItemSource` | [apps](apps/apps.go): every installed app |
| answers what was typed | `Answerer` | [calculator](calculator/calculator.go): `2+2` |
| has settings of its own | `Configurable` | [example](example/example.go): the order of to-dos |

An extension can be all of them. [scripts](scripts/scripts.go) turns the scripts folder into
commands, and [example](example/example.go) is the template: a to-do list with a list, a detail
view, a form, storage and a preference, tested in [example_test.go](example/example_test.go).
It is built and tested but not in `builtin.go`.

## Adding one

1. Copy `example/` to `extensions/<name>/` and give its `Manifest` a stable `ID`, such as
   `althea.weather`. The ID namespaces its messages, storage and settings, so do not change it
   once people use it.
2. Keep `en.toml` and `pt-BR.toml` in its `locales/` folder. Translate user-facing text with
   `ctx.T("key")`.
3. Add one line to [`builtin.go`](builtin.go). The order there is the order answers show in.
4. Test it with [`sdktest`](../sdk/sdktest/sdktest.go): a `Context` that records what the
   extension opened, copied, toasted and pushed, keeps its storage in memory and translates with
   its real catalogs.

## Things to keep

- **Reach the system through what the host gives you.** When an extension needs the OS, as
  Applications does to list apps, take the capability as a function in a `Platform` struct and
  let `internal/app` provide it. The extension stays testable with a fake and free of MyGo.
- **Answer fast.** An `Answerer` that takes longer than 50 ms is dropped for that keystroke.
  Return `nil` quickly for queries that are not yours. Slow work goes in a view's `Load` or
  `Search`, or in an action.
- **Ask before what cannot be undone.** Set `Confirm` on the command or action, as Restart does.
- **Errors are for the user.** An action's error is shown as a toast, so make it readable.
- **Keywords in both languages.** Search matches keywords whatever the interface language, so
  "bloquear" finds Lock Screen.
