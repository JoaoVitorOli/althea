// Package calculator is the Calculator extension: math typed into the root search is answered
// at the top, as other launchers does, and Enter copies the answer.
package calculator

import (
	"embed"
	"io/fs"

	"github.com/JoaoVitorOli/althea/extensions/calculator/calc"
	"github.com/JoaoVitorOli/althea/sdk"
)

//go:embed locales/*.toml
var locales embed.FS

// Extension is the Calculator.
type Extension struct{}

// New returns the Calculator extension.
func New() *Extension { return &Extension{} }

// Manifest implements sdk.Extension.
func (*Extension) Manifest() sdk.Manifest {
	messages, _ := fs.Sub(locales, "locales")
	return sdk.Manifest{ID: "althea.calculator", Title: "Calculator", Icon: sdk.Icon{Glyph: "calculator"}, Messages: messages}
}

// Commands implements sdk.Extension: the calculator answers rather than runs.
func (*Extension) Commands(sdk.Context) []sdk.Command { return nil }

// Answer implements sdk.Answerer.
func (*Extension) Answer(ctx sdk.Context, query string) (*sdk.Item, error) {
	if !calc.LooksLikeMath(query) {
		return nil, nil
	}
	r, err := calc.Eval(query, ctx.Language())
	if err != nil {
		return nil, nil //nolint:nilerr // what is not math has no answer, which is no error
	}
	copyText := func(text string) func(sdk.Context) error {
		return func(ctx sdk.Context) error {
			if err := ctx.CopyText(text); err != nil {
				return err
			}
			ctx.Toast(ctx.T("copied", "answer", r.Text))
			ctx.Hide()
			return nil
		}
	}
	return &sdk.Item{
		ID: "answer", Title: r.Text, Subtitle: query, Icon: sdk.Icon{Glyph: "calculator"},
		Accessory: ctx.T("kind"),
		Actions: []sdk.Action{
			{Title: ctx.T("copy"), Run: copyText(r.Plain)},
			{Title: ctx.T("copy_expression"), Run: copyText(query + " = " + r.Text)},
		},
	}, nil
}
