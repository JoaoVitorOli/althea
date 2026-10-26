package quicklinks_test

import (
	"strings"
	"testing"

	"github.com/JoaoVitorOli/althea/extensions/quicklinks"
	"github.com/JoaoVitorOli/althea/sdk"
	"github.com/JoaoVitorOli/althea/sdk/sdktest"
)

func create(t *testing.T, e *quicklinks.Extension, ctx *sdktest.Context, v sdk.Values) error {
	t.Helper()
	if err := e.Commands(ctx)[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	form := ctx.Pushed[len(ctx.Pushed)-1].(*sdk.Form)
	for _, f := range form.Fields {
		if _, ok := v[f.ID]; !ok {
			v[f.ID] = ""
		}
		if f.Validate != nil {
			if msg := f.Validate(v[f.ID]); msg != "" {
				t.Fatalf("%s: %s", f.ID, msg)
			}
		}
	}
	return form.Submit(ctx, v)
}

func TestExpandFillsPlaceholders(t *testing.T) {
	for _, c := range []struct{ target, want string }{
		{"https://github.com/search?q={query}", "https://github.com/search?q=go+%26+rust"},
		{"https://translate.app/?text={clipboard}", "https://translate.app/?text=ol%C3%A1"},
		{`C:\Projects\{query}`, `C:\Projects\go & rust`},
	} {
		if got := quicklinks.Expand(c.target, "go & rust", "olá"); got != c.want {
			t.Errorf("Expand(%q) = %q, want %q", c.target, got, c.want)
		}
	}
}

func TestCreatedLinksAreItemsFoundByAlias(t *testing.T) {
	e := quicklinks.New()
	ctx := sdktest.New(t, e, "en")
	if err := create(t, e, ctx, sdk.Values{"name": "GitHub Search", "target": "https://github.com/search?q={query}", "alias": "gh", "icon": "🐙"}); err != nil {
		t.Fatal(err)
	}
	if ctx.Refreshed != 1 {
		t.Error("the root search was not asked to refresh")
	}
	items, err := e.Items(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("items %v, %v", items, err)
	}
	it := items[0]
	if it.Title != "GitHub Search" || it.Keywords[0] != "gh" || it.Icon.Text != "🐙" || it.Accessory != "Quicklink" {
		t.Errorf("item %+v", it)
	}

	// Opening asks for the query, then opens the expanded link.
	if err := it.Actions[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	form := ctx.Pushed[len(ctx.Pushed)-1].(*sdk.Form)
	if err := form.Submit(ctx, sdk.Values{"query": "mygo"}); err != nil {
		t.Fatal(err)
	}
	if len(ctx.Opened) != 1 || ctx.Opened[0] != "https://github.com/search?q=mygo" || ctx.Hidden != 1 {
		t.Errorf("opened %q, hidden %d", ctx.Opened, ctx.Hidden)
	}

	// Another link cannot take the alias.
	err = create(t, e, ctx, sdk.Values{"name": "Gmail", "target": "https://mail.google.com", "alias": "GH"})
	if err == nil || !strings.Contains(err.Error(), "GitHub Search") {
		t.Errorf("err = %v, want the alias refused", err)
	}
}

func TestLinksWithoutQueryOpenAtOnceWithTheClipboard(t *testing.T) {
	e := quicklinks.New()
	ctx := sdktest.New(t, e, "en")
	ctx.Clipboard = "hello world"
	if err := create(t, e, ctx, sdk.Values{"name": "Translate", "target": "https://translate.app/?text={clipboard}"}); err != nil {
		t.Fatal(err)
	}
	items, _ := e.Items(ctx)
	if err := items[0].Actions[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(ctx.Opened) != 1 || ctx.Opened[0] != "https://translate.app/?text=hello+world" {
		t.Errorf("opened %q", ctx.Opened)
	}
}

func TestEditAndDelete(t *testing.T) {
	e := quicklinks.New()
	ctx := sdktest.New(t, e, "pt-BR")
	_ = create(t, e, ctx, sdk.Values{"name": "Docs", "target": `C:\Docs`})
	items, _ := e.Items(ctx)
	if err := items[0].Actions[2].Run(ctx); err != nil {
		t.Fatal(err)
	}
	form := ctx.Pushed[len(ctx.Pushed)-1].(*sdk.Form)
	if form.Title != "Editar quicklink" || form.Fields[0].Default != "Docs" {
		t.Fatalf("edit form %+v", form)
	}
	if err := form.Submit(ctx, sdk.Values{"name": "Documentos", "target": `C:\Docs`, "alias": "", "icon": ""}); err != nil {
		t.Fatal(err)
	}
	items, _ = e.Items(ctx)
	if len(items) != 1 || items[0].Title != "Documentos" || items[0].Icon.File != `C:\Docs` {
		t.Fatalf("items %+v", items)
	}
	if err := items[0].Actions[3].Run(ctx); err != nil {
		t.Fatal(err)
	}
	if items, _ = e.Items(ctx); len(items) != 0 {
		t.Errorf("items after delete %+v", items)
	}
}
