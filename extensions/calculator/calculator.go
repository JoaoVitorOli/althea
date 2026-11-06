// Package calculator is the Calculator extension: math typed into the root search is answered
// at the top, and Enter copies the answer. Amounts of money convert by daily exchange rates,
// fetched the first time a calculation needs them.
package calculator

import (
	"embed"
	"errors"
	"io/fs"
	"strconv"
	"strings"
	"time"

	"github.com/JoaoVitorOli/althea/extensions/calculator/calc"
	"github.com/JoaoVitorOli/althea/sdk"
)

//go:embed locales/*.toml
var locales embed.FS

// Extension is the Calculator.
type Extension struct {
	rates *rates
}

// New returns the Calculator extension.
func New() *Extension { return NewWithRates(ratesURL, time.Now) }

// NewWithRates returns the Calculator fetching exchange rates from url, as a test server, with
// now telling the time.
func NewWithRates(url string, now func() time.Time) *Extension {
	return &Extension{rates: &rates{url: url, now: now}}
}

// Manifest implements sdk.Extension.
func (*Extension) Manifest() sdk.Manifest {
	messages, _ := fs.Sub(locales, "locales")
	return sdk.Manifest{ID: "althea.calculator", Title: "Calculator", Icon: sdk.Icon{Glyph: "calculator"}, Messages: messages}
}

// Commands implements sdk.Extension: the calculator answers rather than runs.
func (*Extension) Commands(sdk.Context) []sdk.Command { return nil }

// Preferences implements sdk.Configurable.
func (*Extension) Preferences(ctx sdk.Context) []sdk.Preference {
	return []sdk.Preference{
		{
			Name: "currency", Title: ctx.T("prefs.currency"), Kind: sdk.TextPreference,
			Description: ctx.T("prefs.currency_info", "region", regionCurrency(ctx.Region())),
		},
		{
			Name: "exchange_rates", Title: ctx.T("prefs.exchange_rates"), Kind: sdk.BoolPreference,
			Description: ctx.T("prefs.exchange_rates_info"), Default: "true",
		},
	}
}

// currency is the user's currency: the one they set, or their region's.
func currency(ctx sdk.Context) string {
	if c := strings.ToUpper(strings.TrimSpace(ctx.Preferences().String("currency"))); len(c) == 3 {
		return c
	}
	return regionCurrency(ctx.Region())
}

// Answer implements sdk.Answerer.
func (e *Extension) Answer(ctx sdk.Context, query string) (*sdk.Item, error) {
	if !calc.LooksLikeMath(query) {
		return nil, nil
	}
	r, err := calc.EvalWith(query, calc.Options{Lang: ctx.Language()})
	if errors.Is(err, calc.ErrNeedRates) {
		return e.money(ctx, query)
	}
	if err != nil {
		return nil, nil //nolint:nilerr // what is not math has no answer, which is no error
	}
	return answer(ctx, query, r, query, ctx.T("kind")), nil
}

// money answers a calculation with currencies, once the rates are at hand.
func (e *Extension) money(ctx sdk.Context, query string) (*sdk.Item, error) {
	if !ctx.Preferences().Bool("exchange_rates") {
		return nil, nil
	}
	rates, updated := e.rates.get(ctx)
	if rates == nil {
		return e.waiting(ctx, query), nil
	}
	lang := ctx.Language()
	r, err := calc.EvalWith(query, calc.Options{Lang: lang, Rates: rates, Currency: currency(ctx)})
	if err != nil {
		return nil, nil //nolint:nilerr // as a code the rates lack: no answer
	}
	subtitle := query
	if r.Rate != nil {
		subtitle = ctx.T("rate", "from", r.Rate.From, "rate", number(r.Rate.Of, lang), "to", r.Rate.To)
	}
	// Rates a few days old, as after a while offline, say when they are of.
	if age := time.Since(updated); age > 48*time.Hour {
		layout := "01/02/2006"
		if lang == "pt-BR" {
			layout = "02/01/2006"
		}
		subtitle += " · " + ctx.T("rates_of", "date", updated.Format(layout))
	}
	item := answer(ctx, query, r, subtitle, ctx.T("rates_by"))
	item.Actions = append(item.Actions[:1], append([]sdk.Action{{
		Title: ctx.T("copy_amount"), Run: copyText(r.Text, r.Text),
	}}, item.Actions[1:]...)...)
	return item, nil
}

// waiting is the answer while the rates are on their way, or after they could not be fetched.
func (e *Extension) waiting(ctx sdk.Context, query string) *sdk.Item {
	item := &sdk.Item{
		ID: "answer", Title: ctx.T("rates_fetching"), Subtitle: query, Icon: sdk.Icon{Glyph: "calculator"},
		Accessory: ctx.T("kind"),
	}
	if !e.rates.pending() {
		item.Title = ctx.T("rates_failed")
		item.Actions = []sdk.Action{{Title: ctx.T("retry"), Icon: sdk.Icon{Glyph: "restart"}, Run: func(ctx sdk.Context) error {
			e.rates.retry()
			e.rates.get(ctx)
			ctx.Refresh()
			return nil
		}}}
	}
	return item
}

func answer(ctx sdk.Context, query string, r calc.Result, subtitle, accessory string) *sdk.Item {
	return &sdk.Item{
		ID: "answer", Title: r.Text, Subtitle: subtitle, Icon: sdk.Icon{Glyph: "calculator"},
		Accessory: accessory,
		Actions: []sdk.Action{
			{Title: ctx.T("copy"), Run: copyText(r.Plain, r.Text)},
			{Title: ctx.T("copy_expression"), Run: copyText(query+" = "+r.Text, r.Text)},
		},
	}
}

func copyText(text, answer string) func(sdk.Context) error {
	return func(ctx sdk.Context) error {
		if err := ctx.CopyText(text); err != nil {
			return err
		}
		ctx.Toast(ctx.T("copied", "answer", answer))
		ctx.Hide()
		return nil
	}
}

// number writes a rate with five significant digits, as the user's language does: "5,3712".
func number(f float64, lang string) string {
	s := strconv.FormatFloat(f, 'g', 5, 64)
	if strings.ContainsAny(s, "e") {
		s = strconv.FormatFloat(f, 'f', -1, 64)
	}
	if lang == "pt-BR" {
		s = strings.Replace(s, ".", ",", 1)
	}
	return s
}
