// Package scripts is the Script Commands extension: every script in the scripts folder becomes
// a command of the root search, described by comments in it, such as
//
//	# @althea.title Restart Explorer
//	# @althea.mode silent
//
// Scripts run as child processes, in their own folder, without a console window. See
// docs/extensions.md for the metadata they take.
package scripts

import (
	"context"
	"embed"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JoaoVitorOli/althea/sdk"
)

//go:embed locales/*.toml
var locales embed.FS

// Platform is what the extension needs from the host.
type Platform struct {
	// Dir is the scripts folder.
	Dir string
	// Command makes the command that runs a program, as exec.CommandContext, with no console
	// window on systems that would open one.
	Command func(ctx context.Context, name string, args ...string) *exec.Cmd
	// Edit opens a script in an editor, never running it; nil opens the scripts folder.
	Edit func(path string) error
	// LookPath finds a program on PATH; exec.LookPath when nil.
	LookPath func(file string) (string, error)
	// Now is the clock; time.Now when nil.
	Now func() time.Time
}

// Extension is the Script Commands extension.
type Extension struct {
	p Platform

	mu     sync.Mutex
	inline map[string]inlineOutput // by script path
}

// inlineOutput is the last output of an inline script.
type inlineOutput struct {
	line    string
	at      time.Time
	running bool
}

// New returns the Script Commands extension.
func New(p Platform) *Extension {
	if p.Command == nil {
		p.Command = exec.CommandContext
	}
	if p.Now == nil {
		p.Now = time.Now
	}
	return &Extension{p: p, inline: map[string]inlineOutput{}}
}

// Manifest implements sdk.Extension.
func (*Extension) Manifest() sdk.Manifest {
	messages, _ := fs.Sub(locales, "locales")
	return sdk.Manifest{ID: "althea.scripts", Title: "Script Commands", Icon: sdk.Icon{Glyph: "script"}, Messages: messages}
}

// Commands implements sdk.Extension.
func (e *Extension) Commands(ctx sdk.Context) []sdk.Command {
	return []sdk.Command{
		{
			ID: "create", Title: ctx.T("create.title"), Subtitle: ctx.T("create.subtitle"),
			Icon: sdk.Icon{Glyph: "script"}, Keywords: []string{"script", "new", "novo", "criar"},
			Run: func(ctx sdk.Context) error {
				ctx.Push(e.createForm(ctx))
				return nil
			},
		},
		{
			ID: "folder", Title: ctx.T("folder.title"), Subtitle: e.p.Dir,
			Icon: sdk.Icon{Glyph: "script"}, Keywords: []string{"scripts", "folder", "pasta"},
			Run: func(ctx sdk.Context) error {
				if err := os.MkdirAll(e.p.Dir, 0o700); err != nil {
					return err
				}
				ctx.Hide()
				return ctx.Open(e.p.Dir)
			},
		},
	}
}

// Items implements sdk.ItemSource: a row for each script in the folder. A script whose metadata
// has a mistake is left out, and the mistake logged.
func (e *Extension) Items(ctx sdk.Context) ([]sdk.Item, error) {
	scripts, err := e.Scripts(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]sdk.Item, 0, len(scripts))
	for _, s := range scripts {
		items = append(items, e.item(ctx, s))
	}
	return items, nil
}

// Scripts reads the scripts of the folder that have valid metadata. Files without any are
// helpers or notes beside the scripts, and names starting with . or _ are skipped.
func (e *Extension) Scripts(ctx sdk.Context) ([]Script, error) {
	entries, err := os.ReadDir(e.p.Dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Script
	for _, de := range entries {
		name := de.Name()
		if de.IsDir() || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}
		path := filepath.Join(e.p.Dir, name)
		data, err := head(path, 16<<10)
		if err != nil {
			ctx.Log().Warn("script not read", "path", path, "err", err)
			continue
		}
		if !strings.Contains(string(data), "@althea.") {
			continue
		}
		s, err := Parse(path, data)
		if err != nil {
			ctx.Log().Warn("script left out", "err", err)
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// head reads the first n bytes of a file.
func head(path string, n int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(io.LimitReader(f, n))
}

// item is the row of a script: running it, opening it and copying its path.
func (e *Extension) item(ctx sdk.Context, s Script) sdk.Item {
	rel, _ := filepath.Rel(e.p.Dir, s.Path)
	it := sdk.Item{
		ID: rel, Title: s.Title, Subtitle: s.Subtitle, Icon: s.Icon,
		Keywords:  append([]string{strings.TrimSuffix(rel, filepath.Ext(rel))}, s.Keywords...),
		Accessory: ctx.T("kind"),
		Actions: []sdk.Action{
			{Title: ctx.T("run"), Confirm: confirm(ctx, s), Run: func(ctx sdk.Context) error { return e.start(ctx, s) }},
			{Title: ctx.T("edit"), Shortcut: "Ctrl+E", Run: func(ctx sdk.Context) error {
				ctx.Hide()
				return e.edit(ctx, s.Path)
			}},
			{Title: ctx.T("copy_path"), Shortcut: "Ctrl+Shift+C", Run: func(ctx sdk.Context) error {
				return ctx.CopyText(s.Path)
			}},
		},
	}
	if s.Mode == Inline {
		if line := e.inlineLine(s); line != "" {
			it.Subtitle = line
		}
	}
	return it
}

func confirm(ctx sdk.Context, s Script) string {
	if !s.Confirm {
		return ""
	}
	return ctx.T("confirm", "title", s.Title)
}

// start runs a script, asking for its arguments first when it takes some.
func (e *Extension) start(ctx sdk.Context, s Script) error {
	if len(s.Arguments) == 0 {
		return e.run(ctx, s, nil)
	}
	fields := make([]sdk.Field, len(s.Arguments))
	for i, a := range s.Arguments {
		f := sdk.Field{ID: strconv.Itoa(i + 1), Title: a.Placeholder, Required: !a.Optional}
		if f.Title == "" {
			f.Title = ctx.T("argument", "n", i+1)
		}
		if a.Secure {
			f.Kind = sdk.Password
		}
		fields[i] = f
	}
	ctx.Push(&sdk.Form{
		Title: s.Title, Fields: fields, SubmitTitle: ctx.T("run"),
		Submit: func(ctx sdk.Context, v sdk.Values) error {
			args := make([]string, len(fields))
			for i, f := range fields {
				args[i] = v[f.ID]
			}
			return e.run(ctx, s, args)
		},
	})
	return nil
}

// run runs a script and shows its output as its mode says.
func (e *Extension) run(ctx sdk.Context, s Script, args []string) error {
	if s.Mode == Silent {
		ctx.Hide()
	}
	out, err := e.Run(ctx, s, args)
	if err != nil {
		return err
	}
	switch s.Mode {
	case Silent:
	case Detail:
		ctx.Push(e.detail(ctx, s, out))
	case List:
		ctx.Push(e.list(ctx, s, out))
	case Inline:
		e.setInline(s, firstLine(out.Stdout))
		toast(ctx, s, out)
	default:
		toast(ctx, s, out)
	}
	return nil
}

// toast shows the last line a script printed, or that it finished.
func toast(ctx sdk.Context, s Script, out Output) {
	msg := lastLine(out.Stdout)
	if msg == "" {
		msg = ctx.T("done", "title", s.Title)
	}
	ctx.Toast(msg)
}

// detail shows all of a script's output, as it printed it.
func (e *Extension) detail(ctx sdk.Context, s Script, out Output) *sdk.Detail {
	text := strings.TrimRight(out.Stdout, "\n")
	return &sdk.Detail{
		Title:    s.Title,
		Markdown: fence(text),
		Actions: []sdk.Action{
			{Title: ctx.T("copy_output"), Run: func(ctx sdk.Context) error {
				ctx.Toast(ctx.T("copied"))
				return ctx.CopyText(text)
			}},
			{Title: ctx.T("run_again"), Shortcut: "Ctrl+R", Run: func(ctx sdk.Context) error {
				ctx.Pop()
				return e.start(ctx, s)
			}},
		},
	}
}

// fence makes text a block of code in Markdown, its fence longer than any run of backticks in it.
func fence(text string) string {
	f := "```"
	for strings.Contains(text, f) {
		f += "`"
	}
	return f + "\n" + text + "\n" + f
}

// list shows each line of a script's output as an item, to copy.
func (e *Extension) list(ctx sdk.Context, s Script, out Output) *sdk.List {
	ls := lines(out.Stdout)
	items := make([]sdk.Item, len(ls))
	for i, l := range ls {
		items[i] = sdk.Item{Title: l, Actions: []sdk.Action{{Title: ctx.T("copy"), Run: func(ctx sdk.Context) error {
			ctx.Hide()
			return ctx.CopyText(l)
		}}}}
	}
	return &sdk.List{Title: s.Title, Items: items, Empty: sdk.Empty{Title: ctx.T("no_output")}}
}

// inlineLine returns what an inline script showed last, running it again in the background when
// that is older than its refresh. The new line shows the next time the items are read.
func (e *Extension) inlineLine(s Script) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	o := e.inline[s.Path]
	if !o.running && e.p.Now().Sub(o.at) >= s.Refresh {
		o.running = true
		e.inline[s.Path] = o
		go func() {
			out, err := e.Run(context.Background(), s, nil)
			line := firstLine(out.Stdout)
			if err != nil {
				line = ""
			}
			e.setInline(s, line)
		}()
	}
	return o.line
}

func (e *Extension) setInline(s Script, line string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.inline[s.Path] = inlineOutput{line: line, at: e.p.Now()}
}
