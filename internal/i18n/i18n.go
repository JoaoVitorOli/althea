// Package i18n translates Althea's user-facing text. Messages live in TOML catalogs, one file
// per language (en.toml, pt-BR.toml), with {name} placeholders. English is the fallback.
package i18n

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"sync"

	"github.com/pelletier/go-toml/v2"
)

// Languages are the languages Althea ships, English first.
var Languages = []string{"en", "pt-BR"}

const fallback = "en"

// Catalog holds the messages of every language and translates into one of them. It is safe
// for concurrent use.
type Catalog struct {
	lang string

	mu       sync.RWMutex
	messages map[string]map[string]string // language → key → message
}

// New returns an empty catalog translating into lang, one of Languages.
func New(lang string) *Catalog {
	return &Catalog{lang: lang, messages: map[string]map[string]string{}}
}

// Add loads the catalogs in fsys, one <language>.toml per language. Keys are prefixed with
// namespace and a dot, unless namespace is empty, as for the host's own messages.
func (c *Catalog) Add(namespace string, fsys fs.FS) error {
	files, err := fs.Glob(fsys, "*.toml")
	if err != nil {
		return err
	}
	loaded := map[string]map[string]string{}
	for _, file := range files {
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			return fmt.Errorf("i18n %s: %w", file, err)
		}
		var tree map[string]any
		if err := toml.Unmarshal(data, &tree); err != nil {
			return fmt.Errorf("i18n %s: %w", file, err)
		}
		msgs := map[string]string{}
		if err := flatten(namespace, tree, msgs); err != nil {
			return fmt.Errorf("i18n %s: %w", file, err)
		}
		loaded[strings.TrimSuffix(path.Base(file), ".toml")] = msgs
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	for lang, msgs := range loaded {
		if c.messages[lang] == nil {
			c.messages[lang] = map[string]string{}
		}
		for k, v := range msgs {
			c.messages[lang][k] = v
		}
	}
	return nil
}

func flatten(prefix string, tree map[string]any, into map[string]string) error {
	for k, v := range tree {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		switch v := v.(type) {
		case string:
			into[key] = v
		case map[string]any:
			if err := flatten(key, v, into); err != nil {
				return err
			}
		default:
			return errors.New(key + ": messages must be strings")
		}
	}
	return nil
}

// T returns the message for key in the catalog's language, falling back to English and then
// to the key itself, with its placeholders filled from args: pairs of a name and a value, as
// T("search.results", "count", 3, "query", q).
func (c *Catalog) T(key string, args ...any) string {
	c.mu.RLock()
	msg, ok := c.messages[c.lang][key]
	if !ok {
		msg, ok = c.messages[fallback][key]
	}
	c.mu.RUnlock()
	if !ok {
		return key
	}
	for i := 0; i+1 < len(args); i += 2 {
		msg = strings.ReplaceAll(msg, "{"+fmt.Sprint(args[i])+"}", fmt.Sprint(args[i+1]))
	}
	return msg
}

// Match returns the supported language closest to a system locale such as "pt_BR.UTF-8" or
// "en-US": the same language in another region still matches.
func Match(locale string) string {
	tag := strings.ReplaceAll(strings.SplitN(locale, ".", 2)[0], "_", "-")
	base, _, _ := strings.Cut(tag, "-")
	for _, lang := range Languages {
		if strings.EqualFold(lang, tag) {
			return lang
		}
	}
	for _, lang := range Languages {
		if langBase, _, _ := strings.Cut(lang, "-"); strings.EqualFold(langBase, base) {
			return lang
		}
	}
	return fallback
}
