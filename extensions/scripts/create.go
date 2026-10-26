package scripts

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/JoaoVitorOli/althea/sdk"
)

// language is a language a new script can be written in: its file extension, how its comments
// start and a body that prints a line.
type language struct {
	name, ext, comment, header, body string
}

var languages = []language{
	{"PowerShell", ".ps1", "#", "", `Write-Output "Hello from PowerShell"`},
	{"Batch", ".cmd", "::", "@echo off\r\nchcp 65001 >nul\r\n", "echo Hello from Batch"},
	{"Python", ".py", "#", "#!/usr/bin/env python3\n", `print("Hello from Python")`},
	{"JavaScript (Node.js)", ".js", "//", "#!/usr/bin/env node\n", `console.log("Hello from Node.js");`},
	{"Bash", ".sh", "#", "#!/usr/bin/env bash\n", `echo "Hello from Bash"`},
}

// createForm asks for a new script's title, language and output, then writes it from a
// template into the scripts folder and opens it to edit.
func (e *Extension) createForm(ctx sdk.Context) *sdk.Form {
	langs := make([]sdk.Option, len(languages))
	for i, l := range languages {
		langs[i] = sdk.Option{Value: l.ext, Title: l.name}
	}
	var modes []sdk.Option
	for _, m := range []Mode{Toast, Silent, Detail, List, Inline} {
		modes = append(modes, sdk.Option{Value: string(m), Title: ctx.T("mode." + string(m))})
	}
	return &sdk.Form{
		Title: ctx.T("create.title"),
		Fields: []sdk.Field{
			{ID: "title", Title: ctx.T("create.name"), Placeholder: ctx.T("create.name_placeholder"), Required: true},
			{ID: "language", Title: ctx.T("create.language"), Kind: sdk.Dropdown, Options: langs},
			{ID: "mode", Title: ctx.T("create.mode"), Kind: sdk.Dropdown, Options: modes},
		},
		Submit: func(ctx sdk.Context, v sdk.Values) error {
			path, err := e.Create(v["title"], v["language"], Mode(v["mode"]))
			if errors.Is(err, fs.ErrExist) {
				return errors.New(ctx.T("create.exists", "file", filepath.Base(path)))
			}
			if err != nil {
				return err
			}
			ctx.Hide()
			return e.edit(ctx, path)
		},
	}
}

// Create writes a new script with a title, in the language of a file extension, and returns
// its path. A script of that name already there is left alone, with fs.ErrExist.
func (e *Extension) Create(title, ext string, mode Mode) (string, error) {
	var lang language
	for _, l := range languages {
		if l.ext == ext {
			lang = l
		}
	}
	if lang.ext == "" {
		return "", fmt.Errorf("no template for %q", ext)
	}
	if err := os.MkdirAll(e.p.Dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(e.p.Dir, slug(title)+lang.ext)
	nl := "\n"
	if lang.ext == ".cmd" {
		nl = "\r\n"
	}
	var b strings.Builder
	b.WriteString(lang.header)
	for _, kv := range [][2]string{{"title", title}, {"mode", string(mode)}, {"icon", "📜"}} {
		fmt.Fprintf(&b, "%s @althea.%s %s%s", lang.comment, kv[0], kv[1], nl)
	}
	b.WriteString(nl + lang.body + nl)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700) //nolint:gosec // a script runs: on Linux it must be executable
	if err != nil {
		return path, err
	}
	if _, err := f.WriteString(b.String()); err != nil {
		_ = f.Close()
		return path, err
	}
	return path, f.Close()
}

// slug makes a file name of a title: lowercase letters and digits joined by hyphens.
func slug(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
			continue
		}
		dash = true
	}
	if b.Len() == 0 {
		return "script"
	}
	return b.String()
}

// edit opens a script in an editor. Opening it with the system would run it.
func (e *Extension) edit(ctx sdk.Context, path string) error {
	if e.p.Edit != nil {
		return e.p.Edit(path)
	}
	return ctx.Open(filepath.Dir(path))
}
