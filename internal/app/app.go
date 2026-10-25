// Package app is Althea's composition root: it builds every service, wires them together and
// runs the application. It is the only package that knows all the others.
package app

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/egoist/mygo"

	"github.com/JoaoVitorOli/althea/internal/platform/host"
	"github.com/JoaoVitorOli/althea/internal/shell"
	"github.com/JoaoVitorOli/althea/internal/ui"
)

// Run starts Althea and blocks until it quits.
func Run() error {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	services := host.Services()
	root := &ui.Root{}

	mygo.App.WhenReady(func() {
		shell.Open(shell.Options{
			View:   root.View,
			Border: root.Theme().Border,
			Chrome: services.Chrome,
			Log:    log,
		})
	})
	if err := mygo.App.Run(); err != nil {
		return fmt.Errorf("run app: %w", err)
	}
	return nil
}
