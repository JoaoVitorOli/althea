package sdk

// Configurable is an extension with settings of its own, as an AI provider's model or API key.
// The user sets them in the settings screen or under [extensions."<id>"] in settings.toml;
// secrets go to the system's vault instead. A command of an extension with a required
// preference unset first asks for it.
type Configurable interface {
	// Preferences declares the settings; ctx translates their titles.
	Preferences(ctx Context) []Preference
}

// PreferenceKind is the type of a preference's value.
type PreferenceKind int

// The kinds of preferences.
const (
	TextPreference   PreferenceKind = iota
	NumberPreference                // a number, as "0.7"
	BoolPreference                  // "true" or "false"
	ChoicePreference                // one of Options, by Value
	PathPreference                  // a file or folder
	SecretPreference                // kept in the system's vault, never in the settings file
	HotkeyPreference                // a combination such as "Ctrl+Shift+V"
)

// Preference is a setting of an extension.
type Preference struct {
	// Name is the setting's key in settings.toml: lowercase letters, digits and underscores.
	Name        string
	Title       string
	Description string
	Kind        PreferenceKind
	// Default is the value while the user sets none, written as the kind reads it.
	Default  string
	Options  []Option
	Required bool
}

// Preferences are the values of an extension's preferences, the defaults where the user set
// none or set one that is not valid.
type Preferences interface {
	String(name string) string
	Bool(name string) bool
	Number(name string) float64
	// Secret reads a secret preference from the system's vault; "" when it is not set.
	Secret(name string) (string, error)
}
