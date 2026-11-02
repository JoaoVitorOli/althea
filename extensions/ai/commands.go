package ai

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/JoaoVitorOli/althea/sdk"
)

// aiCommand is an AI Command, as its TOML file says: a default the extension ships, or a file in
// the user's folder, which replaces the default of the same name.
type aiCommand struct {
	id string
	// Title and Subtitle are one text, or one per language as `title = { en = "…", pt-BR = "…" }`.
	Title    any      `toml:"title"`
	Subtitle any      `toml:"subtitle"`
	Icon     string   `toml:"icon"`
	Keywords []string `toml:"keywords"`
	// Input is where the text comes from: "selection" (the default), which falls back to the
	// clipboard, or "clipboard".
	Input string `toml:"input"`
	// Output is what happens to the answer: "show" (the default) writes it in a view, "paste" pastes
	// it over the selection, "copy" copies it.
	Output string `toml:"output"`
	// System tells the model how to behave. Prompt is what is asked, with {text} where the text
	// goes, or at its end without one; {language} is the user's language.
	System string `toml:"system"`
	Prompt string `toml:"prompt"`
	// Model replaces the model in the preferences.
	Model string `toml:"model"`
	// Disabled hides a default.
	Disabled bool `toml:"disabled"`
}

// text picks the value for a language from a title or subtitle.
func localized(v any, lang string) string {
	switch t := v.(type) {
	case string:
		return t
	case map[string]any:
		for _, key := range []string{lang, "en"} {
			if s, ok := t[key].(string); ok {
				return s
			}
		}
	}
	return ""
}

func parseCommand(id string, data []byte) (aiCommand, error) {
	c := aiCommand{id: id}
	if err := toml.Unmarshal(data, &c); err != nil {
		return c, err
	}
	if strings.TrimSpace(c.Prompt) == "" {
		return c, errors.New("it has no prompt")
	}
	switch c.Input {
	case "":
		c.Input = "selection"
	case "selection", "clipboard":
	default:
		return c, errors.New(`input is neither "selection" nor "clipboard"`)
	}
	switch c.Output {
	case "":
		c.Output = "show"
	case "show", "paste", "copy":
	default:
		return c, errors.New(`output is not "show", "paste" or "copy"`)
	}
	return c, nil
}

// readCommands reads the defaults and the user's folder. A file with a mistake costs that command,
// and says why in the log. Files starting with _ are left alone, as the example.
func (e *Extension) readCommands(ctx sdk.Context) []aiCommand {
	byID := map[string]aiCommand{}
	read := func(fsys fs.FS, where string) {
		entries, err := fs.ReadDir(fsys, ".")
		if err != nil {
			return
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".toml") || strings.HasPrefix(name, "_") {
				continue
			}
			data, err := fs.ReadFile(fsys, name)
			if err == nil {
				id := strings.TrimSuffix(name, ".toml")
				var c aiCommand
				if c, err = parseCommand(id, data); err == nil {
					byID[id] = c
					continue
				}
			}
			ctx.Log().Warn("AI Command left out", "file", filepath.Join(where, name), "err", err)
		}
	}
	defaults, _ := fs.Sub(files, "commands")
	read(defaults, "")
	if e.p.CommandsDir != "" {
		read(os.DirFS(e.p.CommandsDir), e.p.CommandsDir)
	}
	out := make([]aiCommand, 0, len(byID))
	for _, c := range byID {
		if !c.Disabled {
			out = append(out, c)
		}
	}
	slices.SortFunc(out, func(a, b aiCommand) int { return strings.Compare(a.id, b.id) })
	return out
}

// aiCommands makes a command of each AI Command.
func (e *Extension) aiCommands(ctx sdk.Context) []sdk.Command {
	lang := ctx.Language()
	var out []sdk.Command
	for _, c := range e.readCommands(ctx) {
		title := localized(c.Title, lang)
		if title == "" {
			title = c.id
		}
		icon := sdk.Icon{Glyph: "sparkles"}
		if c.Icon != "" {
			icon = sdk.Icon{Text: c.Icon}
		}
		out = append(out, sdk.Command{
			ID: "command/" + c.id, Title: title, Subtitle: localized(c.Subtitle, lang), Icon: icon,
			Keywords: append([]string{"ai", "ia"}, c.Keywords...),
			Run:      func(ctx sdk.Context) error { return e.runCommand(ctx, c, title) },
		})
	}
	return out
}

// input finds the text a command runs on.
func input(ctx sdk.Context, c aiCommand) (string, error) {
	if c.Input == "selection" {
		text, err := ctx.SelectedText()
		if err != nil {
			ctx.Log().Warn("selection not read", "err", err)
		}
		if strings.TrimSpace(text) != "" {
			return text, nil
		}
	}
	text := ctx.ClipboardText()
	if strings.TrimSpace(text) == "" {
		return "", errors.New(ctx.T("commands.no_text"))
	}
	return text, nil
}

// prompt puts the text in a command's prompt.
func prompt(ctx sdk.Context, c aiCommand, text string) string {
	p := strings.ReplaceAll(c.Prompt, "{language}", ctx.T("language"))
	if strings.Contains(p, "{text}") {
		return strings.ReplaceAll(p, "{text}", text)
	}
	return strings.TrimRight(p, "\n") + "\n\n" + text
}

// runCommand runs an AI Command on its text, as its output says.
func (e *Extension) runCommand(ctx sdk.Context, c aiCommand, title string) error {
	text, err := input(ctx, c)
	if err != nil {
		return err
	}
	req := Request{Model: c.Model, System: c.System, Messages: []Message{{Text: prompt(ctx, c, text)}}}
	if c.Output != "show" {
		written, err := e.Complete(ctx, req, func(string) {})
		if err != nil {
			return err
		}
		if c.Output == "paste" {
			return ctx.Paste(sdk.Clip{Text: written})
		}
		ctx.Toast(ctx.T("chat.copied"))
		return ctx.CopyText(written)
	}
	ctx.Push(e.commandView(ctx, req, title))
	return nil
}

// commandView writes a command's answer, with what can be done with it.
func (e *Extension) commandView(ctx sdk.Context, req Request, title string) *sdk.Detail {
	var a answer
	return &sdk.Detail{
		Title: title,
		Stream: func(ctx sdk.Context, show func(string)) error {
			written, err := e.Complete(ctx, req, func(s string) {
				a.set(s)
				show(s)
			})
			if err != nil && ctx.Err() != nil && written != "" {
				return nil
			}
			return err
		},
		Actions: []sdk.Action{
			{Title: ctx.T("chat.copy"), Shortcut: "Ctrl+Shift+C", Run: func(ctx sdk.Context) error {
				text := a.String()
				if text == "" {
					return nil // still on its way: the clipboard stays as it is
				}
				ctx.Toast(ctx.T("chat.copied"))
				return ctx.CopyText(text)
			}},
			{Title: ctx.T("chat.paste"), Shortcut: "Ctrl+Shift+V", Run: func(ctx sdk.Context) error {
				text := a.String()
				if text == "" {
					return nil
				}
				return ctx.Paste(sdk.Clip{Text: text})
			}},
			{Title: ctx.T("commands.continue"), Shortcut: "Ctrl+T", Run: func(ctx sdk.Context) error {
				c := newConversation()
				for _, m := range req.Messages {
					c.say(m)
				}
				c.title = title
				c.say(Message{Assistant: true, Text: a.String()})
				c.setLast(a.String())
				ctx.Pop()
				ctx.Push(e.chatView(ctx, c, ""))
				return nil
			}},
		},
	}
}

// exampleCommand is what the folder starts with, to copy.
const exampleCommand = `# An AI Command: a file here named <name>.toml becomes a command of the root search.
# A file named as a built-in one (fix-grammar, summarize, explain-code, shorten) replaces it.
# Files starting with _, as this one, are left alone. Copy it without the _ to try it.

# One text, or one per language: title = { en = "Make Formal", pt-BR = "Tornar formal" }
title = "Make Formal"
subtitle = "Rewrite the text in a formal tone"
icon = "🎩"
keywords = ["formal", "tone"]

# Where the text comes from: "selection" (falls back to the clipboard) or "clipboard".
input = "selection"

# What happens to the answer: "show" in a view, "paste" over the selection, or "copy".
output = "show"

# Tells the model how to behave. Optional.
system = "You rewrite text. Reply with the text alone."

# What is asked: {text} is the text, or it goes at the end without one; {language} is your language.
prompt = """
Rewrite this in a formal tone, in the language it is written in:

{text}
"""

# Replaces the model of the AI preferences. Optional.
# model = "llama3.2"

# Hides a built-in command, in a file with its name. Optional.
# disabled = true
`

// openCommandsFolder opens the folder of the user's AI Commands, with an example to copy.
func (e *Extension) openCommandsFolder(ctx sdk.Context) error {
	if e.p.CommandsDir == "" {
		return errors.New(ctx.T("commands.no_folder"))
	}
	if err := os.MkdirAll(e.p.CommandsDir, 0o700); err != nil {
		return err
	}
	example := filepath.Join(e.p.CommandsDir, "_example.toml")
	if _, err := os.Stat(example); errors.Is(err, fs.ErrNotExist) {
		if err := os.WriteFile(example, []byte(exampleCommand), 0o600); err != nil {
			return err
		}
	}
	ctx.Hide()
	return ctx.Open(e.p.CommandsDir)
}
