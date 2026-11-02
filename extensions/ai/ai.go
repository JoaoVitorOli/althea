// Package ai is the AI extension: providers behind one interface, Quick AI from the root search,
// AI Chat with saved conversations, and AI Commands defined in TOML that run on the selected text
// or the clipboard. Answers come as they are written. Nothing is sent until the user runs something
// (docs/adr/0012-privacy.md), and API keys stay in the system's vault.
package ai

import (
	"embed"
	"errors"
	"io/fs"
	"strings"
	"sync"
	"time"

	"github.com/JoaoVitorOli/althea/sdk"
)

//go:embed locales/*.toml migrations/*.sql commands/*.toml
var files embed.FS

// ID is the extension's ID.
const ID = "althea.ai"

// Platform is what the extension needs from the host.
type Platform struct {
	// CommandsDir is the folder of the user's AI Commands.
	CommandsDir string
}

// Extension is the AI extension.
type Extension struct {
	p Platform
	// Now is the clock.
	Now func() time.Time
}

// New returns the AI extension.
func New(p Platform) *Extension { return &Extension{p: p, Now: time.Now} }

// Manifest implements sdk.Extension.
func (*Extension) Manifest() sdk.Manifest {
	messages, _ := fs.Sub(files, "locales")
	migrations, _ := fs.Sub(files, "migrations")
	return sdk.Manifest{ID: ID, Title: "AI", Icon: sdk.Icon{Glyph: "sparkles"}, Messages: messages, Migrations: migrations}
}

// preset is a service to choose from; a local one asks for no key.
type preset struct {
	id, title, address string
	kind               string // "openai", "anthropic" or "gemini"
	local              bool
}

// presets are the services to choose from, the default first.
var presets = []preset{
	{"openai", "OpenAI", "https://api.openai.com/v1", "openai", false},
	{"anthropic", "Anthropic", "https://api.anthropic.com", "anthropic", false},
	{"gemini", "Google Gemini", "https://generativelanguage.googleapis.com", "gemini", false},
	{"openrouter", "OpenRouter", "https://openrouter.ai/api/v1", "openai", false},
	{"groq", "Groq", "https://api.groq.com/openai/v1", "openai", false},
	{"ollama", "Ollama", "http://localhost:11434/v1", "openai", true},
	{"lmstudio", "LM Studio", "http://localhost:1234/v1", "openai", true},
	{"llamacpp", "llama.cpp", "http://localhost:8080/v1", "openai", true},
	{"custom", "", "", "openai", true},
}

// Preferences implements sdk.Configurable. Only the model is required, which asks for all of them
// the first time: a local server needs no key, and no model's name lasts long enough to default to.
func (*Extension) Preferences(ctx sdk.Context) []sdk.Preference {
	options := make([]sdk.Option, len(presets))
	for i, p := range presets {
		title := p.title
		if title == "" {
			title = ctx.T("prefs.custom")
		}
		options[i] = sdk.Option{Value: p.id, Title: title}
	}
	return []sdk.Preference{
		{Name: "provider", Title: ctx.T("prefs.provider"), Kind: sdk.ChoicePreference, Default: presets[0].id, Options: options},
		{Name: "model", Title: ctx.T("prefs.model"), Description: ctx.T("prefs.model_info"), Kind: sdk.TextPreference, Required: true},
		{Name: "api_key", Title: ctx.T("prefs.api_key"), Description: ctx.T("prefs.api_key_info"), Kind: sdk.SecretPreference},
		{Name: "base_url", Title: ctx.T("prefs.base_url"), Description: ctx.T("prefs.base_url_info"), Kind: sdk.TextPreference},
		{Name: "instructions", Title: ctx.T("prefs.instructions"), Description: ctx.T("prefs.instructions_info"), Kind: sdk.TextPreference},
		{Name: "save_chats", Title: ctx.T("prefs.save_chats"), Description: ctx.T("prefs.save_chats_info"), Kind: sdk.BoolPreference, Default: "true"},
	}
}

// provider builds the provider the user chose, from their preferences.
func provider(ctx sdk.Context) (Provider, error) {
	prefs := ctx.Preferences()
	id := prefs.String("provider")
	chosen := presets[0]
	for _, p := range presets {
		if p.id == id {
			chosen = p
		}
	}
	address := strings.TrimSpace(prefs.String("base_url"))
	if address == "" {
		address = chosen.address
	}
	if address == "" {
		return nil, errors.New(ctx.T("error.no_address"))
	}
	key, err := prefs.Secret("api_key")
	if err != nil {
		ctx.Log().Warn("API key not read", "err", err)
		return nil, errors.New(ctx.T("error.key_unread"))
	}
	if key == "" && !chosen.local {
		return nil, errors.New(ctx.T("error.no_key", "provider", chosen.title))
	}
	switch chosen.kind {
	case "anthropic":
		return Anthropic{BaseURL: address, Key: key}, nil
	case "gemini":
		return Gemini{BaseURL: address, Key: key}, nil
	default:
		return OpenAI{BaseURL: address, Key: key}, nil
	}
}

// answer is an answer being written, read by actions while it comes.
type answer struct {
	mu   sync.Mutex
	text strings.Builder
}

func (a *answer) add(s string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.text.WriteString(s)
	return a.text.String()
}

// set replaces the answer so far.
func (a *answer) set(s string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.text.Reset()
	a.text.WriteString(s)
}

func (a *answer) String() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.text.String()
}

// Complete asks the provider the user chose, with ctx a Context of this extension: other extensions
// get here through the host, as the Translator does to translate with AI. Each call of show gets
// the answer so far.
func (*Extension) Complete(ctx sdk.Context, r Request, show func(string)) (string, error) {
	p, err := provider(ctx)
	if err != nil {
		return "", err
	}
	if r.Model == "" {
		r.Model = strings.TrimSpace(ctx.Preferences().String("model"))
	}
	if r.Model == "" {
		return "", errors.New(ctx.T("error.no_model"))
	}
	if extra := strings.TrimSpace(ctx.Preferences().String("instructions")); extra != "" {
		r.System = strings.TrimSpace(r.System + "\n\n" + extra)
	}
	var a answer
	err = p.Stream(ctx, r, func(delta string) { show(a.add(delta)) })
	if err != nil && a.String() == "" {
		return "", err
	}
	return a.String(), err
}

// Commands implements sdk.Extension.
func (e *Extension) Commands(ctx sdk.Context) []sdk.Command {
	commands := []sdk.Command{
		{
			ID: "chat", Title: ctx.T("chat.title"), Subtitle: ctx.T("chat.subtitle"), Icon: sdk.Icon{Glyph: "sparkles"},
			Keywords: []string{"ai", "ia", "chat", "gpt", "claude", "gemini", "ollama", "conversa", "conversation", "assistant", "assistente"},
			Run: func(ctx sdk.Context) error {
				ctx.Push(e.conversations(ctx))
				return nil
			},
		},
		{
			ID: "commands-folder", Title: ctx.T("commands.folder"), Subtitle: ctx.T("commands.folder_subtitle"), Icon: sdk.Icon{Glyph: "folder"},
			Keywords: strings.Fields(ctx.T("commands.folder_keywords")),
			Run:      e.openCommandsFolder,
		},
	}
	return append(commands, e.aiCommands(ctx)...)
}

// TakeQuery implements sdk.QueryTaker: Tab asks AI.
func (e *Extension) TakeQuery(ctx sdk.Context, query string) *sdk.Action {
	return &sdk.Action{Title: ctx.T("quick.ask"), Run: e.quick(query)}
}

// Fallbacks implements sdk.FallbackSource.
func (e *Extension) Fallbacks(ctx sdk.Context, query string) []sdk.Item {
	return []sdk.Item{{
		ID: "ask", Title: ctx.T("quick.ask_about", "query", query), Icon: sdk.Icon{Glyph: "sparkles"},
		Actions: []sdk.Action{{Title: ctx.T("quick.ask"), Run: e.quick(query)}},
	}}
}

// quick asks query in a new conversation.
func (e *Extension) quick(query string) func(sdk.Context) error {
	return func(ctx sdk.Context) error {
		ctx.Push(e.chatView(ctx, newConversation(), query))
		return nil
	}
}
