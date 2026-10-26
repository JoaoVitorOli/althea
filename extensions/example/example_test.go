package example_test

import (
	"testing"
	"time"

	"github.com/JoaoVitorOli/althea/extensions/example"
	"github.com/JoaoVitorOli/althea/sdk"
	"github.com/JoaoVitorOli/althea/sdk/sdktest"
)

// add adds a to-do through the form, as the user would.
func add(t *testing.T, e *example.Extension, ctx *sdktest.Context, title string) {
	t.Helper()
	if err := e.Commands(ctx)[1].Run(ctx); err != nil {
		t.Fatal(err)
	}
	form := ctx.Pushed[len(ctx.Pushed)-1].(*sdk.Form)
	if err := form.Submit(ctx, sdk.Values{"title": title, "notes": ""}); err != nil {
		t.Fatal(err)
	}
}

// open shows the list and returns its items.
func open(t *testing.T, e *example.Extension, ctx *sdktest.Context) []sdk.Item {
	t.Helper()
	if err := e.Commands(ctx)[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	list := ctx.Pushed[len(ctx.Pushed)-1].(*sdk.List)
	items, err := list.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func titles(items []sdk.Item) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.Title)
	}
	return out
}

func TestAddedTodosAreListedNewestFirst(t *testing.T) {
	e := example.New()
	now := time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)
	e.Now = func() time.Time { now = now.Add(time.Minute); return now }
	ctx := sdktest.New(t, e, "pt-BR")

	add(t, e, ctx, "Regar as plantas")
	add(t, e, ctx, "Comprar pão")
	if got := ctx.Toasts[1]; got != `"Comprar pão" adicionada` {
		t.Errorf("toast %q", got)
	}
	items := open(t, e, ctx)
	if got := titles(items); len(got) != 2 || got[0] != "Comprar pão" {
		t.Fatalf("items %q", got)
	}
	if items[0].Section != "A fazer" {
		t.Errorf("section %q", items[0].Section)
	}

	// The preference turns the order around.
	ctx.Prefs["order"] = "oldest"
	if got := titles(open(t, e, ctx)); got[0] != "Regar as plantas" {
		t.Errorf("oldest first gives %q", got)
	}
}

func TestDoneTodosGoLast(t *testing.T) {
	e := example.New()
	ctx := sdktest.New(t, e, "en")
	add(t, e, ctx, "One")
	add(t, e, ctx, "Two")
	items := open(t, e, ctx)
	if err := items[0].Actions[0].Run(ctx); err != nil { // mark "Two" done
		t.Fatal(err)
	}
	items = open(t, e, ctx)
	if got := titles(items); got[0] != "One" || items[1].Section != "Done" || items[1].Actions[0].Title != "Mark as Not Done" {
		t.Errorf("items %q, %+v", got, items[1])
	}
	if err := items[1].Actions[3].Run(ctx); err != nil { // delete it
		t.Fatal(err)
	}
	if got := titles(open(t, e, ctx)); len(got) != 1 {
		t.Errorf("after deleting: %q", got)
	}
}

func TestFormRefusesLongTitles(t *testing.T) {
	e := example.New()
	ctx := sdktest.New(t, e, "en")
	_ = e.Commands(ctx)[1].Run(ctx)
	form := ctx.Pushed[0].(*sdk.Form)
	if msg := form.Fields[0].Validate(string(make([]rune, 121))); msg == "" {
		t.Error("a title of 121 characters is accepted")
	}
}
