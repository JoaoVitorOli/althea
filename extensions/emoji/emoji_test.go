package emoji_test

import (
	"slices"
	"testing"

	"github.com/JoaoVitorOli/althea/extensions/emoji"
	"github.com/JoaoVitorOli/althea/sdk"
	"github.com/JoaoVitorOli/althea/sdk/sdktest"
)

func TestDataHasEmojiAndSymbolsInBothLanguages(t *testing.T) {
	all := emoji.All()
	if len(all) < 2000 {
		t.Fatalf("%d emoji and symbols", len(all))
	}
	var wave, arrow *emoji.Emoji
	for i := range all {
		switch all[i].Chars {
		case "👋":
			wave = &all[i]
		case "→":
			arrow = &all[i]
		}
	}
	if wave == nil || wave.Name["pt-BR"] != "mão acenando" || !slices.Contains(wave.Keywords["pt-BR"], "tchau") || len(wave.Tones) != 5 {
		t.Errorf("waving hand %+v", wave)
	}
	if arrow == nil || arrow.Group != "Text Symbols" {
		t.Errorf("arrow %+v", arrow)
	}
	if all[0].Chars != "😀" {
		t.Errorf("first is %q, want Unicode's order", all[0].Chars)
	}
}

func items(t *testing.T, e *emoji.Extension, ctx *sdktest.Context) []sdk.Item {
	t.Helper()
	if err := e.Commands(ctx)[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	its, err := ctx.Pushed[len(ctx.Pushed)-1].(*sdk.Grid).Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return its
}

func find(its []sdk.Item, id string) (sdk.Item, bool) {
	for _, it := range its {
		if it.ID == id {
			return it, true
		}
	}
	return sdk.Item{}, false
}

func TestItemsSpeakTheLanguageAndFindInBoth(t *testing.T) {
	e := emoji.New()
	ctx := sdktest.New(t, e, "pt-BR")
	wave, ok := find(items(t, e, ctx), "👋")
	if !ok || wave.Title != "mão acenando" || wave.Section != "Pessoas e corpo" {
		t.Fatalf("item %+v", wave)
	}
	if !slices.Contains(wave.Keywords, "bye") || !slices.Contains(wave.Keywords, "tchau") {
		t.Errorf("keywords %q", wave.Keywords)
	}
}

func TestSkinTonesAndRecent(t *testing.T) {
	e := emoji.New()
	ctx := sdktest.New(t, e, "en")
	ctx.Prefs["tone"] = "medium"
	wave, _ := find(items(t, e, ctx), "👋")
	if wave.Icon.Text != "👋🏽" {
		t.Errorf("icon %q, want the medium tone", wave.Icon.Text)
	}
	if err := wave.Actions[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.Pasted[0].Text != "👋🏽" {
		t.Errorf("pasted %q", ctx.Pasted[0].Text)
	}
	its := items(t, e, ctx)
	if its[0].ID != "recent:👋" || its[0].Section != "Recently Used" {
		t.Errorf("first item %+v", its[0])
	}

	// Ctrl+T offers every tone.
	if err := wave.Actions[2].Run(ctx); err != nil {
		t.Fatal(err)
	}
	g := ctx.Pushed[len(ctx.Pushed)-1].(*sdk.Grid)
	if len(g.Items) != 6 || g.Items[5].Icon.Text != "👋🏿" {
		t.Errorf("tones %+v", g.Items)
	}
}
