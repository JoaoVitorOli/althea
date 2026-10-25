package apps_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/JoaoVitorOli/althea/extensions/apps"
	"github.com/JoaoVitorOli/althea/sdk"
	"github.com/JoaoVitorOli/althea/sdk/sdktest"
)

var installed = []apps.App{
	{Name: "Visual Studio Code", ID: "Microsoft.VisualStudioCode", Path: `shell:AppsFolder\Microsoft.VisualStudioCode`},
	{Name: "Calculadora", ID: "Microsoft.WindowsCalculator_8wekyb3d8bbwe!App", Path: `shell:AppsFolder\Microsoft.WindowsCalculator_8wekyb3d8bbwe!App`},
}

func TestItemsAreTheInstalledApps(t *testing.T) {
	var launched []string
	ext := apps.New(apps.Platform{
		List:   func() ([]apps.App, error) { return installed, nil },
		Launch: func(a apps.App) error { launched = append(launched, a.ID); return nil },
	})
	ctx := sdktest.New(t, ext, "pt-BR")
	items, err := ext.Items(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Title != "Visual Studio Code" || items[0].Icon.File != installed[0].Path {
		t.Fatalf("items = %+v", items)
	}
	if items[0].Accessory != "Aplicativo" {
		t.Errorf("accessory = %q, want it translated", items[0].Accessory)
	}

	open := items[1].Actions[0]
	if err := open.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(launched, []string{installed[1].ID}) || ctx.Hidden != 1 {
		t.Errorf("open launched %v, hid %d times", launched, ctx.Hidden)
	}

	copyName := items[0].Actions[1]
	if err := copyName.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ctx.Copied, []string{"Visual Studio Code"}) {
		t.Errorf("copied %v", ctx.Copied)
	}
}

func TestLaunchFailureIsReported(t *testing.T) {
	ext := apps.New(apps.Platform{
		List:   func() ([]apps.App, error) { return installed[:1], nil },
		Launch: func(apps.App) error { return errors.New("access denied") },
	})
	ctx := sdktest.New(t, ext, "en")
	items, _ := ext.Items(ctx)
	if err := items[0].Actions[0].Run(ctx); err == nil {
		t.Error("a failed launch reports no error")
	}
	if ctx.Hidden != 0 {
		t.Error("the launcher hid although the app did not open")
	}
}

func TestExtensionContract(t *testing.T) {
	var ext sdk.Extension = apps.New(apps.Platform{})
	if _, ok := ext.(sdk.ItemSource); !ok {
		t.Error("apps is not an item source")
	}
	if ext.Manifest().ID != "althea.apps" {
		t.Errorf("ID = %q", ext.Manifest().ID)
	}
}
