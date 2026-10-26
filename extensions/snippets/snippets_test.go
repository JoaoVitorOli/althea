package snippets_test

import (
	"strings"
	"testing"
	"time"

	"github.com/JoaoVitorOli/althea/extensions/snippets"
	"github.com/JoaoVitorOli/althea/sdk"
	"github.com/JoaoVitorOli/althea/sdk/sdktest"
)

func newExt() *snippets.Extension {
	e := snippets.New()
	e.Now = func() time.Time { return time.Date(2026, 10, 26, 14, 5, 0, 0, time.UTC) }
	return e
}

func create(t *testing.T, e *snippets.Extension, ctx *sdktest.Context, name, text, keyword string) error {
	t.Helper()
	if err := e.Commands(ctx)[1].Run(ctx); err != nil {
		t.Fatal(err)
	}
	form := ctx.Pushed[len(ctx.Pushed)-1].(*sdk.Form)
	return form.Submit(ctx, sdk.Values{"name": name, "text": text, "keyword": keyword})
}

func items(t *testing.T, e *snippets.Extension, ctx *sdktest.Context) []sdk.Item {
	t.Helper()
	if err := e.Commands(ctx)[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	its, err := ctx.Pushed[len(ctx.Pushed)-1].(*sdk.List).Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return its
}

func TestExpandFillsPlaceholdersInTheLanguage(t *testing.T) {
	e := newExt()
	ctx := sdktest.New(t, e, "pt-BR")
	ctx.Clipboard = "COPIADO"
	text, back := e.Expand(ctx, "Em {date} às {time}: {clipboard}")
	if text != "Em 26/10/2026 às 14:05: COPIADO" || back != 0 {
		t.Errorf("Expand = %q, %d", text, back)
	}
	ctx = sdktest.New(t, e, "en")
	if text, _ := e.Expand(ctx, "{datetime}"); text != "10/26/2026 2:05 PM" {
		t.Errorf("en datetime = %q", text)
	}
}

func TestCursorSetsHowFarTheCaretGoesBack(t *testing.T) {
	e := newExt()
	ctx := sdktest.New(t, e, "en")
	text, back := e.Expand(ctx, "<b>{cursor}</b>\r\nção{cursor}")
	if text != "<b></b>\r\nção" || back != 8 {
		t.Errorf("Expand = %q, %d; want the caret 8 steps back", text, back)
	}
}

func TestSnippetsPasteAndCopyExpanded(t *testing.T) {
	e := newExt()
	ctx := sdktest.New(t, e, "en")
	if err := create(t, e, ctx, "Signature", "Best,\nJoão {cursor}", ";sig"); err != nil {
		t.Fatal(err)
	}
	its := items(t, e, ctx)
	if len(its) != 1 || its[0].Title != "Signature" || its[0].Accessory != ";sig" || its[0].Detail == nil {
		t.Fatalf("items %+v", its)
	}
	if err := its[0].Actions[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	if want := (sdk.Clip{Text: "Best,\nJoão ", CaretBack: 0}); len(ctx.Pasted) != 1 || ctx.Pasted[0].Text != want.Text {
		t.Errorf("pasted %+v", ctx.Pasted)
	}
	if err := its[0].Actions[1].Run(ctx); err != nil || ctx.Copied[0] != "Best,\nJoão " {
		t.Errorf("copy: %v, %q", err, ctx.Copied)
	}
}

func TestKeywordsMustNotClash(t *testing.T) {
	e := newExt()
	ctx := sdktest.New(t, e, "en")
	if err := create(t, e, ctx, "Address", "Rua A, 1", ";addr"); err != nil {
		t.Fatal(err)
	}
	for _, kw := range []string{";addr", ";ad", ";address"} {
		err := create(t, e, ctx, "Other", "x", kw)
		if err == nil || !strings.Contains(err.Error(), "Address") {
			t.Errorf("keyword %q: err = %v, want a clash with Address", kw, err)
		}
	}
	if err := create(t, e, ctx, "Phone", "555", ";tel"); err != nil {
		t.Errorf("a distinct keyword: %v", err)
	}
}

func TestEditAndDelete(t *testing.T) {
	e := newExt()
	ctx := sdktest.New(t, e, "en")
	_ = create(t, e, ctx, "Hi", "Hello", "")
	its := items(t, e, ctx)
	if err := its[0].Actions[2].Run(ctx); err != nil {
		t.Fatal(err)
	}
	form := ctx.Pushed[len(ctx.Pushed)-1].(*sdk.Form)
	if err := form.Submit(ctx, sdk.Values{"name": "Hi", "text": "Hello there", "keyword": ";hi"}); err != nil {
		t.Fatal(err)
	}
	its = items(t, e, ctx)
	if len(its) != 1 || !strings.Contains(its[0].Detail.Markdown, "Hello there") {
		t.Fatalf("after edit %+v", its)
	}
	if err := its[0].Actions[4].Run(ctx); err != nil {
		t.Fatal(err)
	}
	if its = items(t, e, ctx); len(its) != 0 {
		t.Errorf("after delete %+v", its)
	}
}
