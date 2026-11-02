package ai_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/JoaoVitorOli/althea/extensions/ai"
	"github.com/JoaoVitorOli/althea/sdk"
	"github.com/JoaoVitorOli/althea/sdk/sdktest"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

var (
	_ sdk.Configurable   = (*ai.Extension)(nil)
	_ sdk.QueryTaker     = (*ai.Extension)(nil)
	_ sdk.FallbackSource = (*ai.Extension)(nil)
)

// seen is a request a server heard.
type seen struct {
	path   string
	header http.Header
	body   map[string]any
}

// server answers every request with lines, recording what it heard.
func server(t *testing.T, status int, lines ...string) (*httptest.Server, *[]seen) {
	t.Helper()
	var heard []seen
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		s := seen{path: r.URL.Path + "?" + r.URL.RawQuery, header: r.Header}
		_ = json.Unmarshal(data, &s.body)
		heard = append(heard, s)
		w.WriteHeader(status)
		for _, line := range lines {
			_, _ = io.WriteString(w, line+"\n\n")
			w.(http.Flusher).Flush()
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &heard
}

func context(t *testing.T, e *ai.Extension, provider, address string) *sdktest.Context {
	t.Helper()
	ctx := sdktest.New(t, e, "en")
	ctx.Prefs["provider"] = provider
	ctx.Prefs["model"] = "test-model"
	ctx.Prefs["base_url"] = address
	ctx.Secrets["api_key"] = "secret-key"
	return ctx
}

func extension(t *testing.T) *ai.Extension {
	t.Helper()
	e := ai.New(ai.Platform{CommandsDir: filepath.Join(t.TempDir(), "ai-commands")})
	e.Now = func() time.Time { return time.UnixMilli(1000) }
	return e
}

func complete(t *testing.T, e *ai.Extension, ctx sdk.Context, r ai.Request) ([]string, error) {
	t.Helper()
	var shown []string
	_, err := e.Complete(ctx, r, func(s string) { shown = append(shown, s) })
	return shown, err
}

var question = ai.Request{System: "Be brief.", Messages: []ai.Message{
	{Text: "hi"}, {Assistant: true, Text: "hello"}, {Text: "why"},
}}

func TestOpenAICompatibleStreamsAndSendsWhatItShould(t *testing.T) {
	srv, heard := server(t, 200,
		`data: {"choices":[{"delta":{"role":"assistant"}}]}`,
		`data: {"choices":[{"delta":{"content":"Be"}}]}`,
		`data: {"choices":[{"delta":{"content":"cause"}}]}`,
		`data: [DONE]`,
		`data: {"choices":[{"delta":{"content":"after the end"}}]}`,
	)
	e := extension(t)
	shown, err := complete(t, e, context(t, e, "openrouter", srv.URL+"/v1"), question)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(shown, []string{"Be", "Because"}) {
		t.Errorf("shown %q", shown)
	}
	h := (*heard)[0]
	if h.path != "/v1/chat/completions?" || h.header.Get("Authorization") != "Bearer secret-key" {
		t.Errorf("path %q, header %v", h.path, h.header)
	}
	if h.body["model"] != "test-model" || h.body["stream"] != true {
		t.Errorf("body %v", h.body)
	}
	messages, _ := h.body["messages"].([]any)
	if len(messages) != 4 || messages[0].(map[string]any)["role"] != "system" || messages[2].(map[string]any)["role"] != "assistant" {
		t.Errorf("messages %v", messages)
	}
}

func TestALocalServerNeedsNoKey(t *testing.T) {
	srv, heard := server(t, 200, `data: {"choices":[{"delta":{"content":"ok"}}]}`)
	e := extension(t)
	ctx := context(t, e, "lmstudio", srv.URL)
	delete(ctx.Secrets, "api_key")
	if _, err := complete(t, e, ctx, question); err != nil {
		t.Fatal(err)
	}
	if got := (*heard)[0].header.Get("Authorization"); got != "" {
		t.Errorf("Authorization %q", got)
	}
}

func TestAnthropicStreamsAndSendsWhatItShould(t *testing.T) {
	srv, heard := server(t, 200,
		`event: message_start`+"\n"+`data: {"type":"message_start"}`,
		`data: {"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"hm"}}`,
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hel"}}`,
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"lo"}}`,
		`data: {"type":"message_stop"}`,
	)
	e := extension(t)
	shown, err := complete(t, e, context(t, e, "anthropic", srv.URL), question)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(shown, []string{"Hel", "Hello"}) {
		t.Errorf("shown %q", shown)
	}
	h := (*heard)[0]
	if h.path != "/v1/messages?" || h.header.Get("X-Api-Key") != "secret-key" || h.header.Get("Anthropic-Version") == "" {
		t.Errorf("path %q, header %v", h.path, h.header)
	}
	if h.body["system"] != "Be brief." || h.body["max_tokens"] == nil {
		t.Errorf("body %v", h.body)
	}
	if messages, _ := h.body["messages"].([]any); len(messages) != 3 {
		t.Errorf("the system prompt belongs apart from the messages: %v", messages)
	}
}

func TestAnthropicErrorsInTheStreamReachTheUser(t *testing.T) {
	srv, _ := server(t, 200,
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hel"}}`,
		`data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`,
	)
	e := extension(t)
	written, err := e.Complete(context(t, e, "anthropic", srv.URL), question, func(string) {})
	if err == nil || err.Error() != "Overloaded" || written != "Hel" {
		t.Errorf("written %q, err %v", written, err)
	}
}

func TestGeminiStreamsAndKeepsTheKeyOutOfTheAddress(t *testing.T) {
	srv, heard := server(t, 200,
		`data: {"candidates":[{"content":{"parts":[{"text":"Ol"}],"role":"model"}}]}`,
		`data: {"candidates":[{"content":{"parts":[{"text":"á"}],"role":"model"}}]}`,
	)
	e := extension(t)
	shown, err := complete(t, e, context(t, e, "gemini", srv.URL), question)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(shown, []string{"Ol", "Olá"}) {
		t.Errorf("shown %q", shown)
	}
	h := (*heard)[0]
	if h.path != "/v1beta/models/test-model:streamGenerateContent?alt=sse" || h.header.Get("X-Goog-Api-Key") != "secret-key" {
		t.Errorf("path %q, header %v", h.path, h.header)
	}
	contents, _ := h.body["contents"].([]any)
	if len(contents) != 3 || contents[1].(map[string]any)["role"] != "model" || h.body["systemInstruction"] == nil {
		t.Errorf("body %v", h.body)
	}
}

func TestGeminiSaysWhenItWillNotAnswer(t *testing.T) {
	srv, _ := server(t, 200, `data: {"promptFeedback":{"blockReason":"SAFETY"}}`)
	e := extension(t)
	if _, err := complete(t, e, context(t, e, "gemini", srv.URL), question); err == nil || !strings.Contains(err.Error(), "SAFETY") {
		t.Errorf("err %v", err)
	}
}

func TestRefusalsAreForTheUser(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   string
	}{
		{401, `{"error":{"message":"Incorrect API key"}}`, "refused the API key (401): Incorrect API key"},
		{404, `{"error":"model not found"}`, "does not know that model or address (404): model not found"},
		{429, `{"message":"slow down"}`, "asks to slow down (429): slow down"},
		{500, `oops`, "answered with an error (500)"},
	}
	for _, c := range cases {
		srv, _ := server(t, c.status, c.body)
		e := extension(t)
		_, err := complete(t, e, context(t, e, "openai", srv.URL), question)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("status %d: err %v, want %q", c.status, err, c.want)
		}
		if err != nil && strings.Contains(err.Error(), "secret-key") {
			t.Errorf("the error holds the key: %v", err)
		}
	}
}

func TestAServerThatIsNotThereIsSaidPlainly(t *testing.T) {
	srv, _ := server(t, 200)
	address := srv.URL
	srv.Close()
	e := extension(t)
	_, err := complete(t, e, context(t, e, "ollama", address), question)
	if err == nil || !strings.Contains(err.Error(), "is it running") {
		t.Errorf("err %v", err)
	}
}

func TestAHostedProviderWithoutAKeyAsksForOneWithoutSendingAnything(t *testing.T) {
	e := extension(t)
	ctx := context(t, e, "anthropic", "")
	delete(ctx.Secrets, "api_key")
	_, err := complete(t, e, ctx, question)
	if err == nil || !strings.Contains(err.Error(), "Anthropic needs an API key") {
		t.Errorf("err %v", err)
	}
	if len(ctx.Requests) != 0 {
		t.Errorf("requests %v", ctx.Requests)
	}
}

func TestTheModelIsRequiredAndNothingLeavesAsItStarts(t *testing.T) {
	e := extension(t)
	ctx := sdktest.New(t, e, "pt-BR")
	var required []string
	for _, p := range e.Preferences(ctx) {
		if p.Required {
			required = append(required, p.Name)
		}
	}
	if !slices.Equal(required, []string{"model"}) {
		t.Errorf("required %v", required)
	}
	_ = e.Commands(ctx)
	_ = e.Fallbacks(ctx, "oi")
	if a := e.TakeQuery(ctx, "oi"); a == nil || a.Title != "Perguntar à IA" {
		t.Errorf("take %+v", a)
	}
	if len(ctx.Requests) != 0 || ctx.Selections != 0 {
		t.Errorf("requests %v, selections %d", ctx.Requests, ctx.Selections)
	}
}

func openAI(t *testing.T, answer string) (*httptest.Server, *[]seen) {
	t.Helper()
	data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": answer}}}})
	return server(t, 200, "data: "+string(data), "data: [DONE]")
}

func TestQuickAISavesTheConversationToGoBackTo(t *testing.T) {
	srv, _ := openAI(t, "Because of *Rayleigh*")
	e := extension(t)
	ctx := context(t, e, "openai", srv.URL)
	if err := e.TakeQuery(ctx, "why is the sky blue").Run(ctx); err != nil {
		t.Fatal(err)
	}
	chat := ctx.Pushed[0].(*sdk.Chat)
	if chat.First != "why is the sky blue" || chat.Title != "why is the sky blue" {
		t.Fatalf("chat %+v", chat)
	}
	var shown string
	if err := chat.Send(ctx, chat.First, func(s string) { shown = s }); err != nil {
		t.Fatal(err)
	}
	if shown != "Because of *Rayleigh*" {
		t.Errorf("shown %q", shown)
	}

	// The answer can be copied, and the conversation found again in AI Chat.
	if err := chat.Actions[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ctx.Copied, []string{"Because of *Rayleigh*"}) {
		t.Errorf("copied %q", ctx.Copied)
	}
	if err := commandNamed(t, e, ctx, "chat").Run(ctx); err != nil {
		t.Fatal(err)
	}
	items, err := ctx.Pushed[1].(*sdk.List).Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "new" || items[1].Title != "why is the sky blue" {
		t.Fatalf("items %+v", items)
	}

	// Opening it brings back what was said, and asking more sends it all.
	if err := items[1].Actions[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	reopened := ctx.Pushed[2].(*sdk.Chat)
	want := []sdk.ChatMessage{{Mine: true, Text: "why is the sky blue"}, {Text: "Because of *Rayleigh*"}}
	if !slices.Equal(reopened.Messages, want) || reopened.First != "" {
		t.Errorf("messages %+v", reopened.Messages)
	}
}

func TestAFollowUpSendsTheWholeConversation(t *testing.T) {
	srv, heard := openAI(t, "Sure")
	e := extension(t)
	ctx := context(t, e, "openai", srv.URL)
	if err := e.TakeQuery(ctx, "first").Run(ctx); err != nil {
		t.Fatal(err)
	}
	chat := ctx.Pushed[0].(*sdk.Chat)
	for _, text := range []string{"first", "second"} {
		if err := chat.Send(ctx, text, func(string) {}); err != nil {
			t.Fatal(err)
		}
	}
	if messages, _ := (*heard)[1].body["messages"].([]any); len(messages) != 3 {
		t.Errorf("messages %v", messages)
	}
}

func TestConversationsAreNotSavedWhenTurnedOff(t *testing.T) {
	srv, _ := openAI(t, "Sure")
	e := extension(t)
	ctx := context(t, e, "openai", srv.URL)
	ctx.Prefs["save_chats"] = "false"
	if err := e.TakeQuery(ctx, "private").Run(ctx); err != nil {
		t.Fatal(err)
	}
	if err := ctx.Pushed[0].(*sdk.Chat).Send(ctx, "private", func(string) {}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := ctx.Database().QueryRow(ctx, `SELECT count(*) FROM ai_messages`).Scan(&n); err != nil || n != 0 {
		t.Errorf("%d messages saved, err %v", n, err)
	}
}

func TestDeletingAConversationRemovesItsMessages(t *testing.T) {
	srv, _ := openAI(t, "Sure")
	e := extension(t)
	ctx := context(t, e, "openai", srv.URL)
	if err := e.TakeQuery(ctx, "gone").Run(ctx); err != nil {
		t.Fatal(err)
	}
	if err := ctx.Pushed[0].(*sdk.Chat).Send(ctx, "gone", func(string) {}); err != nil {
		t.Fatal(err)
	}
	if err := commandNamed(t, e, ctx, "chat").Run(ctx); err != nil {
		t.Fatal(err)
	}
	items, err := ctx.Pushed[1].(*sdk.List).Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := items[1].Actions[1].Run(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := ctx.Database().QueryRow(ctx, `SELECT count(*) FROM ai_messages`).Scan(&n); err != nil || n != 0 {
		t.Errorf("%d messages left, err %v", n, err)
	}
}

func commandNamed(t *testing.T, e *ai.Extension, ctx sdk.Context, id string) sdk.Command {
	t.Helper()
	for _, c := range e.Commands(ctx) {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no command %q", id)
	return sdk.Command{}
}

func TestDefaultCommandsAreTranslated(t *testing.T) {
	e := extension(t)
	if c := commandNamed(t, e, sdktest.New(t, e, "pt-BR"), "command/fix-grammar"); c.Title != "Corrigir gramática" {
		t.Errorf("title %q", c.Title)
	}
	if c := commandNamed(t, e, sdktest.New(t, e, "en"), "command/fix-grammar"); c.Title != "Fix Grammar" {
		t.Errorf("title %q", c.Title)
	}
}

func TestACommandRunsOnTheSelectionAndWritesItsAnswer(t *testing.T) {
	srv, heard := openAI(t, "A short summary")
	e := extension(t)
	ctx := context(t, e, "openai", srv.URL)
	ctx.Selection = "a very long text"
	ctx.Clipboard = "the clipboard"
	if err := commandNamed(t, e, ctx, "command/summarize").Run(ctx); err != nil {
		t.Fatal(err)
	}
	d := ctx.Pushed[0].(*sdk.Detail)
	if len(ctx.Requests) != 0 {
		t.Error("asked before the view opened")
	}
	var shown string
	if err := d.Stream(ctx, func(s string) { shown = s }); err != nil {
		t.Fatal(err)
	}
	if shown != "A short summary" {
		t.Errorf("shown %q", shown)
	}
	messages, _ := (*heard)[0].body["messages"].([]any)
	text, _ := messages[len(messages)-1].(map[string]any)["content"].(string)
	if !strings.Contains(text, "a very long text") || strings.Contains(text, "{text}") || strings.Contains(text, "clipboard") || !strings.Contains(text, "Write in English") {
		t.Errorf("prompt %q", text)
	}

	if err := d.Actions[1].Run(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.Pasted[0].Text != "A short summary" {
		t.Errorf("pasted %+v", ctx.Pasted)
	}

	// Continuing goes on from the answer, in a chat.
	if err := d.Actions[2].Run(ctx); err != nil {
		t.Fatal(err)
	}
	chat := ctx.Pushed[1].(*sdk.Chat)
	if len(chat.Messages) != 2 || chat.Messages[1].Text != "A short summary" || ctx.Popped != 1 {
		t.Errorf("chat %+v", chat)
	}
}

func TestACommandFallsBackToTheClipboard(t *testing.T) {
	srv, heard := openAI(t, "ok")
	e := extension(t)
	ctx := context(t, e, "openai", srv.URL)
	ctx.SelectionErr = errors.New("no window")
	ctx.Clipboard = "the clipboard"
	if err := commandNamed(t, e, ctx, "command/summarize").Run(ctx); err != nil {
		t.Fatal(err)
	}
	if err := ctx.Pushed[0].(*sdk.Detail).Stream(ctx, func(string) {}); err != nil {
		t.Fatal(err)
	}
	messages, _ := (*heard)[0].body["messages"].([]any)
	if text, _ := messages[0].(map[string]any)["content"].(string); !strings.Contains(text, "the clipboard") {
		t.Errorf("prompt %q", text)
	}
}

func TestACommandWithNoTextSaysSoWithoutAsking(t *testing.T) {
	e := extension(t)
	ctx := context(t, e, "openai", "http://localhost:1")
	err := commandNamed(t, e, ctx, "command/summarize").Run(ctx)
	if err == nil || !strings.Contains(err.Error(), "Select some text") {
		t.Errorf("err %v", err)
	}
	if len(ctx.Requests) != 0 || len(ctx.Pushed) != 0 {
		t.Errorf("requests %v, pushed %v", ctx.Requests, ctx.Pushed)
	}
}

func userCommand(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTheUsersCommandsComeFromTheFolder(t *testing.T) {
	dir := t.TempDir()
	e := ai.New(ai.Platform{CommandsDir: dir})
	userCommand(t, dir, "pirate.toml", `title = "Pirate"
input = "clipboard"
output = "paste"
model = "other-model"
prompt = "Say this as a pirate: {text}"`)
	userCommand(t, dir, "summarize.toml", `title = "My Summary"
prompt = "Shorter: {text}"`)
	userCommand(t, dir, "fix-grammar.toml", `disabled = true
prompt = "x"`)
	userCommand(t, dir, "broken.toml", `title = "Broken`)
	userCommand(t, dir, "noprompt.toml", `title = "No Prompt"`)
	userCommand(t, dir, "badoutput.toml", `prompt = "x"
output = "fax"`)
	userCommand(t, dir, "_example.toml", `title = "Example"
prompt = "x"`)

	srv, heard := openAI(t, "Arr")
	ctx := context(t, e, "openai", srv.URL)
	ctx.Clipboard = "hello"
	titles := map[string]string{}
	for _, c := range e.Commands(ctx) {
		titles[c.ID] = c.Title
	}
	if titles["command/summarize"] != "My Summary" {
		t.Errorf("a file should replace the default of its name: %v", titles)
	}
	for _, id := range []string{"command/fix-grammar", "command/broken", "command/noprompt", "command/badoutput", "command/_example"} {
		if _, ok := titles[id]; ok {
			t.Errorf("%s should be left out", id)
		}
	}

	if err := commandNamed(t, e, ctx, "command/pirate").Run(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.Selections != 0 {
		t.Error("input = clipboard pressed Ctrl+C in the user's app")
	}
	if len(ctx.Pasted) != 1 || ctx.Pasted[0].Text != "Arr" {
		t.Errorf("pasted %+v", ctx.Pasted)
	}
	if got := (*heard)[0].body["model"]; got != "other-model" {
		t.Errorf("model %v", got)
	}
}

func TestTheFolderOpensWithAnExampleThatIsNotACommand(t *testing.T) {
	e := extension(t)
	ctx := sdktest.New(t, e, "en")
	if err := commandNamed(t, e, ctx, "commands-folder").Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(ctx.Opened) != 1 {
		t.Fatalf("opened %v", ctx.Opened)
	}
	data, err := os.ReadFile(filepath.Join(ctx.Opened[0], "_example.toml"))
	if err != nil {
		t.Fatal(err)
	}
	// The example is real: copied without the underscore, it is a command.
	userCommand(t, ctx.Opened[0], "formal.toml", string(data))
	if c := commandNamed(t, e, ctx, "command/formal"); c.Title != "Make Formal" {
		t.Errorf("title %q", c.Title)
	}
}
