// Package extensions lists Althea's built-in extensions. A fork adds a feature by writing an
// extension in its own package and adding one line to Builtin.
package extensions

import (
	"github.com/JoaoVitorOli/althea/extensions/apps"
	"github.com/JoaoVitorOli/althea/extensions/calculator"
	"github.com/JoaoVitorOli/althea/extensions/clipboard"
	"github.com/JoaoVitorOli/althea/extensions/emoji"
	"github.com/JoaoVitorOli/althea/extensions/files"
	"github.com/JoaoVitorOli/althea/extensions/quicklinks"
	"github.com/JoaoVitorOli/althea/extensions/scripts"
	"github.com/JoaoVitorOli/althea/extensions/snippets"
	"github.com/JoaoVitorOli/althea/extensions/system"
	"github.com/JoaoVitorOli/althea/sdk"
)

// Platforms are what the built-in extensions need from the system; the host provides them.
type Platforms struct {
	Apps      apps.Platform
	System    system.Platform
	Scripts   scripts.Platform
	Snippets  snippets.Platform
	Clipboard clipboard.Platform
	Files     files.Platform
}

// Builtin returns the built-in extensions, in the order their answers show.
func Builtin(p Platforms) []sdk.Extension {
	return []sdk.Extension{
		calculator.New(),
		apps.New(p.Apps),
		system.New(p.System),
		scripts.New(p.Scripts),
		quicklinks.New(),
		snippets.New(p.Snippets),
		clipboard.New(p.Clipboard),
		files.New(p.Files),
		emoji.New(),
	}
}
