// Package app is Althea's composition root: it builds every service, wires them together and
// runs the application. It is the only package that knows all the others.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo"

	"github.com/JoaoVitorOli/althea/internal/paths"
	"github.com/JoaoVitorOli/althea/internal/platform/host"
	"github.com/JoaoVitorOli/althea/internal/shell"
	"github.com/JoaoVitorOli/althea/internal/ui"
)

// Run starts Althea and blocks until it quits.
func Run() (err error) {
	ctx := context.Background()
	services := host.Services()

	p, err := paths.Resolve(runtime.GOOS, os.Getenv, portableDir())
	if err != nil {
		return err
	}
	c, err := bootstrap(ctx, p, services.Locale())
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, c.Close()) }()

	root := &ui.Root{T: c.catalog.T}
	mygo.App.WhenReady(func() {
		shell.Open(shell.Options{
			View:   root.View,
			Border: root.Theme().Border,
			Chrome: services.Chrome,
			Log:    c.log,
		})
	})
	if err := mygo.App.Run(); err != nil {
		c.log.Error("app stopped", "err", err)
		return fmt.Errorf("run app: %w", err)
	}
	c.log.Info("quit")
	return nil
}

// portableDir returns the executable's directory when an althea.portable file sits beside it,
// as in the portable zip, and "" otherwise.
func portableDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	if _, err := os.Stat(filepath.Join(dir, "althea.portable")); err != nil {
		return ""
	}
	return dir
}
