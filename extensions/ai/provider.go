package ai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/JoaoVitorOli/althea/sdk"
)

// Message is something said to a model or by it.
type Message struct {
	// Assistant marks what the model said, rather than the user.
	Assistant bool
	Text      string
}

// Request is what a provider is asked.
type Request struct {
	Model string
	// System tells the model how to behave; "" for nothing.
	System   string
	Messages []Message
}

// Provider is a service that answers: every one Althea talks to is behind this.
type Provider interface {
	// Stream asks and calls delta with each piece of the answer, until the answer ends, ctx is
	// canceled or it fails. Its errors are for the user.
	Stream(ctx sdk.Context, r Request, delta func(string)) error
}

// maxTokens is the most an answer may take where the provider asks to be told.
const maxTokens = 8192

// post sends a JSON body and returns the response, its failures turned into errors for the user.
func post(ctx sdk.Context, address string, header http.Header, body any) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(data))
	if err != nil {
		return nil, errors.New(ctx.T("error.address", "address", address))
	}
	req.Header = header.Clone()
	req.Header.Set("Content-Type", "application/json")
	res, err := ctx.HTTP().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		ctx.Log().Warn("provider not reached", "err", err)
		return nil, errors.New(ctx.T("error.unreachable", "host", req.URL.Host))
	}
	if res.StatusCode >= 300 {
		defer func() { _ = res.Body.Close() }()
		return nil, statusError(ctx, res)
	}
	return res, nil
}

// statusError says what a provider's refusal means, with the provider's own words when it has any.
func statusError(ctx sdk.Context, res *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	said := providerMessage(data)
	key := "error.status"
	switch res.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		key = "error.refused_key"
	case http.StatusNotFound:
		key = "error.not_found"
	case http.StatusTooManyRequests:
		key = "error.too_many"
	}
	msg := ctx.T(key, "status", res.StatusCode)
	if said != "" {
		msg += ": " + said
	}
	return errors.New(msg)
}

// providerMessage finds the message in a provider's error, in any of the shapes they use:
// {"error": {"message": ...}}, {"error": "..."} or {"message": ...}.
func providerMessage(data []byte) string {
	var e struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	if json.Unmarshal(data, &e) != nil {
		return ""
	}
	msg := e.Message
	var nested struct {
		Message string `json:"message"`
	}
	var plain string
	switch {
	case json.Unmarshal(e.Error, &nested) == nil && nested.Message != "":
		msg = nested.Message
	case json.Unmarshal(e.Error, &plain) == nil && plain != "":
		msg = plain
	}
	if runes := []rune(msg); len(runes) > 240 {
		msg = string(runes[:240]) + "…"
	}
	return msg
}

// sse calls data with the data of each server-sent event of body, until it returns false or the
// body ends. Data longer than a line, which none of the providers send, is not put together.
func sse(ctx sdk.Context, body io.Reader, data func(string) (more bool, err error)) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64<<10), 8<<20)
	for scanner.Scan() {
		line, ok := strings.CutPrefix(scanner.Text(), "data:")
		if !ok {
			continue
		}
		more, err := data(strings.TrimSpace(line))
		if err != nil {
			return err
		}
		if !more {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New(ctx.T("error.cut_off"))
	}
	return nil
}

// OpenAI talks to OpenAI and whatever speaks its API: Ollama, LM Studio, llama.cpp, OpenRouter,
// Groq.
type OpenAI struct {
	// BaseURL ends before /chat/completions, as "https://api.openai.com/v1".
	BaseURL string
	// Key is "" for servers that ask for none.
	Key string
}

// Stream implements Provider.
func (p OpenAI) Stream(ctx sdk.Context, r Request, delta func(string)) error {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	var messages []message
	if r.System != "" {
		messages = append(messages, message{"system", r.System})
	}
	for _, m := range r.Messages {
		role := "user"
		if m.Assistant {
			role = "assistant"
		}
		messages = append(messages, message{role, m.Text})
	}
	header := http.Header{}
	if p.Key != "" {
		header.Set("Authorization", "Bearer "+p.Key)
	}
	res, err := post(ctx, strings.TrimRight(p.BaseURL, "/")+"/chat/completions", header, map[string]any{
		"model": r.Model, "messages": messages, "stream": true,
	})
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	return sse(ctx, res.Body, func(data string) (bool, error) {
		if data == "[DONE]" {
			return false, nil
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			return true, nil
		}
		if len(chunk.Choices) == 0 {
			if msg := providerMessage([]byte(data)); msg != "" {
				return false, errors.New(msg)
			}
			return true, nil
		}
		if c := chunk.Choices[0].Delta.Content; c != "" {
			delta(c)
		}
		return true, nil
	})
}

// Anthropic talks to Anthropic's API.
type Anthropic struct {
	// BaseURL ends before /v1/messages, as "https://api.anthropic.com".
	BaseURL string
	Key     string
}

// Stream implements Provider.
func (p Anthropic) Stream(ctx sdk.Context, r Request, delta func(string)) error {
	type message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	messages := make([]message, len(r.Messages))
	for i, m := range r.Messages {
		messages[i] = message{"user", m.Text}
		if m.Assistant {
			messages[i].Role = "assistant"
		}
	}
	body := map[string]any{"model": r.Model, "max_tokens": maxTokens, "messages": messages, "stream": true}
	if r.System != "" {
		body["system"] = r.System
	}
	header := http.Header{}
	header.Set("X-Api-Key", p.Key)
	header.Set("Anthropic-Version", "2023-06-01")
	res, err := post(ctx, strings.TrimRight(p.BaseURL, "/")+"/v1/messages", header, body)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	return sse(ctx, res.Body, func(data string) (bool, error) {
		var event struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		}
		if json.Unmarshal([]byte(data), &event) != nil {
			return true, nil
		}
		switch event.Type {
		case "content_block_delta":
			if event.Delta.Type == "text_delta" && event.Delta.Text != "" {
				delta(event.Delta.Text)
			}
		case "message_stop":
			return false, nil
		case "error":
			return false, errors.New(providerMessage([]byte(data)))
		}
		return true, nil
	})
}

// Gemini talks to Google's Gemini API.
type Gemini struct {
	// BaseURL ends before /v1beta, as "https://generativelanguage.googleapis.com".
	BaseURL string
	Key     string
}

// Stream implements Provider.
func (p Gemini) Stream(ctx sdk.Context, r Request, delta func(string)) error {
	type part struct {
		Text string `json:"text"`
	}
	type content struct {
		Role  string `json:"role,omitempty"`
		Parts []part `json:"parts"`
	}
	contents := make([]content, len(r.Messages))
	for i, m := range r.Messages {
		contents[i] = content{Role: "user", Parts: []part{{m.Text}}}
		if m.Assistant {
			contents[i].Role = "model"
		}
	}
	body := map[string]any{"contents": contents}
	if r.System != "" {
		body["systemInstruction"] = content{Parts: []part{{r.System}}}
	}
	// The key goes in a header: an address ends up in logs.
	header := http.Header{}
	header.Set("X-Goog-Api-Key", p.Key)
	address := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse",
		strings.TrimRight(p.BaseURL, "/"), url.PathEscape(r.Model))
	res, err := post(ctx, address, header, body)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	return sse(ctx, res.Body, func(data string) (bool, error) {
		var chunk struct {
			Candidates []struct {
				Content struct {
					Parts []part `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
			PromptFeedback struct {
				BlockReason string `json:"blockReason"`
			} `json:"promptFeedback"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			return true, nil
		}
		if reason := chunk.PromptFeedback.BlockReason; reason != "" {
			return false, errors.New(ctx.T("error.blocked", "reason", reason))
		}
		if msg := providerMessage([]byte(data)); msg != "" && len(chunk.Candidates) == 0 {
			return false, errors.New(msg)
		}
		for _, c := range chunk.Candidates {
			for _, pt := range c.Content.Parts {
				if pt.Text != "" {
					delta(pt.Text)
				}
			}
		}
		return true, nil
	})
}
