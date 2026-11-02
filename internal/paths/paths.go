// Package paths says where Althea keeps its files on each system.
package paths

import (
	"errors"
	"path"
	"path/filepath"
	"strings"
)

// Paths are the directories and files Althea uses.
type Paths struct {
	Config   string // what the user edits and syncs
	Settings string // settings.toml
	Themes   string // user themes, *.toml
	Scripts  string // Script Commands
	Commands string // AI Commands
	Data     string // what Althea keeps for itself
	Database string // althea.db
	Cache    string // what can be rebuilt, such as icons
	Logs     string
}

// Resolve returns the paths for goos, reading the environment with getenv. A non-empty
// portable directory, where the executable sits beside an althea.portable file, keeps
// everything in its data folder instead.
func Resolve(goos string, getenv func(string) string, portable string) (Paths, error) {
	if portable != "" {
		data := filepath.Join(portable, "data")
		return layout(data, data, filepath.Join(data, "cache"), filepath.Join(data, "logs")), nil
	}
	if goos == "windows" {
		roaming, local := getenv("APPDATA"), getenv("LOCALAPPDATA")
		if roaming == "" {
			return Paths{}, errors.New("paths: APPDATA is not set")
		}
		if local == "" {
			local = roaming
		}
		data := filepath.Join(local, "Althea")
		return layout(filepath.Join(roaming, "Althea"), data,
			filepath.Join(data, "cache"), filepath.Join(data, "logs")), nil
	}

	home := getenv("HOME")
	if home == "" {
		return Paths{}, errors.New("paths: HOME is not set")
	}
	// Unix paths, whatever system resolves them, so tests of this branch run on Windows too.
	xdg := func(name, fallback string) string {
		if dir := getenv(name); strings.HasPrefix(dir, "/") {
			return path.Join(dir, "althea")
		}
		return path.Join(home, fallback, "althea")
	}
	return unixLayout(
		xdg("XDG_CONFIG_HOME", ".config"),
		xdg("XDG_DATA_HOME", ".local/share"),
		xdg("XDG_CACHE_HOME", ".cache"),
		path.Join(xdg("XDG_STATE_HOME", ".local/state"), "logs"),
	), nil
}

func layout(config, data, cache, logs string) Paths {
	return layoutWith(filepath.Join, config, data, cache, logs)
}

func unixLayout(config, data, cache, logs string) Paths {
	return layoutWith(path.Join, config, data, cache, logs)
}

func layoutWith(join func(...string) string, config, data, cache, logs string) Paths {
	return Paths{
		Config:   config,
		Settings: join(config, "settings.toml"),
		Themes:   join(config, "themes"),
		Scripts:  join(config, "scripts"),
		Commands: join(config, "ai-commands"),
		Data:     data,
		Database: join(data, "althea.db"),
		Cache:    cache,
		Logs:     logs,
	}
}
