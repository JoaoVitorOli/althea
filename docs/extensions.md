# Writing extensions

Everything Althea does is an extension: the calculator, the applications, the system commands,
Script Commands. They use the public [`sdk`](../sdk) package and nothing else of Althea's, so
an extension you write can do what the built-in ones do. This guide walks through
[`extensions/example`](../extensions/example), a small to-do list that uses every part of the
SDK. Copy that folder to start.

Not a Go programmer? [Script Commands](#script-commands) turn any script into a command, with no
compiling.

## The shape of an extension

An extension is a Go type with a manifest and commands. It can also offer items to the root
search, answer queries, and declare preferences, by implementing more interfaces:

| Interface | What it adds | Built-in example |
|---|---|---|
| `sdk.Extension` (required) | `Manifest()` and `Commands(ctx)`: things to run by name | [system](../extensions/system/system.go) |
| `sdk.ItemSource` | `Items(ctx)`: things to search through, as installed apps | [apps](../extensions/apps/apps.go) |
| `sdk.Answerer` | `Answer(ctx, query)`: a direct answer above the results | [calculator](../extensions/calculator/calculator.go) |
| `sdk.FallbackSource` | `Fallbacks(ctx, query)`: an offer when the root search finds nothing | [web](../extensions/web/web.go) |
| `sdk.QueryTaker` | `TakeQuery(ctx, query)`: what Tab does with what was typed | [ai](../extensions/ai/ai.go) |
| `sdk.Configurable` | `Preferences(ctx)`: settings of its own | [example](../extensions/example/example.go) |

```go
type Extension struct{}

func (*Extension) Manifest() sdk.Manifest {
	messages, _ := fs.Sub(locales, "locales")
	return sdk.Manifest{ID: "althea.example", Title: "To-do", Icon: sdk.Icon{Text: "✅"}, Messages: messages}
}
```

The **ID** names the extension's storage, settings and messages. Choose it once, as
`yourname.feature`, and never change it: people's data hangs off it.

To ship it, add one line to [`extensions/builtin.go`](../extensions/builtin.go). The order there is
the order answers show in.

## Commands and actions

A command is something the user finds in the root search and runs:

```go
sdk.Command{
	ID: "list", Title: ctx.T("list.title"), Icon: sdk.Icon{Text: "✅"},
	Keywords: []string{"todo", "tasks", "tarefas"},
	Run: func(ctx sdk.Context) error {
		ctx.Push(e.list(ctx)) // show a view
		return nil
	},
}
```

Items carry **actions**. The first runs on `Enter`, the second on `Ctrl+Enter`, and all of them
from the action panel (`Ctrl+K`). An action with a `Shortcut` runs from it directly. An action
with `Confirm` runs only on a second press, with its message shown in between: use it for what
cannot be undone.

```go
sdk.Action{Title: "Delete", Shortcut: "Ctrl+D", Confirm: "Press Ctrl+D again to delete", Run: del}
```

`Run` gets a `Context` and runs off the main thread, so it may take its time; a returned error
is shown to the user as a toast, so write it for them.

## Views

A command or an action shows a view with `ctx.Push`; `Escape` goes back, and `ctx.Pop` does
the same from code. Althea draws views with the user's theme, motion and keys. You describe
what to show, not how.

### List and Grid

A list of items with a search field. Where its items come from decides who filters them:

| Field | When the items come | Who filters |
|---|---|---|
| `Items` | given at once | Althea, by title and keywords |
| `Load` | once, as the view opens, with a loading indicator | Althea |
| `Search` | for every query, the previous call cancelled | your function |

```go
&sdk.List{
	Title:       ctx.T("list.title"),
	Placeholder: ctx.T("list.search"),
	Empty:       sdk.Empty{Title: ctx.T("list.empty"), Description: ctx.T("list.empty_hint")},
	Load:        func(ctx sdk.Context) ([]sdk.Item, error) { /* read storage */ },
}
```

Items with a `Section` are grouped under headings while the search is empty. `ShowDetail: true`
shows the chosen item's `Detail` beside the list, as a preview.

A `Grid` takes the same fields plus `Columns`, and shows each item's icon large: emoji, colors,
images (`Icon.Image` is a path to a PNG, JPEG, GIF, WebP or BMP file). The arrows move in both
directions.

### Detail

A Markdown document with labeled values, and actions:

```go
&sdk.Detail{
	Title:    t.Title,
	Markdown: "# " + t.Title + "\n\n" + t.Notes,
	Metadata: []sdk.Metadata{{Label: "Added", Text: "2026-10-26 09:00"}},
	Actions:  []sdk.Action{{Title: "Copy Title", Run: copyTitle}},
}
```

Markdown is CommonMark with GitHub's tables, task lists and fenced code, which is highlighted.

For text that comes little by little, as a model's answer or a download's progress, set `Stream`.
It runs as the view opens, and each call of `show` replaces what shows. Escape stops it, cancelling
`ctx`, and leaves what came:

```go
&sdk.Detail{
	Title: "Summary",
	Stream: func(ctx sdk.Context, show func(markdown string)) error {
		return ask(ctx, text, show)
	},
}
```

### Chat

A conversation: what was said, over a field to say more. `Send` answers what the user said, as
`Stream` does, and `First` says something as the view opens. Keep what is said yourself, as
[ai](../extensions/ai/chat.go) keeps its conversations:

```go
&sdk.Chat{
	Title: "AI Chat",
	Send: func(ctx sdk.Context, text string, show func(markdown string)) error {
		return ask(ctx, text, show)
	},
}
```

### Form

Fields to fill: `TextField`, `TextArea`, `Password`, `Checkbox` and `Dropdown`. `Ctrl+Enter`
submits once every `Required` field is filled and every `Validate` returns "":

```go
&sdk.Form{
	Title: "Add To-do",
	Fields: []sdk.Field{
		{ID: "title", Title: "Title", Required: true},
		{ID: "notes", Title: "Notes", Kind: sdk.TextArea},
	},
	Submit: func(ctx sdk.Context, v sdk.Values) error {
		return save(ctx, v["title"], v["notes"])
	},
}
```

When `Submit` returns nil the form closes; if it pushed a view, that view takes the form's
place. An error keeps the form open and shows the error. Checkbox values are "true" or
"false" (`v.Bool(id)`), dropdown values are the chosen option's `Value`.

### Converter

Turns a text into another, as the Translator does: two choices with a swap between them, the text,
and what it became. The user finds an option by searching for it. `Convert` runs on Enter, never as
the user types, since it may send the text somewhere, and `show` replaces what shows:

```go
&sdk.Converter{
	Title: "Translate",
	From:  sdk.Picker{Title: "From", Value: "en", Options: languages},
	To:    sdk.Picker{Title: "To", Value: "pt", Options: languages},
	Convert: func(ctx sdk.Context, c sdk.Conversion, show func(sdk.Converted)) error {
		out, err := translate(ctx, c.Text, c.From, c.To)
		show(sdk.Converted{Text: out})
		return err
	},
}
```

A form's `Dropdown` with more than a few options can be searched too, by typing in it.

## Storage

`ctx.Storage()` is a key-value store of your own in Althea's database, kept across runs and
apart from every other extension's. Values are bytes; JSON is a fine encoding. Put records
under a prefix to list them:

```go
data, _ := json.Marshal(t)
err := ctx.Storage().Set(ctx, "todo/"+t.ID, data)

entries, err := ctx.Storage().List(ctx, "todo/") // in key order
```

## Preferences

Implement `sdk.Configurable` to declare settings. They appear under **Extension Preferences**
in Althea and as a table in `settings.toml`:

```go
func (*Extension) Preferences(ctx sdk.Context) []sdk.Preference {
	return []sdk.Preference{
		{Name: "order", Title: ctx.T("prefs.order"), Kind: sdk.ChoicePreference, Default: "newest",
			Options: []sdk.Option{{Value: "newest", Title: "Newest first"}, {Value: "oldest", Title: "Oldest first"}}},
		{Name: "api_key", Title: "API Key", Kind: sdk.SecretPreference, Required: true},
	}
}
```

```toml
[extensions."althea.example"]
order = "oldest"
```

Read them with `ctx.Preferences().String("order")`, `.Bool`, `.Number`, and
`.Secret("api_key")`. A value of the wrong type in the file is logged with its name and replaced
by the default, so you always get a valid value.

Kinds: `TextPreference`, `NumberPreference`, `BoolPreference`, `ChoicePreference`,
`PathPreference`, `SecretPreference` and `HotkeyPreference`. **Secrets** are never written to
`settings.toml`: they go to Windows Credential Manager. If a **required** preference is not set,
running any of the extension's commands first shows a form asking for it, then runs the
command.

## Hotkeys

Users can give any command a hotkey or an alias under **Hotkeys and Aliases**, with nothing to do
on your side. Both are kept by the command's `ID`, so choose it once, as you do the extension's. The
command runs with the launcher hidden unless it pushes a view, so write `Run` to work from
anywhere, and don't count on a toast being seen right away.

## Translations

Put `en.toml` and `pt-BR.toml` in a `locales/` folder, embed it, and return it as
`Manifest.Messages`. Then `ctx.T("list.title")` translates into the user's language, filling
`{name}` placeholders from name, value pairs:

```toml
# locales/en.toml
[add]
saved = "Added \"{title}\""
```

```go
ctx.Toast(ctx.T("add.saved", "title", t.Title))
```

Give **keywords** in both languages: search matches them whatever language the interface is
in, so "tarefas" finds the to-do list for an English user too.

## The network and the selection

Make every request with `ctx.HTTP()` and the `Context` you were given. The host logs it and lets it
through only if it comes from something the user did: running a command or an action, or opening
a view. From `Commands`, `Items`, `Answer`, `Fallbacks`, `Preferences` and `Serve` it is refused.

```go
req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, http.NoBody)
res, err := ctx.HTTP().Do(req)
```

An answer that needs data, as the Calculator's exchange rates, fetches it with `ctx.Go`: it runs
off the call, with a `Context` that outlives the answer's 50 ms and may reach the network, since
the user typed something that needs it. Show what is at hand meanwhile, keep what arrives, and call
`ctx.Refresh()` to be asked again. Never send what the user typed.

```go
ctx.Go(func(ctx sdk.Context) {
	rates, err := fetch(ctx)
	// keep them, then
	ctx.Refresh()
})
```

`ctx.Region()` is the user's country, as "BR", for defaults such as a currency.

`ctx.SelectedText()` reads the text selected in the app the user was in, "" when there is none. It
presses Ctrl+C there, hiding the launcher for a moment and leaving the clipboard as it was, so call
it only when the user asked for something to be done with the selection.

## AI Commands

A TOML file in the AI Commands folder (`%APPDATA%\Althea\ai-commands`; **Open AI Commands Folder**
opens it, with an example to copy) becomes a command, needing no Go. The file's name is its ID, and a
file named as a built-in one (`fix-grammar`, `summarize`, `explain-code`, `shorten`) replaces it.

```toml
title = { en = "Make Formal", pt-BR = "Tornar formal" } # or one text
subtitle = "Rewrite the text in a formal tone"
icon = "🎩"
keywords = ["formal", "tone"]
input = "selection"
output = "show"
system = "You rewrite text. Reply with the text alone."
prompt = """
Rewrite this in a formal tone:

{text}
"""
```

| Key | Value |
|---|---|
| `prompt` | Required: what is asked. `{text}` is the text, or it goes at the end without one; `{language}` is the user's language |
| `input` | `selection`, which falls back to the clipboard, by default; or `clipboard` |
| `output` | `show` in a view by default; `paste` over the selection; or `copy` |
| `system` | How the model should behave |
| `model` | Replaces the model of the AI preferences |
| `disabled` | `true` hides a built-in command, in a file with its name |

A file with a mistake is left out, and the log says why.

## Reaching the system

Extensions must not import Althea's `internal/` packages, nor MyGo; `depguard` rejects it. When
an extension needs the operating system, take the capability as a function in a `Platform`
struct, and let [`internal/app`](../internal/app) provide it. Script Commands get their scripts
folder and a way to run programs without a console window this way:

```go
type Platform struct {
	Dir     string
	Command func(ctx context.Context, name string, args ...string) *exec.Cmd
}
```

The extension stays testable with a fake, and runs wherever the host can provide the function.

## Testing

[`sdktest`](../sdk/sdktest/sdktest.go) gives a `Context` that records what an extension asks of
the host: what it opened, copied, toasted and pushed. It translates with the extension's real
catalogs, keeps Storage in a map and starts Prefs at their defaults:

```go
ctx := sdktest.New(t, example.New(), "pt-BR")
_ = ext.Commands(ctx)[1].Run(ctx)            // Add To-do
form := ctx.Pushed[0].(*sdk.Form)
_ = form.Submit(ctx, sdk.Values{"title": "Comprar pão"})
ctx.Prefs["order"] = "oldest"                // change a preference
```

See [example_test.go](../extensions/example/example_test.go). Test what the user sees: the
titles, the order, the toasts, what an action pushes.

## Things to keep

- **Answer fast.** An `Answerer` that takes longer than 50 ms is dropped for that keystroke.
  Return nil quickly for queries that are not yours.
- **Do slow work in `Load`, `Search` and actions,** never in `Commands` or `Items`, which the
  host calls as the launcher opens.
- **Ask before what cannot be undone** with `Confirm`.
- **Errors are for the user.** They show as toasts: "Server did not answer", not "EOF".
- **Panics are contained** (logged, and your extension's results dropped), but they are bugs.

## Script Commands

Any script in the scripts folder (`%APPDATA%\Althea\scripts`; **Open Script Commands Folder**
opens it) with an `@althea.title` comment becomes a command. **Create Script Command** writes
one from a template. Metadata goes in comments near the top, in whatever comment style the
language has (`#`, `//`, `--`, `::`, `REM`, `;`):

```powershell
# @althea.title Restart Explorer
# @althea.subtitle When the taskbar freezes
# @althea.mode silent
# @althea.icon 🔄
# @althea.confirm true

Stop-Process -Name explorer -Force
```

| Key | Value |
|---|---|
| `title` | Required: the command's name |
| `subtitle` | A line under the title |
| `mode` | How the output shows (below); `toast` by default |
| `icon` | An emoji, or an image file beside the script |
| `keywords` | Comma-separated extra search terms |
| `argument1`…`argument3` | Asked in a form before running and passed in order: `{"placeholder": "Query", "optional": true, "secure": true}`, or just the placeholder |
| `confirm` | `true` asks before running |
| `timeout` | Stops the script after this long, as `10s`; 30 s by default |
| `refresh` | How often an `inline` script runs again, as `5m`; at least 10 s |

| Mode | Shows |
|---|---|
| `toast` | The last line printed, as a message |
| `silent` | Nothing: the launcher hides, and only a failure shows |
| `detail` | Everything printed, in a view to read and copy |
| `list` | Each line printed as an item; Enter copies it |
| `inline` | The first line printed, under the title in the root search, refreshed in the background |

A script that exits with an error shows the last line it wrote to stderr.

Scripts run in their own folder, without a console window, with `ALTHEA=1` in their
environment. By extension:

| Extension | Runs with |
|---|---|
| `.ps1` | PowerShell 7 (`pwsh`) if installed, else Windows PowerShell; output in UTF-8 |
| `.cmd`, `.bat` | `cmd.exe`; start with `chcp 65001 >nul` for accents (the template does) |
| `.py` | `python3`, `python` or `py`, in UTF-8 mode |
| `.js`, `.mjs`, `.cjs` | `node` |
| `.sh` | `bash` (Git Bash or WSL's) |
| `.rb` | `ruby` |
| `.exe` | itself |
| anything else | the program its `#!` line names, found on `PATH` |

**Edit Script** opens a script in its editor, never with the program that runs it. Files whose
name starts with `.` or `_`, and files without `@althea.` metadata, are left alone, so helpers
can live beside the scripts.
