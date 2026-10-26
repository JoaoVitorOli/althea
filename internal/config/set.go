package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/JoaoVitorOli/althea/internal/tomlpos"
)

// Set writes one setting into the settings file at path, changing only its value: comments,
// order and layout stay as the user left them. A key the file lacks is added to its table, and
// a missing table at the end. A missing file is first written with the defaults.
//
// Key is dotted, as "general.language"; SetPath takes keys whose parts have dots of their own.
func Set(path, key string, value any) error {
	return SetPath(path, strings.Split(key, "."), value)
}

// SetPath is Set of a key given by its parts, as {"extensions", "althea.ai", "model"}.
func SetPath(path string, parts []string, value any) error {
	if len(parts) == 0 {
		return errors.New("set: no key")
	}
	key := strings.Join(parts, ".")
	if err := WriteDefault(path); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("set %s: %w", key, err)
	}
	encoded, err := encode(value)
	if err != nil {
		return fmt.Errorf("set %s: %w", key, err)
	}
	doc, err := tomlpos.Parse(data)
	if err != nil {
		return fmt.Errorf("set %s: the settings file does not parse: %w", key, err)
	}

	// The document's layout joins the parts of keys with dots, whatever their quotes, as key.
	var out []byte
	tableParts, name := parts[:len(parts)-1], parts[len(parts)-1]
	table := strings.Join(tableParts, ".")
	v, isSet := doc.Values[key]
	at, hasTable := doc.TableEnds[table]
	switch {
	case isSet:
		out = splice(data, v.Start, v.End, encoded)
	case hasTable:
		out = splice(data, at, at, bareOrQuoted(name)+" = "+encoded+"\n")
	default:
		header := make([]string, len(tableParts))
		for i, p := range tableParts {
			header[i] = bareOrQuoted(p)
		}
		out = bytes.TrimRight(data, "\n")
		out = append(out, "\n\n["+strings.Join(header, ".")+"]\n"+bareOrQuoted(name)+" = "+encoded+"\n"...)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return fmt.Errorf("set %s: %w", key, err)
	}
	return nil
}

// Unset removes a setting from the settings file at path, the whole line of it, so that it
// takes its default again; the rest of the file stays as it is. Removing a setting the file
// lacks is not an error.
func Unset(path string, parts []string) error {
	key := strings.Join(parts, ".")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("unset %s: %w", key, err)
	}
	doc, err := tomlpos.Parse(data)
	if err != nil {
		return fmt.Errorf("unset %s: the settings file does not parse: %w", key, err)
	}
	v, ok := doc.Values[key]
	if !ok {
		return nil
	}
	start := bytes.LastIndexByte(data[:v.Start], '\n') + 1
	end := len(data)
	if i := bytes.IndexByte(data[v.End:], '\n'); i >= 0 {
		end = v.End + i + 1
	}
	if err := os.WriteFile(path, splice(data, start, end, ""), 0o600); err != nil {
		return fmt.Errorf("unset %s: %w", key, err)
	}
	return nil
}

// bareOrQuoted writes a part of a key bare where TOML allows it, else quoted, as "althea.ai".
func bareOrQuoted(part string) string {
	bare := part != ""
	for _, r := range part {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			bare = false
			break
		}
	}
	if bare {
		return part
	}
	s, _ := encode(part)
	return s
}

func splice(data []byte, start, end int, s string) []byte {
	out := make([]byte, 0, len(data)+len(s))
	out = append(out, data[:start]...)
	out = append(out, s...)
	return append(out, data[end:]...)
}

// encode writes a value as TOML does, strings in double quotes as the settings file has them.
func encode(v any) (string, error) {
	if s, ok := v.(string); ok {
		var b strings.Builder
		b.WriteByte('"')
		for _, r := range s {
			switch {
			case r == '"' || r == '\\':
				b.WriteByte('\\')
				b.WriteRune(r)
			case r < 0x20 || r == 0x7f:
				fmt.Fprintf(&b, `\u%04X`, r)
			default:
				b.WriteRune(r)
			}
		}
		b.WriteByte('"')
		return b.String(), nil
	}
	b, err := toml.Marshal(map[string]any{"v": v})
	if err != nil {
		return "", err
	}
	_, s, ok := strings.Cut(strings.TrimSpace(string(b)), "=")
	if !ok {
		return "", fmt.Errorf("cannot encode %v", v)
	}
	return strings.TrimSpace(s), nil
}
