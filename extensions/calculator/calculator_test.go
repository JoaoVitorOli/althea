package calculator_test

import (
	"slices"
	"testing"

	"github.com/JoaoVitorOli/althea/extensions/calculator"
	"github.com/JoaoVitorOli/althea/sdk/sdktest"
)

func TestAnswersMathAndCopiesTheResult(t *testing.T) {
	ext := calculator.New()
	ctx := sdktest.New(t, ext, "pt-BR")
	item, err := ext.Answer(ctx, "1234,5 * 2")
	if err != nil || item == nil {
		t.Fatalf("Answer = %v, %v", item, err)
	}
	if item.Title != "2.469" || item.Subtitle != "1234,5 * 2" || item.Accessory != "Calculadora" {
		t.Errorf("item = %+v", item)
	}
	if err := item.Actions[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	if err := item.Actions[1].Run(ctx); err != nil {
		t.Fatal(err)
	}
	if want := []string{"2469", "1234,5 * 2 = 2.469"}; !slices.Equal(ctx.Copied, want) {
		t.Errorf("copied %q, want %q", ctx.Copied, want)
	}
	if ctx.Hidden != 2 || len(ctx.Toasts) == 0 {
		t.Errorf("hid %d times, toasts %v", ctx.Hidden, ctx.Toasts)
	}
}

func TestDoesNotAnswerWhatIsNotMath(t *testing.T) {
	ext := calculator.New()
	ctx := sdktest.New(t, ext, "en")
	for _, q := range []string{"chrome", "7", "windows 11", "2 +"} {
		if item, err := ext.Answer(ctx, q); item != nil || err != nil {
			t.Errorf("Answer(%q) = %+v, %v; want nothing", q, item, err)
		}
	}
}
