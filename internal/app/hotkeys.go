package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"

	"github.com/JoaoVitorOli/althea/internal/config"
	"github.com/JoaoVitorOli/althea/internal/hotkey"
	"github.com/JoaoVitorOli/althea/internal/registry"
	"github.com/JoaoVitorOli/althea/sdk"
)

// wantedHotkeys returns the combination of each command with a hotkey, by the command's key. A
// combination goes to one: the launcher's own, else the first command by key, the same one each
// time; the others are logged.
func wantedHotkeys(s config.Hotkey, log *slog.Logger) map[string]string {
	toggle, _ := hotkey.Parse(s.Toggle)
	want := map[string]string{}
	taken := map[string]string{}
	for _, command := range slices.Sorted(maps.Keys(s.Commands)) {
		b, err := hotkey.Parse(s.Commands[command])
		if err != nil || b.WinKey {
			continue // the settings report these
		}
		if b.Accelerator == toggle.Accelerator {
			log.Warn("hotkey conflict", "hotkey", b.Accelerator, "command", command, "taken_by", "launcher")
			continue
		}
		if other, ok := taken[b.Accelerator]; ok {
			log.Warn("hotkey conflict", "hotkey", b.Accelerator, "command", command, "taken_by", other)
			continue
		}
		taken[b.Accelerator] = command
		want[command] = b.Accelerator
	}
	return want
}

// commandHotkeys keeps the commands' hotkeys bound. It is used on the main thread, and bind and
// the unbinds it returns must not wait for it, as bindCommandHotkey does not.
type commandHotkeys struct {
	bind func(shortcut string, run func()) (unbind func())
	run  func(command string)

	unbinds []func()
}

// release lets go of every hotkey.
func (h *commandHotkeys) release() {
	for _, unbind := range h.unbinds {
		unbind()
	}
	h.unbinds = nil
}

// sync binds want, a combination by command, in place of what was bound. All are bound anew, so
// that one the system refused gets another chance as the settings change.
func (h *commandHotkeys) sync(want map[string]string) {
	h.release()
	for _, command := range slices.Sorted(maps.Keys(want)) {
		h.unbinds = append(h.unbinds, h.bind(want[command], func() { h.run(command) }))
	}
}

// commandRunner runs a command from its hotkey, off the main thread, wherever the user is.
type commandRunner struct {
	// note remembers the window the user is in, for what the command does to it.
	note func()
	find func(command string) (registry.Result, bool)
	// refresh collects the commands again, as one the last refresh did not find may exist now.
	refresh func()
	// context is the context of what the command does: its views show the launcher.
	context func(ext string) sdk.Context
	perform func(res registry.Result, act sdk.Action)
	log     *slog.Logger
}

func (r commandRunner) run(command string) {
	r.note()
	res, ok := r.find(command)
	if !ok {
		r.refresh()
		res, ok = r.find(command)
	}
	if !ok || len(res.Item.Actions) == 0 {
		r.log.Warn("hotkey of an unknown command", "command", command)
		return
	}
	act := res.Item.Actions[0]
	if act.Confirm == "" {
		r.perform(res, act)
		return
	}
	// A hotkey is easily pressed by mistake, and what asks for confirmation cannot be undone: it
	// is asked in the launcher, as from the search.
	item := res.Item
	item.Actions = []sdk.Action{{
		Title: act.Title, Icon: act.Icon, Confirm: act.Confirm,
		Run: func(ctx sdk.Context) error {
			ctx.Pop()
			r.perform(res, act)
			return nil
		},
	}}
	r.context(res.Extension).Push(&sdk.List{Title: item.Title, Items: []sdk.Item{item}})
}

// runCommand runs a command whose hotkey was pressed.
func (a *althea) runCommand(command string) {
	if s := a.search; s != nil {
		s.commandRunner().run(command)
	}
}

func (s *searcher) commandRunner() commandRunner {
	return commandRunner{
		note:    s.a.noteTarget,
		find:    s.registry.Command,
		refresh: s.refreshNow,
		context: s.a.hotkeyContext,
		perform: func(res registry.Result, act sdk.Action) {
			if s.needsPreferences(res.Extension) {
				s.a.reveal() // the form asking for them shows in the launcher
			}
			s.perform(s.a.hotkeyContext(res.Extension), res, act)
		},
		log: s.a.log,
	}
}

// noteTarget remembers the window in front for Paste, as the launcher does when it shows: with
// the launcher hidden, that is the window a command pastes into.
func (a *althea) noteTarget() {
	if p := a.services.Paster; p != nil && !a.launcher.Window().IsVisible() {
		a.pasteTarget.Store(p.Foreground())
	}
}

// reveal shows the launcher, unless it shows. It runs before what is queued after it.
func (a *althea) reveal() {
	a.launcher.Window().Update(func() {
		if !a.launcher.Window().IsVisible() {
			a.launcher.Show()
		}
	})
}

// hotkeyContext is the context of a command run from its hotkey. Nothing shows the launcher for
// it, so the views it pushes must.
func (a *althea) hotkeyContext(ext string) sdk.Context {
	return revealing{a.extensionContext(context.Background(), ext), a.reveal}
}

// revealing is a context whose views show the launcher.
type revealing struct {
	sdk.Context
	reveal func()
}

func (r revealing) Push(v sdk.View) {
	r.reveal()
	r.Context.Push(v)
}

// titles returns the extensions' titles, by ID.
func titles(exts []sdk.Extension) map[string]string {
	out := make(map[string]string, len(exts))
	for _, e := range exts {
		m := e.Manifest()
		out[m.ID] = m.Title
	}
	return out
}

// savedHotkeys reads the hotkeys from the file, which holds a change before the watcher reports
// it. A file that cannot be read has none to show: the watcher logs what is wrong with it.
func (s *settingsExtension) savedHotkeys() config.Hotkey {
	settings, _, err := config.Load(s.prefs.path)
	if err != nil {
		return config.Hotkey{}
	}
	return settings.Hotkey
}

// hotkeyList lists every command with its hotkey, by extension.
func (s *settingsExtension) hotkeyList(ctx sdk.Context) *sdk.List {
	saved := s.savedHotkeys()
	commands := s.commands()
	slices.SortFunc(commands, func(a, b registry.Result) int {
		return cmp.Or(cmp.Compare(s.titles[a.Extension], s.titles[b.Extension]), cmp.Compare(a.Item.Title, b.Item.Title))
	})
	items := make([]sdk.Item, 0, len(commands))
	for _, res := range commands {
		item := sdk.Item{
			ID: res.Key, Title: res.Item.Title, Subtitle: res.Item.Subtitle, Icon: res.Item.Icon,
			Keywords: res.Item.Keywords, Section: s.titles[res.Extension], Accessory: saved.Commands[res.Key],
			Actions: []sdk.Action{{Title: ctx.T("hotkeys.set"), Run: func(ctx sdk.Context) error {
				ctx.Push(s.hotkeyForm(ctx, res))
				return nil
			}}},
		}
		if item.Accessory != "" {
			item.Actions = append(item.Actions, sdk.Action{Title: ctx.T("hotkeys.remove"), Run: func(ctx sdk.Context) error {
				if err := s.saveHotkey(res.Key, ""); err != nil {
					return fmt.Errorf("%s: %w", ctx.T("hotkeys.not_saved"), err)
				}
				ctx.Pop()
				ctx.Push(s.hotkeyList(ctx))
				return nil
			}})
		}
		items = append(items, item)
	}
	return &sdk.List{Title: ctx.T("hotkeys.title"), Placeholder: ctx.T("hotkeys.search"), Items: items}
}

// hotkeyForm asks for the hotkey of a command, which an empty field removes. It takes the place
// of the list under it, so that the list shows the new hotkey.
func (s *settingsExtension) hotkeyForm(ctx sdk.Context, res registry.Result) *sdk.Form {
	saved := s.savedHotkeys()
	return &sdk.Form{
		Title: ctx.T("hotkeys.form_title", "command", res.Item.Title), SubmitTitle: ctx.T("preferences.save"),
		Fields: []sdk.Field{{
			ID: "hotkey", Title: ctx.T("hotkeys.field"), Info: ctx.T("hotkeys.info"), Placeholder: "Ctrl+Alt+K",
			Default:  saved.Commands[res.Key],
			Validate: func(v string) string { return s.hotkeyProblem(ctx, saved, res.Key, v) },
		}},
		Submit: func(ctx sdk.Context, v sdk.Values) error {
			if err := s.saveHotkey(res.Key, v["hotkey"]); err != nil {
				return fmt.Errorf("%s: %w", ctx.T("hotkeys.not_saved"), err)
			}
			ctx.Pop() // the form
			ctx.Pop() // the list, as it was
			ctx.Push(s.hotkeyList(ctx))
			return nil
		},
	}
}

// hotkeyProblem says what is wrong with a hotkey typed for a command, or "" when it can be had.
func (s *settingsExtension) hotkeyProblem(ctx sdk.Context, saved config.Hotkey, command, text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	b, err := hotkey.Parse(text)
	if err != nil || b.WinKey {
		return ctx.T("preferences.invalid_hotkey")
	}
	if toggle, _ := hotkey.Parse(saved.Toggle); toggle.Accelerator == b.Accelerator {
		return ctx.T("hotkeys.taken_launcher")
	}
	for other, shortcut := range saved.Commands {
		if o, _ := hotkey.Parse(shortcut); other != command && o.Accelerator == b.Accelerator {
			return ctx.T("hotkeys.taken", "command", s.commandTitle(other))
		}
	}
	return ""
}

// commandTitle names a command by its key, which is all that is known of one no longer offered.
func (s *settingsExtension) commandTitle(command string) string {
	for _, res := range s.commands() {
		if res.Key == command {
			return res.Item.Title
		}
	}
	return command
}

// saveHotkey writes the hotkey of a command to settings.toml, or removes it when empty; the
// watcher then binds it.
func (s *settingsExtension) saveHotkey(command, text string) error {
	key := []string{"hotkey", "commands", command}
	if strings.TrimSpace(text) == "" {
		return config.Unset(s.prefs.path, key)
	}
	b, err := hotkey.Parse(text)
	if err == nil && b.WinKey {
		err = errors.New("the Win key alone is the launcher's")
	}
	if err != nil {
		return err
	}
	return config.SetPath(s.prefs.path, key, b.Accelerator)
}
