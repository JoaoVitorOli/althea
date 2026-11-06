package app

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"strings"
	"sync"

	"github.com/JoaoVitorOli/althea/internal/config"
	"github.com/JoaoVitorOli/althea/internal/platform"
	"github.com/JoaoVitorOli/althea/internal/prefs"
	"github.com/JoaoVitorOli/althea/internal/registry"
	"github.com/JoaoVitorOli/althea/sdk"
)

// preferences keeps what each extension declares of its preferences and the values the user
// set, reread as settings.toml changes. It is safe for concurrent use: extensions read their
// values from goroutines of their own.
type preferences struct {
	path  string         // settings.toml
	vault platform.Vault // nil where secrets cannot be kept
	log   *slog.Logger

	mu     sync.RWMutex
	decl   map[string][]sdk.Preference // by extension
	values map[string]prefs.Values
	// changed hears of each extension whose values changed, outside the lock.
	changed func(ext string)
}

func newPreferences(path string, vault platform.Vault, log *slog.Logger) *preferences {
	return &preferences{
		path: path, vault: vault, log: log,
		decl: map[string][]sdk.Preference{}, values: map[string]prefs.Values{},
	}
}

// declare records an extension's preferences.
func (p *preferences) declare(ext string, decl []sdk.Preference) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.decl[ext] = decl
}

// resolve reads every extension's values from the settings, logging what is wrong in them.
func (p *preferences) resolve(s config.Settings) {
	var changed []string
	p.mu.Lock()
	for ext, decl := range p.decl {
		values, problems := prefs.Resolve(decl, s.Extensions[ext])
		for _, pr := range problems {
			p.log.Warn("preference problem", "extension", ext, "preference", pr.Name, "problem", pr.Message)
		}
		if old, ok := p.values[ext]; ok && !maps.Equal(old, values) {
			changed = append(changed, ext)
		}
		p.values[ext] = values
	}
	for ext := range s.Extensions {
		if _, ok := p.decl[ext]; !ok {
			p.log.Warn("preferences of an unknown extension", "extension", ext)
		}
	}
	notify := p.changed
	p.mu.Unlock()
	if notify != nil {
		for _, ext := range changed {
			notify(ext)
		}
	}
}

// of returns an extension's preferences, as it reads them.
func (p *preferences) of(ext string) sdk.Preferences {
	p.mu.RLock()
	values := p.values[ext]
	p.mu.RUnlock()
	return prefs.Preferences{Values: values, ReadSecret: func(name string) (string, error) {
		if p.vault == nil {
			return "", nil
		}
		s, _, err := p.vault.Get(secretName(ext, name))
		return s, err
	}}
}

// declared returns an extension's preferences and their values.
func (p *preferences) declared(ext string) ([]sdk.Preference, prefs.Values) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.decl[ext], maps.Clone(p.values[ext])
}

// missing returns the required preferences of an extension without a value.
func (p *preferences) missing(ext string) []sdk.Preference {
	decl, values := p.declared(ext)
	return prefs.Missing(decl, values, func(name string) bool { return p.hasSecret(ext, name) })
}

func (p *preferences) hasSecret(ext, name string) bool {
	if p.vault == nil {
		return false
	}
	_, ok, err := p.vault.Get(secretName(ext, name))
	return ok && err == nil
}

// save writes the values a form gave: secrets to the vault, the rest to settings.toml, where an
// empty value unsets the preference. An empty secret keeps the one saved.
func (p *preferences) save(ext string, typed sdk.Values) error {
	decl, current := p.declared(ext)
	updated := maps.Clone(current)
	if updated == nil {
		updated = prefs.Values{}
	}
	var errs []error
	for _, pref := range decl {
		text, ok := typed[pref.Name]
		if !ok {
			continue
		}
		if pref.Kind == sdk.SecretPreference {
			if text != "" {
				errs = append(errs, p.saveSecret(ext, pref.Name, text))
			}
			continue
		}
		if text == current[pref.Name] {
			continue
		}
		key := []string{"extensions", ext, pref.Name}
		if text == "" {
			errs = append(errs, config.Unset(p.path, key))
			delete(updated, pref.Name)
			if pref.Default != "" {
				updated[pref.Name] = pref.Default
			}
			continue
		}
		v, err := prefs.Encode(pref, text)
		if err == nil {
			err = config.SetPath(p.path, key, v)
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		updated[pref.Name] = text
	}
	// The watcher rereads the file in a moment; the values hold until then, for what runs now.
	p.mu.Lock()
	p.values[ext] = updated
	notify := p.changed
	p.mu.Unlock()
	if notify != nil {
		notify(ext) // a secret may have changed, which the file does not show
	}
	return errors.Join(errs...)
}

func (p *preferences) saveSecret(ext, name, secret string) error {
	if p.vault == nil {
		return errors.New("this system cannot keep secrets yet")
	}
	return p.vault.Set(secretName(ext, name), secret)
}

// secretName names an extension's secret in the vault.
func secretName(ext, name string) string { return "Althea/" + ext + "/" + name }

// settingsID is the ID of the host's own settings commands, an extension like the others.
const settingsID = "althea.settings"

//go:embed locales/*.toml
var locales embed.FS

// settingsMessages are the catalogs of the settings commands.
var settingsMessages, _ = fs.Sub(locales, "locales")

// settingsExtension offers the settings of the extensions: a list of those with preferences,
// each opening a form of them. It lives in the host because it writes settings.toml and the
// vault, which no extension may touch.
type settingsExtension struct {
	prefs *preferences
	// exts are the extensions with preferences, in the order of the built-ins.
	exts []sdk.Manifest
	// commands returns every extension's commands, to give them hotkeys; nil offers none.
	commands func() []registry.Result
	// titles names the extensions, by ID.
	titles map[string]string
	// editThemes opens the theme editor; nil offers none.
	editThemes func()
}

func (s *settingsExtension) Manifest() sdk.Manifest {
	return sdk.Manifest{ID: settingsID, Title: "Settings", Icon: sdk.Icon{Glyph: "settings"}, Messages: settingsMessages}
}

func (s *settingsExtension) Commands(ctx sdk.Context) []sdk.Command {
	var cmds []sdk.Command
	if len(s.exts) > 0 {
		cmds = append(cmds, sdk.Command{
			ID: "extension-preferences", Title: ctx.T("preferences.title"), Subtitle: ctx.T("preferences.subtitle"),
			Icon: sdk.Icon{Glyph: "settings"}, Keywords: strings.Fields(ctx.T("preferences.keywords")),
			Run: func(ctx sdk.Context) error {
				ctx.Push(s.list(ctx))
				return nil
			},
		})
	}
	if s.commands != nil {
		cmds = append(cmds, sdk.Command{
			ID: "command-hotkeys", Title: ctx.T("hotkeys.title"), Subtitle: ctx.T("hotkeys.subtitle"),
			Icon: sdk.Icon{Glyph: "command"}, Keywords: strings.Fields(ctx.T("hotkeys.keywords")),
			Run: func(ctx sdk.Context) error {
				ctx.Push(s.hotkeyList(ctx))
				return nil
			},
		})
	}
	if s.editThemes != nil {
		cmds = append(cmds, sdk.Command{
			ID: "theme-editor", Title: ctx.T("themes.title"), Subtitle: ctx.T("themes.subtitle"),
			Icon: sdk.Icon{Glyph: "color"}, Keywords: strings.Fields(ctx.T("themes.keywords")),
			Run: func(ctx sdk.Context) error {
				ctx.Hide()
				s.editThemes()
				return nil
			},
		})
	}
	return cmds
}

// list lists the extensions with preferences.
func (s *settingsExtension) list(ctx sdk.Context) *sdk.List {
	items := make([]sdk.Item, 0, len(s.exts))
	for _, m := range s.exts {
		decl, _ := s.prefs.declared(m.ID)
		item := sdk.Item{
			ID: m.ID, Title: m.Title, Icon: m.Icon,
			Subtitle: ctx.T("preferences.count", "count", len(decl)),
			Actions: []sdk.Action{{Title: ctx.T("preferences.edit"), Run: func(ctx sdk.Context) error {
				ctx.Push(s.form(ctx, m, nil))
				return nil
			}}},
		}
		if len(s.prefs.missing(m.ID)) > 0 {
			item.Accessory = ctx.T("preferences.needs_setup")
		}
		items = append(items, item)
	}
	return &sdk.List{Title: ctx.T("preferences.title"), Placeholder: ctx.T("preferences.search"), Items: items}
}

// form edits the preferences of an extension; then, when set, runs after they are saved, as the
// command that asked for them.
func (s *settingsExtension) form(ctx sdk.Context, m sdk.Manifest, then func(ctx sdk.Context) error) *sdk.Form {
	decl, values := s.prefs.declared(m.ID)
	fields := make([]sdk.Field, 0, len(decl))
	for _, p := range decl {
		f := sdk.Field{
			ID: p.Name, Title: p.Title, Info: p.Description, Required: p.Required,
			Default: values[p.Name], Placeholder: p.Default,
		}
		switch p.Kind {
		case sdk.BoolPreference:
			f.Kind, f.Placeholder = sdk.Checkbox, ""
		case sdk.ChoicePreference:
			f.Kind = sdk.Dropdown
			for _, o := range p.Options {
				if o.Title == "" {
					o.Title = o.Value
				}
				f.Options = append(f.Options, o)
			}
		case sdk.SecretPreference:
			f.Kind, f.Placeholder = sdk.Password, ""
			if s.prefs.hasSecret(m.ID, p.Name) {
				f.Placeholder, f.Required = ctx.T("preferences.secret_saved"), false
			}
		case sdk.NumberPreference, sdk.HotkeyPreference:
			msg := ctx.T("preferences.invalid_number")
			if p.Kind == sdk.HotkeyPreference {
				msg = ctx.T("preferences.invalid_hotkey")
			}
			pref := p
			f.Validate = func(v string) string {
				if prefs.Check(pref, v) != "" {
					return msg
				}
				return ""
			}
		default:
		}
		fields = append(fields, f)
	}
	title := ctx.T("preferences.form_title", "extension", m.Title)
	return &sdk.Form{
		Title: title, Fields: fields, SubmitTitle: ctx.T("preferences.save"),
		Submit: func(ctx sdk.Context, v sdk.Values) error {
			if err := s.prefs.save(m.ID, v); err != nil {
				return fmt.Errorf("%s: %w", ctx.T("preferences.not_saved"), err)
			}
			if then != nil {
				return then(ctx)
			}
			ctx.Toast(ctx.T("preferences.saved"))
			return nil
		},
	}
}
