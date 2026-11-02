package web_test

import (
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
	if len(items) != 1 || items[0].Title != `Buscar "café & pão" na web` || items[0].Subtitle != "DuckDuckGo" {
		t.Fatalf("items %+v", items)
	}
	if err := items[0].Actions[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	want := []string{"https://duckduckgo.com/?q=caf%C3%A9+%26+p%C3%A3o"}
	if !slices.Equal(ctx.Opened, want) || ctx.Hidden != 1 {
		t.Errorf("opened %v, hidden %d, want %v", ctx.Opened, ctx.Hidden, want)
	}

	ctx.Prefs["engine"] = "google"
	if err := e.Fallbacks(ctx, "go")[0].Actions[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	if got := ctx.Opened[1]; got != "https://www.google.com/search?q=go" {
		t.Errorf("opened %q", got)
	}
}
