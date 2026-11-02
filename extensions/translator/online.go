package translator

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/JoaoVitorOli/althea/sdk"
)

// deepLAddress is DeepL's, by the kind of key: free ones end in ":fx" and have an address of their
// own.
func (e *Extension) deepLAddress(key string) string {
	if e.DeepLBase != "" {
		return e.DeepLBase
	}
	if strings.HasSuffix(key, ":fx") {
		return "https://api-free.deepl.com"
	}
	return "https://api.deepl.com"
}

// deepLTarget spells a language as DeepL takes it to translate into, which wants a variety.
func deepLTarget(lang string) string {
	switch lang {
	case "pt":
		return "PT-BR"
	case "en":
		return "EN-US"
	}
	return strings.ToUpper(lang)
}

// postJSON sends body and decodes the answer into out, its failures for the user.
func postJSON(ctx sdk.Context, address string, header http.Header, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(data))
	if err != nil {
		return errors.New(ctx.T("online.address", "address", address))
	}
	if header != nil {
		req.Header = header
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := ctx.HTTP().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		ctx.Log().Warn("translation provider not reached", "err", err)
		return errors.New(ctx.T("online.unreachable", "host", req.URL.Host))
	}
	defer func() { _ = res.Body.Close() }()
	reply, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return errors.New(ctx.T("online.unreachable", "host", req.URL.Host))
	}
	switch {
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return errors.New(ctx.T("online.refused_key", "status", res.StatusCode))
	case res.StatusCode >= 300:
		return errors.New(ctx.T("online.status", "status", res.StatusCode))
	}
	if err := json.Unmarshal(reply, out); err != nil {
		return errors.New(ctx.T("online.unclear"))
	}
	return nil
}

// deepL translates with DeepL.
func (e *Extension) deepL(ctx sdk.Context, from, to, text string) (string, error) {
	key, err := ctx.Preferences().Secret("deepl_key")
	if err != nil {
		ctx.Log().Warn("DeepL key not read", "err", err)
	}
	if key == "" {
		return "", errors.New(ctx.T("online.no_key", "provider", "DeepL"))
	}
	header := http.Header{}
	header.Set("Authorization", "DeepL-Auth-Key "+key)
	var out struct {
		Translations []struct {
			Text string `json:"text"`
		} `json:"translations"`
	}
	err = postJSON(ctx, e.deepLAddress(key)+"/v2/translate", header, map[string]any{
		"text": []string{text}, "source_lang": strings.ToUpper(from), "target_lang": deepLTarget(to),
	}, &out)
	if err != nil {
		return "", err
	}
	if len(out.Translations) == 0 {
		return "", errors.New(ctx.T("online.unclear"))
	}
	return out.Translations[0].Text, nil
}

// libreTranslate translates with a LibreTranslate server.
func libreTranslate(ctx sdk.Context, from, to, text string) (string, error) {
	address := strings.TrimRight(strings.TrimSpace(ctx.Preferences().String("libretranslate_url")), "/")
	if address == "" {
		return "", errors.New(ctx.T("online.no_server"))
	}
	body := map[string]any{"q": text, "source": from, "target": to, "format": "text"}
	if key, err := ctx.Preferences().Secret("libretranslate_key"); err == nil && key != "" {
		body["api_key"] = key
	}
	var out struct {
		TranslatedText string `json:"translatedText"`
	}
	if err := postJSON(ctx, address+"/translate", nil, body, &out); err != nil {
		return "", err
	}
	return out.TranslatedText, nil
}
