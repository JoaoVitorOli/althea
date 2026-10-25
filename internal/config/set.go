package config

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/JoaoVitorOli/althea/internal/tomlpos"
)

// Set writes one setting into the settings file at path, changing only its value: comments,
// order and layout stay as the user left them. A key the file lacks is added to its table, and
// a missing table at the end. A missing file is first written with the defaults.
func Set(path, key string, value any) error {
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

	var out []byte
	table, name := "", key
	if i := strings.LastIndex(key, "."); i >= 0 {
		table, name = key[:i], key[i+1:]
	}
	v, isSet := doc.Values[key]
	at, hasTable := doc.TableEnds[table]
	switch {
	case isSet:
		out = splice(data, v.Start, v.End, encoded)
	case hasTable:
		out = splice(data, at, at, name+" = "+encoded+"\n")
	default:
		out = bytes.TrimRight(data, "\n")
		out = append(out, "\n\n["+table+"]\n"+name+" = "+encoded+"\n"...)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return fmt.Errorf("set %s: %w", key, err)
	}
	return nil
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
