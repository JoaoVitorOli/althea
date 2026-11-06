package calc_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/JoaoVitorOli/althea/extensions/calculator/calc"
)

// rates are round on purpose: one dollar buys 5 reais, 0.8 euro, 150 yen and 0.9 franc.
var rates = calc.Rates{"USD": 1, "BRL": 5, "EUR": 0.8, "GBP": 0.75, "JPY": 150, "CHF": 0.9, "ALL": 90}

func TestEvalCurrencies(t *testing.T) {
	nbsp := func(s string) string { return strings.ReplaceAll(s, " ", " ") }
	tests := []struct {
		in, lang, currency, want string
	}{
		// A lone amount goes to the user's currency.
		{"50 dollars", "pt-BR", "BRL", nbsp("R$ 250,00")},
		{"50 dollar", "en", "BRL", nbsp("R$ 250.00")},
		{"$50", "en", "BRL", nbsp("R$ 250.00")},
		{"50 usd", "pt-BR", "BRL", nbsp("R$ 250,00")},
		// An amount already in it goes to dollars, and dollars to euros.
		{"100 reais", "pt-BR", "BRL", nbsp("US$ 20,00")},
		{"R$ 100", "pt-BR", "BRL", nbsp("US$ 20,00")},
		{"10 dollars", "en", "USD", "€8.00"},
		// Any currency to any other, by code, symbol or name, in both languages.
		{"50 dollars to brazilian real", "en", "USD", nbsp("R$ 250.00")},
		{"50 dólares em reais", "pt-BR", "BRL", nbsp("R$ 250,00")},
		{"50 dólares americanos para euros", "pt-BR", "BRL", nbsp("€ 40,00")},
		{"€ 8 in usd", "en", "BRL", "$10.00"},
		{"1000 yen in brl", "en", "BRL", nbsp("R$ 33.33")},
		{"10 eur to chf", "en", "BRL", "11.25 CHF"},
		{"1 brl in jpy", "en", "USD", "¥30"},
		{"1 jpy in usd", "en", "BRL", "$0.0067"},
		// Arithmetic with amounts, in the first's currency unless converted.
		{"10 usd + 10 eur", "en", "USD", "€18.00"},
		{"10 usd + 8 eur in usd", "en", "BRL", "$20.00"},
		{"2 * 50 dollars in reais", "pt-BR", "BRL", nbsp("R$ 500,00")},
		{"20% of 50 dollars in reais", "pt-BR", "BRL", nbsp("R$ 50,00")},
		{"1234.5 usd in usd", "en", "BRL", "$1,234.50"},
		// Units still win over codes: "2 pounds" weighs.
		{"2 pounds in kg", "en", "BRL", "0.90718474 kg"},
	}
	for _, tc := range tests {
		got, err := calc.EvalWith(tc.in, calc.Options{Lang: tc.lang, Rates: rates, Currency: tc.currency})
		if err != nil || got.Text != tc.want {
			t.Errorf("EvalWith(%q, %s, %s) = %q, %v; want %q", tc.in, tc.lang, tc.currency, got.Text, err, tc.want)
		}
	}
}

func TestEvalCurrencyRate(t *testing.T) {
	got, err := calc.EvalWith("50 dollars", calc.Options{Lang: "en", Rates: rates, Currency: "BRL"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Rate == nil || *got.Rate != (calc.Rate{From: "USD", To: "BRL", Of: 5}) {
		t.Errorf("Rate = %+v, want 1 USD = 5 BRL", got.Rate)
	}
	if got.Plain != "250.00" || got.Unit != "BRL" {
		t.Errorf("Plain, Unit = %q, %q; want 250.00, BRL", got.Plain, got.Unit)
	}
	mixed, _ := calc.EvalWith("10 usd + 10 eur in brl", calc.Options{Lang: "en", Rates: rates})
	if mixed.Rate != nil {
		t.Errorf("Rate of two currencies = %+v, want none", mixed.Rate)
	}
}

func TestEvalCurrencyNeedsRates(t *testing.T) {
	for _, in := range []string{"50 dollars", "$5", "10 eur to chf", "50 dollars to brazilian real", "3 ALL"} {
		if _, err := calc.EvalWith(in, calc.Options{Lang: "en"}); !errors.Is(err, calc.ErrNeedRates) {
			t.Errorf("EvalWith(%q) without rates = %v, want ErrNeedRates", in, err)
		}
		if !calc.LooksLikeMath(in) {
			t.Errorf("LooksLikeMath(%q) = false, want true", in)
		}
	}
	// What is no math stays none, rates or not.
	for _, in := range []string{"dollar", "real madrid", "euro 2024 final", "$", "5 dollars in kg", "50 xyz"} {
		if _, err := calc.EvalWith(in, calc.Options{Lang: "en", Rates: rates}); err == nil {
			t.Errorf("EvalWith(%q) = nil, want an error", in)
		}
	}
	// A code the rates lack is no answer.
	if _, err := calc.EvalWith("5 ves", calc.Options{Lang: "en", Rates: rates}); err == nil || errors.Is(err, calc.ErrNeedRates) {
		t.Errorf("EvalWith(5 ves) = %v, want a missing rate", err)
	}
}
