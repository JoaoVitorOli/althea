package web_test

import (
	"bytes"
	"image/png"
	"slices"
	"testing"

	"github.com/JoaoVitorOli/althea/extensions/web"
	"github.com/JoaoVitorOli/althea/sdk"
	"github.com/JoaoVitorOli/althea/sdk/sdktest"
)

var _ sdk.FallbackSource = (*web.Extension)(nil)

func TestFallbackOpensTheQueryInTheChosenEngine(t *testing.T) {
	e := web.New()
	ctx := sdktest.New(t, e, "pt-BR")

	items := e.Fallbacks(ctx, "café & pão")
	if len(items) != 1 || items[0].Title != `Buscar "café & pão" na web` || items[0].Subtitle != "Google" {
		t.Fatalf("items %+v", items)
	}
	if err := items[0].Actions[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	want := []string{"https://www.google.com/search?q=caf%C3%A9+%26+p%C3%A3o"}
	if !slices.Equal(ctx.Opened, want) || ctx.Hidden != 1 {
		t.Errorf("opened %v, hidden %d, want %v", ctx.Opened, ctx.Hidden, want)
	}

	ctx.Prefs["engine"] = "duckduckgo"
	if err := e.Fallbacks(ctx, "go")[0].Actions[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	if got := ctx.Opened[1]; got != "https://duckduckgo.com/?q=go" {
		t.Errorf("opened %q", got)
	}
}

func TestTheIconFollowsTheEngine(t *testing.T) {
	e := web.New()
	ctx := sdktest.New(t, e, "en")

	icon := e.Fallbacks(ctx, "go")[0].Icon
	if _, err := png.Decode(bytes.NewReader(icon.Data)); err != nil {
		t.Errorf("Google's icon: %v", err)
	}

	ctx.Prefs["engine"] = "bing"
	if icon := e.Fallbacks(ctx, "go")[0].Icon; icon.Data != nil || icon.Glyph != "search" {
		t.Errorf("Bing's icon: %+v", icon)
	}
}
