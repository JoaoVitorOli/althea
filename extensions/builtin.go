// Package extensions lists Althea's built-in extensions. A fork adds a feature by writing an
// extension in its own package and adding one line to Builtin.
package extensions

import (
	"github.com/JoaoVitorOli/althea/extensions/ai"
	"github.com/JoaoVitorOli/althea/extensions/apps"
	"github.com/JoaoVitorOli/althea/extensions/calculator"
	"github.com/JoaoVitorOli/althea/extensions/clipboard"
	"github.com/JoaoVitorOli/althea/extensions/emoji"
	"github.com/JoaoVitorOli/althea/extensions/files"
	"github.com/JoaoVitorOli/althea/extensions/quicklinks"
	"github.com/JoaoVitorOli/althea/extensions/scripts"
	"github.com/JoaoVitorOli/althea/extensions/snippets"
	"github.com/JoaoVitorOli/althea/extensions/system"
	"github.com/JoaoVitorOli/althea/extensions/translator"
	"github.com/JoaoVitorOli/althea/extensions/web"
	"github.com/JoaoVitorOli/althea/extensions/windows"
	"github.com/JoaoVitorOli/althea/sdk"
)

// Platforms are what the built-in extensions need from the system; the host provides them.
type Platforms struct {
	Apps       apps.Platform
	System     system.Platform
	Scripts    scripts.Platform
	Snippets   snippets.Platform
	Clipboard  clipboard.Platform
	Files      files.Platform
	Windows    windows.Platform
	AI         ai.Platform
	Translator translator.Platform
	// AsExtension gives a Context of something being done the identity of another extension, for
	// one built-in to reach another's preferences, as the Translator does the AI provider's.
	AsExtension func(ctx sdk.Context, ext string) sdk.Context
}

// Builtin returns the built-in extensions, in the order their answers and fallbacks show.
func Builtin(p Platforms) []sdk.Extension {
	intelligence := ai.New(p.AI)
	p.Translator.AI = func(ctx sdk.Context, system, text string, show func(string)) (string, error) {
		return intelligence.Complete(p.AsExtension(ctx, ai.ID), ai.Request{
			System: system, Messages: []ai.Message{{Text: text}},
		}, show)
	}
	return []sdk.Extension{
		calculator.New(),
		apps.New(p.Apps),
		system.New(p.System),
		scripts.New(p.Scripts),
		quicklinks.New(),
		snippets.New(p.Snippets),
		clipboard.New(p.Clipboard),
		web.New(),
		intelligence,
		translator.New(p.Translator),
		files.New(p.Files),
		emoji.New(),
		windows.New(p.Windows),
	}
}
