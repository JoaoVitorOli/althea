package registry_test

import (
	"context"
	"slices"
	"testing"

	"github.com/JoaoVitorOli/althea/sdk"
)

func TestAliasesPutTheirCommandFirstAndSurviveARefresh(t *testing.T) {
	r := newRegistry(t, apps, system)
	ctx := context.Background()
	if got := r.Query(ctx, "zq"); len(got) != 0 {
		t.Fatalf("Query(zq) before the alias = %v", titles(got))
	}

	r.SetAliases(map[string]string{"althea.system/lock": "zq"})
	if got := titles(r.Query(ctx, "ZQ")); !slices.Equal(got, []string{"Lock Screen"}) {
		t.Errorf("Query(ZQ) = %v", got)
	}
	if err := r.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if got := titles(r.Query(ctx, "zq")); !slices.Equal(got, []string{"Lock Screen"}) {
		t.Errorf("Query(zq) after a refresh = %v", got)
	}

	r.SetAliases(nil)
	if got := r.Query(ctx, "zq"); len(got) != 0 {
		t.Errorf("Query(zq) after removing the alias = %v", titles(got))
	}
}

// fallback is an extension offering something for what nothing matched.
type fallback struct {
	*fake
	items func(q string) []sdk.Item
}

func (f fallback) Fallbacks(_ sdk.Context, q string) []sdk.Item { return f.items(q) }

func TestFallbacksShowOnlyWhenNothingMatches(t *testing.T) {
	var asked []string
	web := fallback{&fake{id: "althea.web"}, func(q string) []sdk.Item {
		asked = append(asked, q)
		return []sdk.Item{{ID: "search", Title: "Search the web for " + q}}
	}}
	bad := fallback{&fake{id: "althea.bad"}, func(string) []sdk.Item { panic("no") }}
	files := fallback{&fake{id: "althea.files"}, func(q string) []sdk.Item {
		return []sdk.Item{{ID: "search", Title: "Search files for " + q}}
	}}
	r := newRegistry(t, web, bad, files, system)
	ctx := context.Background()

	got := r.Query(ctx, "  zzz ")
	if want := []string{"Search the web for zzz", "Search files for zzz"}; !slices.Equal(titles(got), want) {
		t.Fatalf("Query(zzz) = %v, want %v", titles(got), want)
	}
	if !got[0].Fallback || got[0].Key != "althea.web/search" || got[0].Extension != "althea.web" {
		t.Errorf("result %+v", got[0])
	}

	asked = nil
	if got := titles(r.Query(ctx, "lock")); !slices.Equal(got, []string{"Lock Screen"}) {
		t.Errorf("Query(lock) = %v", got)
	}
	r2 := newRegistry(t, web)
	for _, q := range []string{"", "   "} {
		if got := r2.Query(ctx, q); len(got) != 0 {
			t.Errorf("Query(%q) = %v", q, titles(got))
		}
	}
	if len(asked) != 0 {
		t.Errorf("asked for fallbacks with %q, when something matched or nothing was typed", asked)
	}
}
