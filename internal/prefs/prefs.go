// Package prefs reads extensions' preferences: the values settings.toml gives, checked against
// what each extension declares, and the secrets kept in the system's vault.
package prefs

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/JoaoVitorOli/althea/internal/hotkey"
	"github.com/JoaoVitorOli/althea/sdk"
)

// Values are an extension's preferences by name, written as their kind reads them: "true",
// "0.7", an option's value. Secrets are not among them.
type Values map[string]string

// Problem is a preference set in settings.toml to what it cannot be.
type Problem struct {
	Name    string
	Message string
}

func (p Problem) String() string { return p.Name + ": " + p.Message }

// Resolve reads the values of the declared preferences from an extension's table in
// settings.toml. Each takes its default where the table sets none, or sets one it cannot take,
// which is a problem; secrets are never read from the file.
func Resolve(decl []sdk.Preference, table map[string]any) (Values, []Problem) {
	values := Values{}
	var problems []Problem
	known := map[string]bool{}
	for _, p := range decl {
		known[p.Name] = true
		if p.Kind != sdk.SecretPreference && p.Default != "" {
			values[p.Name] = p.Default
		}
		raw, ok := table[p.Name]
		if !ok {
			continue
		}
		v, msg := read(p, raw)
		if msg != "" {
			problems = append(problems, Problem{Name: p.Name, Message: msg})
			continue
		}
		values[p.Name] = v
	}
	for _, name := range sortedKeys(table) {
		if !known[name] {
			problems = append(problems, Problem{Name: name, Message: "unknown preference"})
		}
	}
	return values, problems
}

// read turns a value of the file into the text of a preference, or says what is wrong.
func read(p sdk.Preference, raw any) (string, string) {
	switch p.Kind {
	case sdk.SecretPreference:
		return "", "secrets are kept in the system's vault, not in this file: set it in Althea"
	case sdk.BoolPreference:
		b, ok := raw.(bool)
		if !ok {
			return "", "must be true or false"
		}
		return strconv.FormatBool(b), ""
	case sdk.NumberPreference:
		switch n := raw.(type) {
		case int64:
			return strconv.FormatInt(n, 10), ""
		case float64:
			return strconv.FormatFloat(n, 'g', -1, 64), ""
		}
		return "", "must be a number"
	}
	s, ok := raw.(string)
	if !ok {
		return "", "must be text in quotes"
	}
	switch p.Kind {
	case sdk.ChoicePreference:
		var values []string
		for _, o := range p.Options {
			if o.Value == s {
				return s, ""
			}
			values = append(values, strconv.Quote(o.Value))
		}
		return "", "must be one of " + strings.Join(values, ", ")
	case sdk.HotkeyPreference:
		if _, err := hotkey.Parse(s); err != nil {
			return "", `must be a combination such as "Ctrl+Shift+V"`
		}
	}
	return s, ""
}

// Check reports what is wrong with a value typed for a preference, as in the settings screen,
// or "" when it is fine. An empty value is fine: it unsets the preference.
func Check(p sdk.Preference, text string) string {
	if text == "" || p.Kind == sdk.SecretPreference {
		return ""
	}
	var raw any = text
	switch p.Kind {
	case sdk.BoolPreference:
		b, err := strconv.ParseBool(text)
		if err != nil {
			return "must be true or false"
		}
		raw = b
	case sdk.NumberPreference:
		n, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return "must be a number"
		}
		raw = n
	}
	_, msg := read(p, raw)
	return msg
}

// Encode turns the text of a preference into the value settings.toml keeps: a bool or a number
// for those kinds, text for the rest.
func Encode(p sdk.Preference, text string) (any, error) {
	switch p.Kind {
	case sdk.BoolPreference:
		return strconv.ParseBool(text)
	case sdk.NumberPreference:
		if n, err := strconv.ParseInt(text, 10, 64); err == nil {
			return n, nil
		}
		return strconv.ParseFloat(text, 64)
	case sdk.SecretPreference:
		return nil, fmt.Errorf("%s: secrets are not written to the settings file", p.Name)
	}
	return text, nil
}

// Missing returns the required preferences that have no value; has reports whether a secret is
// set.
func Missing(decl []sdk.Preference, values Values, has func(name string) bool) []sdk.Preference {
	var out []sdk.Preference
	for _, p := range decl {
		if !p.Required {
			continue
		}
		set := strings.TrimSpace(values[p.Name]) != ""
		if p.Kind == sdk.SecretPreference {
			set = has(p.Name)
		}
		if !set {
			out = append(out, p)
		}
	}
	return out
}

// Preferences are an extension's preferences, as sdk.Preferences.
type Preferences struct {
	Values Values
	// ReadSecret reads a secret by name from the vault; nil reads none.
	ReadSecret func(name string) (string, error)
}

// String returns a preference's value as written.
func (p Preferences) String(name string) string { return p.Values[name] }

// Bool reports whether a preference is "true".
func (p Preferences) Bool(name string) bool { return p.Values[name] == "true" }

// Number reads a preference as a number, 0 when it is not one.
func (p Preferences) Number(name string) float64 {
	n, _ := strconv.ParseFloat(p.Values[name], 64)
	return n
}

// Secret reads a secret preference; "" when it is not set.
func (p Preferences) Secret(name string) (string, error) {
	if p.ReadSecret == nil {
		return "", nil
	}
	return p.ReadSecret(name)
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
